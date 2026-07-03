package server

import (
	"net/http"
	"strings"
	"testing"

	"smellyfeet/internal/apiclient"
)

func TestRobotsTxt(t *testing.T) {
	h := newTestServer(t, stubService{})
	rec := getPath(t, h, "/robots.txt")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "User-agent: *") {
		t.Fatalf("robots.txt = %d %q", rec.Code, rec.Body.String())
	}
}

func TestArticlePageHasOpenGraphTags(t *testing.T) {
	sum := "Threat actors exploited a zero-day in WidgetCorp firewalls to gain initial access."
	h := newTestServer(t, stubService{article: apiclient.Article{
		ID: 7, Title: "WidgetCorp zero-day", URL: "https://example.com/a", Summary: &sum,
	}})
	body := getPath(t, h, "/article/7").Body.String()
	for _, want := range []string{
		`property="og:title" content="WidgetCorp zero-day"`,
		`property="og:type" content="article"`,
		`property="og:site_name" content="Information Broker"`,
		`name="description"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("article page missing %q", want)
		}
	}
}

func TestListPageHasDescriptionAndStatsDoesNot(t *testing.T) {
	h := newTestServer(t, stubService{})
	if !strings.Contains(getPath(t, h, "/").Body.String(), `name="description"`) {
		t.Error("list page missing meta description")
	}
	if strings.Contains(getPath(t, h, "/stats").Body.String(), `property="og:`) {
		t.Error("stats page should not emit OG tags")
	}
}

func TestArticleWithoutSummaryStillHasOGTitle(t *testing.T) {
	h := newTestServer(t, stubService{article: apiclient.Article{ID: 9, Title: "No summary article"}})
	body := getPath(t, h, "/article/9").Body.String()
	if !strings.Contains(body, `property="og:title" content="No summary article"`) {
		t.Error("article without summary missing og:title")
	}
	if strings.Contains(body, `property="og:description"`) {
		t.Error("article without summary should not emit og:description")
	}
}

func TestTrimDesc(t *testing.T) {
	tests := []struct {
		name, in string
		n        int
		want     string
	}{
		{"short passes through", "hello world", 200, "hello world"},
		{"collapses whitespace", "a\n\n b", 200, "a b"},
		{"truncates with ellipsis", strings.Repeat("x", 300), 10, strings.Repeat("x", 10) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimDesc(tt.in, tt.n); got != tt.want {
				t.Fatalf("trimDesc = %q, want %q", got, tt.want)
			}
		})
	}
}
