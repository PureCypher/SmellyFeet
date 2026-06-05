package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
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
