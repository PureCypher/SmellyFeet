# Architecture & Data Flow

PureCypher is two Go services backed by one PostgreSQL 15 database, publicly served through a Cloudflare Tunnel. This document describes the deployed reality (as of 2026-10-27); `ARCHITECTURE_DECISIONS.md` records the "why".

## Repositories and responsibilities

| | **Information-Broker** (backend) | **SmellyFeet** (frontend) |
|---|---|---|
| Role | RSS ingestion, article extraction, AI summarisation, story clustering, digest clustering, public JSON API | Server-rendered reading UI (zero JS), calls the API per request |
| Runtime | Docker Compose stack: `postgres` + `rss-monitor` (single binary: monitor + schedulers + API server) + optional flaresolverr + alerting sidecar | Docker Compose (`deploy/`): `smellyfeet` + `cloudflared` sidecar |
| Stack | Go, stdlib `net/http` (legacy path-only mux), `gofeed`, `goquery`, `lib/pq`, Prometheus metrics; Ollama (`granite4.1:3b` for summaries, `nomic-embed-text` for embeddings) on the host | Go 1.22, stdlib only — `html/template` (`//go:embed`), no JS, CSP forbids scripts |
| Database access | Owns schema (`schema.sql` + programmatic `summary_logs`) | **None** — only HTTP calls to the API |

The frontend never touches Postgres and the backend never renders HTML; the only contract is the JSON API.

## Article lifecycle (backend)

```
feeds.txt (156 sources)
  │  RSSFetchInterval (5 min default)
  ▼
RSSMonitor.processArticle (monitor.go)
  ├─ parse pubDate → missing ⇒ skip; >24h future ⇒ clamped to now (event promos)
  ├─ rejected/reserved CVE titles ⇒ skip (cvefeed noise)
  ├─ cutoff-date filter (config ArticleCutoffDate)
  ├─ dedupe: in-memory seenArticles + UNIQUE(url), UNIQUE(content_hash)
  ├─ content extraction: fetchFullContent via goquery
  │    └─ cvefeed.io ⇒ skip page (Bootstrap chrome), use structured feed description
  ▼
INSERT INTO articles (summary NULL, full_content)
  ▼
SummarizationScheduler (queue cap 100, 1 in flight)
  ├─ summarizer.SummarizeWithOllama → POST host Ollama /api/generate
  │    3 attempts, then literal fallback "summary unavailable"
  ├─ UPDATE articles SET summary (NULL stays ⇒ "pending" state)
  └─ every attempt logged to summary_logs (status, model, duration, error)
  ▼
ClusteringScheduler (15 min): embeds summaries (nomic-embed-text), similarity
  comparison in Go, writes story_cluster_id (cluster = seed article's own id)
  ▼
Discord webhook (posted_to_discord flag + webhook_logs) — optional
```

**Summary states and their ground truth:** `summary IS NULL` → genuinely pending (summarizer hasn't reached it; honest because the scheduler queue is bounded and it works strictly through the backlog). `summary = 'summary unavailable'` → failed after retries. Anything else → available. The API exposes this as `summary_state` (`pending`/`failed`/`available`); the frontend renders the three differently and never invents progress.

## JSON API (Information-Broker, :8080 on the host)

All GET-only, CORS-enabled, path-only mux:

| Endpoint | Returns | Notes |
|---|---|---|
| `/articles` | `articles[]` (ArticleView), `count`, `total` (omitted if count query failed), `limit`, `offset` | params: `limit ≤ 100`, `offset`, `feed` (exact URL), `q` (ILIKE title/summary/full_content, <2 chars ignored), `sort` (``/`oldest`) |
| `/articles/get?id=N` | single ArticleView (full content) | 404 on unknown id |
| `/articles/latest` | `articles[]` newest by `fetch_time` | was 500 (`fetched_at` bug), now the public shape |
| `/articles/digest?range=` | clustered stories per named range (daily…yearly), important/other buckets, `story_cluster_id` | ranges whitelisted both sides |
| `/feeds` | per-feed URL + article count + latest article time | feeds the source filter + stats |
| `/stats` | totals, fetch window counts, 24h fetch health | |
| `/summarization/stats` | summarizer health: lifetime successes, 24h failures, avg latency, last error | powers the stats page's health panel |
| `/health` | DB + circuit-breaker state | |

ArticleView: `id, title, url, summary, summary_state, content, published_at, fetch_duration, feed_url, content_hash, cross_feed_count?, story_cluster_id?, fetch_time?`.

## Request flow (frontend, :3000 in the compose network)

```
Cloudflare edge (per-route Cache-Control; errors no-store)
  └─ cloudflared tunnel sidecar → smellyfeet (from-scratch container, non-root)
       ├─ mux: /{$} /digest /article/{id} /stats /about /feed.xml /robots.txt /static/
       ├─ middleware: security headers (strict CSP, no script-src) → request log
       ├─ handlers parse query, whitelist params, call apiclient (10 s timeout)
       │    ├─ backend failure ⇒ clean 502 error page (renderError)
       │    └─ feeds-list failure ⇒ non-fatal DEGRADED callout, page still renders
       └─ html/template render into buffer (template error ⇒ clean 500)
```

- No in-process cache: every request is one or two synchronous API calls; Cloudflare edge caching (60s list / 300s article / 30s stats, s-maxage + SWR) absorbs repeat traffic.
- All hostile content (titles, summaries, extracted bodies, URLs) flows through `html/template` contextual auto-escaping; zero `template.HTML` usage anywhere (enforced by the redesign plan's check O5).
- The frontend renders unknown/missing fields honestly: unknown `summary_state` (older backend) falls back to plain summary rendering; unknown `total` renders "Page N" without "of M".

## Deployment topology

- Host 192.168.1.135 runs Ollama directly (host service, bound for LAN + docker bridge) and the two compose stacks; public ingress is exclusively Cloudflare → tunnel → `smellyfeet:3000`. LAN IPs and internal service names never appear in public UI.
- Backend API is host-local (:8080); the frontend reaches it over the host network — it is not published to the internet.

## Change control

Contract changes must land backend-first (additive fields are backward-compatible either way), with regression tests in both repos; see `DEPLOYMENT.md` for the current pending deployment.
