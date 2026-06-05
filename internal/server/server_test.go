package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"smellyfeet/internal/apiclient"
)

// stubService implements ArticleService for tests.
type stubService struct {
	list     apiclient.ListResult
	article  apiclient.Article
	feeds    []apiclient.Feed
	stats    apiclient.Stats
	listErr  error
	getErr   error
	feedsErr error
	statsErr error
}

func (s stubService) ListArticles(ctx context.Context, p apiclient.ListParams) (apiclient.ListResult, error) {
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
