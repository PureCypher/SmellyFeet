# SmellyFeet Public Exposure via Cloudflare — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make SmellyFeet safe to expose fully-public on a Cloudflare domain: self-hosted assets (no CDN/JS), security headers, edge caching, Atom/OG/robots/favicon, and a docker-compose + cloudflared deployment for host 192.168.1.135.

**Architecture:** Compile Tailwind once with the standalone CLI into a committed `app.css`, embed it (plus fonts and favicon) via `go:embed`, and serve at `/static/`. One middleware layer adds security headers and request logging; handlers set per-route `Cache-Control`. A `deploy/` compose file runs the app plus a token-based cloudflared tunnel sidecar.

**Tech Stack:** Go 1.22 stdlib only (no new Go dependencies). Tailwind CSS standalone CLI v3.4.17 (build-time only). Docker Compose + `cloudflare/cloudflared`.

**Spec:** `docs/superpowers/specs/2026-07-03-cloudflare-public-design.md`

## Global Constraints

- Go 1.22, stdlib only — do **not** add any `go.mod` dependency.
- All Go code gofmt-clean; run tests with `-race`; keep coverage ≥ 80%.
- Commit format: `<type>: <description>` (feat/fix/docs/test/chore/build). No attribution trailers.
- Tailwind standalone CLI version: **v3.4.17** (matches Play-CDN v3 class semantics).
- After this work the site must make **zero external requests** and ship **zero JavaScript**.
- CSP must be exactly: `default-src 'none'; style-src 'self'; font-src 'self'; img-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'` — this forbids inline `style=""` attributes; the list page's staggered animation must use `nth-child` CSS instead.
- Existing tests in `internal/server/server_test.go` use `stubService` and `newTestServer(t, svc)`; new server tests must reuse them.

---

### Task 1: CSS build pipeline + static assets

**Files:**
- Create: `tailwind.config.js`
- Create: `assets/tailwind.input.css`
- Create: `scripts/build-css.sh`
- Create: `scripts/fetch-fonts.sh`
- Create: `internal/server/static/favicon.svg`
- Generate (committed): `internal/server/static/app.css`, `internal/server/static/fonts/*.woff2`
- Modify: `.gitignore` (add `.cache/`)

**Interfaces:**
- Consumes: nothing (first task).
- Produces: `internal/server/static/` populated with `app.css`, `favicon.svg`, `fonts/ibm-plex-sans-{400,500,600,700}.woff2`, `fonts/ibm-plex-mono-{400,500,600}.woff2`. Task 2 embeds this directory.

- [ ] **Step 1: Write `tailwind.config.js`** (theme copied verbatim from the inline Play-CDN config in `partials.html`)

```js
/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./internal/server/templates/*.html"],
  theme: {
    extend: {
      colors: {
        ink:  { 950: "#0a0a0c", 900: "#0f0f13", 850: "#15151b", 800: "#1c1c24", 700: "#26262f" },
        line: "#26262f",
        fog:  "#8b8b97",
        accent: { DEFAULT: "#f5b13d", bright: "#ffc964", dim: "#c08a33" },
      },
      fontFamily: {
        sans: ['"IBM Plex Sans"', "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ['"IBM Plex Mono"', "ui-monospace", "SFMono-Regular", "monospace"],
      },
    },
  },
};
```

- [ ] **Step 2: Write `assets/tailwind.input.css`** — Tailwind directives + `@font-face` + every rule currently in the inline `<style>` block of `partials.html`, plus `nth-child` stagger rules replacing the inline `animation-delay` style (page size is 20):

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

/* Self-hosted IBM Plex (latin subset). Paths relative to /static/app.css. */
@font-face { font-family: "IBM Plex Sans"; font-style: normal; font-weight: 400; font-display: swap; src: url("fonts/ibm-plex-sans-400.woff2") format("woff2"); }
@font-face { font-family: "IBM Plex Sans"; font-style: normal; font-weight: 500; font-display: swap; src: url("fonts/ibm-plex-sans-500.woff2") format("woff2"); }
@font-face { font-family: "IBM Plex Sans"; font-style: normal; font-weight: 600; font-display: swap; src: url("fonts/ibm-plex-sans-600.woff2") format("woff2"); }
@font-face { font-family: "IBM Plex Sans"; font-style: normal; font-weight: 700; font-display: swap; src: url("fonts/ibm-plex-sans-700.woff2") format("woff2"); }
@font-face { font-family: "IBM Plex Mono"; font-style: normal; font-weight: 400; font-display: swap; src: url("fonts/ibm-plex-mono-400.woff2") format("woff2"); }
@font-face { font-family: "IBM Plex Mono"; font-style: normal; font-weight: 500; font-display: swap; src: url("fonts/ibm-plex-mono-500.woff2") format("woff2"); }
@font-face { font-family: "IBM Plex Mono"; font-style: normal; font-weight: 600; font-display: swap; src: url("fonts/ibm-plex-mono-600.woff2") format("woff2"); }

html { color-scheme: dark; }
body {
  background-color: #0a0a0c;
  background-image:
    radial-gradient(1100px 520px at 50% -8%, rgba(245,177,61,0.08), transparent 62%),
    linear-gradient(180deg, #0c0c10 0%, #0a0a0c 55%);
  background-attachment: fixed;
}
body::before {
  content: ""; position: fixed; inset: 0; pointer-events: none; z-index: 0;
  background-image:
    linear-gradient(rgba(255,255,255,0.022) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255,255,255,0.022) 1px, transparent 1px);
  background-size: 46px 46px;
  -webkit-mask-image: radial-gradient(circle at 50% 0%, #000 0%, transparent 78%);
          mask-image: radial-gradient(circle at 50% 0%, #000 0%, transparent 78%);
}
header, main, footer { position: relative; z-index: 1; }
::selection { background: rgba(245,177,61,0.32); color: #fff; }
::-webkit-scrollbar { width: 11px; height: 11px; }
::-webkit-scrollbar-track { background: #0a0a0c; }
::-webkit-scrollbar-thumb { background: #2a2a33; border-radius: 8px; border: 2px solid #0a0a0c; }
::-webkit-scrollbar-thumb:hover { background: #3a3a46; }

/* Full-text typography (replaces the unavailable Tailwind prose plugin) */
.article-body {
  white-space: pre-line;
  font-size: 0.94rem;
  line-height: 1.85;
  color: #c4c7d0;
  max-width: 72ch;
  word-break: break-word;
}
.article-body a { color: #f5b13d; text-decoration: underline; text-underline-offset: 2px; }

@keyframes riseIn { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: none; } }
.reveal { opacity: 0; animation: riseIn .5s cubic-bezier(.21,.7,.2,1) forwards; }
@media (prefers-reduced-motion: reduce) { .reveal { animation: none; opacity: 1; } }

/* Staggered reveal without inline styles (CSP style-src 'self'); list page size is 20. */
.reveal:nth-child(2)  { animation-delay: 35ms; }
.reveal:nth-child(3)  { animation-delay: 70ms; }
.reveal:nth-child(4)  { animation-delay: 105ms; }
.reveal:nth-child(5)  { animation-delay: 140ms; }
.reveal:nth-child(6)  { animation-delay: 175ms; }
.reveal:nth-child(7)  { animation-delay: 210ms; }
.reveal:nth-child(8)  { animation-delay: 245ms; }
.reveal:nth-child(9)  { animation-delay: 280ms; }
.reveal:nth-child(10) { animation-delay: 315ms; }
.reveal:nth-child(11) { animation-delay: 350ms; }
.reveal:nth-child(12) { animation-delay: 385ms; }
.reveal:nth-child(13) { animation-delay: 420ms; }
.reveal:nth-child(14) { animation-delay: 455ms; }
.reveal:nth-child(15) { animation-delay: 490ms; }
.reveal:nth-child(16) { animation-delay: 525ms; }
.reveal:nth-child(17) { animation-delay: 560ms; }
.reveal:nth-child(18) { animation-delay: 595ms; }
.reveal:nth-child(19) { animation-delay: 630ms; }
.reveal:nth-child(20) { animation-delay: 665ms; }
```

- [ ] **Step 3: Write `scripts/fetch-fonts.sh`** (one-time; output committed)

```bash
#!/usr/bin/env bash
# One-time: download latin-subset IBM Plex woff2 files (committed to the repo).
set -euo pipefail
cd "$(dirname "$0")/.."
dir=internal/server/static/fonts
mkdir -p "$dir"
base=https://cdn.jsdelivr.net/fontsource/fonts
for w in 400 500 600 700; do
  curl -fsSL -o "$dir/ibm-plex-sans-$w.woff2" "$base/ibm-plex-sans@latest/latin-$w-normal.woff2"
done
for w in 400 500 600; do
  curl -fsSL -o "$dir/ibm-plex-mono-$w.woff2" "$base/ibm-plex-mono@latest/latin-$w-normal.woff2"
done
ls -l "$dir"
```

- [ ] **Step 4: Write `scripts/build-css.sh`**

```bash
#!/usr/bin/env bash
# Rebuild internal/server/static/app.css from templates + assets/tailwind.input.css.
# The output is committed; re-run only when templates or the theme change.
set -euo pipefail
cd "$(dirname "$0")/.."
TW_VERSION=v3.4.17
TW_BIN=".cache/tailwindcss-$TW_VERSION"
if [ ! -x "$TW_BIN" ]; then
  mkdir -p .cache
  curl -fsSL -o "$TW_BIN" "https://github.com/tailwindlabs/tailwindcss/releases/download/$TW_VERSION/tailwindcss-linux-x64"
  chmod +x "$TW_BIN"
fi
"$TW_BIN" -c tailwind.config.js -i assets/tailwind.input.css -o internal/server/static/app.css --minify
echo "wrote internal/server/static/app.css ($(wc -c < internal/server/static/app.css) bytes)"
```

- [ ] **Step 5: Write `internal/server/static/favicon.svg`**

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="7" fill="#0a0a0c"/><circle cx="16" cy="16" r="6" fill="#f5b13d"/></svg>
```

- [ ] **Step 6: Add `.cache/` to `.gitignore`**, make scripts executable, run both scripts

Run: `chmod +x scripts/*.sh && ./scripts/fetch-fonts.sh && ./scripts/build-css.sh`
Expected: 7 `.woff2` files listed; "wrote internal/server/static/app.css (… bytes)" with a non-trivial byte count (> 10000).

- [ ] **Step 7: Sanity-check the compiled CSS contains the custom theme classes used by templates**

Run: `grep -c "bg-ink-900\|text-fog\|bg-accent" internal/server/static/app.css`
Expected: count ≥ 1 (minified file, classes present).
Run: `grep -c "IBM Plex Sans" internal/server/static/app.css`
Expected: ≥ 1 (font-face emitted).

- [ ] **Step 8: Commit**

```bash
git add tailwind.config.js assets/ scripts/ internal/server/static/ .gitignore
git commit -m "build: add Tailwind build pipeline and self-hosted static assets"
```

---

### Task 2: Serve embedded static assets and swap the template head

**Files:**
- Modify: `internal/server/server.go` (embed static, asset hash, `/static/` route)
- Modify: `internal/server/templates/partials.html` (head rewrite)
- Modify: `internal/server/templates/list.html` (remove inline `style` attribute)
- Test: `internal/server/static_test.go`

**Interfaces:**
- Consumes: `internal/server/static/` from Task 1.
- Produces: `staticFS embed.FS` (rooted so `static/app.css` is a valid path), package-level `assetHash string`, funcMap entry `"assetHash"`, route `GET /static/`. Task 4 replaces the static handler's literal cache header with a constant.

- [ ] **Step 1: Write the failing test** — `internal/server/static_test.go`

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run 'TestStatic|TestPagesLink' -v`
Expected: FAIL (undefined `assetHash`, 404 on /static/app.css).

- [ ] **Step 3: Implement in `internal/server/server.go`**

Add imports `crypto/sha256`, `encoding/hex`. Below the templates embed:

```go
//go:embed static
var staticFS embed.FS

// assetHash is a short content hash of app.css, used to bust Cloudflare's
// immutable /static/ cache when the stylesheet changes.
var assetHash = func() string {
	b, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		return "dev"
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:4])
}()
```

Add to `funcMap`:

```go
"assetHash": func() string { return assetHash },
```

In `Routes()` add:

```go
staticSrv := http.FileServerFS(staticFS)
mux.Handle("GET /static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/") { // no directory listings
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	staticSrv.ServeHTTP(w, r)
}))
```

- [ ] **Step 4: Rewrite the `<head>` in `internal/server/templates/partials.html`** — replace everything from the `preconnect` links through the closing `</style>` with:

```html
  <link rel="icon" type="image/svg+xml" href="/static/favicon.svg">
  <link rel="stylesheet" href="/static/app.css?v={{ assetHash }}">
```

(The `<meta charset>`, `<meta viewport>`, and `<title>` lines stay.)

- [ ] **Step 5: Remove the inline stagger style in `internal/server/templates/list.html`**

Change:
```html
<li class="reveal" style="animation-delay:{{ mul $i 35 }}ms">
```
to:
```html
<li class="reveal">
```
Then remove `"mul"` from `funcMap` in `server.go` (now unused), and simplify the loop from `{{ range $i, $a := .Articles }}` to `{{ range $a := .Articles }}` (the index is no longer used).

- [ ] **Step 6: Run the full package tests**

Run: `go test ./internal/server/ -race`
Expected: PASS (existing render tests still pass — if a test asserted the old CDN head, update it to the new expectations).

- [ ] **Step 7: Commit**

```bash
git add internal/server/
git commit -m "feat(frontend): serve embedded static assets and drop CDN/Google Fonts"
```

---

### Task 3: Security headers + request logging middleware

**Files:**
- Create: `internal/server/middleware.go`
- Modify: `internal/server/server.go:84-92` (`Routes()` wraps the mux)
- Test: `internal/server/middleware_test.go`

**Interfaces:**
- Consumes: `Routes()` mux from Task 2.
- Produces: `withSecurityHeaders(http.Handler) http.Handler`, `withRequestLog(http.Handler) http.Handler`, `clientIP(*http.Request) string`. `Routes()` now returns `withRequestLog(withSecurityHeaders(mux))`.

- [ ] **Step 1: Write the failing test** — `internal/server/middleware_test.go`

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run 'TestSecurityHeaders|TestClientIP' -v`
Expected: FAIL (undefined `clientIP`, missing headers).

- [ ] **Step 3: Implement `internal/server/middleware.go`**

```go
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
```

In `server.go` `Routes()`, change the return to:

```go
return withRequestLog(withSecurityHeaders(mux))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat(frontend): security headers and request logging middleware"
```

---

### Task 4: Per-route Cache-Control (edge caching)

**Files:**
- Modify: `internal/server/server.go` (cache constants, `renderError`, static handler uses constant)
- Modify: `internal/server/handlers.go` (set cache per handler; `no-store` on 404/healthz)
- Test: `internal/server/cache_test.go`

**Interfaces:**
- Consumes: handlers and static route from Tasks 2–3.
- Produces: constants `cacheList, cacheArticle, cacheStats, cacheAbout, cacheFeed, cacheStatic, cacheNone string` and helper `setCache(w http.ResponseWriter, v string)`. Task 6's feed handler uses `cacheFeed`/`cacheNone`.

- [ ] **Step 1: Write the failing test** — `internal/server/cache_test.go`

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run 'TestCacheControl|TestUpstreamError' -v`
Expected: FAIL (missing Cache-Control headers).

- [ ] **Step 3: Implement** — in `server.go` add:

```go
// Cache-Control values; Cloudflare's edge honors s-maxage once the HTML
// cache rule from deploy/README.md is enabled.
const (
	cacheList    = "public, max-age=60, s-maxage=120, stale-while-revalidate=300"
	cacheArticle = "public, max-age=300, s-maxage=3600"
	cacheStats   = "public, max-age=30, s-maxage=60"
	cacheAbout   = "public, max-age=3600, s-maxage=86400"
	cacheFeed    = "public, max-age=300, s-maxage=300"
	cacheStatic  = "public, max-age=31536000, immutable"
	cacheNone    = "no-store"
)

func setCache(w http.ResponseWriter, v string) { w.Header().Set("Cache-Control", v) }
```

Replace the literal in the static handler with `setCache(w, cacheStatic)`. In `renderError`, add `setCache(w, cacheNone)` as the first line (errors must never be cached at the edge).

In `handlers.go`:
- `handleHealthz`: first line `setCache(w, cacheNone)`.
- `handleList`: after the `ListArticles` error check, `setCache(w, cacheList)`.
- `handleArticle`: `setCache(w, cacheNone)` before each `notfound` render; `setCache(w, cacheArticle)` after both error checks, before the success render.
- `handleStats`: after the error check, `setCache(w, cacheStats)`.
- `handleAbout`: first line `setCache(w, cacheAbout)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat(frontend): per-route Cache-Control for Cloudflare edge caching"
```

---

### Task 5: robots.txt + OpenGraph/meta description

**Files:**
- Modify: `internal/server/server.go` (`Routes()`: robots route)
- Modify: `internal/server/handlers.go` (`listView` fields, `trimDesc`, handler data)
- Modify: `internal/server/templates/partials.html` (conditional meta tags in head)
- Test: `internal/server/meta_test.go`

**Interfaces:**
- Consumes: `listView` struct and handlers from Task 4; `getPath` helper from Task 4's `cache_test.go`.
- Produces: `listView` gains `Desc string` and `OGArticle bool`; `trimDesc(s string, n int) string`; route `GET /robots.txt`. Template contract: header partial reads optional `.Desc`/`.OGArticle`.

- [ ] **Step 1: Write the failing test** — `internal/server/meta_test.go`

```go
package server

import (
	"net/http"
	"strings"
	"testing"

	"smellyfeet/internal/apiclient"
)

func TestRobotsTxt(t *testing.T) {
	h := newTestServer(t, stubService{})
	rec := getPath(t, h, "/robots.txt")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "User-agent: *") {
		t.Fatalf("robots.txt = %d %q", rec.Code, rec.Body.String())
	}
}

func TestArticlePageHasOpenGraphTags(t *testing.T) {
	sum := "Threat actors exploited a zero-day in WidgetCorp firewalls to gain initial access."
	h := newTestServer(t, stubService{article: apiclient.Article{
		ID: 7, Title: "WidgetCorp zero-day", URL: "https://example.com/a", Summary: &sum,
	}})
	body := getPath(t, h, "/article/7").Body.String()
	for _, want := range []string{
		`property="og:title" content="WidgetCorp zero-day"`,
		`property="og:type" content="article"`,
		`property="og:site_name" content="Information Broker"`,
		`name="description"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("article page missing %q", want)
		}
	}
}

func TestListPageHasDescriptionAndStatsDoesNot(t *testing.T) {
	h := newTestServer(t, stubService{})
	if !strings.Contains(getPath(t, h, "/").Body.String(), `name="description"`) {
		t.Error("list page missing meta description")
	}
	if strings.Contains(getPath(t, h, "/stats").Body.String(), `property="og:`) {
		t.Error("stats page should not emit OG tags")
	}
}

func TestTrimDesc(t *testing.T) {
	tests := []struct {
		name, in string
		n        int
		want     string
	}{
		{"short passes through", "hello world", 200, "hello world"},
		{"collapses whitespace", "a\n\n b", 200, "a b"},
		{"truncates with ellipsis", strings.Repeat("x", 300), 10, strings.Repeat("x", 10) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimDesc(tt.in, tt.n); got != tt.want {
				t.Fatalf("trimDesc = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run 'TestRobots|TestArticlePageHasOpenGraph|TestListPageHasDescription|TestTrimDesc' -v`
Expected: FAIL (no robots route, no OG tags, undefined `trimDesc`).

- [ ] **Step 3: Implement**

`handlers.go` — extend `listView` and add helper:

```go
type listView struct {
	Title     string
	Desc      string
	OGArticle bool
	Articles  []apiclient.Article
	Feeds     []apiclient.Feed
	Q         string
	Feed      string
	Page      int
	HasPrev   bool
	HasNext   bool
}

// trimDesc collapses whitespace and truncates to n runes for meta descriptions.
func trimDesc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
```

(Add `"strings"` to imports.) In `handleList`, set `Desc: "AI-summarized cybersecurity intelligence — the latest articles from monitored threat feeds."` in the `listView` literal. In `handleArticle`'s success render:

```go
desc := ""
if a.Summary != nil {
	desc = trimDesc(*a.Summary, 200)
}
s.render(w, http.StatusOK, "article", map[string]any{
	"Title":     a.Title,
	"Article":   a,
	"Desc":      desc,
	"OGArticle": true,
})
```

`server.go` `Routes()` — add:

```go
mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
	setCache(w, cacheAbout)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "User-agent: *\nAllow: /\n")
})
```

(Add `"io"` to imports.)

`partials.html` — in the head, after the `<title>` line:

```html
  {{ if .Desc }}<meta name="description" content="{{ .Desc }}">
  <meta property="og:site_name" content="Information Broker">
  <meta property="og:title" content="{{ .Title }}">
  <meta property="og:type" content="{{ if .OGArticle }}article{{ else }}website{{ end }}">
  <meta property="og:description" content="{{ .Desc }}">{{ end }}
```

Note: pages rendered with `map[string]any` lacking `Desc` skip the block (missing map keys are falsy); `listView` has the fields explicitly. The `error`/`notfound`/`stats`/`about` maps need no change.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat(frontend): robots.txt and OpenGraph/meta description tags"
```

---

### Task 6: Atom feed at /feed.xml

**Files:**
- Create: `internal/server/feed.go`
- Modify: `internal/server/server.go` (`Routes()`: feed route)
- Modify: `internal/server/templates/partials.html` (rel=alternate link)
- Test: `internal/server/feed_test.go`

**Interfaces:**
- Consumes: `setCache`/`cacheFeed`/`cacheNone` from Task 4; `ArticleService.ListArticles`; `getPath` from Task 4.
- Produces: `GET /feed.xml` returning `application/atom+xml`; `baseURL(r *http.Request) string` honoring `X-Forwarded-Proto` (cloudflared sets it to `https`).

- [ ] **Step 1: Write the failing test** — `internal/server/feed_test.go`

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run TestFeed -v`
Expected: FAIL (404 — route not registered).

- [ ] **Step 3: Implement `internal/server/feed.go`**

```go
package server

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"time"

	"smellyfeet/internal/apiclient"
)

const feedEntries = 50

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type atomEntry struct {
	Title   string   `xml:"title"`
	ID      string   `xml:"id"`
	Link    atomLink `xml:"link"`
	Updated string   `xml:"updated"`
	Summary string   `xml:"summary,omitempty"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomDoc struct {
	XMLName xml.Name    `xml:"feed"`
	Xmlns   string      `xml:"xmlns,attr"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Author  atomAuthor  `xml:"author"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

// baseURL reconstructs the public origin. cloudflared sets X-Forwarded-Proto.
func baseURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	res, err := s.svc.ListArticles(r.Context(), apiclient.ListParams{Limit: feedEntries})
	if err != nil {
		setCache(w, cacheNone)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}

	base := baseURL(r)
	now := time.Now().UTC().Format(time.RFC3339)
	doc := atomDoc{
		Xmlns:   "http://www.w3.org/2005/Atom",
		Title:   "Information Broker",
		ID:      base + "/",
		Updated: now,
		Author:  atomAuthor{Name: "Information Broker"},
		Links: []atomLink{
			{Href: base + "/feed.xml", Rel: "self", Type: "application/atom+xml"},
			{Href: base + "/"},
		},
	}
	for _, a := range res.Articles {
		link := fmt.Sprintf("%s/article/%d", base, a.ID)
		e := atomEntry{Title: a.Title, ID: link, Link: atomLink{Href: link}, Updated: now}
		if !a.PublishedAt.IsZero() {
			e.Updated = a.PublishedAt.UTC().Format(time.RFC3339)
		}
		if a.Summary != nil {
			e.Summary = *a.Summary
		}
		doc.Entries = append(doc.Entries, e)
	}

	setCache(w, cacheFeed)
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	if err := xml.NewEncoder(w).Encode(doc); err != nil {
		log.Printf("feed encode: %v", err) // headers already sent; log only
	}
}
```

Register in `Routes()`:

```go
mux.HandleFunc("GET /feed.xml", s.handleFeed)
```

- [ ] **Step 4: Advertise the feed** — in `partials.html` head, after the stylesheet link:

```html
  <link rel="alternate" type="application/atom+xml" title="Information Broker" href="/feed.xml">
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/server/ -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/
git commit -m "feat(frontend): Atom feed at /feed.xml"
```

---

### Task 7: Deploy-as-code (docker-compose + cloudflared)

**Files:**
- Create: `deploy/docker-compose.yml`
- Create: `deploy/.env.example`
- Create: `deploy/README.md`

**Interfaces:**
- Consumes: the repo Dockerfile (unchanged).
- Produces: a compose stack the homelab host runs; nothing in Go consumes this.

- [ ] **Step 1: Write `deploy/docker-compose.yml`**

```yaml
services:
  smellyfeet:
    build:
      context: ..
    restart: unless-stopped
    environment:
      API_BASE_URL: ${API_BASE_URL:-http://host.docker.internal:8080}
    ports:
      - "3000:3000"           # keep LAN access at http://192.168.1.135:3000
    extra_hosts:
      - "host.docker.internal:host-gateway"

  cloudflared:
    image: cloudflare/cloudflared:latest
    restart: unless-stopped
    command: tunnel run --token ${TUNNEL_TOKEN}
    depends_on:
      - smellyfeet
```

- [ ] **Step 2: Write `deploy/.env.example`**

```bash
# Cloudflare Tunnel token (Zero Trust -> Networks -> Tunnels -> your tunnel -> token)
TUNNEL_TOKEN=

# Where the Information-Broker API lives, as seen from inside the container.
# host.docker.internal resolves to the docker host (192.168.1.135).
API_BASE_URL=http://host.docker.internal:8080
```

- [ ] **Step 3: Write `deploy/README.md`**

```markdown
# Deploying SmellyFeet publicly via Cloudflare Tunnel

Target host: 192.168.1.135 (LAN). No router port forwarding is needed; the only
inbound path is Cloudflare -> tunnel -> `smellyfeet` container.

## One-time Cloudflare setup (dashboard)

1. **Create the tunnel:** Zero Trust -> Networks -> Tunnels -> *Create a tunnel*
   -> Cloudflared -> name it `smellyfeet` -> copy the **token** (long string
   starting `eyJ`).
2. **Public hostname:** in the tunnel config, add a public hostname:
   - Subdomain/domain: `smellyfeet.<yourdomain>` (any name you like)
   - Service: `HTTP` -> `smellyfeet:3000`
   (cloudflared and the app share a compose network, so the container name resolves.)
3. **Edge-cache the HTML:** Caching -> Cache Rules -> *Create rule*:
   - Name: `smellyfeet html`
   - When: Hostname equals `smellyfeet.<yourdomain>`
   - Then: **Eligible for cache**, Edge TTL: "Use cache-control header if present".
   The app sends `s-maxage` per route and `no-store` on errors, so the edge does
   the right thing.

## On the host

    git clone https://github.com/PureCypher/SmellyFeet.git && cd SmellyFeet   # or git pull
    cp deploy/.env.example deploy/.env
    # edit deploy/.env: paste TUNNEL_TOKEN, adjust API_BASE_URL if the broker
    # API is not on the host at :8080
    docker compose --project-directory deploy up -d --build

Check: `docker compose --project-directory deploy logs -f cloudflared` should
show "Registered tunnel connection". Then open `https://smellyfeet.<yourdomain>`.

## Updating

    git pull
    docker compose --project-directory deploy up -d --build

## Notes

- LAN access continues to work at http://192.168.1.135:3000.
- If the Information-Broker API runs in its own compose stack, replace the
  `API_BASE_URL` host with that stack's published address, or attach both
  stacks to a shared external network.
- Never commit `deploy/.env` (the repo `.gitignore` already ignores `.env`).
```

- [ ] **Step 4: Validate the compose file**

Run: `cd deploy && TUNNEL_TOKEN=dummy docker compose config -q && echo VALID; cd ..`
Expected: `VALID`. If docker is unavailable locally, run `python3 -c "import yaml; yaml.safe_load(open('deploy/docker-compose.yml'))" && echo VALID`.

- [ ] **Step 5: Confirm `.gitignore` covers `deploy/.env`**

Run: `git check-ignore -v deploy/.env || echo NOT_IGNORED`
Expected: a match from the existing `.env` pattern. If `NOT_IGNORED`, add `deploy/.env` to `.gitignore` in this commit.

- [ ] **Step 6: Commit**

```bash
git add deploy/ .gitignore
git commit -m "build: docker-compose deployment with cloudflared tunnel sidecar"
```

---

### Task 8: Full verification + README update

**Files:**
- Modify: `README.md` (public deployment section, feed mention, CSS build note)
- Test: whole repo

**Interfaces:**
- Consumes: everything above.
- Produces: green build, updated docs.

- [ ] **Step 1: Run the full suite**

Run: `gofmt -l . && go vet ./... && go test -race -cover ./...`
Expected: `gofmt -l` prints nothing; vet clean; all tests PASS with `internal/server` and `internal/apiclient` coverage ≥ 80%.

- [ ] **Step 2: Update `README.md`**

In **Run**, after the `go run .` block add:

```markdown
Styling is a pre-built, committed CSS file. If you change templates or the theme,
regenerate it with `./scripts/build-css.sh` (downloads the Tailwind standalone CLI
on first run).
```

Add a new section after **Configuration**:

```markdown
## Public deployment

The site is designed to be exposed through a Cloudflare Tunnel — see
[deploy/README.md](deploy/README.md). The app sends per-route `Cache-Control`
headers (articles cache at the edge for an hour), strict security headers, and
serves everything — CSS, fonts, favicon — from the binary with zero JavaScript
and zero third-party requests. An Atom feed is available at `/feed.xml`.
```

- [ ] **Step 3: Boot check** (manual smoke)

Run: `go build -o /tmp/smellyfeet-check . && rm /tmp/smellyfeet-check && echo BUILD_OK`
Expected: `BUILD_OK`.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document CSS pipeline and public deployment"
```
