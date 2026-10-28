package server

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"smellyfeet/internal/apiclient"
)

// maxFeedSelection mirrors the backend's cap on sources per /articles request,
// so a hand-edited URL can never make the list page hit the backend's 400.
const maxFeedSelection = 25

// parseFeedSelection cleans the repeatable ?feed= values: trims, drops empties
// (the legacy "All sources" submit sent feed=), dedupes in first-seen order,
// and keeps at most maxFeedSelection.
func parseFeedSelection(values []string) []string {
	var out []string
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
		if len(out) == maxFeedSelection {
			break
		}
	}
	return out
}

// listURL builds a list-page path from the active filters. Empty values are
// omitted, "oldest" is the only sort that is ever emitted, and page 1 is
// implicit. url.Values escapes every value, so feed URLs containing & + # ?
// or non-ASCII survive the round trip.
func listURL(q string, feeds []string, order string, page int) string {
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	for _, f := range feeds {
		if f != "" {
			v.Add("feed", f)
		}
	}
	if order == "oldest" {
		v.Set("sort", "oldest")
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	if len(v) == 0 {
		return "/"
	}
	return "/?" + v.Encode()
}

// sourceChip is one removable "source: name" chip above the results.
type sourceChip struct {
	Name       string
	RemoveHref string // the same list with only this source dropped
}

func buildSourceChips(selected []string, q, order string) []sourceChip {
	chips := make([]sourceChip, 0, len(selected))
	for i, f := range selected {
		rest := make([]string, 0, len(selected))
		rest = append(rest, selected[:i]...)
		rest = append(rest, selected[i+1:]...)
		chips = append(chips, sourceChip{Name: sourceName(f), RemoveHref: listURL(q, rest, order, 0)})
	}
	return chips
}

// sourceOption is one checkbox row in the picker.
type sourceOption struct {
	URL, Name string
	Count     int
	Selected  bool
	// Unlisted marks a selected feed that /feeds no longer offers (a stale
	// link, or a secondary feed dedupeFeedList collapsed). It still renders so
	// that Apply re-submits it instead of silently dropping it.
	Unlisted bool
}

// sourceGroup is one labelled section of the picker.
type sourceGroup struct {
	Label, Anchor string
	Options       []sourceOption
}

const (
	selectedLabel  = "Selected"
	selectedAnchor = "src-selected"
	otherLabel     = "#"
	otherAnchor    = "src-other"
)

func groupLabel(name string) string {
	for _, r := range name {
		if u := unicode.ToUpper(r); u >= 'A' && u <= 'Z' {
			return string(u)
		}
		break
	}
	return otherLabel
}

// buildSourceGroups arranges the picker: the selection pinned first (in
// selection order, including unlisted feeds), then every other feed grouped by
// the first letter of its display name, A-Z then "#", each group sorted
// case-insensitively. A selected source appears exactly once.
func buildSourceGroups(feeds []apiclient.Feed, selected []string) []sourceGroup {
	isSelected := make(map[string]bool, len(selected))
	for _, u := range selected {
		isSelected[u] = true
	}
	listed := make(map[string]apiclient.Feed, len(feeds))
	for _, f := range feeds {
		listed[f.FeedURL] = f
	}

	var groups []sourceGroup
	if len(selected) > 0 {
		opts := make([]sourceOption, 0, len(selected))
		for _, u := range selected {
			f, ok := listed[u]
			opts = append(opts, sourceOption{URL: u, Name: sourceName(u), Count: f.ArticleCount, Selected: true, Unlisted: !ok})
		}
		groups = append(groups, sourceGroup{Label: selectedLabel, Anchor: selectedAnchor, Options: opts})
	}

	byLabel := make(map[string][]sourceOption)
	for _, f := range feeds {
		if isSelected[f.FeedURL] {
			continue
		}
		name := sourceName(f.FeedURL)
		label := groupLabel(name)
		byLabel[label] = append(byLabel[label], sourceOption{URL: f.FeedURL, Name: name, Count: f.ArticleCount})
	}
	labels := make([]string, 0, len(byLabel))
	for l := range byLabel {
		labels = append(labels, l)
	}
	sort.Slice(labels, func(i, j int) bool {
		if labels[i] == otherLabel || labels[j] == otherLabel {
			return labels[j] == otherLabel && labels[i] != otherLabel
		}
		return labels[i] < labels[j]
	})
	for _, l := range labels {
		opts := byLabel[l]
		sort.SliceStable(opts, func(i, j int) bool {
			a, b := strings.ToLower(opts[i].Name), strings.ToLower(opts[j].Name)
			if a != b {
				return a < b
			}
			return opts[i].Name < opts[j].Name
		})
		anchor := otherAnchor
		if l != otherLabel {
			anchor = "src-" + strings.ToLower(l)
		}
		groups = append(groups, sourceGroup{Label: l, Anchor: anchor, Options: opts})
	}
	return groups
}
