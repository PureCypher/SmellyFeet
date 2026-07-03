package server

import (
	"strings"
	"testing"
	"time"

	"smellyfeet/internal/apiclient"
)

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
		">brighttalk.com</span>",
		`title="https://www.brighttalk.com/channel/7451/feed/rss"`,
		">CVE-2026-14615</span>",
		"2h ago",
		"brighttalk.com (12)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list page missing %q", want)
		}
	}
	if strings.Contains(body, "READ FULL ARTICLE") {
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
	if !strings.Contains(body, ">feeds.feedburner.com</span>") {
		t.Error("article page missing source pill")
	}
}
