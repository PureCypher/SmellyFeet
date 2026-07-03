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

func TestStatsOmitsSourcesOnFeedError(t *testing.T) {
	svc := stubService{feedsErr: errBoom}
	body := getPath(t, newTestServer(t, svc), "/stats").Body.String()
	if strings.Contains(body, "top sources") {
		t.Error("top-sources section should be omitted when ListFeeds fails")
	}
}
