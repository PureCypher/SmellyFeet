# Filtering & Sort Improvements — Design

**Date:** 2026-07-03
**Status:** Approved
**Goal:** Real newest/oldest sorting end-to-end, un-pin future-dated articles from the top of
the feed, one-click source filtering, and visible/removable active filters. Zero JS, CSP
unchanged.

## Context

The Information-Broker API hardcodes `ORDER BY publish_date DESC`; future-dated webinar
entries therefore pin to the top of page 1 ("broken newest-first" symptom). `q` search works;
there is no sort parameter. Scope decision: frontend + backend (both repos are the user's).
Backend source of truth: `/home/pure/Documents/github/Information-Broker` (local clone, in
sync with `~/Information-Broker` on host 192.168.1.135, which is the compose build context
for the running `information-broker-app` container).

## 1. Backend (Information-Broker)

- `buildArticlesQuery(feed, q, sort string, limit, offset int)` — new `sort` param.
  Whitelist: `"oldest"` → `ORDER BY publish_date ASC`; any other value → `publish_date DESC`
  (default, unchanged). No user input is interpolated into SQL.
- `getArticles` reads `r.URL.Query().Get("sort")` and passes it through.
- Tests extend `api_query_test.go`: `sort="oldest"` → ASC; `sort=""` and `sort="garbage"` →
  DESC; existing cases updated for the new signature.
- Backward compatible; old clients unaffected.
- Deploy: push to GitHub, `git pull` + `docker compose up -d --build` of the API service in
  `~/Information-Broker` on the host.

## 2. Frontend API client (SmellyFeet)

- `apiclient.ListParams` gains `Sort string`; `ListArticles` sets `sort=oldest` in the query
  only when `Sort == "oldest"`. Test asserts encoding (and absence when empty).

## 3. List handler

- Parse `sort` from the request; normalize (`"oldest"` or `""`); pass to the API; include in
  pagination links.
- New pure func `splitUpcoming(articles []apiclient.Article, now time.Time) (upcoming,
  current []apiclient.Article)` — `PublishedAt.After(now)` → upcoming; order preserved;
  applied **only when sort is newest AND page == 1**.
- `listView` gains `Sort string` and `Upcoming []apiclient.Article`.
- `stubService` gains a field capturing the last `ListParams` so tests can assert what the
  handler sent.

## 4. Templates (list.html, article.html)

- **Sort control:** third field in the filter form — `<select name="sort">` with
  `Newest first` (value "") / `Oldest first` (value "oldest"), selected from `.Sort`; same
  Apply button. Pagination links append `&sort={{ .Sort }}` (harmless when empty).
- **Upcoming section:** `{{ if .Upcoming }}` → collapsed `<details>` above the stream;
  `<summary>` reads `upcoming ({{ len .Upcoming }})` in the section-label style. Cards inside
  reuse the shared card template.
- **Card restructure (stretched link):** cards become `<div class="group relative ...">`;
  the title becomes `<a href="/article/{id}" class="after:absolute after:inset-0 ...">` so
  the whole card stays clickable with valid HTML; the source pill becomes its own
  `<a href="/?feed={{ .FeedURL | urlquery }}" class="relative z-10 ...">` (one-click source
  filter). Card markup extracted to `{{ define "articleCard" }}` used by both the main list
  and the upcoming block. Article page pill links to `/?feed=...` likewise.
- **Filter chips:** `{{ if or .Q .Feed (eq .Sort "oldest") }}` → slim bar above results with
  removable chips, each a plain link rebuilding the URL without that param (via the built-in
  `urlquery` escaper):
  - `source: <sourceName> ×` → drops `feed`
  - `search: "<q>" ×` → drops `q`
  - `oldest first ×` → drops `sort`
  - `clear all` → `/`

## 5. Caching

No changes: Cloudflare keys on the full URL, so `sort`/`feed`/`q` variants cache
independently under the existing rule; error paths remain `no-store`.

## 6. Testing & deploy order

- Backend: query-builder table tests (ASC/DESC/whitelist).
- Frontend: apiclient sort encoding; `splitUpcoming` boundaries (exactly-now is NOT
  upcoming; zero dates are not upcoming); handler passes sort + page-1-only split (captured
  params); render tests — upcoming details only on page 1 newest, chips render with correct
  removal hrefs, pill is a link with escaped feed URL, pagination carries sort.
- CSS rebuilt once (`./scripts/build-css.sh`) after template changes.
- Deploy backend first (compatible), then frontend; verify live through the edge.

## Out of scope

- Date-range filters, multi-source selection, JS interactivity.
- Changing the Atom feed order (stays newest-first).
