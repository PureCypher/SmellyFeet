package server

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// The image proxy lets article bodies embed publisher images while keeping
// the site's CSP at img-src 'self': the browser only ever talks to
// /img?u=... on this origin, and this server makes the upstream request
// under strict SSRF rules:
//
//   - http/https only, on ports 80/443
//   - every dial target (including redirect hops) must resolve to a public
//     IP — loopback, RFC1918, link-local, CGNAT, ULA and multicast are
//     rejected at dial time via Dialer.Control, so a DNS rebinding that
//     flips a public name onto a private address fails
//   - redirects are followed but re-validated hop by hop
//   - the response must sniff as image/*, and is capped in size and duration
//
// The proxy is GET-only, sets long-lived cache headers (publisher image URLs
// are effectively content-addressed), and never forwards visitor credentials
// upstream.

const (
	maxImageBytes  = 8 << 20 // 8 MiB — generous for a hero image, bounded
	imageFetchDist = 15 * time.Second
)

// handleImg serves GET /img?u=<image-url>.
func handleImg(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=86400")

	raw := r.URL.Query().Get("u")
	if raw == "" {
		http.Error(w, "Missing u parameter", http.StatusBadRequest)
		return
	}
	target, err := url.Parse(raw)
	if err != nil || !isHTTPScheme(target.Scheme) || target.Host == "" ||
		(target.Port() != "" && target.Port() != "80" && target.Port() != "443") {
		http.Error(w, "Invalid image URL", http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		http.Error(w, "Invalid image URL", http.StatusBadRequest)
		return
	}
	req.Header.Set("User-Agent", "SmellyFeet-ImageProxy/1.0")
	// Never forward visitor credentials/cookies upstream.
	req.Header.Del("Cookie")
	req.Header.Del("Authorization")

	resp, err := imgClient.Do(req)
	if err != nil {
		http.Error(w, "Image fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Image fetch failed", http.StatusBadGateway)
		return
	}

	// Sniff the real content type from the body's leading bytes; serving a
	// non-image payload (e.g. HTML) would let a publisher frame content on
	// this origin. The probe bytes are re-attached so the copy is complete.
	buf := make([]byte, 512)
	n, _ := io.ReadFull(resp.Body, buf)
	body := io.MultiReader(bytes.NewReader(buf[:n]), resp.Body)
	ct := resp.Header.Get("Content-Type")
	sniffed := http.DetectContentType(buf[:n])
	if !isImageType(sniffed) {
		http.Error(w, "Not an image", http.StatusBadGateway)
		return
	}
	if !isImageType(ct) {
		ct = sniffed
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = io.Copy(w, io.LimitReader(body, maxImageBytes))
}

func isImageType(ct string) bool {
	return len(ct) >= 6 && ct[:6] == "image/"
}

// publicIP reports whether ip is globally routable — the guard applied to
// every dial the proxy makes.
func publicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		// Reject loopback, private, link-local, CGNAT, multicast,
		// documentation and unspecified ranges.
		for _, cidr := range []string{
			"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
			"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24",
			"192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16",
			"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
			"224.0.0.0/4", "240.0.0.0/4",
		} {
			_, network, _ := net.ParseCIDR(cidr)
			if network.Contains(v4) {
				return false
			}
		}
		return true
	}
	// IPv6: loopback, link-local, unique-local, multicast.
	for _, cidr := range []string{"::1/128", "fe80::/10", "fc00::/7", "ff00::/8"} {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

// imgControl is the Dialer.Control hook validating every resolved address —
// including per-connection resolutions — so a hostname can't be rebound to a
// private IP between check and dial.
func imgControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errors.New("image proxy: non-IP address")
	}
	if !publicIP(ip) {
		return errors.New("image proxy: private address blocked")
	}
	return nil
}

// imgClient is the HTTP client used for all image fetches. Redirects are
// followed but re-validated: scheme, port, and the public-IP guard at dial
// time on every hop.
var imgClient = &http.Client{
	Timeout: imageFetchDist,
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: imageFetchDist, Control: imgControl}).DialContext,
		Proxy:                 nil, // no corporate proxy: direct egress, guarded
		MaxIdleConns:          32,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return errors.New("image proxy: too many redirects")
		}
		if !isHTTPScheme(req.URL.Scheme) {
			return errors.New("image proxy: redirect to non-http scheme")
		}
		if p := req.URL.Port(); p != "" && p != "80" && p != "443" {
			return errors.New("image proxy: redirect to unsupported port")
		}
		return nil
	},
}
