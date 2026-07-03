package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStaticServesCSS(t *testing.T) {
	h := newTestServer(t, stubService{})
	req := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/app.css = %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("Cache-Control = %q, want immutable", cc)
	}
}

func TestStaticDirectoryListingBlocked(t *testing.T) {
	h := newTestServer(t, stubService{})
	req := httptest.NewRequest(http.MethodGet, "/static/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /static/ = %d, want 404", rec.Code)
	}
}

func TestPagesLinkLocalStylesheetOnly(t *testing.T) {
	h := newTestServer(t, stubService{})
	req := httptest.NewRequest(http.MethodGet, "/about", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `/static/app.css?v=`+assetHash) {
		t.Fatalf("page missing local stylesheet link")
	}
	for _, banned := range []string{"cdn.tailwindcss.com", "fonts.googleapis.com", "<script"} {
		if strings.Contains(body, banned) {
			t.Fatalf("page still references %q", banned)
		}
	}
}
