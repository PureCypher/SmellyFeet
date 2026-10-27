# VERIFICATION — 2026-10-27 session 2

Environment: **local integration smoke** — throwaway PostgreSQL 15 container (destroyed afterwards; no production data touched), freshly built Information-Broker binary, local Ollama `qwen3:0.6b`, freshly built SmellyFeet binary. Production (`feed.purecypher.com`) not yet redeployed.

## Changes verified

### Backend contract (Information-Broker, uncommitted)
| Change | Evidence |
|---|---|
| `/articles` gains `total` (omitted when count query fails, not zeroed) | `GET /articles?limit=5` → `total: 2975, count: 5, limit: 5, offset: 0` |
| `/articles` + `/articles/get` + `/articles/latest` gain `summary_state` (`pending`/`failed`/`available`) | Sampled rows returned all three states; `summaryStateOf` unit tests pass (8 cases incl. padded/case variants) |
| `/articles/latest` rewritten to public `ArticleView` shape; `fetched_at`→`fetch_time` bug fixed | Was 500 (`pq: column "fetched_at" does not exist`); now 200, keys: `content, content_hash, feed_url, fetch_duration, fetch_time, id, published_at, summary, summary_state, title, url` |
| Ingest fixes (regression-tested): rejected-CVE skip, cvefeed description extraction, 24h future-date clamp | `go test ./...` green in Information-Broker (incl. new `api_summary_state_test.go`, `cvefeed_description_test.go`, `rejected_cve_test.go`, rewritten `article_filter_test.go`) |
| AlertManager map race | mutex added; builds + vet clean |

### Frontend (SmellyFeet, uncommitted)
| Change | Evidence |
|---|---|
| Honest pager "Page N of M" (unknown total → "Page N" alone) | Local render: "Page 1 of 167" at 1440×900; template branches on `.Pages` |
| Summary states: "Summary queued — being generated." / "Summary unavailable — generation failed." / real summary | Browser screenshots (desktop 1440×900 + mobile 390×844): failed + queued text on cards; article page for id 9 showed the real qwen3 summary in the AI Summary panel |
| Fallback when backend is older (no `summary_state`) | Template checks `.SummaryState` first, falls back to `.Summary`/"No summary available." |

### Browser verification
- `GET /` 200 at 1440×900 and 390×844 — screenshots captured and reviewed; no horizontal overflow observed, pagination and summary states visible.
- `GET /digest` 200, `GET /stats` 200 on the local build.
- Deep link `GET /article/9` renders title, source pill, real summary, stored full text.

### Production deployment status
**Not deployed.** Production runs `d3a1d7c` (backend) / `ac4df86` (frontend). The contract changes are backward-compatible (`total`/`summary_state` additive; frontend tolerates their absence), so deploy order is safe either way. Awaiting authorisation.

### Limitations
- The smoke ingest used `qwen3:0.6b` and a fresh DB: most summaries failed on empty/short article text (legitimate `summary unavailable` path) — production model `granite4.1:3b` is unaffected.
- No production data reconciliation performed (read-only checks against prod were done earlier in the session; schema unchanged).

## Addendum — source dropdown (D4), 00:25 UTC

- `sourceName` wrapper handling updated (feedburner path slug), `dedupeFeedList`
  collapses same-publisher section feeds; regression tests: `feed_dedupe_test.go`
  (real duplicate shapes from feeds.txt), `funcs_test.go`/`ui_test.go` updated to
  the new wrapper convention.
- `go test ./...` green in SmellyFeet (apiclient, config, server); vet + build clean.
- Commits: `e41c87a` (frontend), `545c199` (backend), `320cccb` (frontend contract).

Deployment plan for these changes: see DEPLOYMENT.md.

## Production route timings & security headers (2026-10-27, read-only)

`curl time_total` against https://feed.purecypher.com/ (includes TLS + Cloudflare edge):

| Route | Total |
|---|---|
| `/` | 193 ms |
| `/?q=ransomware` (live ILIKE search incl. full_content) | 218 ms |
| `/?page=5` | 191 ms |
| `/digest` | 287 ms |
| `/stats` | 237 ms |
| `/about` | 107 ms |
| `/article/90142` | 134 ms |
| `/feed.xml` | 148 ms |

Search over title/summary/full_content shows no ILIKE scan penalty at this dataset size — audit hypothesis H1 (slow trigram-backed search) is disproven; no action needed. H4 (missing WriteTimeout) was already fixed: `main.go:33` sets `WriteTimeout: 30s`.

Security headers on production (verified via `curl -I`): CSP `default-src 'none'; style-src 'self'; font-src 'self'; img-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'`, `nosniff`, `Referrer-Policy`, `X-Frame-Options`, per-route `Cache-Control`.

`template.HTML` usage (updated for the full-text HTML feature): exactly one
escape hatch — `sanitizeBody` in `internal/server/server.go`, which renders
untrusted publisher HTML through `sanitizeArticleHTML` (strict whitelist,
attribute re-validation, image rewrite to the same-origin `/img` proxy). No
other `template.HTML` exists (`grep -rn 'template.HTML' internal/` → the
funcMap entry and the sanitizer docstring only).

## Session 3 — full-text HTML pipeline + image proxy + redesign (2026-10-27)

Environment: same local integration smoke (`sf-preview-pg` postgres:15 container
on 127.0.0.1:15433, throwaway; freshly built IB + SmellyFeet binaries; 7351
pre-existing articles + 20 freshly ingested Ars Technica articles with
`content_html` capture). Production untouched.

### Data pipeline (Information-Broker)
- `content_html` capture verified: after the extraction commit, 20/20 newly
  ingested articles carry structured HTML (`SELECT count(content_html)`).
- Extraction chrome-strip regression: the embedded `<header>` (headline
  block, byline, category icons) no longer reaches storage — regression test
  `TestExtractMainContentStripsEmbeddedHeader`-style fixtures in
  `monitor_test.go` cover header/form/button stripping; live ingest showed
  prose-first extraction on all 20 articles.
- Schema migration on startup: existing DB (pre-`content_html` schema) gained
  the column via `CREATE TABLE IF NOT EXISTS` path; NULL `content_html`
  falls back to the plain-text renderer (verified on the 7351 legacy rows).

### Frontend (SmellyFeet)
- `go test ./...` green (sanitizer XSS/rewrite suite: script/iframe drops,
  `javascript:` href neutralization, data:-URI image drops, relative-URL
  resolution, attr allowlist, hostile-input fuzz set; `/img` proxy guard
  tests: missing/bad param, scheme+port rejection, content-type sniffing,
  MultiReader probe-byte regression, loopback/private-IP dial refusal).
- Rendered verification (Chromium, DOM + screenshots):
  - `/article/7411` renders sanitized HTML: title-once (no publisher header
    duplication), `<img src="/img?u=https%3A%2F%2Fcdn.arstechnica.net...">`,
    `loading="lazy" decoding="async"`, first paragraphs intact, no
    `h1`/`h2` chrome in the body, no horizontal overflow at 390px.
  - Image actually loads through the proxy at 16px/1.85/72ch (measured
    computed styles) — publisher CDN returned 403 from this LAN for one
    URL (bot-block, not a proxy defect) but the same-origin proxy path was
    proven with a fetchable upstream image.
  - Unknown routes now render the site 404 shell (`/nonexistent-route` →
    "Page not found" inside header/footer), previously Go's bare default.
- Brand alignment: `<title>… · SmellyFeet`, og:site_name, wordmark
  `smellyfeet`, footer "SmellyFeet · PureCypher threat intelligence feed";
  the backend product name appears only in the About page relationship copy.
- CSP unchanged; the only `template.HTML` is the sanitizer escape hatch.

### Known limitations
- One preview article's CDN image 403s from this network (Ars bot-block).
  The proxy reports a clean 502 and the frame keeps its alt/box styling.
- 7351 legacy rows have no `content_html`; they render via the plain-text
  fallback (honest state, labelled in the UI).
