package server

import (
	"errors"
	"strings"
	"testing"
	"time"

	"smellyfeet/internal/apiclient"
)

var errBoom = errors.New("boom")

func TestListCardShowsPillBadgeAndRelTime(t *testing.T) {
	sum := "A keycloak bypass."
	svc := stubService{
		list: apiclient.ListResult{Articles: []apiclient.Article{{
			ID: 1, Title: "CVE-2026-14615 - Keycloak-services bypass", Summary: &sum,
			FeedURL:     "https://www.brighttalk.com/channel/7451/feed/rss",
			PublishedAt: time.Now().Add(-2 * time.Hour),
		}}},
		feeds: []apiclient.Feed{{FeedURL: "https://www.brighttalk.com/channel/7451/feed/rss", ArticleCount: 12}},
	}
	body := getPath(t, newTestServer(t, svc), "/").Body.String()

	for _, want := range []string{
		">brighttalk.com</a>",
		`title="Filter by brighttalk.com"`,
		">CVE-2026-14615</span>",
		"2h ago",
		"brighttalk.com (12)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list page missing %q", want)
		}
	}
	if strings.Contains(body, "Read full article") {
		t.Error("redundant card CTA still present")
	}
}

func TestArticlePageSingleURLAndPill(t *testing.T) {
	svc := stubService{article: apiclient.Article{
		ID: 7, Title: "Some report", URL: "https://example.com/original-report",
		FeedURL: "https://feeds.feedburner.com/TheHackersNews",
	}}
	body := getPath(t, newTestServer(t, svc), "/article/7").Body.String()

	if n := strings.Count(body, "https://example.com/original-report"); n != 1 {
		t.Errorf("original URL appears %d times, want exactly 1 (href only)", n)
	}
	if !strings.Contains(body, ">feeds.feedburner.com</a>") {
		t.Error("article page missing source pill")
	}
}

func TestStatsShowsTopSources(t *testing.T) {
	svc := stubService{feeds: []apiclient.Feed{
		{FeedURL: "https://small.example.com/rss", ArticleCount: 10},
		{FeedURL: "https://big.example.com/rss", ArticleCount: 200},
	}}
	body := getPath(t, newTestServer(t, svc), "/stats").Body.String()
	if !strings.Contains(body, "top sources") {
		t.Fatal("stats missing top-sources section")
	}
	if !strings.Contains(body, "bar-100") {
		t.Error("largest source should render bar-100")
	}
	i := strings.Index(body, "big.example.com")
	j := strings.Index(body, "small.example.com")
	if i == -1 || j == -1 || i > j {
		t.Errorf("sources not sorted descending (big at %d, small at %d)", i, j)
	}
}

func TestStatsShowsUnavailableOnFeedError(t *testing.T) {
	svc := stubService{feedsErr: errBoom}
	body := getPath(t, newTestServer(t, svc), "/stats").Body.String()
	if !strings.Contains(body, "top sources") {
		t.Error("top-sources heading should still render when ListFeeds fails")
	}
	if !strings.Contains(body, "unavailable") {
		t.Error("expected an explicit unavailable message when ListFeeds fails")
	}
	if strings.Contains(body, "bar-") {
		t.Error("no source bars should render when ListFeeds fails")
	}
}

// TestIngestionHealthDerivation exhaustively covers every boundary of the
// Signal Lamp status-derivation rule (REDESIGN_PLAN.md's "/stats" route).
// This is the one place a wrong branch fabricates system state (a false
// green over failing fetches, or a scary red on a fresh install), so every
// condition combination gets its own case rather than a few spot checks.
func TestIngestionHealthDerivation(t *testing.T) {
	cases := []struct {
		name      string
		stats     apiclient.Stats
		wantState string
	}{
		{"fresh install, no history at all", apiclient.Stats{TotalArticles: 0, SuccessfulFetches24h: 0, FailedFetches24h: 0}, "nodata"},
		{"history exists, zero fetches attempted in 24h", apiclient.Stats{TotalArticles: 100, SuccessfulFetches24h: 0, FailedFetches24h: 0}, "failing"},
		{"history exists, all attempts failed", apiclient.Stats{TotalArticles: 100, SuccessfulFetches24h: 0, FailedFetches24h: 5}, "failing"},
		{"history exists, some failures", apiclient.Stats{TotalArticles: 100, SuccessfulFetches24h: 10, FailedFetches24h: 1}, "degraded"},
		{"history exists, all succeeded", apiclient.Stats{TotalArticles: 100, SuccessfulFetches24h: 10, FailedFetches24h: 0}, "ok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ingestionHealth(tc.stats)
			if got.State != tc.wantState {
				t.Errorf("ingestionHealth(%+v).State = %q, want %q", tc.stats, got.State, tc.wantState)
			}
			if got.Message == "" {
				t.Error("every health state must carry a non-empty message — never dot-alone")
			}
		})
	}
}

func TestStatsHealthPanelRendersStateAndLabel(t *testing.T) {
	svc := stubService{stats: apiclient.Stats{TotalArticles: 50, SuccessfulFetches24h: 3, FailedFetches24h: 2}}
	body := getPath(t, newTestServer(t, svc), "/stats").Body.String()
	if !strings.Contains(body, "DEGRADED") {
		t.Error("expected the DEGRADED text label, not just a colored dot")
	}
	if !strings.Contains(body, "2 of 5 fetches failed") {
		t.Error("expected the real fetch counts in the sentence, never invented")
	}
}

func TestStatsHealthPanelNoDataOnFreshInstall(t *testing.T) {
	svc := stubService{stats: apiclient.Stats{TotalArticles: 0}}
	body := getPath(t, newTestServer(t, svc), "/stats").Body.String()
	if !strings.Contains(body, "NO DATA YET") {
		t.Error("a fresh install with zero articles must show NO DATA YET, never a red FAILING panel")
	}
	if strings.Contains(body, "FAILING") {
		t.Error("must not show FAILING on a fresh install with no history")
	}
}
