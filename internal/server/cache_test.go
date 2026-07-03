package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func getPath(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCacheControlPerRoute(t *testing.T) {
	h := newTestServer(t, stubService{})
	tests := []struct{ path, want string }{
		{"/", "public, max-age=60, s-maxage=120, stale-while-revalidate=300"},
		{"/stats", "public, max-age=30, s-maxage=60"},
		{"/about", "public, max-age=3600, s-maxage=86400"},
		{"/static/app.css", "public, max-age=31536000, immutable"},
		{"/healthz", "no-store"},
		{"/article/notanumber", "no-store"},
	}
	for _, tt := range tests {
		if got := getPath(t, h, tt.path).Header().Get("Cache-Control"); got != tt.want {
			t.Errorf("%s Cache-Control = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestUpstreamErrorIsNeverCached(t *testing.T) {
	h := newTestServer(t, stubService{listErr: errors.New("boom")})
	rec := getPath(t, h, "/")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("error Cache-Control = %q, want no-store", got)
	}
}
