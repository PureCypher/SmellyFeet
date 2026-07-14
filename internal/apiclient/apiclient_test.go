package apiclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListArticles(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"articles":[{"id":7,"title":"T","url":"u","summary":"s","content":"c","feed_url":"f","content_hash":"h"}],"count":1,"limit":20,"offset":0}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.ListArticles(context.Background(), ListParams{Limit: 20, Offset: 0, Feed: "f", Q: "ransom"})
	if err != nil {
		t.Fatalf("ListArticles error: %v", err)
	}
	if res.Count != 1 || len(res.Articles) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Articles[0].ID != 7 || res.Articles[0].Summary == nil || *res.Articles[0].Summary != "s" {
		t.Fatalf("unexpected article: %+v", res.Articles[0])
	}
	if gotPath != "/articles?feed=f&limit=20&q=ransom" {
		t.Fatalf("unexpected request path: %s", gotPath)
	}
}

func TestGetJSONNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.ListArticles(context.Background(), ListParams{Limit: 20})
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestGetArticleNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.GetArticle(context.Background(), 42)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetArticleOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/articles/get?id=42" {
			t.Errorf("unexpected path: %s", r.URL.RequestURI())
		}
		w.Write([]byte(`{"id":42,"title":"Deep Dive","content":"full body","feed_url":"f"}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	a, err := c.GetArticle(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetArticle error: %v", err)
	}
	if a.ID != 42 || a.Content != "full body" {
		t.Fatalf("unexpected article: %+v", a)
	}
}

func TestListFeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/feeds" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"feeds":[{"feed_url":"https://a/rss","article_count":5}],"count":1}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	feeds, err := c.ListFeeds(context.Background())
	if err != nil {
		t.Fatalf("ListFeeds error: %v", err)
	}
	if len(feeds) != 1 || feeds[0].FeedURL != "https://a/rss" || feeds[0].ArticleCount != 5 {
		t.Fatalf("unexpected feeds: %+v", feeds)
	}
}

func TestListArticlesSortParam(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"articles":[],"count":0,"limit":0,"offset":0}`))
	}))
	defer srv.Close()
	c := New(srv.URL)

	if _, err := c.ListArticles(context.Background(), ListParams{Sort: "oldest"}); err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if !strings.Contains(gotQuery, "sort=oldest") {
		t.Fatalf("query %q missing sort=oldest", gotQuery)
	}

	if _, err := c.ListArticles(context.Background(), ListParams{}); err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if strings.Contains(gotQuery, "sort=") {
		t.Fatalf("query %q should not contain sort when unset", gotQuery)
	}
}

func TestGetStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stats" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"total_articles":100,"total_feeds":12,"last_fetch":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	s, err := c.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats error: %v", err)
	}
	if s.TotalArticles != 100 || s.TotalFeeds != 12 {
		t.Fatalf("unexpected stats: %+v", s)
	}
}

func TestGetDigest(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Write([]byte(`{"range":"weekly","since":"2026-07-07T00:00:00Z","important":[{"id":1,"title":"Big story","cross_feed_count":3}],"other":[{"id":2,"title":"Minor","cross_feed_count":0}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.GetDigest(context.Background(), "weekly")
	if err != nil {
		t.Fatalf("GetDigest error: %v", err)
	}
	if gotPath != "/articles/digest?range=weekly" {
		t.Fatalf("unexpected request path: %s", gotPath)
	}
	if res.Range != "weekly" || len(res.Important) != 1 || res.Important[0].CrossFeedCount != 3 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(res.Other) != 1 || res.Other[0].ID != 2 {
		t.Fatalf("unexpected other: %+v", res)
	}
}
