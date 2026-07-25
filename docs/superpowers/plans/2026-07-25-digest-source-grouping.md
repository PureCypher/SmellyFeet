# Digest Source Grouping Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On `/digest`, collapse the multiple articles covering the same story into one expandable topic group (collapsed by default) instead of one card per feed.

**Architecture:** The backend already clusters same-story articles via `story_cluster_id` but never exposes it. Task 1 adds it to the digest response (additive, no breaking change). The frontend then folds the flat `Important`/`Other` slices into `[]articleGroup` with a pure function, and renders each group as a lead card plus a native `<details>` disclosure listing the other sources.

**Tech Stack:** Go 1.x standard library only (both repos are deliberately zero-dependency), Go `html/template`, Tailwind CSS built via `./scripts/build-css.sh`, Postgres.

**Spec:** `docs/superpowers/specs/2026-07-25-digest-source-grouping-design.md`

## Global Constraints

- Two repos. Backend: `/home/pure/Documents/github/Information-Broker` (package `main`, flat files at repo root). Frontend: `/home/pure/Documents/github/SmellyFeet` (module `smellyfeet`).
- Neither repo may gain a third-party dependency. Both `go.mod` files are deliberately zero-dependency.
- All backend changes are **additive**: no schema change, no new index, no change to any existing endpoint's response for existing consumers.
- Deploy order is backend first, frontend second. The frontend must behave correctly when `story_cluster_id` is absent from the JSON (every article becomes a singleton group — today's behaviour).
- The site runs under a CSP with `style-src 'self'`: **no inline `style=` attributes**. Dynamic styling is expressed as static classes in `assets/tailwind.input.css`.
- Tests are standard `go test` table-driven style. Run with `-race`.
- Commit messages follow conventional commits (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`), scoped `(digest)` where it applies. No attribution trailers.
- `articleCardBody` is shared by the list, upcoming, and article pages. Its rendered output must not change except for the badge label in Task 6.

---

### Task 1: Backend — expose `story_cluster_id` on the digest response

**Repo:** `/home/pure/Documents/github/Information-Broker`

**Files:**
- Modify: `api.go:98` (the `ArticleView` struct)
- Modify: `digest.go` — imports, `buildDigestQuery`, the row scan in `getArticlesDigest:135-149`
- Test: `digest_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `ArticleView.StoryClusterID *int64` serialized as `"story_cluster_id"` (omitted when nil) on `GET /articles/digest` only.

- [ ] **Step 1: Write the failing test**

Append to `digest_test.go`:

```go
func TestBuildDigestQuerySelectsStoryClusterID(t *testing.T) {
	since := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	q, _ := buildDigestQuery(since)

	// The frontend groups digest rows by cluster, so the cluster key itself
	// has to come back on the row -- cross_feed_count alone says how many
	// feeds covered a story but not which articles those were.
	if !strings.Contains(q, "a.story_cluster_id") {
		t.Fatalf("digest SELECT must expose a.story_cluster_id for client-side grouping: %s", q)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/pure/Documents/github/Information-Broker && go test -run TestBuildDigestQuerySelectsStoryClusterID -v ./...`

Expected: FAIL — `digest SELECT must expose a.story_cluster_id for client-side grouping`

- [ ] **Step 3: Add the struct field**

In `api.go`, add one line to `ArticleView` immediately after `CrossFeedCount`:

```go
	CrossFeedCount int           `json:"cross_feed_count,omitempty"`
	// StoryClusterID is the digest clustering key (the seed article's own id).
	// Pointer + omitempty: NULL for articles ClusteringScheduler hasn't reached
	// yet, and absent entirely from /articles, which doesn't select the column.
	StoryClusterID *int64        `json:"story_cluster_id,omitempty"`
```

- [ ] **Step 4: Add the column to the digest SELECT**

In `digest.go`, `buildDigestQuery` — add `a.story_cluster_id` to the end of the third SELECT line. The full query becomes:

```go
	query := `SELECT a.id, a.title, a.url, a.summary, a.full_content, a.publish_date,
		a.fetch_duration_ms, a.feed_url, a.content_hash,
		COALESCE(cluster_counts.distinct_feeds - 1, 0) AS cross_feed_count, a.story_cluster_id
		FROM articles a
		LEFT JOIN (
			SELECT story_cluster_id, COUNT(DISTINCT feed_url) AS distinct_feeds
			FROM articles
			WHERE publish_date >= $1 AND story_cluster_id IS NOT NULL
			GROUP BY story_cluster_id
		) cluster_counts ON cluster_counts.story_cluster_id = a.story_cluster_id
		WHERE a.publish_date >= $1
		ORDER BY cross_feed_count DESC, a.publish_date DESC`
```

The `LEFT JOIN`, `WHERE`, and `ORDER BY` are unchanged.

- [ ] **Step 5: Scan the new column**

In `digest.go`, add `"database/sql"` to the import block:

```go
import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"
)
```

Then replace the scan loop in `getArticlesDigest` (currently `digest.go:135-149`) with:

```go
	all := []ArticleView{}
	for rows.Next() {
		var a ArticleView
		var fetchDurationMs int64
		var clusterID sql.NullInt64
		err := rows.Scan(
			&a.ID, &a.Title, &a.URL, &a.Summary, &a.Content, &a.PublishedAt,
			&fetchDurationMs, &a.FeedURL, &a.ContentHash, &a.CrossFeedCount, &clusterID,
		)
		if err != nil {
			log.Printf("Row scan error: %v", err)
			continue
		}
		a.FetchDuration = time.Duration(fetchDurationMs) * time.Millisecond
		if clusterID.Valid {
			a.StoryClusterID = &clusterID.Int64
		}
		all = append(all, a)
	}
```

Note `&clusterID.Int64` is safe here: `clusterID` is redeclared each loop iteration, so each article gets a distinct pointer.

- [ ] **Step 6: Run the full backend suite**

Run: `cd /home/pure/Documents/github/Information-Broker && go build ./... && go test -race ./...`

Expected: PASS, including the pre-existing `TestBuildDigestQuery`, `TestBuildDigestQueryIncludesUnclusteredArticles`, and `TestSplitImportant`.

- [ ] **Step 7: Commit**

```bash
cd /home/pure/Documents/github/Information-Broker
git add api.go digest.go digest_test.go
git commit -m "feat(digest): expose story_cluster_id on the digest response"
```

---

### Task 2: Frontend API client — mirror the field

**Repo:** `/home/pure/Documents/github/SmellyFeet` (all remaining tasks)

**Files:**
- Modify: `internal/apiclient/apiclient.go:20-30` (the `Article` struct)
- Test: `internal/apiclient/apiclient_test.go:170-196` (`TestGetDigest`)

**Interfaces:**
- Consumes: the `"story_cluster_id"` JSON field produced by Task 1.
- Produces: `apiclient.Article.StoryClusterID *int64`, used by `groupByCluster` in Task 3.

- [ ] **Step 1: Write the failing test**

In `internal/apiclient/apiclient_test.go`, update `TestGetDigest`. Change the stub response body to include `story_cluster_id` on both important articles:

```go
		w.Write([]byte(`{"range":"weekly","since":"2026-07-07T00:00:00Z","important":[{"id":1,"title":"Big story","cross_feed_count":3,"story_cluster_id":42}],"other":[{"id":2,"title":"Minor","cross_feed_count":0}]}`))
```

and append these assertions just before the closing brace of `TestGetDigest`:

```go
	if res.Important[0].StoryClusterID == nil || *res.Important[0].StoryClusterID != 42 {
		t.Fatalf("StoryClusterID = %v, want 42", res.Important[0].StoryClusterID)
	}
	// Absent in the JSON means "not yet clustered", which must stay
	// distinguishable from cluster 0 -- hence a pointer, not an int.
	if res.Other[0].StoryClusterID != nil {
		t.Fatalf("StoryClusterID = %v, want nil when absent from the JSON", res.Other[0].StoryClusterID)
	}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/pure/Documents/github/SmellyFeet && go test -run TestGetDigest ./internal/apiclient/`

Expected: FAIL — compile error, `res.Important[0].StoryClusterID undefined`.

- [ ] **Step 3: Add the field**

In `internal/apiclient/apiclient.go`, add one line to `Article` after `CrossFeedCount`:

```go
	CrossFeedCount int       `json:"cross_feed_count,omitempty"`
	// StoryClusterID groups articles covering the same story. Populated only
	// by /articles/digest; nil elsewhere and nil for articles the backend's
	// clustering job hasn't reached yet.
	StoryClusterID *int64    `json:"story_cluster_id,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/pure/Documents/github/SmellyFeet && go test -race ./internal/apiclient/`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/pure/Documents/github/SmellyFeet
git add internal/apiclient/apiclient.go internal/apiclient/apiclient_test.go
git commit -m "feat(digest): carry story_cluster_id through the API client"
```

---

### Task 3: `groupByCluster` — the pure grouping function

**Files:**
- Create: `internal/server/digest.go`
- Test: `internal/server/digest_test.go` (exists; append)

**Interfaces:**
- Consumes: `apiclient.Article.StoryClusterID` from Task 2.
- Produces:
  - `type articleGroup struct { Lead apiclient.Article; Members []apiclient.Article }`
  - `func groupByCluster(articles []apiclient.Article) []articleGroup`
  - Test helper `func clusterRef(id int64) *int64` (defined in `digest_test.go`, reused by Task 5's tests).

- [ ] **Step 1: Write the failing tests**

Append to `internal/server/digest_test.go`:

```go
// clusterRef builds a *int64 for table-test literals.
func clusterRef(id int64) *int64 { return &id }

func TestGroupByClusterMergesSharedClusterIDs(t *testing.T) {
	got := groupByCluster([]apiclient.Article{
		{ID: 1, StoryClusterID: clusterRef(1)},
		{ID: 2, StoryClusterID: clusterRef(1)},
		{ID: 3, StoryClusterID: clusterRef(3)},
	})
	if len(got) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(got))
	}
	if got[0].Lead.ID != 1 {
		t.Errorf("group 0 lead = %d, want the first-seen article (1)", got[0].Lead.ID)
	}
	if len(got[0].Members) != 1 || got[0].Members[0].ID != 2 {
		t.Errorf("group 0 members = %+v, want just article 2", got[0].Members)
	}
	if got[1].Lead.ID != 3 || len(got[1].Members) != 0 {
		t.Errorf("group 1 = %+v, want article 3 with no members", got[1])
	}
}

func TestGroupByClusterKeepsUnclusteredArticlesSeparate(t *testing.T) {
	// A nil cluster ID means "the clustering job hasn't reached this article
	// yet", not "same story" -- two nils must never merge.
	got := groupByCluster([]apiclient.Article{{ID: 1}, {ID: 2}})
	if len(got) != 2 {
		t.Fatalf("len(groups) = %d, want 2 singletons", len(got))
	}
	if len(got[0].Members) != 0 || len(got[1].Members) != 0 {
		t.Errorf("unclustered articles must not absorb members: %+v", got)
	}
}

func TestGroupByClusterPreservesFirstAppearanceOrder(t *testing.T) {
	// The API orders by cross_feed_count DESC, publish_date DESC. Grouping
	// must not reshuffle that ranking, even when clusters interleave.
	got := groupByCluster([]apiclient.Article{
		{ID: 1, StoryClusterID: clusterRef(7)},
		{ID: 2, StoryClusterID: clusterRef(9)},
		{ID: 3, StoryClusterID: clusterRef(7)},
		{ID: 4},
	})
	if len(got) != 3 {
		t.Fatalf("len(groups) = %d, want 3", len(got))
	}
	if got[0].Lead.ID != 1 || got[1].Lead.ID != 2 || got[2].Lead.ID != 4 {
		t.Fatalf("leads = %d,%d,%d, want 1,2,4", got[0].Lead.ID, got[1].Lead.ID, got[2].Lead.ID)
	}
	if len(got[0].Members) != 1 || got[0].Members[0].ID != 3 {
		t.Errorf("article 3 should join cluster 7's group: %+v", got[0])
	}
}

func TestGroupByClusterEmptyInput(t *testing.T) {
	if got := groupByCluster(nil); len(got) != 0 {
		t.Fatalf("groupByCluster(nil) = %+v, want empty", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/pure/Documents/github/SmellyFeet && go test -run TestGroupByCluster ./internal/server/`

Expected: FAIL — compile error, `undefined: groupByCluster`.

- [ ] **Step 3: Write the implementation**

Create `internal/server/digest.go`:

```go
package server

import "smellyfeet/internal/apiclient"

// articleGroup is one story: the newest article covering it, plus every other
// article the backend's clustering job put in the same cluster.
type articleGroup struct {
	Lead    apiclient.Article
	Members []apiclient.Article
}

// groupByCluster collapses a digest slice into one group per story cluster,
// so a story covered by four feeds renders once instead of four times.
//
// Incoming order (cross_feed_count DESC, publish_date DESC, set by the API) is
// preserved: groups appear in order of first appearance, and the first article
// seen for a cluster leads it. Every member of a cluster shares the same
// cross_feed_count, so within a cluster the incoming order is purely
// publish_date DESC -- which makes the first-seen article the newest one.
//
// A nil StoryClusterID means the clustering job hasn't reached that article
// yet -- "unclassified", not "same story" -- so each one becomes its own
// singleton group rather than being lumped in with the other nils.
func groupByCluster(articles []apiclient.Article) []articleGroup {
	groups := []articleGroup{}
	indexByCluster := map[int64]int{}
	for _, a := range articles {
		if a.StoryClusterID == nil {
			groups = append(groups, articleGroup{Lead: a})
			continue
		}
		if i, ok := indexByCluster[*a.StoryClusterID]; ok {
			groups[i].Members = append(groups[i].Members, a)
			continue
		}
		indexByCluster[*a.StoryClusterID] = len(groups)
		groups = append(groups, articleGroup{Lead: a})
	}
	return groups
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/pure/Documents/github/SmellyFeet && go test -race -run TestGroupByCluster -v ./internal/server/`

Expected: PASS — all four tests.

- [ ] **Step 5: Commit**

```bash
cd /home/pure/Documents/github/SmellyFeet
git add internal/server/digest.go internal/server/digest_test.go
git commit -m "feat(digest): group digest articles by story cluster"
```

---

### Task 4: Extract `articleCardBody` from `articleCard`

A behaviour-preserving refactor so Task 5's group partial can reuse the card body while owning its own `<li>`. No new test: the existing suite already asserts the rendered card markup across the list, upcoming, article, and digest pages, and this task's contract is that all of it stays green unchanged.

**Files:**
- Modify: `internal/server/templates/list.html:1-24`

**Interfaces:**
- Consumes: nothing.
- Produces: `{{ define "articleCardBody" }}` — takes an `apiclient.Article`, renders the card `<div>` **without** the surrounding `<li>`. Used by Task 5.

- [ ] **Step 1: Establish the green baseline**

Run: `cd /home/pure/Documents/github/SmellyFeet && go test -race ./...`

Expected: PASS. Note the result — this is the before/after comparison for the refactor.

- [ ] **Step 2: Split the partial**

In `internal/server/templates/list.html`, replace lines 1-24 (the whole `articleCard` define) with:

```
{{ define "articleCardBody" }}
  <div class="group relative overflow-hidden bg-ink-900 border border-line rounded-xl p-5 hover:border-accent/50 hover:bg-ink-850 transition-colors">
    <span class="absolute left-0 top-0 h-full w-[2px] bg-accent origin-top scale-y-0 group-hover:scale-y-100 transition-transform duration-300"></span>
    <div class="flex flex-wrap items-center justify-between font-mono text-[11px] mb-2 gap-x-3 gap-y-1 [overflow-wrap:anywhere]">
      <span class="flex items-center gap-2 min-w-0">
        <a href="/?feed={{ .FeedURL | urlquery }}" title="Filter by {{ sourceName .FeedURL }}" class="relative z-10 truncate rounded border border-line bg-ink-950 px-2 py-0.5 text-accent-dim hover:text-accent hover:border-accent/60 transition-colors">{{ sourceName .FeedURL }}</a>
        {{ with cveID .Title }}<span class="shrink-0 rounded border border-accent/40 bg-accent/10 px-2 py-0.5 text-accent">{{ . }}</span>{{ end }}
        {{ if gt .CrossFeedCount 1 }}<span class="shrink-0 rounded border border-accent/40 bg-accent/10 px-2 py-0.5 text-accent">{{ .CrossFeedCount }} sources</span>{{ end }}
      </span>
      <span class="shrink-0 flex items-center gap-1.5">
        <time class="text-fog" datetime="{{ .PublishedAt.Format "2006-01-02T15:04:05Z07:00" }}">{{ .PublishedAt | relTime }}</time>
        <span class="text-structural" aria-hidden="true">·</span>
        <span class="text-structural">{{ .PublishedAt | formatDate }} UTC</span>
      </span>
    </div>
    <h2 class="text-lg font-semibold text-primary leading-snug group-hover:text-accent transition-colors"><a href="/article/{{ .ID }}" class="after:absolute after:inset-0">{{ .Title }}</a></h2>
    <p class="mt-2 text-sm text-body leading-relaxed line-clamp-3">
      {{ if .Summary }}{{ .Summary }}{{ else }}<span class="text-fog italic">No summary available.</span>{{ end }}
    </p>
    <span aria-hidden="true" class="absolute right-5 bottom-4 font-mono text-accent opacity-0 group-hover:opacity-100 transition-opacity">→</span>
  </div>
{{ end }}

{{ define "articleCard" }}
<li class="reveal">{{ template "articleCardBody" . }}</li>
{{ end }}
```

The card markup is byte-identical to what it was; only the `<li>` wrapper moved out. The badge still reads `{{ .CrossFeedCount }} sources` — Task 6 fixes that separately.

- [ ] **Step 3: Verify nothing changed**

Run: `cd /home/pure/Documents/github/SmellyFeet && go test -race ./...`

Expected: PASS, same as Step 1. Any failure here means the refactor altered rendered output — fix it rather than adjusting the test.

- [ ] **Step 4: Commit**

```bash
cd /home/pure/Documents/github/SmellyFeet
git add internal/server/templates/list.html
git commit -m "refactor(templates): split articleCardBody out of articleCard"
```

---

### Task 5: Render collapsible source groups on the digest page

**Files:**
- Modify: `internal/server/handlers.go:244-255` (`digestView`), `:280-289` (the success render)
- Modify: `internal/server/templates/digest.html` (new `articleGroup` partial; both section loops)
- Modify: `assets/tailwind.input.css` (the `.grouped` join rule)
- Test: `internal/server/digest_test.go`

**Interfaces:**
- Consumes: `articleGroup`, `groupByCluster` (Task 3); `{{ define "articleCardBody" }}` (Task 4); `clusterRef` test helper (Task 3).
- Produces: `digestView.Important` and `digestView.Other` typed `[]articleGroup`; template `{{ define "articleGroup" }}`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/server/digest_test.go`:

```go
func TestDigestGroupsCorrelatedSources(t *testing.T) {
	svc := stubService{digest: apiclient.DigestResult{
		Range: "daily",
		Important: []apiclient.Article{
			{ID: 1, Title: "Fortinet patches RCE", FeedURL: "https://a.example/feed", CrossFeedCount: 2, StoryClusterID: clusterRef(1)},
			{ID: 2, Title: "Fortinet rushes fix", FeedURL: "https://b.example/feed", CrossFeedCount: 2, StoryClusterID: clusterRef(1)},
			{ID: 3, Title: "Fortinet flaw exploited", FeedURL: "https://c.example/feed", CrossFeedCount: 2, StoryClusterID: clusterRef(1)},
		},
	}}
	body := getPath(t, newTestServer(t, svc), "/digest").Body.String()

	if !containsAll(body, "Fortinet patches RCE", "+2 more sources", "Fortinet rushes fix", "Fortinet flaw exploited") {
		t.Fatalf("expected one lead card plus a disclosure listing the other two sources: %s", body)
	}
	// One story, not three rows.
	if !strings.Contains(body, "Important (1)") {
		t.Errorf("the Important heading must count stories, not articles: %s", body)
	}
	if strings.Contains(body, "<details open") {
		t.Errorf("source groups must default to collapsed: %s", body)
	}
}

func TestDigestSingletonStoryHasNoDisclosure(t *testing.T) {
	svc := stubService{digest: apiclient.DigestResult{
		Range:     "daily",
		Important: []apiclient.Article{{ID: 1, Title: "Solo story", CrossFeedCount: 2, StoryClusterID: clusterRef(1)}},
	}}
	body := getPath(t, newTestServer(t, svc), "/digest").Body.String()

	if !strings.Contains(body, "Solo story") {
		t.Fatalf("lead card missing: %s", body)
	}
	if strings.Contains(body, "more source") {
		t.Errorf("a one-article story must not render a disclosure: %s", body)
	}
}

func TestDigestGroupsEverythingElseToo(t *testing.T) {
	svc := stubService{digest: apiclient.DigestResult{
		Range: "daily",
		Other: []apiclient.Article{
			{ID: 1, Title: "Pair lead", FeedURL: "https://a.example/feed", CrossFeedCount: 1, StoryClusterID: clusterRef(5)},
			{ID: 2, Title: "Pair sibling", FeedURL: "https://b.example/feed", CrossFeedCount: 1, StoryClusterID: clusterRef(5)},
		},
	}}
	body := getPath(t, newTestServer(t, svc), "/digest").Body.String()

	// Two articles, one story: the bucket count and the disclosure both say so.
	if !containsAll(body, "everything else (1)", "+1 more source") {
		t.Fatalf("everything-else bucket must group by cluster too: %s", body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/pure/Documents/github/SmellyFeet && go test -run TestDigest ./internal/server/`

Expected: FAIL — `+2 more sources` not found (the page still renders flat cards).

- [ ] **Step 3: Change the view type and group in the handler**

In `internal/server/handlers.go`, change the two fields in `digestView`:

```go
type digestView struct {
	Title       string
	Desc        string
	OG          bool
	OGArticle   bool // required by the shared header partial whenever OG is true; false = og:type "website"
	Nav         string
	Range       string
	Since       time.Time
	Important   []articleGroup
	Other       []articleGroup
	FetchFailed bool
}
```

and in `handleDigest`, wrap both slices in the success render (the `FetchFailed` render above it is unchanged — it passes neither field, and nil slices render as empty):

```go
	setCache(w, cacheList)
	s.render(w, http.StatusOK, "digest", digestView{
		Title:     "Digest",
		Desc:      "Cross-feed importance digest — stories covered by multiple sources, for the current day, week, month, quarter, half-year, or year.",
		OG:        true,
		Nav:       "digest",
		Range:     rangeParam,
		Since:     res.Since,
		Important: groupByCluster(res.Important),
		Other:     groupByCluster(res.Other),
	})
```

Grouping each bucket separately is safe: every article in a cluster shares the same `cross_feed_count`, so `splitImportant` can never split one cluster across the two buckets.

- [ ] **Step 4: Add the group partial and switch both loops**

In `internal/server/templates/digest.html`, insert this partial at the very top of the file, **before** `{{ define "digest" }}`:

```
{{ define "articleGroup" }}
<li class="reveal{{ if .Members }} grouped{{ end }}">
  {{ template "articleCardBody" .Lead }}
  {{ if .Members }}
  <details class="border border-line bg-ink-900/60">
    <summary class="cursor-pointer select-none px-4 py-2 font-mono text-[10px] uppercase tracking-[0.2em] text-fog hover:text-accent transition-colors">+{{ len .Members }} more source{{ if ne (len .Members) 1 }}s{{ end }}</summary>
    <ul class="border-t border-line/60">
      {{ range .Members }}
      <li class="border-b border-line/40 last:border-b-0">
        <a href="/article/{{ .ID }}" class="group/src flex flex-wrap items-baseline gap-x-2.5 gap-y-1 px-4 py-2.5 hover:bg-ink-850 transition-colors">
          <span class="shrink-0 font-mono text-[11px] rounded border border-line bg-ink-950 px-2 py-0.5 text-accent-dim">{{ sourceName .FeedURL }}</span>
          <span class="shrink-0 font-mono text-[11px] text-fog">{{ .PublishedAt | relTime }}</span>
          <span class="min-w-0 flex-1 text-sm text-body group-hover/src:text-accent transition-colors [overflow-wrap:anywhere]">{{ .Title }}</span>
        </a>
      </li>
      {{ end }}
    </ul>
  </details>
  {{ end }}
</li>
{{ end }}
```

The source name is a plain `<span>` here, not the filter link used in the card — the whole row is already an anchor, and anchors cannot nest.

Then change the two render loops. The Important loop (currently `{{ range .Important }}{{ template "articleCard" . }}{{ end }}`):

```
  {{ range .Important }}{{ template "articleGroup" . }}{{ end }}
```

and the Other loop inside the "everything else" `<details>`:

```
    {{ range .Other }}{{ template "articleGroup" . }}{{ end }}
```

Finally, fix the wording of the no-important-stories line, which now counts groups rather than articles. Replace:

```
<p class="text-sm text-fog mb-3">No cross-feed stories detected in this window. {{ len .Other }} single-source article{{ if ne (len .Other) 1 }}s{{ end }} below.</p>
```

with:

```
<p class="text-sm text-fog mb-3">No cross-feed stories detected in this window. {{ len .Other }} other stor{{ if ne (len .Other) 1 }}ies{{ else }}y{{ end }} below.</p>
```

- [ ] **Step 5: Add the CSS join rule**

In `assets/tailwind.input.css`, immediately after the `.reveal:nth-child(20)` line and before the `/* Stats source bars ... */` comment, add:

```css
/* Digest source groups: the lead card and its disclosure are siblings (the
   card's stretched link would swallow clicks on a nested summary), so they're
   joined into one visual unit here. articleCardBody is shared with the list
   and article pages, hence a class on the <li> rather than a template flag. */
.grouped > div:first-child { border-bottom-left-radius: 0; border-bottom-right-radius: 0; border-bottom-width: 0; }
.grouped > details { border-top-left-radius: 0; border-top-right-radius: 0; border-bottom-left-radius: 0.75rem; border-bottom-right-radius: 0.75rem; }
```

- [ ] **Step 6: Rebuild the stylesheet**

Run: `cd /home/pure/Documents/github/SmellyFeet && ./scripts/build-css.sh`

Expected: completes without error, and `git status` shows the generated CSS file modified.

- [ ] **Step 7: Run the full suite**

Run: `cd /home/pure/Documents/github/SmellyFeet && go build ./... && go test -race ./...`

Expected: PASS for the three new tests plus `TestHandleDigestRangeWhitelist`, `TestHandleDigestUpstreamErrorRendersInlineCallout`, and `TestDigestEmptyState`.

`TestDigestImportantAndOtherSections` is expected to **still pass** at this point: its articles have nil cluster IDs, so each is a singleton, and `everything else (1)` is unchanged. If it fails, the grouping changed singleton rendering — fix the template, not the test.

- [ ] **Step 8: Commit**

```bash
cd /home/pure/Documents/github/SmellyFeet
git add internal/server/handlers.go internal/server/templates/digest.html internal/server/digest_test.go assets/
git commit -m "feat(digest): collapsible source groups for correlated stories"
```

---

### Task 6: Fix the off-by-one source badge

`cross_feed_count` is the count of *other* feeds (`distinct_feeds - 1`), so a three-feed story currently badges "2 sources". `inc` is already in the funcmap (`internal/server/server.go:82`).

**Files:**
- Modify: `internal/server/templates/list.html` (the badge line inside `articleCardBody`)
- Test: `internal/server/digest_test.go:44-54` (`TestDigestImportantAndOtherSections`)

**Interfaces:**
- Consumes: `{{ define "articleCardBody" }}` from Task 4.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Write the failing test**

In `internal/server/digest_test.go`, update `TestDigestImportantAndOtherSections` to expect the true source count. `CrossFeedCount: 3` means three *other* feeds, so four sources total:

```go
func TestDigestImportantAndOtherSections(t *testing.T) {
	svc := stubService{digest: apiclient.DigestResult{
		Range:     "daily",
		Important: []apiclient.Article{{ID: 1, Title: "Big story", CrossFeedCount: 3}},
		Other:     []apiclient.Article{{ID: 2, Title: "Minor item"}},
	}}
	body := getPath(t, newTestServer(t, svc), "/digest").Body.String()
	// cross_feed_count counts OTHER feeds, so 3 means 4 distinct sources.
	if !containsAll(body, "Big story", "4 sources", "everything else (1)", "Minor item") {
		t.Fatalf("digest body missing expected markers: %s", body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/pure/Documents/github/SmellyFeet && go test -run TestDigestImportantAndOtherSections ./internal/server/`

Expected: FAIL — the body contains "3 sources", not "4 sources".

- [ ] **Step 3: Fix the label**

In `internal/server/templates/list.html`, inside `articleCardBody`, change the badge line from:

```
        {{ if gt .CrossFeedCount 1 }}<span class="shrink-0 rounded border border-accent/40 bg-accent/10 px-2 py-0.5 text-accent">{{ .CrossFeedCount }} sources</span>{{ end }}
```

to:

```
        {{/* cross_feed_count is the count of OTHER feeds, so +1 is the true distinct-source count. */}}
        {{ if gt .CrossFeedCount 1 }}<span class="shrink-0 rounded border border-accent/40 bg-accent/10 px-2 py-0.5 text-accent">{{ inc .CrossFeedCount }} sources</span>{{ end }}
```

The `gt .CrossFeedCount 1` guard is deliberately unchanged: `CrossFeedCount` is zero on every non-digest page, so the badge still renders only on `/digest`.

- [ ] **Step 4: Run the full suite**

Run: `cd /home/pure/Documents/github/SmellyFeet && go test -race ./...`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/pure/Documents/github/SmellyFeet
git add internal/server/templates/list.html internal/server/digest_test.go
git commit -m "fix(digest): source badge under-reported the feed count by one"
```

---

### Task 7: Verify the rendered page

**Files:** none modified — this is a verification gate.

**Interfaces:**
- Consumes: everything from Tasks 1-6.
- Produces: nothing.

- [ ] **Step 1: Full test run, both repos**

```bash
cd /home/pure/Documents/github/Information-Broker && go build ./... && go test -race ./...
cd /home/pure/Documents/github/SmellyFeet && go build ./... && go test -race ./...
```

Expected: PASS in both.

- [ ] **Step 2: Confirm the stylesheet is committed**

Run: `cd /home/pure/Documents/github/SmellyFeet && git status --porcelain`

Expected: empty. A dirty generated CSS file means Step 6 of Task 5 was run after the commit — stage and amend it.

- [ ] **Step 3: Confirm no unclustered-article regression**

Re-read `groupByCluster` in `internal/server/digest.go` and confirm the nil-`StoryClusterID` branch appends a fresh group every time rather than reusing a zero-key map entry. This is the path that runs against a backend that hasn't been deployed yet, and it must degrade to today's flat behaviour.

Run: `cd /home/pure/Documents/github/SmellyFeet && go test -race -run TestGroupByClusterKeepsUnclusteredArticlesSeparate -v ./internal/server/`

Expected: PASS

- [ ] **Step 4: Report deploy order**

State plainly in the final summary that nothing has been deployed, and that the deploy order is Information-Broker first (additive; the frontend renders singletons without it), SmellyFeet second.
