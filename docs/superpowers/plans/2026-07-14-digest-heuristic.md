# Digest Tab — Cross-Feed Importance Heuristic — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/digest` tab (SmellyFeet) backed by a new `/articles/digest` endpoint
(Information-Broker) that splits recent articles into "important" (covered by multiple feeds)
vs. "everything else" for daily/weekly/monthly windows.

**Architecture:** Backend: one new file `digest.go` in Information-Broker computes a
per-article cross-feed count via a `pg_trgm`-backed self-join scoped to a Go-computed time
window, and splits the result server-side before it's ever sent over the wire. Frontend:
SmellyFeet's existing thin-client pattern (`apiclient` → handler → template) gains one more
route that reuses the existing `articleCard` partial as-is.

**Tech Stack:** Go, Postgres (`pg_trgm`), `html/template`, standard-library `net/http`
`ServeMux`.

**Spec:** `docs/superpowers/specs/2026-07-14-digest-heuristic-design.md`

## Global Constraints

- No schema changes — reuses the existing `idx_articles_title_trgm` GIN index (from
  `2026-07-03-trigram-search-design.md`).
- No new dependencies in either repo.
- This pass is local commits only — no push, no live deploy.
- `range` query param follows the same whitelist-and-normalize pattern as the existing `sort`
  param: unknown/missing values silently fall back to the default, never propagate to SQL.
- `minCrossFeedCountForImportant = 2` (≥2 *other* feeds = important) — named constant.
- Two repos involved: backend at `/home/pure/Documents/github/Information-Broker`, frontend at
  `/home/pure/Documents/github/SmellyFeet` (this repo). Run `go test ./...` from each repo's
  root respectively.

---

## Task 1: Backend — `digestWindowOrDefault`

**Files:**
- Create: `/home/pure/Documents/github/Information-Broker/digest.go`
- Test: `/home/pure/Documents/github/Information-Broker/digest_test.go`

**Interfaces:**
- Produces: `digestWindowOrDefault(rangeParam string) time.Duration`

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"testing"
	"time"
)

func TestDigestWindowOrDefault(t *testing.T) {
	tests := []struct {
		rangeParam string
		want       time.Duration
	}{
		{"daily", 24 * time.Hour},
		{"weekly", 7 * 24 * time.Hour},
		{"monthly", 30 * 24 * time.Hour},
		{"", 24 * time.Hour},
		{"garbage", 24 * time.Hour},
	}
	for _, tt := range tests {
		if got := digestWindowOrDefault(tt.rangeParam); got != tt.want {
			t.Errorf("digestWindowOrDefault(%q) = %v, want %v", tt.rangeParam, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run (from `/home/pure/Documents/github/Information-Broker`): `go test ./... -run TestDigestWindowOrDefault -v`
Expected: FAIL with `undefined: digestWindowOrDefault`

- [ ] **Step 3: Write minimal implementation**

```go
package main

import "time"

// digestWindowOrDefault maps a digest range parameter to a lookback window.
// Unknown or empty values fall back to the daily (24h) window — same
// whitelist-and-normalize style as buildArticlesQuery's sort param.
func digestWindowOrDefault(rangeParam string) time.Duration {
	switch rangeParam {
	case "weekly":
		return 7 * 24 * time.Hour
	case "monthly":
		return 30 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestDigestWindowOrDefault -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/pure/Documents/github/Information-Broker
git add digest.go digest_test.go
git commit -m "feat: add digestWindowOrDefault for the digest endpoint's range param"
```

---

## Task 2: Backend — `buildDigestQuery`

**Files:**
- Modify: `/home/pure/Documents/github/Information-Broker/digest.go`
- Modify: `/home/pure/Documents/github/Information-Broker/digest_test.go`

**Interfaces:**
- Consumes: nothing new
- Produces: `buildDigestQuery(since time.Time) (string, []interface{})`,
  `const minCrossFeedCountForImportant = 2`

- [ ] **Step 1: Write the failing test**

Append to `digest_test.go`:

```go
func TestBuildDigestQuery(t *testing.T) {
	since := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	q, args := buildDigestQuery(since)

	if !strings.Contains(q, "a2.feed_url <> a1.feed_url") {
		t.Fatalf("missing cross-feed condition: %s", q)
	}
	if !strings.Contains(q, "a1.title % a2.title") {
		t.Fatalf("missing trigram similarity condition: %s", q)
	}
	if !strings.Contains(q, "GROUP BY a1.id") {
		t.Fatalf("missing GROUP BY: %s", q)
	}
	if !strings.Contains(q, "ORDER BY cross_feed_count DESC, a1.publish_date DESC") {
		t.Fatalf("missing ORDER BY: %s", q)
	}
	if len(args) != 1 || args[0] != since {
		t.Fatalf("expected single since arg, got %v", args)
	}
}
```

Add `"strings"` to `digest_test.go`'s imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestBuildDigestQuery -v`
Expected: FAIL with `undefined: buildDigestQuery`

- [ ] **Step 3: Write minimal implementation**

Append to `digest.go`:

```go
// minCrossFeedCountForImportant is the cross-feed coverage threshold for the
// "important" bucket of a digest: a story counts as important once at least
// this many *other* feeds ran something with a similar title in the window.
const minCrossFeedCountForImportant = 2

// buildDigestQuery returns the SQL and args for the cross-feed importance
// heuristic: for every article published since `since`, count how many
// *other* feeds (feed_url <> a1.feed_url) ran a similarly-titled story
// (pg_trgm's `%` operator, backed by the existing idx_articles_title_trgm
// GIN index) in the same window.
//
// ponytail: title trigram similarity catches near-duplicate/syndicated
// headlines, not editorially-rewritten cross-outlet coverage of the same
// event — treat cross_feed_count as a duplication signal, not a true
// importance score. Upgrade path: embedding similarity or the existing
// Ollama summarizer, if this proves too weak in practice.
func buildDigestQuery(since time.Time) (string, []interface{}) {
	query := `SELECT a1.id, a1.title, a1.url, a1.summary, a1.full_content, a1.publish_date,
		a1.fetch_duration_ms, a1.feed_url, a1.content_hash, COUNT(DISTINCT a2.feed_url) AS cross_feed_count
		FROM articles a1
		LEFT JOIN articles a2
		  ON a2.publish_date >= $1 AND a2.feed_url <> a1.feed_url AND a1.title % a2.title
		WHERE a1.publish_date >= $1
		GROUP BY a1.id
		ORDER BY cross_feed_count DESC, a1.publish_date DESC`
	return query, []interface{}{since}
}
```

Write the trigram operator as a literal single `%` — `a1.title % a2.title` — this is a plain
backtick string, not `fmt.Sprintf`, so there is no `%`-escaping concern.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestBuildDigestQuery -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/pure/Documents/github/Information-Broker
git add digest.go digest_test.go
git commit -m "feat: add buildDigestQuery cross-feed trigram self-join"
```

---

## Task 3: Backend — `ArticleView.CrossFeedCount` + `splitImportant`

**Files:**
- Modify: `/home/pure/Documents/github/Information-Broker/api.go:86-97` (the `ArticleView` struct)
- Modify: `/home/pure/Documents/github/Information-Broker/digest.go`
- Modify: `/home/pure/Documents/github/Information-Broker/digest_test.go`

**Interfaces:**
- Consumes: `ArticleView` (from `api.go`)
- Produces: `splitImportant(rows []ArticleView) (important, other []ArticleView)`

- [ ] **Step 1: Write the failing test**

Append to `digest_test.go`:

```go
func TestSplitImportant(t *testing.T) {
	rows := []ArticleView{
		{ID: 1, CrossFeedCount: 3},
		{ID: 2, CrossFeedCount: 1},
		{ID: 3, CrossFeedCount: 2},
		{ID: 4, CrossFeedCount: 0},
	}
	important, other := splitImportant(rows)
	if len(important) != 2 || important[0].ID != 1 || important[1].ID != 3 {
		t.Fatalf("important = %+v, want IDs 1,3 in order", important)
	}
	if len(other) != 2 || other[0].ID != 2 || other[1].ID != 4 {
		t.Fatalf("other = %+v, want IDs 2,4 in order", other)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestSplitImportant -v`
Expected: FAIL — `unknown field CrossFeedCount in struct literal of type ArticleView` (the field
doesn't exist yet) or `undefined: splitImportant`.

- [ ] **Step 3: Write minimal implementation**

In `api.go`, change the `ArticleView` struct (currently `api.go:86-97`):

```go
// ArticleView is the JSON representation of an article returned by the API.
type ArticleView struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	URL            string        `json:"url"`
	Summary        *string       `json:"summary"`
	Content        string        `json:"content"`
	PublishedAt    time.Time     `json:"published_at"`
	FetchDuration  time.Duration `json:"fetch_duration"`
	FeedURL        string        `json:"feed_url"`
	ContentHash    string        `json:"content_hash"`
	CrossFeedCount int           `json:"cross_feed_count,omitempty"`
}
```

(Only the added `CrossFeedCount` field is new; every other field is unchanged. It's zero on
every existing endpoint's response and only populated by `/articles/digest`.)

Append to `digest.go`:

```go
// splitImportant partitions digest rows into important (>= minCrossFeedCountForImportant
// other feeds) and everything else, preserving the query's incoming order in both groups.
func splitImportant(rows []ArticleView) (important, other []ArticleView) {
	for _, a := range rows {
		if a.CrossFeedCount >= minCrossFeedCountForImportant {
			important = append(important, a)
		} else {
			other = append(other, a)
		}
	}
	return important, other
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -v`
Expected: all PASS, including `TestSplitImportant` and the existing `TestBuildArticlesQuery` /
`TestArticleDateFiltering` suites (confirms the `ArticleView` field addition didn't break
anything using struct literals elsewhere).

- [ ] **Step 5: Commit**

```bash
cd /home/pure/Documents/github/Information-Broker
git add api.go digest.go digest_test.go
git commit -m "feat: add ArticleView.CrossFeedCount and splitImportant"
```

---

## Task 4: Backend — `getArticlesDigest` handler + route

**Files:**
- Modify: `/home/pure/Documents/github/Information-Broker/digest.go`
- Modify: `/home/pure/Documents/github/Information-Broker/api.go:59-65` (route registration in
  `Start()`)

**Interfaces:**
- Consumes: `digestWindowOrDefault`, `buildDigestQuery`, `splitImportant`, `ArticleView`,
  `APIServer.db`
- Produces: `DigestResult{Range string; Since time.Time; Important, Other []ArticleView}`,
  `(s *APIServer) getArticlesDigest(w http.ResponseWriter, r *http.Request)`

No dedicated test for this task: every other DB-touching handler in this repo (`getArticles`,
`getFeeds`, `getStats`, etc.) is wired the same way with no handler-level test of its own — this
repo's existing convention only unit-tests the pure query-builder/split functions (which Tasks
1-3 already cover) and leaves the `db.Query`/`json.Encode` glue unverified by automated tests.
This task follows that established pattern rather than introducing a new DB-test harness just
for one handler. Verify by building successfully; the manual smoke-test note at the bottom of
this plan covers the rest once actually deployed (out of scope for this local-only pass).

- [ ] **Step 1: Write the implementation**

`digest.go`'s import currently reads `import "time"` (from Task 1). Replace it with a grouped
block adding the three new imports this task needs:

```go
import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)
```

Then append to `digest.go`:

```go
// DigestResult is the response envelope for GET /articles/digest.
type DigestResult struct {
	Range     string        `json:"range"`
	Since     time.Time     `json:"since"`
	Important []ArticleView `json:"important"`
	Other     []ArticleView `json:"other"`
}

var validDigestRanges = map[string]bool{"daily": true, "weekly": true, "monthly": true}

// getArticlesDigest returns articles bucketed into "important" (multi-feed
// coverage) and "other" for the requested daily/weekly/monthly window.
func (s *APIServer) getArticlesDigest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rangeParam := r.URL.Query().Get("range")
	if !validDigestRanges[rangeParam] {
		rangeParam = "daily"
	}
	since := time.Now().Add(-digestWindowOrDefault(rangeParam))

	query, args := buildDigestQuery(since)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("Database query error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	all := []ArticleView{}
	for rows.Next() {
		var a ArticleView
		var fetchDurationMs int64
		err := rows.Scan(
			&a.ID, &a.Title, &a.URL, &a.Summary, &a.Content, &a.PublishedAt,
			&fetchDurationMs, &a.FeedURL, &a.ContentHash, &a.CrossFeedCount,
		)
		if err != nil {
			log.Printf("Row scan error: %v", err)
			continue
		}
		a.FetchDuration = time.Duration(fetchDurationMs) * time.Millisecond
		all = append(all, a)
	}

	important, other := splitImportant(all)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(DigestResult{
		Range: rangeParam, Since: since, Important: important, Other: other,
	})
}
```

In `api.go`'s `Start()`, add one route line right after the `/articles/get` registration
(currently `api.go:61`):

```go
	mux.HandleFunc("/articles/get", corsHandler(s.metrics.HTTPMetricsMiddleware(s.getArticleByID, "/articles/get")))
	mux.HandleFunc("/articles/digest", corsHandler(s.metrics.HTTPMetricsMiddleware(s.getArticlesDigest, "/articles/digest")))
```

- [ ] **Step 2: Build and run the full backend test suite**

Run: `cd /home/pure/Documents/github/Information-Broker && go build ./... && go test ./...`
Expected: build succeeds, all tests PASS (no regressions from the route/handler addition).

- [ ] **Step 3: Commit**

```bash
cd /home/pure/Documents/github/Information-Broker
git add digest.go api.go
git commit -m "feat: add GET /articles/digest endpoint"
```

---

## Task 5: Frontend — `apiclient.GetDigest`

**Files:**
- Modify: `/home/pure/Documents/github/SmellyFeet/internal/apiclient/apiclient.go`
- Modify: `/home/pure/Documents/github/SmellyFeet/internal/apiclient/apiclient_test.go`

**Interfaces:**
- Consumes: `Client.getJSON` (existing), `Client.baseURL` (existing)
- Produces: `Article.CrossFeedCount int`, `DigestResult{Range string; Since time.Time;
  Important, Other []Article}`, `(c *Client) GetDigest(ctx context.Context, rangeParam string)
  (DigestResult, error)`

- [ ] **Step 1: Write the failing test**

Append to `apiclient_test.go`:

```go
func TestGetDigest(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Write([]byte(`{"range":"weekly","since":"2026-07-07T00:00:00Z","important":[{"id":1,"title":"Big story","cross_feed_count":3}],"other":[{"id":2,"title":"Minor","cross_feed_count":0}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.GetDigest(context.Background(), "weekly")
	if err != nil {
		t.Fatalf("GetDigest error: %v", err)
	}
	if gotPath != "/articles/digest?range=weekly" {
		t.Fatalf("unexpected request path: %s", gotPath)
	}
	if res.Range != "weekly" || len(res.Important) != 1 || res.Important[0].CrossFeedCount != 3 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(res.Other) != 1 || res.Other[0].ID != 2 {
		t.Fatalf("unexpected other: %+v", res)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run (from `/home/pure/Documents/github/SmellyFeet`): `go test ./internal/apiclient/... -run TestGetDigest -v`
Expected: FAIL with `c.GetDigest undefined` (or `res.Range undefined` etc.)

- [ ] **Step 3: Write minimal implementation**

In `apiclient.go`, change the `Article` struct (currently lines 20-29) to add one field:

```go
// Article mirrors the JSON returned by the Information-Broker API.
type Article struct {
	ID             int64     `json:"id"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	Summary        *string   `json:"summary"`
	Content        string    `json:"content"`
	PublishedAt    time.Time `json:"published_at"`
	FeedURL        string    `json:"feed_url"`
	ContentHash    string    `json:"content_hash"`
	CrossFeedCount int       `json:"cross_feed_count,omitempty"`
}
```

Then append near `ListResult` (after line 37):

```go
// DigestResult is the envelope returned by GET /articles/digest.
type DigestResult struct {
	Range     string    `json:"range"`
	Since     time.Time `json:"since"`
	Important []Article `json:"important"`
	Other     []Article `json:"other"`
}
```

And append a new method after `ListArticles` (after line 119):

```go
// GetDigest fetches the daily/weekly/monthly cross-feed importance digest.
func (c *Client) GetDigest(ctx context.Context, rangeParam string) (DigestResult, error) {
	var res DigestResult
	err := c.getJSON(ctx, "/articles/digest?range="+url.QueryEscape(rangeParam), &res)
	return res, err
}
```

(`net/url` is already imported in `apiclient.go`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/apiclient/... -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/apiclient/apiclient.go internal/apiclient/apiclient_test.go
git commit -m "feat: add apiclient.GetDigest for the cross-feed importance digest"
```

---

## Task 6: Frontend — `handleDigest` route + `stubService` wiring

**Files:**
- Modify: `/home/pure/Documents/github/SmellyFeet/internal/server/server.go:56-61` (the
  `ArticleService` interface) and `:162-188` (`Routes()`)
- Modify: `/home/pure/Documents/github/SmellyFeet/internal/server/handlers.go`
- Modify: `/home/pure/Documents/github/SmellyFeet/internal/server/server_test.go:14-41`
  (`stubService`)
- Create: `/home/pure/Documents/github/SmellyFeet/internal/server/digest_test.go`

**Interfaces:**
- Consumes: `apiclient.DigestResult` (Task 5), `s.render`, `s.renderError`, `setCache`,
  `cacheList` (all existing in `server.go`)
- Produces: `digestView{Title, Desc string; OG bool; Range string; Important, Other
  []apiclient.Article}`, `(s *Server) handleDigest(w http.ResponseWriter, r *http.Request)`,
  route `GET /digest`

- [ ] **Step 1: Write the failing tests**

Create `digest_test.go`:

```go
package server

import (
	"errors"
	"strings"
	"testing"

	"smellyfeet/internal/apiclient"
)

func TestHandleDigestRangeWhitelist(t *testing.T) {
	h := newTestServer(t, stubService{})
	for _, tt := range []struct{ path, wantSelected string }{
		{"/digest", `<option value="daily" selected>`},
		{"/digest?range=weekly", `<option value="weekly" selected>`},
		{"/digest?range=monthly", `<option value="monthly" selected>`},
		{"/digest?range=garbage", `<option value="daily" selected>`},
	} {
		body := getPath(t, h, tt.path).Body.String()
		if !strings.Contains(body, tt.wantSelected) {
			t.Errorf("%s: missing %q in body: %s", tt.path, tt.wantSelected, body)
		}
	}
}

func TestHandleDigestUpstreamErrorRendersErrorPage(t *testing.T) {
	svc := stubService{digestErr: errors.New("boom")}
	rec := getPath(t, newTestServer(t, svc), "/digest")
	if rec.Code != 502 {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

func TestDigestImportantAndOtherSections(t *testing.T) {
	svc := stubService{digest: apiclient.DigestResult{
		Range:     "daily",
		Important: []apiclient.Article{{ID: 1, Title: "Big story", CrossFeedCount: 3}},
		Other:     []apiclient.Article{{ID: 2, Title: "Minor item"}},
	}}
	body := getPath(t, newTestServer(t, svc), "/digest").Body.String()
	if !containsAll(body, "Big story", "3 sources", "everything else (1)", "Minor item") {
		t.Fatalf("digest body missing expected markers: %s", body)
	}
}

func TestDigestEmptyState(t *testing.T) {
	body := getPath(t, newTestServer(t, stubService{}), "/digest").Body.String()
	if !strings.Contains(body, "No articles found") {
		t.Fatalf("expected empty state message, got: %s", body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/... -run TestHandleDigest -v`
Expected: FAIL to compile — `stubService` doesn't implement the (not-yet-extended)
`ArticleService` interface, and `handleDigest`/route `/digest` don't exist yet.

- [ ] **Step 3: Write minimal implementation**

In `server.go`, extend the `ArticleService` interface (currently `server.go:56-61`):

```go
// ArticleService is the subset of the API the handlers need.
type ArticleService interface {
	ListArticles(ctx context.Context, p apiclient.ListParams) (apiclient.ListResult, error)
	GetArticle(ctx context.Context, id int64) (apiclient.Article, error)
	ListFeeds(ctx context.Context) ([]apiclient.Feed, error)
	GetStats(ctx context.Context) (apiclient.Stats, error)
	GetDigest(ctx context.Context, rangeParam string) (apiclient.DigestResult, error)
}
```

In `server.go`'s `Routes()` (currently `server.go:162-176`), add one line after `GET /{$}`:

```go
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /{$}", s.handleList)
	mux.HandleFunc("GET /digest", s.handleDigest)
	mux.HandleFunc("GET /article/{id}", s.handleArticle)
```

In `server_test.go`, extend `stubService` (currently `server_test.go:14-41`) with two new
fields and one new method:

```go
// stubService implements ArticleService for tests.
type stubService struct {
	list      apiclient.ListResult
	article   apiclient.Article
	feeds     []apiclient.Feed
	stats     apiclient.Stats
	digest    apiclient.DigestResult
	listErr   error
	getErr    error
	feedsErr  error
	statsErr  error
	digestErr error
	lastList  *apiclient.ListParams
}
```

```go
func (s stubService) GetDigest(ctx context.Context, rangeParam string) (apiclient.DigestResult, error) {
	return s.digest, s.digestErr
}
```

In `handlers.go`, add the whitelist, view struct, and handler (after `handleList`, before
`handleArticle`):

```go
var validDigestRanges = map[string]bool{"daily": true, "weekly": true, "monthly": true}

type digestView struct {
	Title     string
	Desc      string
	OG        bool
	Range     string
	Important []apiclient.Article
	Other     []apiclient.Article
}

func (s *Server) handleDigest(w http.ResponseWriter, r *http.Request) {
	rangeParam := r.URL.Query().Get("range")
	if !validDigestRanges[rangeParam] {
		rangeParam = "daily"
	}

	res, err := s.svc.GetDigest(r.Context(), rangeParam)
	if err != nil {
		s.renderError(w, err)
		return
	}

	setCache(w, cacheList)
	s.render(w, http.StatusOK, "digest", digestView{
		Title:     "Digest",
		Desc:      "Cross-feed importance digest — stories covered by multiple sources, grouped by day, week, or month.",
		OG:        true,
		Range:     rangeParam,
		Important: res.Important,
		Other:     res.Other,
	})
}
```

- [ ] **Step 4: Note on the template dependency**

Run: `go test ./internal/server/... -run TestHandleDigest -v`
Expected at this point: still FAIL — `html/template: "digest" is undefined`. `New()` parses all
templates from `templates/*.html` in one `ParseFS` call, so `handleDigest` can't render until
Task 7's template file exists. Proceed directly to Task 7; Tasks 6 and 7 share one green test
run and one commit (Task 7 Step 4 and Step 7).

---

## Task 7: Frontend — `digest.html` template, `articleCard` badge, nav link

**Files:**
- Create: `/home/pure/Documents/github/SmellyFeet/internal/server/templates/digest.html`
- Modify: `/home/pure/Documents/github/SmellyFeet/internal/server/templates/list.html:1-19`
  (the `articleCard` define)
- Modify: `/home/pure/Documents/github/SmellyFeet/internal/server/templates/partials.html:23-27`
  (nav)

**Interfaces:**
- Consumes: `digestView` (Task 6), shared `articleCard` define, `header`/`footer` defines
  (all existing)
- Produces: template `"digest"`, one new nav link

- [ ] **Step 1: Add the cross-feed badge to the shared `articleCard` define**

In `list.html`, the meta row currently reads (lines 5-9):

```html
    <div class="flex items-center justify-between font-mono text-[11px] mb-2 gap-3 [overflow-wrap:anywhere]">
      <span class="flex items-center gap-2 min-w-0">
        <a href="/?feed={{ .FeedURL | urlquery }}" title="Filter by {{ sourceName .FeedURL }}" class="relative z-10 truncate rounded border border-line bg-ink-950 px-2 py-0.5 text-accent-dim hover:text-accent hover:border-accent/60 transition-colors">{{ sourceName .FeedURL }}</a>
        {{ with cveID .Title }}<span class="shrink-0 rounded border border-accent/40 bg-accent/10 px-2 py-0.5 text-accent">{{ . }}</span>{{ end }}
      </span>
```

Add the badge right after the `cveID` span, still inside the same `<span class="flex items-center gap-2 min-w-0">`:

```html
    <div class="flex items-center justify-between font-mono text-[11px] mb-2 gap-3 [overflow-wrap:anywhere]">
      <span class="flex items-center gap-2 min-w-0">
        <a href="/?feed={{ .FeedURL | urlquery }}" title="Filter by {{ sourceName .FeedURL }}" class="relative z-10 truncate rounded border border-line bg-ink-950 px-2 py-0.5 text-accent-dim hover:text-accent hover:border-accent/60 transition-colors">{{ sourceName .FeedURL }}</a>
        {{ with cveID .Title }}<span class="shrink-0 rounded border border-accent/40 bg-accent/10 px-2 py-0.5 text-accent">{{ . }}</span>{{ end }}
        {{ if gt .CrossFeedCount 1 }}<span class="shrink-0 rounded border border-accent/40 bg-accent/10 px-2 py-0.5 text-accent">{{ .CrossFeedCount }} sources</span>{{ end }}
      </span>
```

This renders nowhere except the digest page's Important list, since `CrossFeedCount` is `0` on
every `apiclient.Article` returned by `/articles` (list/article/upcoming).

- [ ] **Step 2: Add the nav link**

In `partials.html`, the nav currently reads (lines 23-27):

```html
      <nav class="flex items-center gap-0.5 sm:gap-1 font-mono text-[10px] sm:text-xs uppercase tracking-[0.12em] sm:tracking-[0.18em]">
        <a href="/" class="px-2 sm:px-3 py-1.5 rounded-md text-fog hover:text-accent hover:bg-ink-800 transition-colors">Feed</a>
        <a href="/stats" class="px-2 sm:px-3 py-1.5 rounded-md text-fog hover:text-accent hover:bg-ink-800 transition-colors">Stats</a>
        <a href="/about" class="px-2 sm:px-3 py-1.5 rounded-md text-fog hover:text-accent hover:bg-ink-800 transition-colors">About</a>
      </nav>
```

Add a `Digest` link between `Feed` and `Stats`:

```html
      <nav class="flex items-center gap-0.5 sm:gap-1 font-mono text-[10px] sm:text-xs uppercase tracking-[0.12em] sm:tracking-[0.18em]">
        <a href="/" class="px-2 sm:px-3 py-1.5 rounded-md text-fog hover:text-accent hover:bg-ink-800 transition-colors">Feed</a>
        <a href="/digest" class="px-2 sm:px-3 py-1.5 rounded-md text-fog hover:text-accent hover:bg-ink-800 transition-colors">Digest</a>
        <a href="/stats" class="px-2 sm:px-3 py-1.5 rounded-md text-fog hover:text-accent hover:bg-ink-800 transition-colors">Stats</a>
        <a href="/about" class="px-2 sm:px-3 py-1.5 rounded-md text-fog hover:text-accent hover:bg-ink-800 transition-colors">About</a>
      </nav>
```

- [ ] **Step 3: Create `digest.html`**

```html
{{ define "digest" }}{{ template "header" . }}
<div class="mb-7">
  <p class="font-mono text-[11px] uppercase tracking-[0.25em] text-accent/80 mb-1.5">// digest</p>
  <h1 class="text-2xl font-semibold text-zinc-100 tracking-tight">Cross-feed importance digest</h1>
</div>

<form method="get" action="/digest" class="mb-5 flex flex-wrap gap-3 items-end bg-ink-900 border border-line rounded-xl p-4">
  <div>
    <label for="range" class="block font-mono text-[10px] uppercase tracking-widest text-fog mb-1.5">Range</label>
    <select id="range" name="range" class="rounded-lg bg-ink-950 border border-line px-3 py-2 text-sm text-zinc-100 focus:outline-none focus:border-accent transition-colors">
      <option value="daily" {{ if eq .Range "daily" }}selected{{ end }}>Daily</option>
      <option value="weekly" {{ if eq .Range "weekly" }}selected{{ end }}>Weekly</option>
      <option value="monthly" {{ if eq .Range "monthly" }}selected{{ end }}>Monthly</option>
    </select>
  </div>
  <button type="submit" class="rounded-lg bg-accent text-ink-950 font-semibold px-5 py-2 text-sm hover:bg-accent-bright transition-colors">Apply</button>
</form>

{{ if and (not .Important) (not .Other) }}
  <div class="text-center py-16 border border-dashed border-line rounded-xl">
    <p class="font-mono text-sm text-fog">No articles found in this window.</p>
  </div>
{{ end }}

{{ if .Important }}
<p class="font-mono text-[11px] uppercase tracking-[0.2em] text-fog mb-3">Important</p>
<ul class="space-y-3 mb-8">
  {{ range .Important }}{{ template "articleCard" . }}{{ end }}
</ul>
{{ end }}

{{ if .Other }}
<details class="mb-5 rounded-xl border border-line bg-ink-900/60">
  <summary class="cursor-pointer select-none px-4 py-3 font-mono text-[11px] uppercase tracking-[0.25em] text-fog hover:text-accent transition-colors">everything else ({{ len .Other }})</summary>
  <ul class="space-y-3 p-4 pt-1">
    {{ range .Other }}{{ template "articleCard" . }}{{ end }}
  </ul>
</details>
{{ end }}
{{ template "footer" . }}{{ end }}
```

- [ ] **Step 4: Run the full frontend test suite**

Run (from `/home/pure/Documents/github/SmellyFeet`): `go test ./...`
Expected: all PASS, including every test added in Task 6 (`TestHandleDigestRangeWhitelist`,
`TestHandleDigestUpstreamErrorRendersErrorPage`, `TestDigestImportantAndOtherSections`,
`TestDigestEmptyState`) and every pre-existing test (confirms the `articleCard` badge addition
didn't break `TestListPageRendersSummary`, `TestSortSelectAndPaginationCarrySort`, etc.).

- [ ] **Step 5: Rebuild CSS**

Run: `./scripts/build-css.sh`
Expected: exits 0; `internal/server/static/app.css` is regenerated (same Tailwind utility
classes are already used elsewhere in `list.html`, e.g. the `cveID` badge, so this is
confirmation, not new class discovery).

- [ ] **Step 6: Add a cache-control coverage case**

In `cache_test.go`'s `TestCacheControlPerRoute` table (currently lines 20-28), add one row:

```go
		{"/", "public, max-age=60, s-maxage=120, stale-while-revalidate=300"},
		{"/digest", "public, max-age=60, s-maxage=120, stale-while-revalidate=300"},
		{"/stats", "public, max-age=30, s-maxage=60"},
```

Run: `go test ./internal/server/... -run TestCacheControlPerRoute -v`
Expected: PASS.

- [ ] **Step 7: Commit everything from Tasks 6 and 7 together**

```bash
git add internal/server/server.go internal/server/handlers.go internal/server/server_test.go \
        internal/server/digest_test.go internal/server/cache_test.go \
        internal/server/templates/digest.html internal/server/templates/list.html \
        internal/server/templates/partials.html internal/server/static/app.css
git commit -m "feat(frontend): add /digest tab with important/everything-else sections"
```

---

## Manual verification (not automated, do once both repos are committed)

- [ ] `cd /home/pure/Documents/github/Information-Broker && go build ./... && go vet ./...`
- [ ] `cd /home/pure/Documents/github/SmellyFeet && go build ./... && go vet ./...`
- [ ] Once actually deployed (out of scope for this pass): hit `/digest`, `/digest?range=weekly`,
  `/digest?range=monthly` live and eyeball whether the "Important" bucket looks meaningfully
  different from "everything else" — this is the real test of the heuristic's known ceiling
  (see the spec's "Known ceiling" section). If it's mostly empty or mostly everything, that's
  the signal to revisit the threshold or the trigram-vs-Ollama tradeoff.
