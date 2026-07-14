# Digest Tab — Cross-Feed Importance Heuristic — Design

**Date:** 2026-07-14
**Status:** Superseded — see `2026-07-14-story-clustering-design.md`

> **Superseded (2026-07-14):** the live `pg_trgm` title-similarity self-join described below
> timed out in practice — trigram GIN indexes don't accelerate column-to-column self-joins, and
> the GROUP-BY-normalized-title fallback was too strict to find real duplicates. It was replaced
> end-to-end by a precomputed `ClusteringScheduler` (Ollama embeddings, cosine-similarity
> clustering, `story_cluster_id` column) documented in `2026-07-14-story-clustering-design.md`.
> The `/digest` route, its daily/weekly/monthly ranges, and the important/other split are still
> live — only the mechanism that populates `cross_feed_count` changed. Do not implement against
> this document; it is kept for historical context only.

**Goal:** A new `/digest` tab with daily/weekly/monthly views that splits articles into
"important" (covered by multiple feeds) vs. "everything else," using a free SQL heuristic —
no new LLM calls, no scheduled job.

## Context

`apiclient.Article` carries no importance signal today — no score, category, or cross-source
link. Two infra pieces already exist that this design reuses instead of adding anything new:
Postgres `pg_trgm` (GIN index on `articles.title`, added for `q` search — see
`2026-07-03-trigram-search-design.md`) for title-similarity matching, and a self-hosted Ollama
pass (`Information-Broker/summarizer.go`) that already produces each article's `summary` field.
Ollama was considered for scoring/clustering directly but rejected for v1 — it would mean new
prompt design and new scheduled load on top of its existing per-article summarization work,
whereas the trigram approach is pure SQL against an index that's already there. Backend source
of truth: `/home/pure/Documents/github/Information-Broker` (in sync with the host running
`information-broker-app`, per the filter-sort spec). Scope: frontend + backend, both the
user's repos.

## 1. Backend (Information-Broker)

- New endpoint `GET /articles/digest?range=daily|weekly|monthly`. Invalid/missing `range` falls
  back to `daily` — same whitelist-and-normalize pattern as the existing `sort` param.
- `digestWindowOrDefault(rangeParam string) time.Duration` — pure function: `"weekly"` → 7
  days, `"monthly"` → 30 days, anything else → 24 hours. The cutoff (`since := time.Now().Add(-window)`)
  is computed in Go and passed as a bound parameter — never interpolated into SQL.
- `buildDigestQuery(since time.Time) (string, []interface{})` — one self-join scoped to the
  window, reusing `idx_articles_title_trgm` (no new index):

  ```sql
  SELECT a1.id, a1.title, a1.url, a1.summary, a1.full_content, a1.publish_date,
         a1.feed_url, a1.content_hash, COUNT(DISTINCT a2.feed_url) AS cross_feed_count
  FROM articles a1
  LEFT JOIN articles a2
    ON a2.publish_date >= $1 AND a2.feed_url <> a1.feed_url AND a1.title % a2.title
  WHERE a1.publish_date >= $1
  GROUP BY a1.id
  ORDER BY cross_feed_count DESC, a1.publish_date DESC
  ```

  `%` is `pg_trgm`'s similarity operator (default threshold 0.3): `cross_feed_count` is "how
  many *other* feeds ran a similarly-titled story in this window."
- `const minCrossFeedCountForImportant = 2` (≥2 other feeds = important). Split happens in Go,
  not SQL, in this repo's API handler — the same after-the-query, split-in-Go-not-SQL style as
  SmellyFeet's existing `splitUpcoming` (`internal/server/handlers.go`), just in
  Information-Broker instead: `splitImportant(rows []Article) (important, other []Article)`.
  The response is pre-split before it reaches the frontend. Order from the query
  (`cross_feed_count DESC, publish_date DESC`) is preserved on both sides of the split.
- Response envelope: `DigestResult{Range string; Since time.Time; Important, Other []Article}`,
  where each `Article` in the digest response additionally carries `cross_feed_count`.
- Computed live per request (not precomputed): at current volume (~46k articles / 158 feeds
  total), even the monthly window is a few thousand rows — a GIN-assisted self-join over that
  is cheap, and Cloudflare caches the rendered page anyway.
- No `feed`/`q` filtering on this endpoint in v1 — digest is its own unfiltered view.

## 2. Frontend API client (SmellyFeet)

- `apiclient.Article` gains one field: `CrossFeedCount int` (`json:"cross_feed_count,omitempty"`).
  Zero/absent on the regular `/articles` response; only `/articles/digest` populates it. This
  lets both pages share the exact same `Article` type and the exact same `articleCard`
  template partial — no parallel wrapper type.
- New `apiclient.DigestResult{Range string; Since time.Time; Important, Other []Article}` and
  `Client.GetDigest(ctx, rangeParam string) (DigestResult, error)` hitting
  `/articles/digest?range=...`, same shape as `ListArticles`.

## 3. Digest handler

- New route: `mux.HandleFunc("GET /digest", s.handleDigest)` in `server.go`'s `Routes()`.
- `handleDigest`: whitelist `range` to daily/weekly/monthly (default `daily`, same
  normalize-invalid-input pattern as `sort`), call `GetDigest`, render a
  `digestView{Range, Since, Important, Other}`.

## 4. Templates (digest.html, partials.html)

- New `digest.html`, following `list.html`'s conventions:
  - `range` `<select>` (daily/weekly/monthly) in `<form method="get" action="/digest">`,
    styled like the existing sort select.
  - **Important** section: plain `<ul>` of `{{ template "articleCard" . }}`, reused as-is.
  - **Everything else** section: the same collapsed `<details>` pattern already used for
    "upcoming" in `list.html`, wrapping the `Other` list.
  - Empty state mirrors `list.html`'s "No articles found."
- One edit to the shared `articleCard` define in `list.html`: a guarded badge next to the
  existing source pill — `{{ if gt .CrossFeedCount 1 }}<span ...>{{ .CrossFeedCount }}
  sources</span>{{ end }}`. Renders nowhere else since `CrossFeedCount` is `0` on every other
  page (list, article, upcoming).
- `partials.html` nav gains one link: `<a href="/digest">Digest</a>`, next to Feed/Stats/About.

## 5. Caching

No changes needed: Cloudflare keys on the full URL, so `/digest?range=daily` etc. cache
independently, same as `sort`/`feed`/`q` today (per the filter-sort spec).

## 6. Testing & deploy order

- **Backend:** `digestWindowOrDefault` table test (`"weekly"`→7d, `"monthly"`→30d, garbage→24h,
  matching the existing `sort`-whitelist test style in `api_query_test.go`); `splitImportant`
  boundary test (`cross_feed_count == 2` → Important, `== 1` → Other, order preserved);
  integration test against the query (style of `article_filter_test.go`) — seed near-duplicate
  titles across different `feed_url`s and assert `cross_feed_count`, seed dissimilar titles and
  assert `0`.
- **Frontend:** `GetDigest` stub-server test asserting `range` encoding (mirrors
  `ListArticles`'s test); `handleDigest` test with a stub API and canned `DigestResult`,
  asserting invalid `range` normalizes to `daily`; render tests — badge shows only when
  `CrossFeedCount > 1`, "everything else" `<details>` only when `Other` is non-empty, nav link
  present.
- One CSS rebuild (`./scripts/build-css.sh`) after the template edits.
- Deploy order (for later — this pass is local/committed only, no push): backend first since
  it's purely additive (new endpoint, no schema change, reuses the existing trigram index),
  then frontend once the endpoint is live.

## 7. Known ceiling

`pg_trgm` similarity on raw titles catches **near-duplicate headlines** — wire-copy
syndication, multiple RSS feeds mirroring the same aggregator — but will likely **miss**
genuine cross-outlet coverage where each outlet writes its own headline for the same
real-world event (e.g. "Fed cuts rates" vs. "Federal Reserve lowers interest rates again" may
fall under the 0.3 threshold). In practice this is closer to a **duplicate/syndication
detector** than a true importance ranker. Mark this at the query site with a `ponytail:`
comment naming the ceiling and the upgrade path (embedding-based similarity, or the
already-running Ollama, if this proves too weak against real data).

Also unverified: `EXPLAIN ANALYZE` on the self-join at monthly-window scale. Worth one check
once live; if slow, fall back to capping the monthly window's row count, or reconsider the
precomputed-job alternative declined for v1.

## Out of scope

- Date-range picker beyond the three presets.
- Per-source filtering combined with the digest view (`feed`/`q` not accepted on `/digest`).
- Semantic clustering of the "Other" bucket — stays a flat list.
- Changing the Atom feed.
- Any deploy/push — this pass stays local and committed only.
