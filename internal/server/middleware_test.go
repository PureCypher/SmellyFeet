package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const wantCSP = "default-src 'none'; style-src 'self'; font-src 'self'; img-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

func TestSecurityHeadersOnAllRoutes(t *testing.T) {
	h := newTestServer(t, stubService{})
	for _, path := range []string{"/", "/stats", "/about", "/healthz", "/static/app.css", "/article/999"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Content-Security-Policy"); got != wantCSP {
			t.Errorf("%s CSP = %q", path, got)
		}
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s X-Content-Type-Options = %q", path, got)
		}
		if got := rec.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
			t.Errorf("%s Referrer-Policy = %q", path, got)
		}
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name, cfIP, remote, want string
	}{
		{"cloudflare header wins", "203.0.113.7", "10.0.0.1:1234", "203.0.113.7"},
		{"falls back to remote addr", "", "10.0.0.1:1234", "10.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remote
			if tt.cfIP != "" {
				req.Header.Set("CF-Connecting-IP", tt.cfIP)
			}
			if got := clientIP(req); got != tt.want {
				t.Fatalf("clientIP = %q, want %q", got, tt.want)
			}
		})
	}
}
