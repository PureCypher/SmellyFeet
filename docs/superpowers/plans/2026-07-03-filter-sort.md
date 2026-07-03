# Filtering & Sort Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Newest/oldest sorting end-to-end (backend param + UI select), collapsed "upcoming" section for future-dated articles, clickable source pills, removable filter chips.

**Architecture:** Backend: whitelist `sort` param in `buildArticlesQuery` (Information-Broker repo). Frontend: `Sort` in ListParams, `splitUpcoming` pure func in the list handler, template restructure to a shared `articleCard` with the stretched-link pattern (valid HTML for nested-looking links), chips/select/upcoming markup, one CSS rebuild.

**Tech Stack:** Go 1.22 stdlib in both repos; Tailwind standalone via `./scripts/build-css.sh` (SmellyFeet only).

**Spec:** `docs/superpowers/specs/2026-07-03-filter-sort-design.md`

## Global Constraints

- Both repos Go stdlib only; gofmt-clean; `-race` green; commit `<type>: <description>`, no attribution trailers, no `--no-verify`. Do NOT push — the controller pushes after review.
- SmellyFeet: zero JS; NO inline `style=""` (CSP `style-src 'self'`); tests reuse `stubService`/`newTestServer`/`getPath`; CSS rebuilt in Task 3 only.
- Backend repo path: `/home/pure/Documents/github/Information-Broker`. Frontend repo path: `/home/pure/Documents/github/SmellyFeet`.
- Sort contract everywhere: the string `"oldest"` means ascending; empty string (or anything else) means newest-first default.
- html/template's built-in `urlquery` function is used for URL escaping in hrefs — no new Go funcs for that.

---

### Task 1: Backend sort param (Information-Broker repo)

**Files (all in /home/pure/Documents/github/Information-Broker):**
- Modify: `api.go` (`buildArticlesQuery` signature + `getArticles` call site)
- Test: `api_query_test.go`

**Interfaces:**
- Produces: `buildArticlesQuery(feed, q, sort string, limit, offset int) (string, []interface{})`; `GET /articles?sort=oldest` returns ascending publish_date. SmellyFeet's client (Task 2) sends `sort=oldest`.

- [ ] **Step 1: Extend the test table** — in `api_query_test.go`, update every existing `buildArticlesQuery(...)` call to insert `""` as the new third argument (e.g. `buildArticlesQuery("", "", "", 50, 0)`), then add two subtests inside `TestBuildArticlesQuery`:

```go
	t.Run("sort oldest", func(t *testing.T) {
		q, _ := buildArticlesQuery("", "", "oldest", 50, 0)
		if !strings.Contains(q, "ORDER BY publish_date ASC") {
			t.Fatalf("expected ASC order: %s", q)
		}
	})

	t.Run("unknown sort falls back to newest", func(t *testing.T) {
		q, _ := buildArticlesQuery("", "", "garbage'; DROP TABLE articles;--", 50, 0)
		if !strings.Contains(q, "ORDER BY publish_date DESC") {
			t.Fatalf("expected DESC fallback: %s", q)
		}
		if strings.Contains(q, "DROP TABLE") {
			t.Fatalf("sort value leaked into SQL: %s", q)
		}
	})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /home/pure/Documents/github/Information-Broker && go test -run TestBuildArticlesQuery ./...`
Expected: FAIL — compile error (wrong arg count) until the signature changes.

- [ ] **Step 3: Implement in `api.go`**

Change the signature of `buildArticlesQuery` (currently `func buildArticlesQuery(feed, q string, limit, offset int)` around line 101):

```go
func buildArticlesQuery(feed, q, sort string, limit, offset int) (string, []interface{}) {
```

and replace the final query line:

```go
	order := "DESC"
	if sort == "oldest" {
		order = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY publish_date %s LIMIT $%d OFFSET $%d", order, i, i+1)
```

In `getArticles` (call site ~line 156), change:

```go
	query, args := buildArticlesQuery(feedURL, searchQ, r.URL.Query().Get("sort"), limit, offset)
```

- [ ] **Step 4: Run tests**

Run: `go build ./... && go test ./...`
Expected: build clean; `TestBuildArticlesQuery` PASS including new subtests; the rest of the suite unchanged (report any pre-existing failures without fixing them — they are out of scope).

- [ ] **Step 5: Commit (in the Information-Broker repo)**

```bash
git add api.go api_query_test.go
git commit -m "feat(api): optional sort=oldest parameter on /articles"
```

---

### Task 2: Frontend Go — Sort param, splitUpcoming, captured stub

**Files (all in /home/pure/Documents/github/SmellyFeet):**
- Modify: `internal/apiclient/apiclient.go` (ListParams.Sort + encoding)
- Modify: `internal/server/handlers.go` (sort parse, splitUpcoming, listView fields)
- Modify: `internal/server/server_test.go` (stubService captures ListParams)
- Test: `internal/apiclient/apiclient_test.go` (append), `internal/server/filter_test.go` (new)

**Interfaces:**
- Consumes: backend contract `sort=oldest`.
- Produces: `ListParams.Sort string`; `splitUpcoming(articles []apiclient.Article, now time.Time) (upcoming, current []apiclient.Article)`; `listView.Sort string` + `listView.Upcoming []apiclient.Article`; `stubService.lastList *apiclient.ListParams` (captures the params the handler sent). Task 3 templates read `.Sort`/`.Upcoming`.

- [ ] **Step 1: Failing tests.** Append to `internal/apiclient/apiclient_test.go` (match the file's existing httptest style — it already asserts request paths):

```go
func TestListArticlesSortParam(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"articles":[],"count":0,"limit":0,"offset":0}`))
	}))
	defer srv.Close()
	c := New(srv.URL)

	if _, err := c.ListArticles(context.Background(), ListParams{Sort: "oldest"}); err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if !strings.Contains(gotQuery, "sort=oldest") {
		t.Fatalf("query %q missing sort=oldest", gotQuery)
	}

	if _, err := c.ListArticles(context.Background(), ListParams{}); err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if strings.Contains(gotQuery, "sort=") {
		t.Fatalf("query %q should not contain sort when unset", gotQuery)
	}
}
```

Create `internal/server/filter_test.go`:

```go
package server

import (
	"strings"
	"testing"
	"time"

	"smellyfeet/internal/apiclient"
)

func TestSplitUpcoming(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	past := apiclient.Article{ID: 1, PublishedAt: now.Add(-time.Hour)}
	exact := apiclient.Article{ID: 2, PublishedAt: now}
	future := apiclient.Article{ID: 3, PublishedAt: now.Add(time.Hour)}
	zero := apiclient.Article{ID: 4}

	up, cur := splitUpcoming([]apiclient.Article{future, past, exact, zero}, now)
	if len(up) != 1 || up[0].ID != 3 {
		t.Fatalf("upcoming = %v, want only ID 3", up)
	}
	if len(cur) != 3 || cur[0].ID != 1 || cur[1].ID != 2 || cur[2].ID != 4 {
		t.Fatalf("current = %v, want IDs 1,2,4 in order", cur)
	}
}

func TestHandleListSortAndSplit(t *testing.T) {
	future := apiclient.Article{ID: 9, Title: "Future webinar", PublishedAt: time.Now().Add(48 * time.Hour)}
	past := apiclient.Article{ID: 8, Title: "Past news", PublishedAt: time.Now().Add(-time.Hour)}

	t.Run("sort passed through and normalized", func(t *testing.T) {
		var got apiclient.ListParams
		h := newTestServer(t, stubService{lastList: &got})
		getPath(t, h, "/?sort=oldest")
		if got.Sort != "oldest" {
			t.Fatalf("Sort sent = %q, want oldest", got.Sort)
		}
		getPath(t, h, "/?sort=garbage")
		if got.Sort != "" {
			t.Fatalf("Sort sent = %q, want empty for unknown value", got.Sort)
		}
	})

	t.Run("page 1 newest splits upcoming", func(t *testing.T) {
		svc := stubService{list: apiclient.ListResult{Articles: []apiclient.Article{future, past}}}
		body := getPath(t, newTestServer(t, svc), "/").Body.String()
		if !containsAll(body, "upcoming (1)", "Future webinar", "Past news") {
			t.Fatalf("page 1 should split upcoming; body missing expected markers")
		}
	})

	t.Run("page 2 does not split", func(t *testing.T) {
		svc := stubService{list: apiclient.ListResult{Articles: []apiclient.Article{future, past}}}
		body := getPath(t, newTestServer(t, svc), "/?page=2").Body.String()
		if strings.Contains(body, "upcoming (") {
			t.Fatal("page 2 must not render the upcoming section")
		}
	})

	t.Run("oldest sort does not split", func(t *testing.T) {
		svc := stubService{list: apiclient.ListResult{Articles: []apiclient.Article{future, past}}}
		body := getPath(t, newTestServer(t, svc), "/?sort=oldest").Body.String()
		if strings.Contains(body, "upcoming (") {
			t.Fatal("oldest sort must not render the upcoming section")
		}
	})
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
```

NOTE: the two render subtests — "page 1 newest splits upcoming" and the chips-dependent behavior — will only fully pass after Task 3 adds the template markup; write them now, and it is acceptable for exactly those to still FAIL at the end of this task. State this explicitly in your report. The sort-normalization subtest, the negative render subtests, and all non-template tests must pass.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/... -run 'TestListArticlesSort|TestSplitUpcoming|TestHandleListSort' -v`
Expected: FAIL — compile errors (`Sort` undefined, `splitUpcoming` undefined, `lastList` undefined).

- [ ] **Step 3: Implement.**

`internal/apiclient/apiclient.go` — `ListParams` gains `Sort string` (comment: `// "" = newest first (default), "oldest" = ascending`); in `ListArticles` add:

```go
	if p.Sort == "oldest" {
		v.Set("sort", p.Sort)
	}
```

`internal/server/server_test.go` — `stubService` gains field `lastList *apiclient.ListParams`, and `ListArticles` becomes:

```go
func (s stubService) ListArticles(ctx context.Context, p apiclient.ListParams) (apiclient.ListResult, error) {
	if s.lastList != nil {
		*s.lastList = p
	}
	return s.list, s.listErr
}
```

`internal/server/handlers.go` — add below `parsePage`:

```go
// splitUpcoming partitions future-dated articles (upcoming webinars/events)
// from the current stream, preserving order within each group.
func splitUpcoming(articles []apiclient.Article, now time.Time) (upcoming, current []apiclient.Article) {
	for _, a := range articles {
		if a.PublishedAt.After(now) {
			upcoming = append(upcoming, a)
		} else {
			current = append(current, a)
		}
	}
	return upcoming, current
}
```

`listView` gains `Sort string` and `Upcoming []apiclient.Article`. `handleList` becomes:

```go
func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := parsePage(r.URL.Query().Get("page"))
	q := r.URL.Query().Get("q")
	feed := r.URL.Query().Get("feed")
	sort := r.URL.Query().Get("sort")
	if sort != "oldest" {
		sort = "" // whitelist: only "oldest" changes the order
	}

	res, err := s.svc.ListArticles(ctx, apiclient.ListParams{
		Limit:  s.pageSize,
		Offset: (page - 1) * s.pageSize,
		Feed:   feed,
		Q:      q,
		Sort:   sort,
	})
	if err != nil {
		s.renderError(w, err)
		return
	}

	feeds, err := s.svc.ListFeeds(ctx)
	if err != nil {
		feeds = nil // non-fatal: filter dropdown simply shows "All feeds"
	}

	var upcoming []apiclient.Article
	current := res.Articles
	if sort == "" && page == 1 {
		upcoming, current = splitUpcoming(res.Articles, time.Now())
	}

	setCache(w, cacheList)
	s.render(w, http.StatusOK, "list", listView{
		Title:    "Articles",
		Desc:     "AI-summarized cybersecurity intelligence — the latest articles from monitored threat feeds.",
		OG:       true,
		Articles: current,
		Upcoming: upcoming,
		Feeds:    feeds,
		Q:        q,
		Feed:     feed,
		Sort:     sort,
		Page:     page,
		HasPrev:  page > 1,
		HasNext:  len(res.Articles) == s.pageSize,
	})
}
```

(Keep the existing `Desc`/`OG`/`OGArticle` handling exactly as in the current file — only add the new fields. `HasNext` stays based on `res.Articles`, the full fetched page.)

- [ ] **Step 4: Run tests**

Run: `go test ./... -race`
Expected: everything passes EXCEPT the Task-3-dependent render subtests named in Step 1. Report exact pass/fail lists.

- [ ] **Step 5: Commit**

```bash
git add internal/
git commit -m "feat(frontend): sort param plumbing and upcoming/current split"
```

---

### Task 3: Templates — card partial, sort select, chips, upcoming; CSS rebuild

**Files (SmellyFeet):**
- Modify: `internal/server/templates/list.html` (full restructure below)
- Modify: `internal/server/templates/article.html` (pill becomes a link)
- Modify: `internal/server/ui_test.go` (update two assertions — pill markup changed from span to anchor)
- Test: `internal/server/filter_ui_test.go` (new)
- Regenerate: `internal/server/static/app.css`

**Interfaces:**
- Consumes: `.Sort`, `.Upcoming`, funcs `sourceName`/`relTime`/`cveID`/`formatDate`, built-in `urlquery`.
- Produces: `{{ define "articleCard" }}` template; final UI.

- [ ] **Step 1: Failing tests.** Create `internal/server/filter_ui_test.go`:

```go
package server

import (
	"strings"
	"testing"

	"smellyfeet/internal/apiclient"
)

func TestSortSelectAndPaginationCarrySort(t *testing.T) {
	arts := make([]apiclient.Article, 20)
	for i := range arts {
		arts[i] = apiclient.Article{ID: int64(i + 1), Title: "A"}
	}
	svc := stubService{list: apiclient.ListResult{Articles: arts}}
	body := getPath(t, newTestServer(t, svc), "/?sort=oldest").Body.String()
	if !strings.Contains(body, `<option value="oldest" selected>`) {
		t.Error("sort select missing selected oldest option")
	}
	if !strings.Contains(body, "sort=oldest") || !strings.Contains(body, "page=2") {
		t.Error("pagination must carry sort=oldest")
	}
}

func TestFilterChips(t *testing.T) {
	svc := stubService{}
	body := getPath(t, newTestServer(t, svc), "/?q=keycloak&feed=https%3A%2F%2Fx.example%2Frss&sort=oldest").Body.String()
	for _, want := range []string{
		"source: x.example", "search: “keycloak”", "oldest first", "clear all",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("chips missing %q", want)
		}
	}
	if !strings.Contains(body, `href="/?q=keycloak&amp;sort=oldest"`) {
		t.Error("source chip removal href should keep q and sort, drop feed")
	}
}

func TestNoChipsWhenUnfiltered(t *testing.T) {
	body := getPath(t, newTestServer(t, stubService{}), "/").Body.String()
	if strings.Contains(body, "clear all") {
		t.Error("chips bar should not render without active filters")
	}
}

func TestSourcePillIsFilterLink(t *testing.T) {
	svc := stubService{list: apiclient.ListResult{Articles: []apiclient.Article{{
		ID: 1, Title: "T", FeedURL: "https://www.brighttalk.com/channel/7451/feed/rss",
	}}}}
	body := getPath(t, newTestServer(t, svc), "/").Body.String()
	if !strings.Contains(body, `href="/?feed=https%3A%2F%2Fwww.brighttalk.com%2Fchannel%2F7451%2Ffeed%2Frss"`) {
		t.Error("card pill should link to feed filter with escaped URL")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/server/ -run 'TestSortSelect|TestFilterChips|TestNoChips|TestSourcePill|TestHandleListSortAndSplit' -v`
Expected: FAIL on the new assertions (markup absent) plus the carried-over Task 2 subtests.

- [ ] **Step 3: Rewrite `internal/server/templates/list.html`** — full new content:

```html
{{ define "articleCard" }}
<li class="reveal">
  <div class="group relative overflow-hidden bg-ink-900 border border-line rounded-xl p-5 hover:border-accent/50 hover:bg-ink-850 transition-colors">
    <span class="absolute left-0 top-0 h-full w-[2px] bg-accent origin-top scale-y-0 group-hover:scale-y-100 transition-transform duration-300"></span>
    <div class="flex items-center justify-between font-mono text-[11px] mb-2 gap-3 [overflow-wrap:anywhere]">
      <span class="flex items-center gap-2 min-w-0">
        <a href="/?feed={{ .FeedURL | urlquery }}" title="Filter by {{ sourceName .FeedURL }}" class="relative z-10 truncate rounded border border-line bg-ink-950 px-2 py-0.5 text-accent-dim hover:text-accent hover:border-accent/60 transition-colors">{{ sourceName .FeedURL }}</a>
        {{ with cveID .Title }}<span class="shrink-0 rounded border border-accent/40 bg-accent/10 px-2 py-0.5 text-accent">{{ . }}</span>{{ end }}
      </span>
      <time class="shrink-0 text-fog" datetime="{{ .PublishedAt.Format "2006-01-02T15:04:05Z07:00" }}" title="{{ .PublishedAt | formatDate }}">{{ .PublishedAt | relTime }}</time>
    </div>
    <h2 class="text-lg font-semibold text-zinc-100 leading-snug group-hover:text-accent transition-colors"><a href="/article/{{ .ID }}" class="after:absolute after:inset-0">{{ .Title }}</a></h2>
    <p class="mt-2 text-sm text-zinc-400 leading-relaxed line-clamp-3">
      {{ if .Summary }}{{ .Summary }}{{ else }}<span class="text-zinc-600 italic">No summary available.</span>{{ end }}
    </p>
    <span aria-hidden="true" class="absolute right-5 bottom-4 font-mono text-accent opacity-0 group-hover:opacity-100 transition-opacity">→</span>
  </div>
</li>
{{ end }}

{{ define "list" }}{{ template "header" . }}
<div class="mb-7">
  <p class="font-mono text-[11px] uppercase tracking-[0.25em] text-accent/80 mb-1.5">// live feed</p>
  <h1 class="text-2xl font-semibold text-zinc-100 tracking-tight">Latest intelligence</h1>
</div>

<form method="get" action="/" class="mb-5 flex flex-wrap gap-3 items-end bg-ink-900 border border-line rounded-xl p-4">
  <div class="flex-1 min-w-[12rem]">
    <label for="q" class="block font-mono text-[10px] uppercase tracking-widest text-fog mb-1.5">Search</label>
    <input type="text" id="q" name="q" value="{{ .Q }}" placeholder="title, summary, or content…"
      class="w-full rounded-lg bg-ink-950 border border-line px-3 py-2 text-sm text-zinc-100 placeholder:text-zinc-600 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent/40 transition-colors">
  </div>
  <div>
    <label for="feed" class="block font-mono text-[10px] uppercase tracking-widest text-fog mb-1.5">Source</label>
    <select id="feed" name="feed" class="rounded-lg bg-ink-950 border border-line px-3 py-2 text-sm text-zinc-100 max-w-[16rem] focus:outline-none focus:border-accent transition-colors">
      <option value="">All sources</option>
      {{ range .Feeds }}
      <option value="{{ .FeedURL }}" {{ if eq .FeedURL $.Feed }}selected{{ end }}>{{ sourceName .FeedURL }} ({{ .ArticleCount }})</option>
      {{ end }}
    </select>
  </div>
  <div>
    <label for="sort" class="block font-mono text-[10px] uppercase tracking-widest text-fog mb-1.5">Sort</label>
    <select id="sort" name="sort" class="rounded-lg bg-ink-950 border border-line px-3 py-2 text-sm text-zinc-100 focus:outline-none focus:border-accent transition-colors">
      <option value="">Newest first</option>
      <option value="oldest" {{ if eq .Sort "oldest" }}selected{{ end }}>Oldest first</option>
    </select>
  </div>
  <button type="submit" class="rounded-lg bg-accent text-ink-950 font-semibold px-5 py-2 text-sm hover:bg-accent-bright transition-colors">Apply</button>
</form>

{{ if or .Q .Feed (eq .Sort "oldest") }}
<div class="mb-5 flex flex-wrap items-center gap-2 font-mono text-[11px]">
  <span class="uppercase tracking-[0.2em] text-fog">Filtered:</span>
  {{ if .Feed }}<a href="/?q={{ .Q | urlquery }}&amp;sort={{ .Sort }}" class="group inline-flex items-center gap-1.5 rounded border border-line bg-ink-900 px-2 py-1 text-zinc-300 hover:border-accent/60 transition-colors">source: {{ sourceName .Feed }} <span class="text-fog group-hover:text-accent">×</span></a>{{ end }}
  {{ if .Q }}<a href="/?feed={{ .Feed | urlquery }}&amp;sort={{ .Sort }}" class="group inline-flex items-center gap-1.5 rounded border border-line bg-ink-900 px-2 py-1 text-zinc-300 hover:border-accent/60 transition-colors">search: “{{ .Q }}” <span class="text-fog group-hover:text-accent">×</span></a>{{ end }}
  {{ if eq .Sort "oldest" }}<a href="/?q={{ .Q | urlquery }}&amp;feed={{ .Feed | urlquery }}" class="group inline-flex items-center gap-1.5 rounded border border-line bg-ink-900 px-2 py-1 text-zinc-300 hover:border-accent/60 transition-colors">oldest first <span class="text-fog group-hover:text-accent">×</span></a>{{ end }}
  <a href="/" class="text-fog underline underline-offset-2 hover:text-accent transition-colors">clear all</a>
</div>
{{ end }}

{{ if .Upcoming }}
<details class="mb-5 rounded-xl border border-line bg-ink-900/60">
  <summary class="cursor-pointer select-none px-4 py-3 font-mono text-[11px] uppercase tracking-[0.25em] text-fog hover:text-accent transition-colors">upcoming ({{ len .Upcoming }}) — future-dated webinars &amp; events</summary>
  <ul class="space-y-3 p-4 pt-1">
    {{ range .Upcoming }}{{ template "articleCard" . }}{{ end }}
  </ul>
</details>
{{ end }}

{{ if and (not .Articles) (not .Upcoming) }}
  <div class="text-center py-16 border border-dashed border-line rounded-xl">
    <p class="font-mono text-sm text-fog">No articles found.</p>
  </div>
{{ end }}

<ul class="space-y-3">
  {{ range .Articles }}{{ template "articleCard" . }}{{ end }}
</ul>

<nav class="mt-10 flex items-center justify-between font-mono text-xs">
  {{ if .HasPrev }}<a href="?page={{ dec .Page }}&amp;q={{ .Q }}&amp;feed={{ .Feed }}&amp;sort={{ .Sort }}" class="px-4 py-2 rounded-lg border border-line text-zinc-300 hover:border-accent hover:text-accent transition-colors">← Prev</a>{{ else }}<span class="px-4 py-2 text-zinc-700">← Prev</span>{{ end }}
  <span class="text-fog uppercase tracking-widest">Page {{ .Page }}</span>
  {{ if .HasNext }}<a href="?page={{ inc .Page }}&amp;q={{ .Q }}&amp;feed={{ .Feed }}&amp;sort={{ .Sort }}" class="px-4 py-2 rounded-lg border border-line text-zinc-300 hover:border-accent hover:text-accent transition-colors">Next →</a>{{ else }}<span class="px-4 py-2 text-zinc-700">Next →</span>{{ end }}
</nav>
{{ template "footer" . }}{{ end }}
```

- [ ] **Step 4: `internal/server/templates/article.html`** — replace the pill span in the meta row with:

```html
    <a href="/?feed={{ .FeedURL | urlquery }}" title="Filter by {{ sourceName .FeedURL }}" class="rounded border border-line bg-ink-950 px-2 py-0.5 text-accent-dim hover:text-accent hover:border-accent/60 transition-colors">{{ sourceName .FeedURL }}</a>
```

- [ ] **Step 5: Update the two changed assertions in `internal/server/ui_test.go`** (pill is now an anchor, tooltip text changed):
- In `TestListCardShowsPillBadgeAndRelTime`: replace expected `">brighttalk.com</span>"` with `">brighttalk.com</a>"`, and replace the raw-URL tooltip expectation `` title="https://www.brighttalk.com/channel/7451/feed/rss" `` with `` title="Filter by brighttalk.com" ``.
- In `TestArticlePageSingleURLAndPill`: replace `">feeds.feedburner.com</span>"` with `">feeds.feedburner.com</a>"`.

- [ ] **Step 6: Run all tests**

Run: `go test ./... -race`
Expected: ALL PASS, including the Task-2 carried-over subtests.

- [ ] **Step 7: Rebuild CSS and check**

Run: `./scripts/build-css.sh && grep -c "after\\:absolute" internal/server/static/app.css`
Expected: CSS written; grep ≥ 1 (stretched-link utilities present).

- [ ] **Step 8: Full verification**

Run: `gofmt -l . && go vet ./... && go test -race -cover ./...`
Expected: gofmt silent; vet clean; PASS; `internal/server` coverage ≥ 80%.

- [ ] **Step 9: Commit**

```bash
git add internal/
git commit -m "feat(frontend): sort select, filter chips, upcoming section, clickable source pills"
```

---

### Task 4 (controller): review, push, deploy, live verification

- [ ] Push Information-Broker and SmellyFeet to their GitHub remotes.
- [ ] Host: `cd ~/Information-Broker && git pull && docker compose up -d --build <api service>` (identify the API service name from its docker-compose.yml first), then `cd ~/smellyfeet-frontend && git pull && docker compose --project-directory deploy up -d --build`.
- [ ] Live checks through the edge: `/?sort=oldest` returns ascending dates; pagination carries sort; upcoming section on page 1 when future items exist; pill links and chips work; error paths still `no-store`.
