# Information-Broker Frontend — Design

**Date:** 2026-06-05
**Status:** Approved

## Purpose

Provide a clean, readable web frontend for the [Information-Broker](https://github.com/PureCypher/Information-Broker)
project, which scrapes cybersecurity RSS feeds, summarizes articles with AI, and stores them in
PostgreSQL. The frontend lets a user:

- Browse all article **summaries** in a nicely presented list.
- Open any article to **read the full stored text**.
- See **where each article came from** (original source URL and the RSS feed).
- Search, filter by feed, page through history, and view high-level stats.

## Architecture

A **new, standalone Go web server** in the `SmellyFeet` repo. It server-renders HTML templates
(`html/template`) styled with Tailwind CSS. It communicates with Information-Broker exclusively over
that project's existing HTTP API — it does **not** connect to the database directly. The two services
deploy and restart independently.

```
Browser ──> [SmellyFeet frontend :3000]  ──HTTP──>  [Information-Broker API :8080] ──> Postgres
            (Go html/template + Tailwind)            (existing Go app, extended)
```

**Alternative considered:** folding the pages into the existing Information-Broker app (single Go
process, templates reading the DB directly). Rejected to keep the viewer decoupled from the scraper's
deploy/restart cycle and to avoid cluttering that repo. The separate-server approach was chosen.

**Go version:** The frontend module targets Go 1.22+ so it can use `net/http`'s `{id}` route patterns.
(The backend stays on Go 1.21, so its new single-article endpoint uses a query param instead.)

## Backend changes (in `../Information-Broker`)

Small, additive changes to `api.go`. No behavior change for existing consumers.

1. Add `id` and `summary` to the `/articles` and `/articles/latest` JSON responses. Both columns
   already exist in the `articles` table; the current `SELECT` simply omits them.
2. Add a `?q=` search parameter to `/articles` — case-insensitive match on `title`, `summary`, and
   `full_content` via `ILIKE`. Composes with the existing `feed`, `limit`, and `offset` params.
3. Add a single-article endpoint: `GET /articles/get?id=N`, returning one article including `summary`
   and `full_content`. Returns 404 if no article has that id. (Query param used rather than a path
   pattern because the backend is on Go 1.21.)

Existing API tests are extended to cover the new `id`/`summary` fields and the search parameter.

## Frontend pages

| Route            | Purpose |
|------------------|---------|
| `/`              | Article list. Cards show **title, source/feed, publish date, and the summary**. Hosts the search box, feed filter, and pagination controls. |
| `/article/{id}`  | Detail page. Title, summary up top, **full text** below, a prominent "Read original →" link to the source URL, and the feed it came from. |
| `/stats`         | Dashboard from the API `/stats` endpoint: total articles, feed count, last fetch time. |
| `/healthz`       | Frontend's own health check (returns 200 when the server is up). |

## Data flow

Each frontend request fetches from the Information-Broker API, then renders a template:

- `/` → `GET {API}/articles?limit=&offset=&feed=&q=`
- `/article/{id}` → `GET {API}/articles/get?id={id}`
- `/stats` → `GET {API}/stats`; feed filter options from `GET {API}/feeds`

A thin `apiclient` package wraps the HTTP calls and JSON decoding, exposing typed methods
(`ListArticles`, `GetArticle`, `ListFeeds`, `GetStats`) and Go structs matching the API JSON.

## Components / boundaries

- **`apiclient`** — HTTP client for the Information-Broker API. Knows the base URL, performs requests,
  decodes JSON into typed structs, maps non-2xx/transport errors to typed errors. No HTML knowledge.
- **`server` (handlers)** — HTTP handlers for the routes above. Calls `apiclient`, prepares view data,
  renders templates. No raw SQL or HTTP-client details.
- **`templates`** — `base.html` layout + per-page templates (`list`, `article`, `stats`, `error`,
  `404`). Presentation only.
- **`config`** — loads `API_BASE_URL` and `PORT` from env with defaults.

## Configuration

Loaded from environment (with a local `.env` for development):

- `API_BASE_URL` — Information-Broker API base. Prod: `http://192.168.1.135:8080`; local:
  `http://localhost:8080`.
- `PORT` — frontend listen port. Default `3000`.

No database credentials are needed by the frontend.

## Error handling

- API unreachable or non-2xx → render a friendly **error page**, never a stack trace.
- Article with no summary → display "No summary available."
- Unknown article id → **404 page**.
- All upstream errors are logged server-side with enough context to debug.

## Testing

- **`apiclient`**: table-driven tests against an `httptest` mock server (success, 404, 5xx, malformed
  JSON, transport error).
- **Handlers/templates**: tests that each page renders without error for representative data plus
  empty/edge cases (no articles, missing summary, unknown id).
- **Backend**: extend existing API tests to cover the new `summary`/`id` fields and the `q` search
  parameter.

## Out of scope (v1)

- Authentication / user accounts.
- Editing, tagging, or deleting articles.
- Real-time updates / websockets.
- Direct database access from the frontend.
