package server

import (
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"smellyfeet/internal/apiclient"
)

func TestFeedRendersValidAtom(t *testing.T) {
	sum := "Summary with <angle brackets> & ampersand."
	svc := stubService{list: apiclient.ListResult{Articles: []apiclient.Article{
		{ID: 7, Title: "Title <needs> escaping", PublishedAt: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC), Summary: &sum},
		{ID: 8, Title: "No summary, zero date"},
	}}}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	req.Host = "intel.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /feed.xml = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/atom+xml") {
		t.Fatalf("Content-Type = %q", ct)
	}

	var doc struct {
		XMLName xml.Name `xml:"feed"`
		Title   string   `xml:"title"`
		Entries []struct {
			Title   string `xml:"title"`
			ID      string `xml:"id"`
			Updated string `xml:"updated"`
			Summary string `xml:"summary"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("feed is not valid XML: %v", err)
	}
	if len(doc.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(doc.Entries))
	}
	if doc.Entries[0].ID != "https://intel.example.com/article/7" {
		t.Fatalf("entry id = %q", doc.Entries[0].ID)
	}
	if doc.Entries[0].Title != "Title <needs> escaping" {
		t.Fatalf("title round-trip = %q", doc.Entries[0].Title)
	}
	if doc.Entries[1].Updated == "" || strings.HasPrefix(doc.Entries[1].Updated, "0001") {
		t.Fatalf("zero publish date leaked into feed: %q", doc.Entries[1].Updated)
	}
}

func TestFeedUpstreamError(t *testing.T) {
	h := newTestServer(t, stubService{listErr: errors.New("boom")})
	rec := getPath(t, h, "/feed.xml")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("error response must be no-store")
	}
}
