package server

import (
	"fmt"

	"smellyfeet/internal/apiclient"
)

// articleGroup is one story: the newest article covering it, plus every other
// article the backend's clustering job put in the same cluster.
type articleGroup struct {
	Lead    apiclient.Article
	Members []apiclient.Article
}

// groupByCluster collapses a digest slice into one group per story cluster,
// so a story covered by four feeds renders once instead of four times.
//
// Incoming order (cross_feed_count DESC, publish_date DESC, set by the API) is
// preserved: groups appear in order of first appearance, and the first article
// seen for a cluster leads it. Every member of a cluster shares the same
// cross_feed_count, so within a cluster the incoming order is purely
// publish_date DESC -- which makes the first-seen article the newest one.
//
// A nil StoryClusterID means the clustering job hasn't reached that article
// yet -- "unclassified", not "same story" -- so each one becomes its own
// singleton group rather than being lumped in with the other nils. This is
// also the whole-page behaviour against a backend that predates the field,
// which degrades to the flat list this page used to render.
func groupByCluster(articles []apiclient.Article) []articleGroup {
	groups := []articleGroup{}
	indexByCluster := map[int64]int{}
	for _, a := range articles {
		if a.StoryClusterID == nil {
			groups = append(groups, articleGroup{Lead: a})
			continue
		}
		if i, ok := indexByCluster[*a.StoryClusterID]; ok {
			groups[i].Members = append(groups[i].Members, a)
			continue
		}
		indexByCluster[*a.StoryClusterID] = len(groups)
		groups = append(groups, articleGroup{Lead: a})
	}
	return groups
}

// Presentation caps for the digest page. A rolling weekly window carries
// ~160 corroborated stories and ~1000 in total, so rendering every group as
// a card buries the ranking the digest exists to express. Only the strongest
// handful get a card; the rest degrade to one-line rows behind a disclosure.
// Whatever a cap drops is counted, never silently dropped -- see
// splitDigestSections.
const (
	// digestLeadStories is how many stories render as full summary cards.
	digestLeadStories = 10
	// digestCompactRows caps each one-line overflow list. Set wide enough
	// that the corroborated list is complete for the daily and weekly ranges
	// (~146 stories at the weekly high-water mark): a row is one anchor
	// inside a collapsed <details>, so the cost of carrying them is small,
	// and truncating the ranked list is what the page is trying to avoid.
	// The unbounded bucket is "everything else" -- thousands of stories on
	// the yearly range -- which is what this cap actually exists to bound.
	digestCompactRows = 150
)

// digestSections is one digest response arranged for rendering: a short lead
// of full cards, then progressively lighter overflow.
//
// The *Total fields are pre-cap counts and the *Hidden fields are what the
// caps dropped, so the page can state "60 of 193" instead of implying the 60
// it shows are everything.
type digestSections struct {
	Top       []articleGroup // corroborated stories, rendered as cards
	MoreTop   []articleGroup // the rest of the corroborated stories, as rows
	TopTotal  int
	TopHidden int

	Rest       []articleGroup // everything else, as rows
	RestTotal  int
	RestHidden int
}

// splitDigestSections applies the presentation caps to the two grouped
// buckets, preserving the API's ranking order throughout.
func splitDigestSections(important, other []articleGroup) digestSections {
	s := digestSections{TopTotal: len(important), RestTotal: len(other)}

	s.Top, important = takeGroups(important, digestLeadStories)
	s.MoreTop, important = takeGroups(important, digestCompactRows)
	s.TopHidden = len(important)

	s.Rest, other = takeGroups(other, digestCompactRows)
	s.RestHidden = len(other)
	return s
}

// takeGroups splits off the first n groups, returning them and the remainder.
// Both results are non-nil so templates can range over them unconditionally.
func takeGroups(groups []articleGroup, n int) (taken, remaining []articleGroup) {
	if len(groups) <= n {
		return groups, []articleGroup{}
	}
	return groups[:n], groups[n:]
}

// digestWindowLabels describes each range in the terms the API now applies:
// a rolling look-back, not the current calendar period. These strings are the
// page's only statement of what window the reader is looking at, so they must
// track digestWindowDays in the backend's digest.go.
var digestWindowLabels = map[string]string{
	"daily":      "last 24 hours",
	"weekly":     "last 7 days",
	"monthly":    "last 30 days",
	"quarterly":  "last 90 days",
	"halfyearly": "last 6 months",
	"yearly":     "last 12 months",
}

// digestWindowLabel describes a range in prose, falling back to daily on any
// value the whitelist would have rejected.
func digestWindowLabel(rangeParam string) string {
	if label, ok := digestWindowLabels[rangeParam]; ok {
		return label
	}
	return digestWindowLabels["daily"]
}

// feedCountLabel renders a story's cross-feed coverage as a badge string, or
// "" for a story only one feed carried (nothing worth a badge).
//
// The count is of feeds covering the STORY, measured by the API over its
// clustering window -- deliberately wider than the digest's display window,
// since corroboration accumulates over days. So this can legitimately read
// "8 feeds" on a story whose only article inside the window is the one shown:
// the other seven ran theirs earlier. That is the signal, not a discrepancy.
func feedCountLabel(crossFeedCount int) string {
	if crossFeedCount < 1 {
		return ""
	}
	// cross_feed_count counts OTHER feeds, so +1 is the distinct-feed total.
	return fmt.Sprintf("%d feeds", crossFeedCount+1)
}

// Coverage tones. A weekly digest carries stories from three feeds and
// stories from fourteen; a badge that renders both identically throws away
// the ranking the reader came for. Two tones only -- a full severity ramp
// would compete with the Signal Lamp status colours, which mean something
// else on this site.
const (
	// coverageHeavyThreshold is the cross_feed_count (OTHER feeds) at which a
	// story reads as wall-to-wall coverage -- 7+ distinct feeds.
	coverageHeavyThreshold = 6
)

// coverageTone classifies a story's cross-feed coverage so the badge's visual
// weight tracks the strength of the signal. "" means no emphasis.
func coverageTone(crossFeedCount int) string {
	if crossFeedCount >= coverageHeavyThreshold {
		return "heavy"
	}
	return ""
}
