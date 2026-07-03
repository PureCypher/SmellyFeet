# SmellyFeet: Public Exposure via Cloudflare — Design

**Date:** 2026-07-03
**Status:** Approved
**Goal:** Make SmellyFeet fully public on a Cloudflare-managed domain, served from the
homelab host `192.168.1.135` via a Cloudflare Tunnel, hardened and edge-cached, with
reader-facing polish (Atom feed, OpenGraph, favicon).

## Context

SmellyFeet is a stdlib-only, server-rendered Go frontend for the Information-Broker API.
Templates are embedded via `go:embed`; a multi-stage Dockerfile produces a scratch image.
Today the templates load Tailwind through the Play CDN (`cdn.tailwindcss.com`) and fonts
from Google Fonts. The site will be **fully public** (no Cloudflare Access). The user's
domain is already on Cloudflare.

Decision: **Approach A** — build-time CSS with self-hosted assets, hardening middleware,
edge caching, and a cloudflared sidecar in docker-compose. (Rejected: B "expose only,
keep CDN" — third-party runtime dependency and weak CSP on a public site; C "hand-write
CSS" — largest diff, visual-regression risk.)

## 1. CSS pipeline & self-hosted assets

- New `internal/server/static/` directory, embedded via `go:embed`, served at `/static/`:
  - `app.css` — compiled Tailwind output merged with the current inline `<style>` rules
    and `@font-face` declarations.
  - IBM Plex Sans (400/500/600/700) and IBM Plex Mono (400/500/600) as `.woff2`.
  - `favicon.svg`.
- Tailwind is compiled with the **standalone CLI** (no Node toolchain in the repo):
  - `tailwind.config.js` carries the theme currently defined in the inline JS config
    (ink/line/fog/accent colors, IBM Plex font stacks).
  - Content globs: `internal/server/templates/*.html`.
  - `build-css.sh` downloads/uses the standalone binary and writes minified
    `internal/server/static/app.css`. **The output is committed**, so `go run .` and
    `go build` need no CSS tooling. The script is re-run only when templates or theme
    change.
- Templates: remove the Play CDN `<script>`, the inline `tailwind.config` script, the
  Google Fonts `<link>`s, and the inline `<style>` block. Replace with one
  `<link rel="stylesheet" href="/static/app.css?v={hash}">` where `{hash}` is a short
  content hash of the embedded CSS computed once at startup (cache busting).
- Result: the site serves **zero JavaScript** and makes zero third-party requests.

## 2. Hardening middleware

One `internal/server/middleware.go` wrapping the mux, applied in `Routes()`:

- `Content-Security-Policy: default-src 'none'; style-src 'self'; font-src 'self';
  img-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'`
- `X-Content-Type-Options: nosniff`
- `Referrer-Policy: strict-origin-when-cross-origin`
- Request logging: method, path, status, duration, client IP taken from
  `CF-Connecting-IP` when present, else `RemoteAddr`.

Rate limiting, TLS, and HSTS are Cloudflare's job at the edge; the origin is only
reachable through the tunnel.

## 3. Edge caching

Per-route `Cache-Control` set by the handlers/middleware:

| Route            | Cache-Control                                            |
|------------------|----------------------------------------------------------|
| `/` (list, search, filter) | `public, max-age=60, s-maxage=120, stale-while-revalidate=300` |
| `/article/{id}`  | `public, max-age=300, s-maxage=3600`                     |
| `/stats`         | `public, max-age=30, s-maxage=60`                        |
| `/about`         | `public, max-age=3600, s-maxage=86400`                   |
| `/feed.xml`      | `public, max-age=300, s-maxage=300`                      |
| `/static/*`      | `public, max-age=31536000, immutable`                    |
| errors & 404s    | `no-store` (Cloudflare must never cache an outage page)  |

Cloudflare side (documented in the deploy guide): one Cache Rule making HTML responses
eligible for edge cache while honoring origin `Cache-Control`.

## 4. Reader features

- `GET /feed.xml` — Atom feed (`encoding/xml`) of the latest 50 articles; entry title =
  article title, content = AI summary, link = `/article/{id}` absolute URL derived from
  the request host. Advertised via `<link rel="alternate" type="application/atom+xml">`.
- OpenGraph + `meta name="description"` on list and article pages: `og:site_name`,
  `og:title`, `og:type` (`website`/`article`), `og:description` = summary trimmed to
  ~200 chars.
- `GET /robots.txt` — allow all.
- `favicon.svg` linked from the header partial.

## 5. Deploy-as-code

New `deploy/` directory:

- `deploy/docker-compose.yml`:
  - `smellyfeet`: built from the repo Dockerfile; `API_BASE_URL` from `.env`;
    port `3000:3000` published so LAN access at `192.168.1.135:3000` keeps working.
  - `cloudflared`: official `cloudflare/cloudflared` image, remotely-managed tunnel:
    `tunnel run --token ${TUNNEL_TOKEN}`; shares the compose network and proxies the
    public hostname to `http://smellyfeet:3000`. No router port forwarding; the origin
    is unreachable from the internet except through Cloudflare.
- `.env.example` gains `TUNNEL_TOKEN=`.
- `deploy/README.md`: dashboard steps (Zero Trust → Networks → Tunnels → create
  "smellyfeet" tunnel → copy token → public hostname `smellyfeet.<domain>` →
  `http://smellyfeet:3000`), the HTML cache rule, and the on-host commands
  (`git pull && docker compose -f deploy/docker-compose.yml up -d --build`).

## 6. Error handling & testing

- Existing 502 (`renderError`) and 404 paths unchanged apart from `no-store`.
- Table-driven tests, run with `-race`, keeping ≥80% coverage:
  - middleware sets all security headers on every route;
  - per-route `Cache-Control` values, including `no-store` on error/404 responses;
  - `/feed.xml` produces valid, escaped Atom XML and correct entry links;
  - `/static/` serves embedded assets with immutable cache headers;
  - article/list renders include OG tags and the stylesheet link.

## Data flow (unchanged shape)

```
browser → Cloudflare edge (cache, TLS, rate limits) → tunnel → SmellyFeet → Information-Broker API
```

## Out of scope

- Cloudflare Access / auth (site is fully public by decision).
- Changes to the Information-Broker API or scraper.
- Sitemap generation, analytics, comments.
