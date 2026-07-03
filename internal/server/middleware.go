package server

import (
	"log"
	"net"
	"net/http"
	"time"
)

const csp = "default-src 'none'; style-src 'self'; font-src 'self'; img-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// statusWriter records the response status for the request log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// clientIP prefers Cloudflare's CF-Connecting-IP header; behind the tunnel,
// RemoteAddr is always cloudflared's address.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s %s", r.Method, r.URL.RequestURI(), sw.status,
			time.Since(start).Round(time.Millisecond), clientIP(r))
	})
}
