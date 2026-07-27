package server

import (
	"strings"
	"testing"

	"smellyfeet/internal/apiclient"
)

// groups builds n distinct singleton groups for cap arithmetic.
func groups(n int) []articleGroup {
	out := make([]articleGroup, n)
	for i := range out {
		out[i] = articleGroup{Lead: apiclient.Article{ID: int64(i + 1)}}
	}
	return out
}

func TestSplitDigestSectionsSmallResultFitsInTheLead(t *testing.T) {
	got := splitDigestSections(groups(3), groups(5))

	if len(got.Top) != 3 || got.TopTotal != 3 {
		t.Errorf("Top = %d (total %d), want all 3 as cards", len(got.Top), got.TopTotal)
	}
	if len(got.MoreTop) != 0 || got.TopHidden != 0 {
		t.Errorf("nothing should overflow: MoreTop=%d hidden=%d", len(got.MoreTop), got.TopHidden)
	}
	if len(got.Rest) != 5 || got.RestTotal != 5 || got.RestHidden != 0 {
		t.Errorf("Rest = %d (total %d, hidden %d), want all 5", len(got.Rest), got.RestTotal, got.RestHidden)
	}
}

func TestSplitDigestSectionsCapsTheCardsAndOverflowsTheRemainder(t *testing.T) {
	got := splitDigestSections(groups(digestLeadStories+4), nil)

	if len(got.Top) != digestLeadStories {
		t.Errorf("len(Top) = %d, want the %d-card cap", len(got.Top), digestLeadStories)
	}
	if len(got.MoreTop) != 4 {
		t.Errorf("len(MoreTop) = %d, want the 4 that did not fit", len(got.MoreTop))
	}
	if got.TopTotal != digestLeadStories+4 {
		t.Errorf("TopTotal = %d, want the pre-cap total", got.TopTotal)
	}
	// The overflow must continue where the cards stopped, with no gap and no repeat.
	if got.MoreTop[0].Lead.ID != int64(digestLeadStories+1) {
		t.Errorf("MoreTop starts at ID %d, want %d", got.MoreTop[0].Lead.ID, digestLeadStories+1)
	}
}

// A truncated list that does not say so reads as a complete list. Whenever a
// cap drops groups, the count that was dropped has to survive into the view
// so the template can state it.
func TestSplitDigestSectionsReportsWhatItTruncated(t *testing.T) {
	overflowing := digestLeadStories + digestCompactRows + 25
	got := splitDigestSections(groups(overflowing), groups(digestCompactRows+40))

	if len(got.MoreTop) != digestCompactRows {
		t.Errorf("len(MoreTop) = %d, want the %d-row cap", len(got.MoreTop), digestCompactRows)
	}
	if got.TopHidden != 25 {
		t.Errorf("TopHidden = %d, want 25", got.TopHidden)
	}
	if len(got.Rest) != digestCompactRows {
		t.Errorf("len(Rest) = %d, want the %d-row cap", len(got.Rest), digestCompactRows)
	}
	if got.RestHidden != 40 {
		t.Errorf("RestHidden = %d, want 40", got.RestHidden)
	}
	if got.TopTotal != overflowing || got.RestTotal != digestCompactRows+40 {
		t.Errorf("totals must stay pre-cap: TopTotal=%d RestTotal=%d", got.TopTotal, got.RestTotal)
	}
}

func TestSplitDigestSectionsEmpty(t *testing.T) {
	got := splitDigestSections(nil, nil)
	if got.TopTotal != 0 || got.RestTotal != 0 || len(got.Top) != 0 || len(got.Rest) != 0 {
		t.Errorf("empty input must produce an empty view: %+v", got)
	}
}

// The window labels are the page's only statement of what "daily" means, and
// they have to match the rolling look-backs the API now applies. A stale label
// here is worse than none: it would assert a window the data does not cover.
func TestDigestWindowLabel(t *testing.T) {
	for _, tt := range []struct{ rangeParam, want string }{
		{"daily", "last 24 hours"},
		{"weekly", "last 7 days"},
		{"monthly", "last 30 days"},
		{"quarterly", "last 90 days"},
		{"halfyearly", "last 6 months"},
		{"yearly", "last 12 months"},
		{"garbage", "last 24 hours"},
		{"", "last 24 hours"},
	} {
		if got := digestWindowLabel(tt.rangeParam); got != tt.want {
			t.Errorf("digestWindowLabel(%q) = %q, want %q", tt.rangeParam, got, tt.want)
		}
	}
}

// Every value in the range <select> must have a label, or a valid range would
// render a blank window description.
func TestDigestWindowLabelCoversEveryValidRange(t *testing.T) {
	for r := range validDigestRanges {
		if digestWindowLabel(r) == "" {
			t.Errorf("range %q has no window label", r)
		}
	}
}

func TestFeedCountLabelsCoverageNotArticles(t *testing.T) {
	// cross_feed_count counts OTHER feeds, so the distinct-feed total is +1.
	for _, tt := range []struct {
		crossFeedCount int
		want           string
	}{
		{0, ""},
		{1, "2 feeds"},
		{4, "5 feeds"},
	} {
		if got := feedCountLabel(tt.crossFeedCount); got != tt.want {
			t.Errorf("feedCountLabel(%d) = %q, want %q", tt.crossFeedCount, got, tt.want)
		}
	}
}

// A window with articles but no multi-feed story is a real state (a quiet
// night, or a backend whose clustering job is behind). It must explain itself
// and point at a wider range rather than rendering as a bare "Top stories (0)".
func TestHandleDigestExplainsAnEmptyTopSection(t *testing.T) {
	svc := stubService{digest: apiclient.DigestResult{
		Range: "daily",
		Other: []apiclient.Article{{ID: 1, Title: "Quiet item"}},
	}}
	body := getPath(t, newTestServer(t, svc), "/digest").Body.String()

	if strings.Contains(body, "Top stories") {
		t.Errorf("no corroborated stories means no Top stories section: %s", body)
	}
	if !strings.Contains(body, "picked up by three or more feeds") {
		t.Errorf("the page must say why the top section is missing: %s", body)
	}
	if !strings.Contains(body, "/digest?range=weekly") {
		t.Errorf("the page must offer a wider range: %s", body)
	}
	if !containsAll(body, "everything else (1)", "Quiet item") {
		t.Errorf("the low-coverage bucket must still render: %s", body)
	}
}

// End-to-end through the handler: a result larger than every cap must still
// render a page that states its own totals rather than quietly showing a slice.
func TestHandleDigestStatesTotalsWhenCapped(t *testing.T) {
	important := make([]apiclient.Article, 0, digestLeadStories+3)
	for i := 0; i < digestLeadStories+3; i++ {
		important = append(important, apiclient.Article{
			ID: int64(i + 1), Title: "Story", CrossFeedCount: 2, StoryClusterID: clusterRef(int64(i + 1)),
		})
	}
	svc := stubService{digest: apiclient.DigestResult{Range: "daily", Important: important}}
	body := getPath(t, newTestServer(t, svc), "/digest").Body.String()

	if !strings.Contains(body, "3 more") {
		t.Errorf("the overflow disclosure must name how many stories it holds: %s", body)
	}
}
