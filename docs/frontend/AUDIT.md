# SmellyFeet Frontend Audit — Phase 0

**Scope:** SmellyFeet (public frontend, this repo) + Information-Broker (backend API/ingestion, sibling repo at `/home/pure/Documents/github/Information-Broker`), audited live at `feed.purecypher.com` and via source inspection, ahead of the "Cyber Terminal" redesign.

**Method:** Six independent read-only research passes (routes/framework, template/design system, API contracts, security posture, testing/CI/deployment, accessibility + prior design history) plus a cross-check pass for contradictions and gaps, all citing `file:line`. Followed by live route screenshots at desktop (1440×900) and mobile (390×844) widths against the deployed site.

**Headline correction to the master prompt's hypothesis:** the backend is **not** Python/Flask/FastAPI. Both services are Go. SmellyFeet is a zero-dependency (`go.mod` has no requires) stdlib `net/http` + `html/template` server; Information-Broker is a separate Go service (goquery, gofeed, lib/pq, Prometheus client) backed by PostgreSQL 15, with an Ollama sidecar for embeddings. This changes every downstream architecture decision in Phase 2 — there is no Jinja/Django template layer, no ASGI, no Python packaging story.

---

## 1. Confirmed stack and route inventory

**Frontend (SmellyFeet):** Go 1.22, stdlib only — no router library, no template engine beyond `html/template`, no JS framework, no JS at all (zero `<script>` tags anywhere; CSP has no `script-src`, inherited `'none'`). Docker: multi-stage build → `FROM scratch`, static binary, non-root (`USER 65534:65534`).

**Backend (Information-Broker):** Go, stdlib `net/http` mux (legacy path-only patterns, no method routing), PostgreSQL 15, goquery for HTML content extraction, gofeed for RSS parsing, Prometheus client for metrics, Ollama (`nomic-embed-text`) for story-clustering embeddings.

### Route inventory (SmellyFeet, `internal/server/server.go:163-190`)

| Method + Path | Handler | Template | File:line |
|---|---|---|---|
| `GET /healthz` | `handleHealthz` | none — plain "ok" | `handlers.go:53` |
| `GET /{$}` (exact root only) | `handleList` | `list` | `handlers.go:140` |
| `GET /digest` | `handleDigest` | `digest` | `handlers.go:203` |
| `GET /article/{id}` | `handleArticle` | `article` / `notfound` | `handlers.go:227` |
| `GET /stats` | `handleStats` | `stats` | `handlers.go:262` |
| `GET /about` | `handleAbout` | `about` | `handlers.go:283` |
| `GET /feed.xml` | `handleFeed` | none — Atom XML via `encoding/xml` | `feed.go:53` |
| `GET /robots.txt` | inline closure | none | `server.go:172-176` |
| `GET /static/` | inline closure over `http.FileServerFS` | none | `server.go:178-188` |

`GET /{$}` is an **exact-match** pattern (Go 1.22 mux syntax) — unknown paths 404 via the mux rather than falling through to the list handler.

### Rendering pipeline

`html/template`, parsed once at startup from `//go:embed templates/*.html` (`server.go:24-25, 154-160`) with a custom `FuncMap` (`server.go:71-80`): `formatDate`, `cleanContent`, `sourceName`, `relTime`, `cveID`, `inc`, `dec`, `assetHash`. Not a base-layout system — `partials.html` defines `header`/`footer` templates, and every page template opens with `{{ template "header" . }}` and closes with `{{ template "footer" }}`. `Server.render()` (`server.go:198-209`) executes into a `bytes.Buffer` first, so a template error yields a clean 500 instead of a torn page.

### Middleware (`internal/server/middleware.go`)

Exactly two, composed at `server.go:189` as `withRequestLog(withSecurityHeaders(mux))`:
- **Security headers** (`middleware.go:35-43`) — CSP, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin` (full CSP in §5).
- **Request logging** (`middleware.go:45-53`) — method/URI/status/duration/client-IP, preferring `CF-Connecting-IP` (`middleware.go:25-33`).

No recovery middleware, no rate limiting, no compression. Server timeouts in `main.go:25-32` (ReadHeader 5s, Read 15s, Write 30s, Idle 60s).

### Frontend → backend communication

`internal/apiclient/apiclient.go` — **synchronous per-request HTTP calls, zero in-process caching or polling.** One `http.Client` with a 10s timeout (`apiclient.go:74-85`); all methods funnel through `getJSON` (`apiclient.go:87-108`), which maps 404 → `ErrNotFound`. `handleList` calls `ListArticles` then `ListFeeds` (feeds failure is non-fatal — dropdown just empties, `handlers.go:150-165`); `handleStats` calls `GetStats` then `ListFeeds` (`handlers.go:263-272`).

The only caching layer is **HTTP-header-based, for Cloudflare's edge** — per-route `Cache-Control` constants (`server.go:43-51`): list 60s/120s s-maxage+SWR, article 300s/3600s, stats 30s/60s, about 3600s/86400s, static `immutable` 1y, errors `no-store`. Every page fully depends on Information-Broker being reachable; there is no in-process stale-content fallback (`renderError`, `server.go:211-218`, returns 502 on backend failure).

### Notable routing behaviors

- Digest is a **named range**, not a date range: `?range=daily|weekly|monthly`, whitelisted independently on both sides (frontend `handlers.go:190,204-207`, backend `digest.go:13-22`) — no arbitrary date params exist.
- Article detail is REST-shaped on the frontend (`GET /article/{id}`, Go 1.22 `PathValue`) but translates to the backend's query-string shape (`GET /articles/get?id=N`) — a shape mismatch to keep in mind for any Phase 2 endpoint work.
- Stats bar-chart widths are quantized to steps of 5 (5–100) and rendered as static `bar-N` CSS classes (`handlers.go:75-104`) **because CSP forbids inline `style=` attributes** — this pattern will recur for any new chart.
- Backend silently drops `q` searches under 2 characters (`Information-Broker/api.go:104-107`); the frontend does not validate this, so a 1-char search shows an active filter chip over unfiltered results (tracked as a contradiction in §9).

---

## 2. Current component/template inventory

| File | Purpose |
|---|---|
| `internal/server/templates/partials.html` | `header` + `footer` — full HTML skeleton, sticky nav (Feed/Digest/Stats/About), footer |
| `internal/server/templates/list.html` | `articleCard` partial (shared with digest) + `list` page (`/`) |
| `internal/server/templates/article.html` | Article detail page |
| `internal/server/templates/digest.html` | Digest page |
| `internal/server/templates/stats.html` | Stats page |
| `internal/server/templates/about.html` | About page |
| `internal/server/templates/error.html`, `notfound.html` | Error/404 pages |
| `assets/tailwind.input.css` | Tailwind source — fonts, background treatment, `.article-body`, `.reveal` animation, `.bar-N` width classes |
| `tailwind.config.js` | Custom palette + font stacks |
| `internal/server/static/app.css` | Compiled/embedded output, served with a content-hash cache-buster (`?v={{ assetHash }}`) |
| `internal/server/static/favicon.svg` | Dark rounded square + amber dot |
| `internal/server/static/fonts/ibm-plex-{sans,mono}-*.woff2` | Self-hosted IBM Plex, **latin subset only** |

**Only one reusable component exists today:** `{{ define "articleCard" }}` (`list.html:1-20`), invoked from `list` (main list + "upcoming" block) and `digest` (Important + "everything else"). Renders: source pill (links to `/?feed=...`), CVE badge (server-side regex `CVE-\d{4}-\d{4,}` extraction, `server.go:148`), "N sources" badge (`cross_feed_count > 1`), relative time, stretched-link title, `line-clamp-3` summary.

### Per-route rendering today

- **`/` (list)** — search box, source `<select>`, sort `<select>` (newest/oldest), removable filter chips, collapsible `<details>` "Upcoming" section (future-dated items, page 1 + default sort only), article cards, Prev/Next pagination.
- **`/digest`** — range `<select>`, "since {date}", `.Important` cards, `.Other` in a collapsed `<details>`.
- **`/article/{id}`** — source pill, relative time, title, "▍ AI Summary" callout, full-text body (`white-space: pre-line`, not HTML), "Read original ↗" (`rel="noopener noreferrer"`, `target="_blank"`).
- **`/stats`** — 3 stat tiles (total articles/feeds/last-fetch), "articles collected" (today/week/month) and "top sources" (top 15) as quantized CSS bar charts.
- **`/about`** — static copy, pipeline diagram as a flex pill chain (RSS → LLM summarize → PostgreSQL → Broker API → SmellyFeet), feature grid.

### Visual design conventions already in place

Dark-only "terminal/intel" theme (`tailwind.config.js`): `ink` scale (`950:#0a0a0c` → `700:#26262f`), `line:#26262f` border, `fog:#8b8b97` muted text, and a **single amber/gold accent** (`accent: {DEFAULT:#f5b13d, bright:#ffc964, dim:#c08a33}`) used everywhere — nav hover, buttons, bars, links, selection highlight. No cyan or indigo exists anywhere in the codebase. The only severity color in use is red, on the error page only. Fixed radial amber glow + a 46px grid overlay give the "blueprint" background look. Typography: IBM Plex Sans for prose, IBM Plex Mono (uppercase, wide tracking, `// comment`-style eyebrows) for all nav/labels/metadata/stats — this convention already matches the master prompt's mono-for-metadata guidance. No dark/light toggle exists (`color-scheme: dark` is hardcoded; no `prefers-color-scheme` query, no JS to implement a toggle even if wanted).

**Gap vs. the requested "Cyber Terminal" palette:** the brief calls for cyan/indigo actives, amber warnings, red critical, green healthy. The shipped palette has amber as the *only* accent and red only on the hard-error page — there is no severity scale (warning/critical/healthy) distinction anywhere today. This is a real palette-design decision for Phase 1, not a two-line token swap.

---

## 3. Existing API and data-shape summary

All backend routes at `Information-Broker/api.go:59-66`; client at `internal/apiclient/apiclient.go`. **Fully public — no auth, no API key, no session, anywhere in the handler chain** (`api.go:43-66`).

| Endpoint | Params | Response shape | Notes |
|---|---|---|---|
| `GET /articles` | `limit` (≤100, default 50), `offset`, `feed` (exact match), `q` (ILIKE, trigram-indexed, <2 chars ignored server-side), `sort` (`oldest` only; anything else = newest) | `{articles:[ArticleView], count, limit, offset}` | `count` is **page size**, not a total — true "page X of Y" / "N results" is not derivable today |
| `GET /articles/get?id=N` | `id` (positive int64) | bare `ArticleView` | 404 → client `ErrNotFound`; invalid id → 400 |
| `GET /articles/digest?range=` | `daily`\|`weekly`\|`monthly` (else → daily) | `{range, since, important:[ArticleView], other:[ArticleView]}` | **No pagination** — returns every article in the window; `important` = ≥2 other feeds share its `story_cluster_id` |
| `GET /stats` | none | 9 fields (see below) | Frontend `Stats` struct only decodes 6 of them |
| `GET /feeds` | none | `{feeds:[{feed_url, article_count, latest_article, oldest_article, avg_fetch_duration_ms}], count}` | Frontend only decodes `feed_url` + `article_count` |

`ArticleView` fields: `id`, `title`, `url`, `summary` (nullable), `content` (plain text, HTML already stripped), `published_at`, `feed_url`, `content_hash`, `cross_feed_count` (digest-only).

**`/stats` full response** (`api.go:388-398`): `total_articles`, `total_feeds`, `last_fetch`, `successful_fetches_24h`, `failed_fetches_24h`, `avg_fetch_time_ms`, `articles_today`, `articles_this_week`, `articles_this_month`. The frontend already drops the three fetch-health fields — a one-line addition to `apiclient.go:55-62` to surface them, no backend change needed.

**Backend endpoints not currently called by the frontend:** `/articles/latest`, `/summarization/stats` (total/failed summaries, avg processing ms, most recent error), `/health`, `/metrics` (Prometheus).

### Schema (frontend-relevant, `Information-Broker/schema.sql`)

Single `articles` table: `id`, `title`, `url` (unique), `publish_date`, `summary` (nullable until summarized), `full_content` (nullable, plain text), `fetch_time`, `feed_url`, `content_hash` (unique), `fetch_duration_ms`, `summary_embedding real[]`, `story_cluster_id bigint` (self-referencing seed article, indexed). No separate `feeds` or `clusters` table — feeds are distinct `feed_url` values, clusters are `story_cluster_id` groupings. Trigram GIN indexes back `q` search. `fetch_logs` and `summary_logs` tables exist (audit trail) but no endpoint exposes them per-feed.

### What `/stats` can honestly show today vs. what needs backend work

**Available now:** totals, 24h/7d/30d ingestion velocity (frontend already charts this per recent work), fetch health (last fetch, success/fail counts, avg duration — wired on the backend, not yet decoded by the client), per-feed article counts/dates via `/feeds`, digest important-vs-other counts (derivable client-side from `/articles/digest`).

**Needs a new backend endpoint:** any clustering/embedding-coverage stats (the data exists in `story_cluster_id`/`summary_embedding` but nothing aggregates it), total-matching-count for filtered/searched lists, day-by-day historical time series (current stats are point-in-time window aggregates only), per-feed error rates (fetch_logs has the rows but nothing joins them per-feed), summarization queue depth/failures (exists at `/summarization/stats`, just unused by the frontend today).

---

## 4. Current UX debt by route

- **`/` (list)** — 1-char searches silently return unfiltered results while showing an active filter chip (contract mismatch, §9). Raw feed URL and absolute timestamps live only in `title` attributes — unreachable by keyboard/touch (`list.html:7,11`). Filter-chip removal links read "source: X ×" with no accessible description of the action. Pagination has no total-count context ("Page 1" with no "of N").
- **`/digest`** — **unbounded**: returns every article in the selected window with no pagination; a monthly digest will grow without a backend limit. "Important" section label is a `<p>`, not a heading — screen-reader users can't jump to it structurally.
- **`/stats`** — if `/feeds` fails, the "top sources" section **silently vanishes** with no error state (`handlers.go:270-272`) — a redesigned stats page needs an explicit partial-failure convention, not silent disappearance. Bar-label truncation (`w-40 md:w-56 truncate`) on narrow screens (confirmed in the mobile-390 screenshot, §8).
- **`/article/{id}`** — content is plain text forever (backend discards all markup at ingest via `goquery .Text()`); no rich formatting is recoverable without re-ingesting the corpus. New-tab "Read original" link has no textual "(opens in new tab)" cue (icon is `aria-hidden`).
- **Global** — no skip link (sticky header + 4 nav links precede `<main>` on every page); no current-page indication in nav (all 4 links look identical regardless of route); two unlabeled `<nav>` landmarks (header nav vs. pagination nav); several text-contrast failures below AA (footer text and the Atom-feed link at 2.56:1; "No summary available" placeholders at 3.96:1); weak focus rings on `<select>` elements (1px border-color change only, no `:focus-visible` anywhere in the stylesheet); non-latin article titles fall back to system fonts (only a latin glyph subset is embedded).

---

## 5. Security findings relevant to frontend rendering

### CSP currently deployed (`internal/server/middleware.go:10`, applied via `withSecurityHeaders`, verified by `middleware_test.go:17`)

```
default-src 'none'; style-src 'self'; font-src 'self'; img-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'
```

No `script-src` (inherits `'none'` — all script execution blocked by design), no `connect-src`/`object-src`/`frame-src` (all `'none'` by fallback). This is a **deliberate, twice-reaffirmed decision** (`docs/superpowers/specs/2026-07-03-cloudflare-public-design.md`, `docs/superpowers/specs/2026-07-03-ui-refinement-design.md`) — zero JavaScript, zero third-party requests, zero inline styles.

### Other headers (`middleware.go:39-40`)

`X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`. No explicit `X-Frame-Options` (covered by `frame-ancestors 'none'` in modern browsers). No HSTS/TLS/rate-limiting at origin — explicitly delegated to Cloudflare's edge per the design doc, since the origin is only reachable through the tunnel. **No `Permissions-Policy` header exists.**

### RSS content sanitization — two independent layers, correcting a common misconception

`Information-Broker/sanitize.go` is **not an HTML allowlist sanitizer** — it's UTF-8 validity + rune-safe truncation only (no bluemonday, no allowlist library in `go.mod`). The actual HTML defanging happens earlier: `extractMainContent` (`Information-Broker/monitor.go:571-611`) parses fetched pages with goquery, strips `script`/`style`/widget nodes, and stores only `.Text()` — **all markup is discarded at ingest time; Postgres never holds HTML.** On the frontend side, Go `html/template` contextual auto-escaping is the second layer — grep confirms **zero uses of `template.HTML`/`template.URL`** anywhere in the repo, so even if raw markup somehow reached a template, it would render as escaped text, not execute. The Atom feed (`feed.go`) uses `encoding/xml` marshaling, which XML-escapes titles/summaries independently. Net effect: this is already a defense-in-depth design that meets the master prompt's "treat RSS content as hostile" requirement — no remediation needed here, only preservation.

### External link handling

The only external link (`article.html:23`, "Read original") correctly pairs `target="_blank"` with `rel="noopener noreferrer"`. No explicit URL-scheme allowlist exists, but `html/template`'s contextual URL filter neutralizes dangerous schemes (e.g. `javascript:` → `#ZgotmplZ`) automatically for any `href="{{ .URL }}"`; CSP `default-src 'none'` is a second layer. One minor correctness quirk (not XSS): pagination links interpolate `.Q`/`.Feed` without `| urlquery` in two places (`list.html:83,85`) — auto-escaping still prevents injection, but an `&` in a query value could split params.

### Deployment topology and secrets

Two containers behind a Cloudflare Tunnel: `smellyfeet` (also published to LAN at `192.168.1.135:3000` — **this LAN path bypasses Cloudflare's edge entirely**, no rate limiting, spoofable `CF-Connecting-IP` in logs from that path) and a `cloudflared` sidecar. `TUNNEL_TOKEN` is environment-only (never in argv), templated in `deploy/.env.example`; the populated `deploy/.env` is gitignored and confirmed untracked. No secret values were reproduced during this audit.

### Gaps worth carrying into Phase 2 (observations, not yet remediated)

No `Permissions-Policy` header; no explicit URL-scheme allowlist (relies on html/template's built-in filter, which is adequate but not documented as a control); Information-Broker's page-fetch step (`monitor.go:614-646`) will fetch any URL a feed supplies — an SSRF surface on the backend, out of this repo's control but worth naming in the security review; LAN exposure at `192.168.1.135:3000` bypasses all edge protections.

---

## 6. Accessibility findings

The site ships **zero JavaScript**, which resolves most keyboard-operability concerns by construction (native `<details>/<summary>` accordions, native `<select>` filters in GET forms, plain pagination links — nothing requires a click handler to operate).

**Already correct — do not regress these in the redesign:** semantic landmarks (`<header>`, `<nav>`, `<main>`, `<footer>`), `<html lang="en">`, zoom-permitting viewport meta, `prefers-reduced-motion` support on the one keyframe animation, all form controls have explicit `<label for>`, CVE/cross-feed badges carry text (not color-only), `<time datetime>` used throughout, decorative icons are `aria-hidden`, the external link has `rel="noopener noreferrer"`.

**Concrete gaps** (all with source citations for direct fixing in Phase 3):
1. No skip-to-content link (`partials.html:16-31`).
2. No current-page nav indication — no `aria-current="page"`, no active-state styling (`partials.html:24-27`).
3. Contrast failures against the actual palette: `text-zinc-600` (2.56:1, fails AA) on footer text and the Atom-feed link; `text-zinc-500` (3.96:1) on "No summary available" placeholders; `text-zinc-700` (1.89:1) on disabled Prev/Next — likely exempt as inactive UI but still conveys state.
4. No `:focus-visible` styles anywhere; `<select>` elements rely on a thin 1px border-color change as their only focus cue (`list.html:36,45`; `digest.html:11`).
5. "Important"/"// articles collected"/"// top sources" section labels are `<p>` elements, not headings — not reachable via screen-reader heading navigation (`digest.html:27`; `stats.html:24,38`). `article.html` gets this right with real `<h2>`s.
6. Filter-chip removal links need an `aria-label` (e.g. "Remove source filter") — current accessible name is just "source: X ×".
7. Raw feed URLs and absolute timestamps exist only in `title` attributes — unreachable by keyboard or touch.
8. "Read original" new-tab link has no textual "(opens in new tab)" cue.
9. Two `<nav>` landmarks (header, pagination) are unlabeled and indistinguishable to assistive tech.
10. Tailwind's `list-style:none` preflight strips list semantics from article `<ul>`s in Safari/VoiceOver with no `role="list"` restoration (low severity).

**No accessibility conformance target is documented anywhere in the repo** — Phase 1 needs to state one (WCAG 2.1/2.2 AA is the master prompt's implicit ask) so the above gaps have a clear in-scope/out-of-scope line.

---

## 7. Constraints that limit framework or deployment choices

- **Server-rendered stdlib Go is a documented decision, not an accident.** The zero-dependency `go.mod` is an explicit acceptance criterion in `docs/superpowers/plans/2026-07-03-cloudflare-public.md`. Introducing htmx, Alpine, templ, or any framework would reverse an approved spec, not fill a gap — Phase 2 must treat this as a decision to revisit deliberately, not default past.
- **CSP `default-src 'none'; style-src 'self'` + zero-JS, chosen twice explicitly.** Rules out inline styles (hence the `bar-N` quantized-class trick for charts — any new chart needs the same approach or SVG with attribute-based sizing), any client-side behavior (theme toggle, live filtering, auto-refresh beyond `<meta refresh>`), icon/font/analytics CDNs. Every interactive widget must be native HTML.
- **Full-URL edge caching forbids per-user state.** No cookies, sessions, or personalization without redesigning the cache strategy — "analyst" features implying saved views or watchlists are architecturally excluded under the current caching model.
- **Article content is plain text forever, as already ingested.** Information-Broker discards all markup at fetch time; rich rendering of the corpus already stored is impossible without re-ingesting from source.
- **Backend contract ceilings:** no total result count, no time-series endpoint, no cluster-stats endpoint, unpaginated digest, feeds derived from a column rather than a table. Any UI wanting these needs Information-Broker changes — a separate, thinner-tested, CI-less repo with its own deploy.
- **CSS pipeline:** theme changes route through `tailwind.config.js` + `scripts/build-css.sh` (committed, hash-busted output). A palette swap is cheap; a different build pipeline is not.

---

## 8. Baseline screenshots and reproducible route-review notes

Captured live against `https://feed.purecypher.com` on 2026-07-14. Files in `docs/frontend/screenshots/`:

| File | Route | Viewport |
|---|---|---|
| `desktop-feed.png` | `/` | 1440×900 |
| `desktop-digest.png` | `/digest` | 1440×900 |
| `desktop-stats.png` | `/stats` | 1440×900 |
| `desktop-about.png` | `/about` | 1440×900 |
| `mobile-390-feed.png` | `/` | 390×844 |
| `mobile-390-article.png` | `/article/50917` | 390×844 |
| `mobile-390-stats.png` | `/stats` | 390×844 |

Reproduction steps: navigate to each URL directly (no auth, no cookies needed — the site is fully public and stateless). The feed page's source `<select>` currently lists 100+ distinct `feed_url` values by article count (`cvefeed.io` leads at 6,308, down to single-digit long-tail feeds) — useful real data for testing any redesigned filter UI against actual cardinality rather than a handful of fixtures. Live data at capture time: page 1 of `/` shows a "7 upcoming" webinar disclosure plus the usual card list; `/stats` and `/digest` rendered with real backend data (no error/empty states were exercised in this pass — those require simulating backend outage or an empty result set, deferred to Phase 3 testing).

---

## 9. Risks, unknowns, and assumptions requiring validation

### Internal contradictions found

- **Two "Approved" specs dated 2026-07-14 describe different digest mechanisms.** `docs/superpowers/specs/2026-07-14-digest-heuristic-design.md` (live pg_trgm title-similarity self-join) carries no superseded marker, yet the same-day story-clustering spec replaced its mechanism entirely with a precomputed `ClusteringScheduler`. The routes/contracts sections above describe only the shipped (clustered) version. **Action: mark the digest-heuristic spec superseded before anyone treats it as ground truth.**
- **Go version story is inconsistent** — `go.mod` says `go 1.22`, the Dockerfile builds with `golang:1.24-alpine`. Not breaking, but Phase 1 should pick one floor.
- **"Zero in-process caching" + "Cloudflare SWR is the only mitigation" together mean availability is weaker than either fact alone suggests** — SWR only protects URLs already in edge cache; any uncached variant (every unique `?q=` search, page 2+, filter combinations) hard-502s the instant Information-Broker is down.
- **Coverage mandate vs. enforcement:** the cloudflare-public spec mandates `-race` + ≥80% coverage; there is zero CI in either repo. Well-tested only for as long as someone remembers to run `go test ./...` — exactly the discipline a large template rewrite tends to erode.
- **Search UX asserts a filter it didn't apply:** 1-character `q` values render an active filter chip over backend-unfiltered results.

### Gaps no research pass covers

- **No usage data** — no metrics endpoint or analytics on the frontend; page/filter/sort popularity is unknown, so redesign prioritization (e.g., is `/digest` used at all?) is a guess.
- **No visual/performance baseline** prior to this audit — no Lighthouse numbers, no page-weight budget, no documented browser-support matrix (implicitly modern-evergreen given `frame-ancestors`-only and woff2-only fonts).
- **Cloudflare configuration is click-ops**, not code — cache rules, hostname, and tunnel live in the dashboard. Per project memory, SSH to the deploy host (192.168.1.135) is currently broken, so even `git pull && docker compose up -d` deploy access is unverified going into this redesign.
- **No staging environment** — deploy is a direct `git pull` on the production host with no preview URL.
- **Non-latin content is untested** — only a latin glyph subset is embedded and CSP forbids remote fonts; a feed with non-latin titles silently falls back to system fonts.

### Constraints (see §7 for full detail — repeated here as items requiring explicit sign-off)

Server-rendered stdlib Go; CSP `default-src 'none'` + zero-JS; full-URL edge caching precludes per-user state; plain-text-only article content; several backend contract ceilings (no total counts, no time-series, no cluster-stats endpoint).

### Assumptions requiring human validation before Phase 1 design work begins

**Decided final (2026-07-15):**

1. ~~Does "Cyber Terminal" stay inside zero-JS + strict CSP?~~ **Decided: yes.** The redesign stays inside the existing zero-JS, `default-src 'none'; style-src 'self'` posture — no CSP renegotiation, no client-side framework. Both Approved-but-conflicting specs (§ Internal contradictions) stand as-is; only the digest-heuristic one needed a supersession marker, now added.
2. ~~Does the site remain fully public and anonymous?~~ **Decided: yes.** No auth, no per-user state, no saved searches/watchlists. The full-URL edge-caching model is retained unmodified.
3. ~~Are Information-Broker API changes in scope?~~ **Decided: in scope, but justified case-by-case.** Each proposed backend field/endpoint (cluster-stats, time-series, per-feed error rates, etc.) needs its own justification in Phase 2's `ARCHITECTURE_DECISIONS.md` rather than a blanket "yes, add everything `/stats` could use."
6. ~~What accessibility conformance target applies?~~ **Decided: WCAG 2.2 AA.** The §6 gap list is now fully in-scope, not nice-to-have.
4. ~~Is the amber/dark identity retained, or does the redesign migrate to cyan/indigo actives + a red/amber/green severity scale?~~ **Decided (2026-07-15): "Signal Lamp."** Chosen from a 3-option live visual comparison (Amber Phosphor / Signal Lamp / NOC Wallboard). Keeps the existing amber identity fully intact — favicon, nav, buttons, links, CVE/cross-feed badges all stay `#f5b13d`/`#ffc964`/`#c08a33`, no cyan/indigo introduced — and adds a real severity system (`sev-healthy #4ade80`, `sev-warning` reusing the accent hue deliberately, `sev-critical #f87171`) plus a `structural #8891a3` neutral, used only where the audit found a genuine gap (feed health, error states). This is the smallest-lift option: no new favicon, no new OG images, no brand re-theme. Phase 1 (`REDESIGN_PLAN.md`, `DESIGN_TOKENS.md`) is now in progress against this decision.

**Still open, non-blocking for Phase 1:**

7. **Is the unpaginated digest response safe to keep as-is**, or does current monthly-window article volume justify a backend limit param before Phase 1 designs a new digest UI on top of it?
5. **Is deploy access currently working?** (`.135` SSH path noted broken in project memory) — not blocking for Phase 1 design work, but blocking before any Phase 3/4 live-verification task is scheduled.

---

## Phase 0 exit criteria — status

- [x] No framework choice made on assumption — stack confirmed as Go/stdlib on both sides (correcting the master prompt's Python hypothesis).
- [x] All four public routes inspected (plus `/article/{id}`, `/healthz`, `/feed.xml`, `/robots.txt`, `/static/`).
- [x] Application remains runnable via documented commands (`go run .` for SmellyFeet; `docker compose up -d` for Information-Broker) — no changes made in this pass.
- [x] Known frontend security risks recorded (§5), including the correction that RSS sanitization is already a defense-in-depth design, not a gap.

**Interim fixes shipped ahead of Phase 1** (commits `2d200c7`, `e15ed64`): the 1-char search chip, pagination `urlquery` encoding, skip link / labelled nav landmarks / `aria-current` / heading hierarchy, `:focus-visible` outline (including a specificity fix caught by code review), WCAG-AA contrast on muted text, a new-tab affordance cue, and an explicit `/stats` partial-failure state — all decoupled from the palette/JS decisions above, so none of it needs to be redone regardless of how Phase 1 resolves.

**Five of the seven gating decisions are now final** (#1, #2, #3, #4 "Signal Lamp", #6). **Phase 1 (redesign strategy + design tokens) is unblocked and in progress.** #5 and #7 remain open but do not block Phase 1 planning.
