package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"smellyfeet/internal/apiclient"
)

// stubService implements ArticleService for tests.
type stubService struct {
	list      apiclient.ListResult
	article   apiclient.Article
	feeds     []apiclient.Feed
	stats     apiclient.Stats
	digest    apiclient.DigestResult
	listErr   error
	getErr    error
	feedsErr  error
	statsErr  error
	digestErr error
	lastList  *apiclient.ListParams
}

func (s stubService) ListArticles(ctx context.Context, p apiclient.ListParams) (apiclient.ListResult, error) {
	if s.lastList != nil {
		*s.lastList = p
	}
	return s.list, s.listErr
}
func (s stubService) GetArticle(ctx context.Context, id int64) (apiclient.Article, error) {
	return s.article, s.getErr
}
func (s stubService) ListFeeds(ctx context.Context) ([]apiclient.Feed, error) {
	return s.feeds, s.feedsErr
}
func (s stubService) GetStats(ctx context.Context) (apiclient.Stats, error) {
	return s.stats, s.statsErr
}
func (s stubService) GetDigest(ctx context.Context, rangeParam string) (apiclient.DigestResult, error) {
	return s.digest, s.digestErr
}

func newTestServer(t *testing.T, svc ArticleService) http.Handler {
	t.Helper()
	srv, err := New(svc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv.Routes()
}

func TestHealthz(t *testing.T) {
	h := newTestServer(t, stubService{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("healthz = %d %q", rec.Code, rec.Body.String())
	}
}

func TestListPageRendersSummary(t *testing.T) {
	sum := "A concise summary."
	svc := stubService{list: apiclient.ListResult{
		Articles: []apiclient.Article{{ID: 7, Title: "Big Breach", Summary: &sum, FeedURL: "https://a/rss"}},
		Count:    1,
	}}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Big Breach") || !strings.Contains(body, "A concise summary.") {
		t.Fatalf("list body missing content: %s", body)
	}
	if !strings.Contains(body, `href="/article/7"`) {
		t.Fatalf("list body missing detail link: %s", body)
	}
}

func TestListPageAPIErrorRendersErrorPage(t *testing.T) {
	svc := stubService{listErr: context.DeadlineExceeded}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Fatalf("expected error page, got: %s", rec.Body.String())
	}
}

func TestArticlePageRendersFullText(t *testing.T) {
	sum := "Short summary."
	svc := stubService{article: apiclient.Article{
		ID: 7, Title: "Deep Dive", Summary: &sum,
		Content: "The complete article body text.",
		URL:     "https://source.example/post",
		FeedURL: "https://a/rss",
	}}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/article/7", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Deep Dive", "Short summary.", "The complete article body text.", "https://source.example/post", "Read original"} {
		if !strings.Contains(body, want) {
			t.Fatalf("article body missing %q: %s", want, body)
		}
	}
}

func TestArticlePageNotFound(t *testing.T) {
	svc := stubService{getErr: apiclient.ErrNotFound}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/article/999", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Article not found") {
		t.Fatalf("expected notfound page: %s", rec.Body.String())
	}
}

func TestArticlePageInvalidID(t *testing.T) {
	h := newTestServer(t, stubService{})
	req := httptest.NewRequest(http.MethodGet, "/article/abc", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestStatsPage(t *testing.T) {
	svc := stubService{stats: apiclient.Stats{TotalArticles: 100, TotalFeeds: 12}}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "100") || !strings.Contains(body, "12") {
		t.Fatalf("stats body missing numbers: %s", body)
	}
}

func TestArticlePageUpstreamErrorRendersErrorPage(t *testing.T) {
	svc := stubService{getErr: context.DeadlineExceeded}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/article/7", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Fatalf("expected error page, got: %s", rec.Body.String())
	}
}

func TestStatsPageUpstreamErrorRendersErrorPage(t *testing.T) {
	svc := stubService{statsErr: context.DeadlineExceeded}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Fatalf("expected error page, got: %s", rec.Body.String())
	}
}

func TestFormatDate(t *testing.T) {
	if got := formatDate(nil); got != "—" {
		t.Errorf("nil -> %q, want —", got)
	}
	if got := formatDate(time.Time{}); got != "—" {
		t.Errorf("zero time -> %q, want —", got)
	}
	var np *time.Time
	if got := formatDate(np); got != "—" {
		t.Errorf("nil *time.Time -> %q, want —", got)
	}
	ts := time.Date(2026, 6, 5, 23, 25, 0, 0, time.UTC)
	if got := formatDate(ts); got != "2026-06-05 23:25" {
		t.Errorf("value -> %q", got)
	}
	if got := formatDate(&ts); got != "2026-06-05 23:25" {
		t.Errorf("ptr -> %q", got)
	}
}

func TestCleanContent(t *testing.T) {
	in := "Hello   world\n   \n\n        \n  foo\t bar  \n\nbaz\n"
	want := "Hello world\nfoo bar\nbaz"
	if got := cleanContent(in); got != want {
		t.Fatalf("cleanContent = %q, want %q", got, want)
	}
	if got := cleanContent("\n\n   \n\t\n"); got != "" {
		t.Fatalf("all-whitespace should be empty, got %q", got)
	}
	if got := cleanContent("single line"); got != "single line" {
		t.Fatalf("single line changed: %q", got)
	}
}

func TestCollectionVolume(t *testing.T) {
	rows := collectionVolume(apiclient.Stats{ArticlesToday: 50, ArticlesThisWeek: 300, ArticlesThisMonth: 1000})
	if len(rows) != 3 {
		t.Fatalf("rows = %+v, want 3", rows)
	}
	if rows[0].Name != "Today" || rows[0].Count != 50 || rows[0].Bar != 5 {
		t.Errorf("today row = %+v, want Count 50, Bar 5 (1000 wide bar quantizes down)", rows[0])
	}
	if rows[2].Name != "This month" || rows[2].Count != 1000 || rows[2].Bar != 100 {
		t.Errorf("this month row = %+v, want Count 1000, Bar 100 (the max)", rows[2])
	}
}

func TestCollectionVolumeEmptyWhenNoArticles(t *testing.T) {
	if rows := collectionVolume(apiclient.Stats{}); rows != nil {
		t.Fatalf("expected nil rows for zero articles, got %+v", rows)
	}
}

func TestStatsPageShowsCollectionVolume(t *testing.T) {
	svc := stubService{stats: apiclient.Stats{ArticlesToday: 12, ArticlesThisWeek: 80, ArticlesThisMonth: 300}}
	body := getPath(t, newTestServer(t, svc), "/stats").Body.String()
	if !containsAll(body, "articles collected", "Today", "This week", "This month", "12", "80", "300") {
		t.Fatalf("stats page missing collection-volume section: %s", body)
	}
}

func TestAboutPage(t *testing.T) {
	h := newTestServer(t, stubService{})
	req := httptest.NewRequest(http.MethodGet, "/about", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"SmellyFeet", "Information Broker", "How it works"} {
		if !strings.Contains(body, want) {
			t.Fatalf("about page missing %q", want)
		}
	}
}
