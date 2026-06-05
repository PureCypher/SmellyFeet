# Information-Broker Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone Go web frontend that lists AI-generated article summaries from Information-Broker, lets the user read each article's full text and source, and supports search, feed filtering, pagination, and a stats page.

**Architecture:** A new server-rendered Go app (in the `SmellyFeet` repo) using `html/template` + Tailwind (CDN). It talks to Information-Broker only over HTTP via a thin `apiclient` package — no direct DB access. Two small additive changes are made to Information-Broker's existing API (`../Information-Broker`) so it exposes `id` + `summary` and a single-article endpoint.

**Tech Stack:** Go 1.22 (frontend), Go 1.21 (backend, unchanged version), `net/http`, `html/template`, Tailwind via CDN, standard-library `testing` + `net/http/httptest`.

---

## Repository layout

Two repos are touched:

- **`../Information-Broker`** (backend) — additive changes to `api.go` only.
- **`SmellyFeet`** (this repo) — the new frontend:

```
SmellyFeet/
  go.mod                              module "smellyfeet", go 1.22
  main.go                             wire config + apiclient + server, ListenAndServe
  .env.example                        API_BASE_URL, PORT
  internal/
    config/
      config.go                       Load() Config from env
      config_test.go
    apiclient/
      apiclient.go                    Client, types, getJSON, List/Get/Feeds/Stats
      apiclient_test.go
    server/
      server.go                       Server, New(), Routes(), render()
      handlers.go                     handleList/handleArticle/handleStats/handleHealthz
      server_test.go
      templates/
        partials.html                 "header" + "footer" definitions
        list.html                     "list"
        article.html                  "article"
        stats.html                    "stats"
        error.html                    "error"
        notfound.html                 "notfound"
```

**File responsibilities:**
- `config` — read `API_BASE_URL` and `PORT` from env with defaults. No other concern.
- `apiclient` — HTTP client for the IB API: typed structs + methods, JSON decode, error mapping. No HTML knowledge.
- `server` — HTTP handlers + template rendering. Depends on an `ArticleService` interface (implemented by `apiclient.Client`) so handlers are testable with a stub. No raw HTTP-client or SQL details.
- `templates` — presentation only, embedded via `//go:embed`.

---

## PART A — Backend changes (`../Information-Broker`)

> All commands in Part A run from `/home/pure/Documents/github/Information-Broker`.

### Task 1: Expose `id` + `summary` and add `q` search to `/articles`

**Files:**
- Modify: `api.go` (add `ArticleView` struct, `buildArticlesQuery`, rewrite `getArticles` body, add `strings` import)
- Test: `api_query_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `api_query_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestBuildArticlesQuery(t *testing.T) {
	t.Run("no filters", func(t *testing.T) {
		q, args := buildArticlesQuery("", "", 50, 0)
		if strings.Contains(q, "WHERE") {
			t.Fatalf("expected no WHERE clause, got: %s", q)
		}
		if !strings.Contains(q, "ORDER BY publish_date DESC") {
			t.Fatalf("missing ORDER BY: %s", q)
		}
		if len(args) != 2 { // limit, offset
			t.Fatalf("expected 2 args, got %d: %v", len(args), args)
		}
	})

	t.Run("feed only", func(t *testing.T) {
		q, args := buildArticlesQuery("https://example.com/rss", "", 50, 0)
		if !strings.Contains(q, "feed_url = $1") {
			t.Fatalf("missing feed filter: %s", q)
		}
		if len(args) != 3 || args[0] != "https://example.com/rss" {
			t.Fatalf("unexpected args: %v", args)
		}
	})

	t.Run("query only", func(t *testing.T) {
		q, args := buildArticlesQuery("", "ransomware", 50, 0)
		if !strings.Contains(q, "ILIKE") {
			t.Fatalf("missing ILIKE search: %s", q)
		}
		if len(args) != 5 { // 3 like args + limit + offset
			t.Fatalf("expected 5 args, got %d: %v", len(args), args)
		}
		if args[0] != "%ransomware%" {
			t.Fatalf("expected wrapped like arg, got %v", args[0])
		}
	})

	t.Run("feed and query", func(t *testing.T) {
		q, args := buildArticlesQuery("https://example.com/rss", "cve", 10, 20)
		if !strings.Contains(q, "feed_url = $1") || !strings.Contains(q, "ILIKE $2") {
			t.Fatalf("expected both filters with correct placeholders: %s", q)
		}
		if len(args) != 6 {
			t.Fatalf("expected 6 args, got %d: %v", len(args), args)
		}
		if args[len(args)-2] != 10 || args[len(args)-1] != 20 {
			t.Fatalf("limit/offset should be last: %v", args)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestBuildArticlesQuery -v`
Expected: FAIL — `undefined: buildArticlesQuery`.

- [ ] **Step 3: Add `ArticleView` + `buildArticlesQuery` and ensure `strings` is imported**

At the top of `api.go`, add `"strings"` to the import block (alongside the existing `"strconv"` / `"time"` imports).

Add these declarations to `api.go` (e.g. just above `func (s *APIServer) getArticles`):

```go
// ArticleView is the JSON representation of an article returned by the API.
type ArticleView struct {
	ID            int64         `json:"id"`
	Title         string        `json:"title"`
	URL           string        `json:"url"`
	Summary       *string       `json:"summary"`
	Content       string        `json:"content"`
	PublishedAt   time.Time     `json:"published_at"`
	FetchDuration time.Duration `json:"fetch_duration"`
	FeedURL       string        `json:"feed_url"`
	ContentHash   string        `json:"content_hash"`
}

// buildArticlesQuery constructs the SQL and ordered args for listing articles,
// applying optional feed and case-insensitive search (q) filters.
func buildArticlesQuery(feed, q string, limit, offset int) (string, []interface{}) {
	query := `SELECT id, title, url, summary, full_content, publish_date, fetch_duration_ms, feed_url, content_hash
		FROM articles`
	var conds []string
	var args []interface{}
	i := 1
	if feed != "" {
		conds = append(conds, fmt.Sprintf("feed_url = $%d", i))
		args = append(args, feed)
		i++
	}
	if q != "" {
		conds = append(conds, fmt.Sprintf("(title ILIKE $%d OR summary ILIKE $%d OR full_content ILIKE $%d)", i, i+1, i+2))
		like := "%" + q + "%"
		args = append(args, like, like, like)
		i += 3
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY publish_date DESC LIMIT $%d OFFSET $%d", i, i+1)
	args = append(args, limit, offset)
	return query, args
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestBuildArticlesQuery -v`
Expected: PASS (all four sub-tests).

- [ ] **Step 5: Rewrite `getArticles` to use the builder + return id/summary**

Replace the body of `getArticles` from the `// Build query` comment through the end of the row loop. The new body (after the `feedURL := r.URL.Query().Get("feed")` line) reads:

```go
	searchQ := r.URL.Query().Get("q")

	query, args := buildArticlesQuery(feedURL, searchQ, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("Database query error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	articles := []ArticleView{}
	for rows.Next() {
		var article ArticleView
		var fetchDurationMs int64

		err := rows.Scan(
			&article.ID,
			&article.Title,
			&article.URL,
			&article.Summary,
			&article.Content,
			&article.PublishedAt,
			&fetchDurationMs,
			&article.FeedURL,
			&article.ContentHash,
		)
		if err != nil {
			log.Printf("Row scan error: %v", err)
			continue
		}

		article.FetchDuration = time.Duration(fetchDurationMs) * time.Millisecond
		articles = append(articles, article)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"articles": articles,
		"count":    len(articles),
		"limit":    limit,
		"offset":   offset,
	})
```

(The earlier `feedURL := r.URL.Query().Get("feed")` line stays; the old `var query string`/`var args []interface{}`/`if feedURL != ""` block and the old `var articles []Article` scan loop are fully replaced by the above.)

- [ ] **Step 6: Verify the whole backend still builds and tests pass**

Run: `go build ./... && go test ./... -run TestBuildArticlesQuery -v`
Expected: build succeeds; tests PASS.

- [ ] **Step 7: Commit**

```bash
git add api.go api_query_test.go
git commit -m "feat(api): expose id+summary and add q search to /articles"
```

---

### Task 2: Add single-article endpoint `GET /articles/get?id=N`

**Files:**
- Modify: `api.go` (add route in `Start()`, add `getArticleByID` + `parseArticleID`)
- Test: `api_article_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `api_article_test.go`:

```go
package main

import "testing"

func TestParseArticleID(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"123", 123, false},
		{"1", 1, false},
		{"", 0, true},
		{"abc", 0, true},
		{"0", 0, true},
		{"-5", 0, true},
	}
	for _, c := range cases {
		got, err := parseArticleID(c.in)
		if c.wantErr && err == nil {
			t.Errorf("parseArticleID(%q): expected error, got nil", c.in)
		}
		if !c.wantErr && err != nil {
			t.Errorf("parseArticleID(%q): unexpected error %v", c.in, err)
		}
		if !c.wantErr && got != c.want {
			t.Errorf("parseArticleID(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestParseArticleID -v`
Expected: FAIL — `undefined: parseArticleID`.

- [ ] **Step 3: Add `parseArticleID` and `getArticleByID`**

Add to `api.go` (e.g. directly below `getArticles`):

```go
// parseArticleID validates and parses an article id query value.
func parseArticleID(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("missing id")
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id: %q", s)
	}
	return id, nil
}

// getArticleByID returns a single article (incl. summary + full content) by id.
func (s *APIServer) getArticleByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, err := parseArticleID(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	query := `SELECT id, title, url, summary, full_content, publish_date, fetch_duration_ms, feed_url, content_hash
		FROM articles WHERE id = $1`

	var article ArticleView
	var fetchDurationMs int64
	err = s.db.QueryRow(query, id).Scan(
		&article.ID,
		&article.Title,
		&article.URL,
		&article.Summary,
		&article.Content,
		&article.PublishedAt,
		&fetchDurationMs,
		&article.FeedURL,
		&article.ContentHash,
	)
	if err == sql.ErrNoRows {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Database query error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	article.FetchDuration = time.Duration(fetchDurationMs) * time.Millisecond

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(article)
}
```

(`sql` is already imported in `api.go` as `"database/sql"`.)

- [ ] **Step 4: Register the route**

In `func (s *APIServer) Start()`, just after the existing `mux.HandleFunc("/articles/latest", ...)` line, add:

```go
	mux.HandleFunc("/articles/get", corsHandler(s.metrics.HTTPMetricsMiddleware(s.getArticleByID, "/articles/get")))
```

- [ ] **Step 5: Run tests + build**

Run: `go build ./... && go test ./... -run TestParseArticleID -v`
Expected: build succeeds; test PASS.

- [ ] **Step 6: Commit**

```bash
git add api.go api_article_test.go
git commit -m "feat(api): add GET /articles/get single-article endpoint"
```

---

## PART B — Frontend (`SmellyFeet`)

> All commands in Part B run from `/home/pure/Documents/github/SmellyFeet`.

### Task 3: Initialize module + config package

**Files:**
- Create: `go.mod`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Initialize the module**

Run:
```bash
go mod init smellyfeet && go mod edit -go=1.22
```
Expected: creates `go.mod` with `module smellyfeet` and `go 1.22`.

- [ ] **Step 2: Write the failing test**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	os.Unsetenv("API_BASE_URL")
	os.Unsetenv("PORT")
	c := Load()
	if c.APIBaseURL != "http://localhost:8080" {
		t.Errorf("APIBaseURL default = %q", c.APIBaseURL)
	}
	if c.Port != "3000" {
		t.Errorf("Port default = %q", c.Port)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("API_BASE_URL", "http://192.168.1.135:8080")
	t.Setenv("PORT", "9999")
	c := Load()
	if c.APIBaseURL != "http://192.168.1.135:8080" {
		t.Errorf("APIBaseURL = %q", c.APIBaseURL)
	}
	if c.Port != "9999" {
		t.Errorf("Port = %q", c.Port)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `undefined: Load` (build error).

- [ ] **Step 4: Implement config**

Create `internal/config/config.go`:

```go
// Package config loads frontend settings from the environment.
package config

import "os"

// Config holds frontend runtime settings.
type Config struct {
	APIBaseURL string // base URL of the Information-Broker API
	Port       string // port the frontend listens on
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		APIBaseURL: getenv("API_BASE_URL", "http://localhost:8080"),
		Port:       getenv("PORT", "3000"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add go.mod internal/config/
git commit -m "feat(frontend): init module and config package"
```

---

### Task 4: apiclient — types, client, `getJSON`, `ListArticles`

**Files:**
- Create: `internal/apiclient/apiclient.go`
- Test: `internal/apiclient/apiclient_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/apiclient/apiclient_test.go`:

```go
package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListArticles(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"articles":[{"id":7,"title":"T","url":"u","summary":"s","content":"c","feed_url":"f","content_hash":"h"}],"count":1,"limit":20,"offset":0}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.ListArticles(context.Background(), ListParams{Limit: 20, Offset: 0, Feed: "f", Q: "ransom"})
	if err != nil {
		t.Fatalf("ListArticles error: %v", err)
	}
	if res.Count != 1 || len(res.Articles) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Articles[0].ID != 7 || res.Articles[0].Summary == nil || *res.Articles[0].Summary != "s" {
		t.Fatalf("unexpected article: %+v", res.Articles[0])
	}
	if gotPath != "/articles?feed=f&limit=20&q=ransom" {
		t.Fatalf("unexpected request path: %s", gotPath)
	}
}

func TestGetJSONNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.ListArticles(context.Background(), ListParams{Limit: 20})
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/apiclient/ -v`
Expected: FAIL — `undefined: New` / `undefined: ListParams`.

- [ ] **Step 3: Implement apiclient core + ListArticles**

Create `internal/apiclient/apiclient.go`:

```go
// Package apiclient is a thin HTTP client for the Information-Broker API.
package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound is returned when the API responds 404.
var ErrNotFound = errors.New("not found")

// Article mirrors the JSON returned by the Information-Broker API.
type Article struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Summary     *string   `json:"summary"`
	Content     string    `json:"content"`
	PublishedAt time.Time `json:"published_at"`
	FeedURL     string    `json:"feed_url"`
	ContentHash string    `json:"content_hash"`
}

// ListResult is the envelope returned by GET /articles.
type ListResult struct {
	Articles []Article `json:"articles"`
	Count    int       `json:"count"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
}

// Feed is one entry from GET /feeds.
type Feed struct {
	FeedURL      string `json:"feed_url"`
	ArticleCount int    `json:"article_count"`
}

// Stats is the payload from GET /stats.
type Stats struct {
	TotalArticles int        `json:"total_articles"`
	TotalFeeds    int        `json:"total_feeds"`
	LastFetch     *time.Time `json:"last_fetch"`
}

// ListParams are the query options for ListArticles.
type ListParams struct {
	Limit  int
	Offset int
	Feed   string
	Q      string
}

// Client calls the Information-Broker API.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates a Client for the given API base URL.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("api %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// ListArticles fetches a page of articles with optional feed/search filters.
func (c *Client) ListArticles(ctx context.Context, p ListParams) (ListResult, error) {
	v := url.Values{}
	if p.Limit > 0 {
		v.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Offset > 0 {
		v.Set("offset", strconv.Itoa(p.Offset))
	}
	if p.Feed != "" {
		v.Set("feed", p.Feed)
	}
	if p.Q != "" {
		v.Set("q", p.Q)
	}
	var res ListResult
	err := c.getJSON(ctx, "/articles?"+v.Encode(), &res)
	return res, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/apiclient/ -v`
Expected: PASS (both tests). Note `url.Values.Encode()` sorts keys alphabetically, so the path is `feed=f&limit=20&q=ransom` (no offset because it's 0).

- [ ] **Step 5: Commit**

```bash
git add internal/apiclient/
git commit -m "feat(frontend): apiclient core and ListArticles"
```

---

### Task 5: apiclient — `GetArticle`, `ListFeeds`, `GetStats`

**Files:**
- Modify: `internal/apiclient/apiclient.go`
- Modify: `internal/apiclient/apiclient_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/apiclient/apiclient_test.go`:

```go
func TestGetArticleNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.GetArticle(context.Background(), 42)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetArticleOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/articles/get?id=42" {
			t.Errorf("unexpected path: %s", r.URL.RequestURI())
		}
		w.Write([]byte(`{"id":42,"title":"Deep Dive","content":"full body","feed_url":"f"}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	a, err := c.GetArticle(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetArticle error: %v", err)
	}
	if a.ID != 42 || a.Content != "full body" {
		t.Fatalf("unexpected article: %+v", a)
	}
}

func TestListFeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"feeds":[{"feed_url":"https://a/rss","article_count":5}],"count":1}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	feeds, err := c.ListFeeds(context.Background())
	if err != nil {
		t.Fatalf("ListFeeds error: %v", err)
	}
	if len(feeds) != 1 || feeds[0].FeedURL != "https://a/rss" || feeds[0].ArticleCount != 5 {
		t.Fatalf("unexpected feeds: %+v", feeds)
	}
}

func TestGetStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"total_articles":100,"total_feeds":12,"last_fetch":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	s, err := c.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats error: %v", err)
	}
	if s.TotalArticles != 100 || s.TotalFeeds != 12 {
		t.Fatalf("unexpected stats: %+v", s)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/apiclient/ -run 'TestGetArticle|TestListFeeds|TestGetStats' -v`
Expected: FAIL — `undefined: ... GetArticle` etc.

- [ ] **Step 3: Implement the three methods**

Append to `internal/apiclient/apiclient.go`:

```go
// GetArticle fetches a single article by id. Returns ErrNotFound on 404.
func (c *Client) GetArticle(ctx context.Context, id int64) (Article, error) {
	var a Article
	err := c.getJSON(ctx, "/articles/get?id="+strconv.FormatInt(id, 10), &a)
	return a, err
}

// ListFeeds returns the known feeds and their article counts.
func (c *Client) ListFeeds(ctx context.Context) ([]Feed, error) {
	var res struct {
		Feeds []Feed `json:"feeds"`
	}
	err := c.getJSON(ctx, "/feeds", &res)
	return res.Feeds, err
}

// GetStats returns overall system statistics.
func (c *Client) GetStats(ctx context.Context) (Stats, error) {
	var s Stats
	err := c.getJSON(ctx, "/stats", &s)
	return s, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/apiclient/ -v`
Expected: PASS (all apiclient tests).

- [ ] **Step 5: Commit**

```bash
git add internal/apiclient/
git commit -m "feat(frontend): apiclient GetArticle, ListFeeds, GetStats"
```

---

### Task 6: Templates

**Files:**
- Create: `internal/server/templates/partials.html`
- Create: `internal/server/templates/list.html`
- Create: `internal/server/templates/article.html`
- Create: `internal/server/templates/stats.html`
- Create: `internal/server/templates/error.html`
- Create: `internal/server/templates/notfound.html`

These are exercised by tests in Task 7/8 (rendering must succeed and contain expected content). No standalone test here.

- [ ] **Step 1: Create `partials.html`**

```html
{{ define "header" }}<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ .Title }} · Information Broker</title>
  <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-slate-50 text-slate-800 min-h-screen">
  <header class="bg-slate-900 text-white">
    <div class="max-w-5xl mx-auto px-4 py-4 flex items-center justify-between">
      <a href="/" class="text-lg font-semibold tracking-tight">🛰️ Information Broker</a>
      <nav class="space-x-4 text-sm">
        <a href="/" class="hover:underline">Articles</a>
        <a href="/stats" class="hover:underline">Stats</a>
      </nav>
    </div>
  </header>
  <main class="max-w-5xl mx-auto px-4 py-8">
{{ end }}

{{ define "footer" }}
  </main>
  <footer class="max-w-5xl mx-auto px-4 py-8 text-xs text-slate-400">
    Information Broker frontend
  </footer>
</body>
</html>
{{ end }}
```

- [ ] **Step 2: Create `list.html`**

```html
{{ define "list" }}{{ template "header" . }}
<form method="get" action="/" class="mb-6 flex flex-wrap gap-3 items-end">
  <div class="flex-1 min-w-[12rem]">
    <label class="block text-xs font-medium text-slate-500 mb-1">Search</label>
    <input type="text" name="q" value="{{ .Q }}" placeholder="keyword in title or summary"
      class="w-full rounded border border-slate-300 px-3 py-2 text-sm">
  </div>
  <div>
    <label class="block text-xs font-medium text-slate-500 mb-1">Feed</label>
    <select name="feed" class="rounded border border-slate-300 px-3 py-2 text-sm max-w-[16rem]">
      <option value="">All feeds</option>
      {{ range .Feeds }}
      <option value="{{ .FeedURL }}" {{ if eq .FeedURL $.Feed }}selected{{ end }}>{{ .FeedURL }} ({{ .ArticleCount }})</option>
      {{ end }}
    </select>
  </div>
  <button type="submit" class="rounded bg-slate-900 text-white px-4 py-2 text-sm">Apply</button>
</form>

{{ if not .Articles }}
  <p class="text-slate-500">No articles found.</p>
{{ end }}

<ul class="space-y-4">
  {{ range .Articles }}
  <li class="bg-white rounded-lg shadow-sm border border-slate-200 p-5">
    <div class="flex items-center justify-between text-xs text-slate-400 mb-1">
      <span>{{ .FeedURL }}</span>
      <span>{{ .PublishedAt | formatDate }}</span>
    </div>
    <a href="/article/{{ .ID }}" class="text-lg font-semibold text-slate-900 hover:text-indigo-600">{{ .Title }}</a>
    <p class="mt-2 text-sm text-slate-600 leading-relaxed">
      {{ if .Summary }}{{ .Summary }}{{ else }}No summary available.{{ end }}
    </p>
    <a href="/article/{{ .ID }}" class="inline-block mt-3 text-sm text-indigo-600 hover:underline">Read full article →</a>
  </li>
  {{ end }}
</ul>

<div class="mt-8 flex justify-between text-sm">
  {{ if .HasPrev }}<a href="?page={{ dec .Page }}&q={{ .Q }}&feed={{ .Feed }}" class="text-indigo-600 hover:underline">← Previous</a>{{ else }}<span></span>{{ end }}
  <span class="text-slate-400">Page {{ .Page }}</span>
  {{ if .HasNext }}<a href="?page={{ inc .Page }}&q={{ .Q }}&feed={{ .Feed }}" class="text-indigo-600 hover:underline">Next →</a>{{ else }}<span></span>{{ end }}
</div>
{{ template "footer" . }}{{ end }}
```

- [ ] **Step 3: Create `article.html`**

```html
{{ define "article" }}{{ template "header" . }}
<a href="/" class="text-sm text-indigo-600 hover:underline">← Back to articles</a>
{{ with .Article }}
<article class="bg-white rounded-lg shadow-sm border border-slate-200 p-6 mt-4">
  <div class="flex items-center justify-between text-xs text-slate-400 mb-2">
    <span>{{ .FeedURL }}</span>
    <span>{{ .PublishedAt | formatDate }}</span>
  </div>
  <h1 class="text-2xl font-bold text-slate-900">{{ .Title }}</h1>

  <section class="mt-4 rounded-md bg-indigo-50 border border-indigo-100 p-4">
    <h2 class="text-xs font-semibold uppercase tracking-wide text-indigo-700 mb-1">Summary</h2>
    <p class="text-sm text-slate-700 leading-relaxed">{{ if .Summary }}{{ .Summary }}{{ else }}No summary available.{{ end }}</p>
  </section>

  <section class="mt-6">
    <h2 class="text-xs font-semibold uppercase tracking-wide text-slate-500 mb-2">Full text</h2>
    <div class="prose prose-sm max-w-none whitespace-pre-wrap text-slate-700 leading-relaxed">{{ .Content }}</div>
  </section>

  <div class="mt-6 pt-4 border-t border-slate-100">
    <a href="{{ .URL }}" target="_blank" rel="noopener noreferrer"
      class="inline-block rounded bg-slate-900 text-white px-4 py-2 text-sm">Read original →</a>
    <p class="mt-2 text-xs text-slate-400 break-all">Source: {{ .URL }}</p>
  </div>
</article>
{{ end }}
{{ template "footer" . }}{{ end }}
```

- [ ] **Step 4: Create `stats.html`**

```html
{{ define "stats" }}{{ template "header" . }}
<h1 class="text-2xl font-bold text-slate-900 mb-6">System statistics</h1>
{{ with .Stats }}
<div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
  <div class="bg-white rounded-lg border border-slate-200 p-5">
    <div class="text-xs uppercase tracking-wide text-slate-400">Total articles</div>
    <div class="text-3xl font-bold text-slate-900 mt-1">{{ .TotalArticles }}</div>
  </div>
  <div class="bg-white rounded-lg border border-slate-200 p-5">
    <div class="text-xs uppercase tracking-wide text-slate-400">Feeds</div>
    <div class="text-3xl font-bold text-slate-900 mt-1">{{ .TotalFeeds }}</div>
  </div>
  <div class="bg-white rounded-lg border border-slate-200 p-5">
    <div class="text-xs uppercase tracking-wide text-slate-400">Last fetch</div>
    <div class="text-lg font-semibold text-slate-900 mt-2">{{ if .LastFetch }}{{ .LastFetch | formatDate }}{{ else }}—{{ end }}</div>
  </div>
</div>
{{ end }}
{{ template "footer" . }}{{ end }}
```

- [ ] **Step 5: Create `error.html`**

```html
{{ define "error" }}{{ template "header" . }}
<div class="bg-white rounded-lg border border-red-200 p-8 text-center">
  <h1 class="text-xl font-bold text-red-700">Something went wrong</h1>
  <p class="mt-2 text-slate-600">{{ .Message }}</p>
  <a href="/" class="inline-block mt-4 text-indigo-600 hover:underline">← Back to articles</a>
</div>
{{ template "footer" . }}{{ end }}
```

- [ ] **Step 6: Create `notfound.html`**

```html
{{ define "notfound" }}{{ template "header" . }}
<div class="bg-white rounded-lg border border-slate-200 p-8 text-center">
  <h1 class="text-xl font-bold text-slate-900">Article not found</h1>
  <p class="mt-2 text-slate-600">We couldn't find that article.</p>
  <a href="/" class="inline-block mt-4 text-indigo-600 hover:underline">← Back to articles</a>
</div>
{{ template "footer" . }}{{ end }}
```

- [ ] **Step 7: Commit**

```bash
git add internal/server/templates/
git commit -m "feat(frontend): add HTML templates"
```

---

### Task 7: Server — `ArticleService` interface, `New`, `render`, list page + healthz

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/handlers.go`
- Test: `internal/server/server_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/server/server_test.go`:

```go
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"smellyfeet/internal/apiclient"
)

// stubService implements ArticleService for tests.
type stubService struct {
	list     apiclient.ListResult
	article  apiclient.Article
	feeds    []apiclient.Feed
	stats    apiclient.Stats
	listErr  error
	getErr   error
	feedsErr error
	statsErr error
}

func (s stubService) ListArticles(ctx context.Context, p apiclient.ListParams) (apiclient.ListResult, error) {
	return s.list, s.listErr
}
func (s stubService) GetArticle(ctx context.Context, id int64) (apiclient.Article, error) {
	return s.article, s.getErr
}
func (s stubService) ListFeeds(ctx context.Context) ([]apiclient.Feed, error) {
	return s.feeds, s.feedsErr
}
func (s stubService) GetStats(ctx context.Context) (apiclient.Stats, error) {
	return s.stats, s.statsErr
}

func newTestServer(t *testing.T, svc ArticleService) http.Handler {
	t.Helper()
	srv, err := New(svc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv.Routes()
}

func TestHealthz(t *testing.T) {
	h := newTestServer(t, stubService{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("healthz = %d %q", rec.Code, rec.Body.String())
	}
}

func TestListPageRendersSummary(t *testing.T) {
	sum := "A concise summary."
	svc := stubService{list: apiclient.ListResult{
		Articles: []apiclient.Article{{ID: 7, Title: "Big Breach", Summary: &sum, FeedURL: "https://a/rss"}},
		Count:    1,
	}}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Big Breach") || !strings.Contains(body, "A concise summary.") {
		t.Fatalf("list body missing content: %s", body)
	}
	if !strings.Contains(body, `href="/article/7"`) {
		t.Fatalf("list body missing detail link: %s", body)
	}
}

func TestListPageAPIErrorRendersErrorPage(t *testing.T) {
	svc := stubService{listErr: context.DeadlineExceeded}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Fatalf("expected error page, got: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -v`
Expected: FAIL — `undefined: New` / `undefined: ArticleService`.

- [ ] **Step 3: Implement `server.go`**

Create `internal/server/server.go`:

```go
// Package server renders the Information-Broker frontend.
package server

import (
	"bytes"
	"context"
	"embed"
	"html/template"
	"log"
	"net/http"

	"smellyfeet/internal/apiclient"
)

//go:embed templates/*.html
var templatesFS embed.FS

// ArticleService is the subset of the API the handlers need.
type ArticleService interface {
	ListArticles(ctx context.Context, p apiclient.ListParams) (apiclient.ListResult, error)
	GetArticle(ctx context.Context, id int64) (apiclient.Article, error)
	ListFeeds(ctx context.Context) ([]apiclient.Feed, error)
	GetStats(ctx context.Context) (apiclient.Stats, error)
}

// Server holds dependencies for the HTTP handlers.
type Server struct {
	svc      ArticleService
	tmpl     *template.Template
	pageSize int
}

var funcMap = template.FuncMap{
	"formatDate": func(t any) string {
		switch v := t.(type) {
		case nil:
			return "—"
		default:
			if tt, ok := asTime(v); ok {
				return tt.Format("2006-01-02 15:04")
			}
			return ""
		}
	},
	"inc": func(n int) int { return n + 1 },
	"dec": func(n int) int { return n - 1 },
}

// New constructs a Server with parsed templates.
func New(svc ArticleService) (*Server, error) {
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{svc: svc, tmpl: tmpl, pageSize: 20}, nil
}

// Routes returns the configured HTTP handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /{$}", s.handleList)
	mux.HandleFunc("GET /article/{id}", s.handleArticle)
	mux.HandleFunc("GET /stats", s.handleStats)
	return mux
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("template error (%s): %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func (s *Server) renderError(w http.ResponseWriter, err error) {
	log.Printf("handler error: %v", err)
	s.render(w, http.StatusBadGateway, "error", map[string]any{
		"Title":   "Error",
		"Message": "The article service is currently unavailable. Please try again later.",
	})
}
```

- [ ] **Step 4: Implement `handlers.go` (healthz + list + asTime + parsePage)**

Create `internal/server/handlers.go`:

```go
package server

import (
	"net/http"
	"strconv"
	"time"

	"smellyfeet/internal/apiclient"
)

// asTime normalizes time.Time and *time.Time for the formatDate func.
func asTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}
		return *t, true
	}
	return time.Time{}, false
}

func parsePage(s string) int {
	if s == "" {
		return 1
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

type listView struct {
	Title    string
	Articles []apiclient.Article
	Feeds    []apiclient.Feed
	Q        string
	Feed     string
	Page     int
	HasPrev  bool
	HasNext  bool
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := parsePage(r.URL.Query().Get("page"))
	q := r.URL.Query().Get("q")
	feed := r.URL.Query().Get("feed")

	res, err := s.svc.ListArticles(ctx, apiclient.ListParams{
		Limit:  s.pageSize,
		Offset: (page - 1) * s.pageSize,
		Feed:   feed,
		Q:      q,
	})
	if err != nil {
		s.renderError(w, err)
		return
	}

	feeds, err := s.svc.ListFeeds(ctx)
	if err != nil {
		feeds = nil // non-fatal: filter dropdown simply shows "All feeds"
	}

	s.render(w, http.StatusOK, "list", listView{
		Title:    "Articles",
		Articles: res.Articles,
		Feeds:    feeds,
		Q:        q,
		Feed:     feed,
		Page:     page,
		HasPrev:  page > 1,
		HasNext:  len(res.Articles) == s.pageSize,
	})
}
```

- [ ] **Step 5: Add temporary stubs so the package compiles**

`Routes()` references `handleArticle` and `handleStats`, implemented in Task 8. Add these temporary stubs to the bottom of `handlers.go` so the package builds and Task 7 tests run:

```go
func (s *Server) handleArticle(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS — `TestHealthz`, `TestListPageRendersSummary`, `TestListPageAPIErrorRendersErrorPage`.

- [ ] **Step 7: Commit**

```bash
git add internal/server/server.go internal/server/handlers.go internal/server/server_test.go
git commit -m "feat(frontend): server scaffolding, list page, healthz"
```

---

### Task 8: Server — article detail + stats handlers

**Files:**
- Modify: `internal/server/handlers.go` (replace the two stubs with real implementations)
- Modify: `internal/server/server_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Append to `internal/server/server_test.go`:

```go
func TestArticlePageRendersFullText(t *testing.T) {
	sum := "Short summary."
	svc := stubService{article: apiclient.Article{
		ID: 7, Title: "Deep Dive", Summary: &sum,
		Content: "The complete article body text.",
		URL:     "https://source.example/post",
		FeedURL: "https://a/rss",
	}}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/article/7", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Deep Dive", "Short summary.", "The complete article body text.", "https://source.example/post", "Read original"} {
		if !strings.Contains(body, want) {
			t.Fatalf("article body missing %q: %s", want, body)
		}
	}
}

func TestArticlePageNotFound(t *testing.T) {
	svc := stubService{getErr: apiclient.ErrNotFound}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/article/999", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Article not found") {
		t.Fatalf("expected notfound page: %s", rec.Body.String())
	}
}

func TestArticlePageInvalidID(t *testing.T) {
	h := newTestServer(t, stubService{})
	req := httptest.NewRequest(http.MethodGet, "/article/abc", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestStatsPage(t *testing.T) {
	svc := stubService{stats: apiclient.Stats{TotalArticles: 100, TotalFeeds: 12}}
	h := newTestServer(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "100") || !strings.Contains(body, "12") {
		t.Fatalf("stats body missing numbers: %s", body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run 'TestArticlePage|TestStatsPage' -v`
Expected: FAIL — article/stats handlers return 501, assertions fail.

- [ ] **Step 3: Replace the stubs with real handlers**

In `internal/server/handlers.go`, delete the two temporary stub functions from Task 7 Step 5 and add these. Also add `"errors"` to the file's import block:

```go
func (s *Server) handleArticle(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		s.render(w, http.StatusNotFound, "notfound", map[string]any{"Title": "Not Found"})
		return
	}

	a, err := s.svc.GetArticle(r.Context(), id)
	if errors.Is(err, apiclient.ErrNotFound) {
		s.render(w, http.StatusNotFound, "notfound", map[string]any{"Title": "Not Found"})
		return
	}
	if err != nil {
		s.renderError(w, err)
		return
	}

	s.render(w, http.StatusOK, "article", map[string]any{
		"Title":   a.Title,
		"Article": a,
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.svc.GetStats(r.Context())
	if err != nil {
		s.renderError(w, err)
		return
	}
	s.render(w, http.StatusOK, "stats", map[string]any{
		"Title": "Statistics",
		"Stats": st,
	})
}
```

The final import block of `handlers.go` must be:

```go
import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"smellyfeet/internal/apiclient"
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS — all server tests including the new four.

- [ ] **Step 5: Commit**

```bash
git add internal/server/handlers.go internal/server/server_test.go
git commit -m "feat(frontend): article detail and stats pages"
```

---

### Task 9: Wire `main.go` + `.env.example` + full build/run verification

**Files:**
- Create: `main.go`
- Create: `.env.example`

- [ ] **Step 1: Create `main.go`**

```go
package main

import (
	"log"
	"net/http"

	"smellyfeet/internal/apiclient"
	"smellyfeet/internal/config"
	"smellyfeet/internal/server"
)

func main() {
	cfg := config.Load()
	client := apiclient.New(cfg.APIBaseURL)

	srv, err := server.New(client)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	addr := ":" + cfg.Port
	log.Printf("frontend listening on %s (API: %s)", addr, cfg.APIBaseURL)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Create `.env.example`**

```bash
# Base URL of the Information-Broker API.
# Local: http://localhost:8080   Prod: http://192.168.1.135:8080
API_BASE_URL=http://localhost:8080

# Port the frontend listens on.
PORT=3000
```

- [ ] **Step 3: Build everything and run the full test suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: build succeeds, vet clean, all packages PASS.

- [ ] **Step 4: Smoke-test against the production API**

Run:
```bash
API_BASE_URL=http://192.168.1.135:8080 PORT=3000 go run . &
sleep 2
curl -s -o /dev/null -w "healthz=%{http_code}\n" http://localhost:3000/healthz
curl -s -o /dev/null -w "list=%{http_code}\n" http://localhost:3000/
curl -s http://localhost:3000/ | grep -o "Read full article" | head -1
kill %1
```
Expected: `healthz=200`, `list=200`, and at least one "Read full article" link (assuming the prod DB has articles). If the prod API is unreachable, the list page returns `502` and renders the error page — note that and continue.

- [ ] **Step 5: Commit**

```bash
git add main.go .env.example
git commit -m "feat(frontend): wire main entrypoint and env example"
```

---

### Task 10: README + .gitignore

**Files:**
- Create: `README.md`
- Create: `.gitignore`

- [ ] **Step 1: Create `.gitignore`**

```gitignore
/bin/
*.exe
.env
```

- [ ] **Step 2: Create `README.md`**

```markdown
# SmellyFeet — Information Broker Frontend

A server-rendered Go web frontend for [Information-Broker](https://github.com/PureCypher/Information-Broker).
Browse AI-generated summaries of cybersecurity articles, read the full stored text, and jump to the
original source. Supports search, feed filtering, pagination, and a stats page.

## Run

```bash
cp .env.example .env   # edit API_BASE_URL / PORT as needed
go run .
```

Then open http://localhost:3000.

## Configuration

| Variable       | Default                  | Description                          |
|----------------|--------------------------|--------------------------------------|
| `API_BASE_URL` | `http://localhost:8080`  | Base URL of the Information-Broker API |
| `PORT`         | `3000`                   | Port the frontend listens on         |

## Architecture

Talks to the Information-Broker HTTP API only (no direct DB). `internal/apiclient` wraps the API,
`internal/server` renders `html/template` pages styled with Tailwind (CDN). See
`docs/superpowers/specs/2026-06-05-information-broker-frontend-design.md`.

## Test

```bash
go test ./...
```
```

- [ ] **Step 3: Verify build still clean**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add README.md .gitignore
git commit -m "docs: add README and gitignore"
```

---

## Self-review notes

- **Spec coverage:** list+summary (Task 7), full-text detail + source (Task 8), search (Tasks 1/4), feed filter (Tasks 4/5/7), pagination (Task 7), stats (Task 8), error/404 handling (Tasks 7/8), API extension for `id`/`summary`/single-article (Tasks 1/2), config (Task 3), testing throughout. All spec sections map to tasks.
- **Backend `/articles/latest`:** the spec mentioned adding `id`/`summary` there too, but the frontend never calls it and its query has a pre-existing `fetched_at` column reference; left untouched to avoid scope creep and an unrelated bugfix. `/articles` is the endpoint the list page uses.
- **Type consistency:** `ArticleView` (backend) and `apiclient.Article` (frontend) share the same JSON keys; `ArticleService` method signatures match `apiclient.Client` methods exactly; template func names (`formatDate`, `inc`, `dec`) are all registered in `funcMap`.
- **Feed identifier:** the filter uses `feed_url` (raw RSS URL) as both value and label, matching what the `/feeds` API returns. Friendlier display names are out of scope for v1.
```
