package server

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func parseIP(s string) net.IP { return net.ParseIP(s) }

// stubTransport replaces the real network client for handler-level tests.
// The SSRF guard itself lives in the dialer and is unit-tested below; the
// stub bypasses it so a loopback httptest server can act as upstream.
type stubTransport struct {
	resp  *http.Response
	seen  *http.Request
	fails bool
}

func (s *stubTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if s.fails {
		return nil, io.ErrUnexpectedEOF
	}
	if s.seen == nil {
		s.seen = r
	}
	return s.resp, nil
}

func withStubClient(t *testing.T, tr *stubTransport) {
	t.Helper()
	orig := imgClient
	imgClient = &http.Client{Transport: tr}
	t.Cleanup(func() { imgClient = orig })
}

func stubImageResp(contentType string, body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func TestImgProxyMissingParam(t *testing.T) {
	req := httptest.NewRequest("GET", "/img", nil)
	rec := httptest.NewRecorder()
	handleImg(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestImgProxyRejectsBadSchemeAndPorts(t *testing.T) {
	for _, raw := range []string{
		"/img?u=" + "javascript%3Aalert(1)",
		"/img?u=" + "ftp%3A%2F%2Fexample.com%2Fx.png",
		"/img?u=" + "https%3A%2F%2Fexample.com%3A8080%2Fx.png",
		"/img?u=not-a-url",
	} {
		req := httptest.NewRequest("GET", raw, nil)
		rec := httptest.NewRecorder()
		handleImg(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: got %d, want 400", raw, rec.Code)
		}
	}
}

func TestImgProxyRejectsNonImageResponse(t *testing.T) {
	withStubClient(t, &stubTransport{
		resp: stubImageResp("text/html", []byte("<html>not an image</html>")),
	})
	req := httptest.NewRequest("GET", "/img?u=https%3A%2F%2Fexample.com%2Fi.png", nil)
	rec := httptest.NewRecorder()
	handleImg(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502", rec.Code)
	}
}

func TestImgProxySniffedTypeServesCompleteBody(t *testing.T) {
	// Lie about Content-Type: the sniff must override it, and the sniffed
	// 512 probe bytes must still reach the client (regression: the probe
	// used to consume them out of the stream).
	// PNG magic + enough padding to exceed the 512-byte probe window.
	body := make([]byte, 600)
	copy(body, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	for i := range body[8:] {
		body[8+i] = byte(i % 251)
	}
	withStubClient(t, &stubTransport{
		resp: stubImageResp("application/octet-stream", body),
	})
	req := httptest.NewRequest("GET", "/img?u=https%3A%2F%2Fexample.com%2Fi.png", nil)
	rec := httptest.NewRecorder()
	handleImg(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type %q, want image/png from sniffing", got)
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, body) {
		t.Fatalf("body truncated: got %d bytes, want %d", len(got), len(body))
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "max-age") {
		t.Fatalf("cache-control missing: %q", got)
	}
}

func TestImgProxyNeverForwardsVisitorHeaders(t *testing.T) {
	stub := &stubTransport{resp: stubImageResp("image/png", []byte{0x89, 'P', 'N', 'G'})}
	withStubClient(t, stub)
	req := httptest.NewRequest("GET", "/img?u=https%3A%2F%2Fexample.com%2Fi.png", nil)
	req.Header.Set("Cookie", "session=secret")
	req.Header.Set("Authorization", "Bearer secret")
	handleImg(httptest.NewRecorder(), req)
	if stub.seen == nil {
		t.Fatalf("upstream never called")
	}
	for _, h := range []string{"Cookie", "Authorization"} {
		if stub.seen.Header.Get(h) != "" {
			t.Fatalf("visitor header %s forwarded upstream", h)
		}
	}
}

func TestImgProxyUpstreamFailure(t *testing.T) {
	withStubClient(t, &stubTransport{fails: true})
	req := httptest.NewRequest("GET", "/img?u=https%3A%2F%2Fexample.com%2Fi.png", nil)
	rec := httptest.NewRecorder()
	handleImg(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502", rec.Code)
	}
}

func TestImgProxyNon200Upstream(t *testing.T) {
	resp := stubImageResp("image/png", []byte{0x89, 'P', 'N', 'G'})
	resp.StatusCode = http.StatusForbidden
	withStubClient(t, &stubTransport{resp: resp})
	req := httptest.NewRequest("GET", "/img?u=https%3A%2F%2Fexample.com%2Fi.png", nil)
	rec := httptest.NewRecorder()
	handleImg(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502", rec.Code)
	}
}

func TestPublicIPGuard(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.1.2.3", "192.168.1.135", "172.16.5.5", "172.31.255.1",
		"169.254.1.1", "100.64.0.1", "0.0.0.0", "224.0.0.1", "240.0.0.1",
		"192.0.2.1", "198.51.100.7", "203.0.113.9", "198.18.0.1",
		"::1", "fe80::1", "fc00::1", "ff02::1",
	}
	for _, s := range blocked {
		if publicIP(parseIP(s)) {
			t.Fatalf("%s must be blocked", s)
		}
	}
	public := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700::1"}
	for _, s := range public {
		if !publicIP(parseIP(s)) {
			t.Fatalf("%s must be allowed", s)
		}
	}
	if publicIP(nil) {
		t.Fatalf("nil IP must be blocked")
	}
}

func TestImgControlBlocksLoopback(t *testing.T) {
	if err := imgControl("tcp", "127.0.0.1:443", nil); err == nil {
		t.Fatalf("loopback dial must be rejected")
	}
	if err := imgControl("tcp", "192.168.1.135:443", nil); err == nil {
		t.Fatalf("RFC1918 dial must be rejected")
	}
}
