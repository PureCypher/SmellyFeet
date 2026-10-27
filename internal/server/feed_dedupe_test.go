package server

import (
	"testing"

	"smellyfeet/internal/apiclient"
)

func TestDedupeFeedList(t *testing.T) {
	// Real-world shape from feeds.txt: several feeds of the same publisher
	// (different sections/paths) all display the same hostname, plus a
	// feedburner wrapper whose display name now comes from its path.
	feeds := []apiclient.Feed{
		{FeedURL: "https://www.ncsc.gov.uk/rss", ArticleCount: 900},
		{FeedURL: "https://www.ncsc.gov.uk/rss-alerts", ArticleCount: 300},
		{FeedURL: "https://www.ncsc.gov.uk/rss-analysis", ArticleCount: 50},
		{FeedURL: "https://feeds.feedburner.com/TheHackersNews", ArticleCount: 400},
		{FeedURL: "https://www.wired.com/feed/category/security/latest/rss", ArticleCount: 300},
	}
	got := dedupeFeedList(feeds)
	if len(got) != 3 {
		t.Fatalf("dedupeFeedList() = %d entries, want 3", len(got))
	}
	// Highest-count feed of a collapsed publisher is kept as the filter target.
	if got[0].FeedURL != "https://www.ncsc.gov.uk/rss" || got[0].ArticleCount != 900 {
		t.Errorf("ncsc entry = %+v, want the 900-count primary feed", got[0])
	}
	// Wrapper feeds keep their own (path-derived) name — no collapse.
	for _, f := range got {
		if f.FeedURL == "https://www.ncsc.gov.uk/rss-alerts" || f.FeedURL == "https://www.ncsc.gov.uk/rss-analysis" {
			t.Errorf("dedupeFeedList() kept secondary section feed %q", f.FeedURL)
		}
	}
	// Order follows first appearance.
	if got[1].FeedURL != "https://feeds.feedburner.com/TheHackersNews" || got[2].FeedURL != "https://www.wired.com/feed/category/security/latest/rss" {
		t.Errorf("dedupeFeedList() order wrong: got %v, %v", got[1].FeedURL, got[2].FeedURL)
	}
	// Count shown must be the count actually filtered (representative's own).
	if got[0].ArticleCount != 900 {
		t.Errorf("dedupeFeedList() count = %d, want representative's own 900", got[0].ArticleCount)
	}
}

func TestDedupeFeedListNoCollision(t *testing.T) {
	in := []apiclient.Feed{
		{FeedURL: "https://a.example/feed", ArticleCount: 1},
		{FeedURL: "https://b.example/feed", ArticleCount: 2},
	}
	got := dedupeFeedList(in)
	if len(got) != 2 {
		t.Fatalf("dedupeFeedList() collapsed distinct publishers, got %d", len(got))
	}
}
