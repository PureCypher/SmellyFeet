# SmellyFeet UI Refinement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refine the terminal theme for scanability and mobile correctness: source pills, relative time, CVE badges, de-noised cards, stats top-sources bars, mobile header fix. Zero JS, CSP untouched, stdlib only.

**Architecture:** Three pure template funcs in `internal/server`, template markup changes in `list.html`/`article.html`/`stats.html`/`partials.html`, one handler change (`handleStats` fetches feeds), 20 static `bar-N` width classes in the input CSS (CSP forbids inline styles), one CSS rebuild at the end.

**Tech Stack:** Go 1.22 stdlib, html/template, Tailwind standalone CLI v3.4.17 via `./scripts/build-css.sh`.

**Spec:** `docs/superpowers/specs/2026-07-03-ui-refinement-design.md`

## Global Constraints

- Go 1.22, stdlib only; no new go.mod deps; zero JavaScript; CSP unchanged (`style-src 'self'` — NO inline `style=""` attributes anywhere).
- gofmt-clean; `go test -race ./...` green; coverage ≥ 80%.
- Commit format `<type>: <description>`, no attribution trailers, no `--no-verify`.
- Existing helpers to reuse: `asTime` (handlers.go), `formatDate`, `stubService`/`newTestServer` (server_test.go), `getPath` (cache_test.go).
- `list.html` iterates with `{{ range $a := .Articles }}` — keep `$a`.
- After template changes the CSS must be rebuilt (`./scripts/build-css.sh`) so new utility classes exist; the rebuilt `internal/server/static/app.css` is committed (Task 4).

---

### Task 1: Template funcs — sourceName, relTime, cveID

**Files:**
- Modify: `internal/server/server.go` (funcs + funcMap entries)
- Test: `internal/server/funcs_test.go`

**Interfaces:**
- Produces: `sourceName(string) string`, `relTimeAt(t, now time.Time) string`, `relTime(any) string`, `cveID(string) string`; funcMap entries `"sourceName"`, `"relTime"`, `"cveID"`. Tasks 2–3 use them in templates.

- [ ] **Step 1: Write the failing test** — `internal/server/funcs_test.go`

```go
package server

import (
	"testing"
	"time"
)

func TestSourceName(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"strips www and path", "https://www.brighttalk.com/channel/7451/feed/rss", "brighttalk.com"},
		{"plain host", "https://feeds.feedburner.com/TheHackersNews", "feeds.feedburner.com"},
		{"not a url", "not a url", "not a url"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceName(tt.in); got != tt.want {
				t.Fatalf("sourceName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRelTimeAt(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"just now", now.Add(-30 * time.Second), "just now"},
		{"minutes", now.Add(-12 * time.Minute), "12m ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"days", now.Add(-5 * 24 * time.Hour), "5d ago"},
		{"old falls back to date", now.Add(-40 * 24 * time.Hour), "2026-05-24"},
		{"future falls back to date", now.Add(54 * 24 * time.Hour), "2026-08-26"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relTimeAt(tt.t, now); got != tt.want {
				t.Fatalf("relTimeAt = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRelTimeNilAndZero(t *testing.T) {
	if got := relTime(nil); got != "—" {
		t.Fatalf("relTime(nil) = %q", got)
	}
	if got := relTime(time.Time{}); got != "—" {
		t.Fatalf("relTime(zero) = %q", got)
	}
}

func TestCveID(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"extracts id", "CVE-2026-14615 - Keycloak-services: fgap v2 bypass", "CVE-2026-14615"},
		{"mid-title", "Freeipa: off-by-one (CVE-2026-14612) during oauth2", "CVE-2026-14612"},
		{"none", "NetNut proxy network disrupted", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cveID(tt.in); got != tt.want {
				t.Fatalf("cveID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/server/ -run 'TestSourceName|TestRelTime|TestCveID' -v`
Expected: FAIL — `undefined: sourceName` (compile error).

- [ ] **Step 3: Implement in `internal/server/server.go`**

Add imports `fmt`, `net/url`, `time` (keep existing). Add below `cleanContent`:

```go
// sourceName returns a compact display name for a feed URL: the hostname
// without a leading "www.". Returns the input unchanged if it doesn't parse.
func sourceName(feedURL string) string {
	u, err := url.Parse(feedURL)
	if err != nil || u.Hostname() == "" {
		return feedURL
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

// relTimeAt renders t relative to now; dates outside the recent past
// (including future-dated webinar feeds) fall back to the plain date.
func relTimeAt(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < 0 || d >= 7*24*time.Hour:
		return t.Format("2006-01-02")
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func relTime(v any) string {
	t, ok := asTime(v)
	if !ok || t.IsZero() {
		return "—"
	}
	return relTimeAt(t, time.Now())
}

var cveRe = regexp.MustCompile(`CVE-\d{4}-\d{4,}`)

// cveID returns the first CVE identifier in s, or "".
func cveID(s string) string { return cveRe.FindString(s) }
```

Add to `funcMap`:

```go
"sourceName": sourceName,
"relTime":    relTime,
"cveID":      cveID,
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/ -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat(frontend): sourceName, relTime, and cveID template funcs"
```

---

### Task 2: List + article template refresh

**Files:**
- Modify: `internal/server/templates/list.html` (meta line, hover arrow, remove CTA row, dropdown labels)
- Modify: `internal/server/templates/article.html` (meta row, remove duplicate URL)
- Test: `internal/server/ui_test.go`

**Interfaces:**
- Consumes: `sourceName`/`relTime`/`cveID` funcMap entries from Task 1; `getPath` from cache_test.go.

- [ ] **Step 1: Write the failing test** — `internal/server/ui_test.go`

```go
package server

import (
	"strings"
	"testing"
	"time"

	"smellyfeet/internal/apiclient"
)

func TestListCardShowsPillBadgeAndRelTime(t *testing.T) {
	sum := "A keycloak bypass."
	svc := stubService{
		list: apiclient.ListResult{Articles: []apiclient.Article{{
			ID: 1, Title: "CVE-2026-14615 - Keycloak-services bypass", Summary: &sum,
			FeedURL:     "https://www.brighttalk.com/channel/7451/feed/rss",
			PublishedAt: time.Now().Add(-2 * time.Hour),
		}}},
		feeds: []apiclient.Feed{{FeedURL: "https://www.brighttalk.com/channel/7451/feed/rss", ArticleCount: 12}},
	}
	body := getPath(t, newTestServer(t, svc), "/").Body.String()

	for _, want := range []string{
		">brighttalk.com</span>",
		`title="https://www.brighttalk.com/channel/7451/feed/rss"`,
		">CVE-2026-14615</span>",
		"2h ago",
		"brighttalk.com (12)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list page missing %q", want)
		}
	}
	if strings.Contains(body, "READ FULL ARTICLE") {
		t.Error("redundant card CTA still present")
	}
}

func TestArticlePageSingleURLAndPill(t *testing.T) {
	svc := stubService{article: apiclient.Article{
		ID: 7, Title: "Some report", URL: "https://example.com/original-report",
		FeedURL: "https://feeds.feedburner.com/TheHackersNews",
	}}
	body := getPath(t, newTestServer(t, svc), "/article/7").Body.String()

	if n := strings.Count(body, "https://example.com/original-report"); n != 1 {
		t.Errorf("original URL appears %d times, want exactly 1 (href only)", n)
	}
	if !strings.Contains(body, ">feeds.feedburner.com</span>") {
		t.Error("article page missing source pill")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/server/ -run 'TestListCardShows|TestArticlePageSingle' -v`
Expected: FAIL — pill markup and dropdown label missing, CTA present, URL appears twice.

- [ ] **Step 3: Edit `internal/server/templates/list.html`**

Replace the card meta `<div>` (the one holding feed URL + formatDate):

```html
      <div class="flex items-center justify-between font-mono text-[11px] mb-2 gap-3 [overflow-wrap:anywhere]">
        <span class="flex items-center gap-2 min-w-0">
          <span class="truncate rounded border border-line bg-ink-950 px-2 py-0.5 text-accent-dim" title="{{ $a.FeedURL }}">{{ sourceName $a.FeedURL }}</span>
          {{ with cveID $a.Title }}<span class="shrink-0 rounded border border-accent/40 bg-accent/10 px-2 py-0.5 text-accent">{{ . }}</span>{{ end }}
        </span>
        <time class="shrink-0 text-fog" datetime="{{ $a.PublishedAt.Format "2006-01-02T15:04:05Z07:00" }}" title="{{ $a.PublishedAt | formatDate }}">{{ $a.PublishedAt | relTime }}</time>
      </div>
```

Delete the CTA line (`<span class="inline-flex items-center gap-1.5 mt-3 ...">Read full article ...</span>`) and add a hover arrow as the last child inside the card `<a>`:

```html
      <span aria-hidden="true" class="absolute right-5 bottom-4 font-mono text-accent opacity-0 group-hover:opacity-100 transition-opacity">→</span>
```

Change the source dropdown option text (value unchanged):

```html
      <option value="{{ .FeedURL }}" {{ if eq .FeedURL $.Feed }}selected{{ end }}>{{ sourceName .FeedURL }} ({{ .ArticleCount }})</option>
```

- [ ] **Step 4: Edit `internal/server/templates/article.html`**

Replace the meta row (feed URL + bullet + date):

```html
  <div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 font-mono text-[11px] text-fog mb-4">
    <span class="rounded border border-line bg-ink-950 px-2 py-0.5 text-accent-dim" title="{{ .FeedURL }}">{{ sourceName .FeedURL }}</span>
    <time datetime="{{ .PublishedAt.Format "2006-01-02T15:04:05Z07:00" }}" title="{{ .PublishedAt | formatDate }}">{{ .PublishedAt | relTime }}</time>
  </div>
```

Delete the duplicate URL paragraph next to the Read original button:

```html
    <p class="font-mono text-[11px] text-zinc-600 break-all">{{ .URL }}</p>
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/server/ -race`
Expected: PASS (if an existing test asserted the removed CTA/URL markup, update it to the new expectations and note it).

- [ ] **Step 6: Commit**

```bash
git add internal/server/
git commit -m "feat(frontend): source pills, CVE badges, relative time on list and article pages"
```

---

### Task 3: Stats top-sources bars

**Files:**
- Modify: `internal/server/handlers.go` (`handleStats` fetches feeds, builds bars)
- Modify: `internal/server/templates/stats.html` (top-sources section, relTime for last fetch)
- Modify: `assets/tailwind.input.css` (bar-5 … bar-100 classes)
- Test: `internal/server/ui_test.go` (append)

**Interfaces:**
- Consumes: `sourceName`/`relTime` funcs; `stubService.feeds`/`feedsErr`.
- Produces: `sourceBar{Name string; Count int; Bar int}` view struct; stats template key `"Sources"`.

- [ ] **Step 1: Append failing tests to `internal/server/ui_test.go`**

```go
func TestStatsShowsTopSources(t *testing.T) {
	svc := stubService{feeds: []apiclient.Feed{
		{FeedURL: "https://small.example.com/rss", ArticleCount: 10},
		{FeedURL: "https://big.example.com/rss", ArticleCount: 200},
	}}
	body := getPath(t, newTestServer(t, svc), "/stats").Body.String()
	if !strings.Contains(body, "top sources") {
		t.Fatal("stats missing top-sources section")
	}
	if !strings.Contains(body, "bar-100") {
		t.Error("largest source should render bar-100")
	}
	i := strings.Index(body, "big.example.com")
	j := strings.Index(body, "small.example.com")
	if i == -1 || j == -1 || i > j {
		t.Errorf("sources not sorted descending (big at %d, small at %d)", i, j)
	}
}

func TestStatsOmitsSourcesOnFeedError(t *testing.T) {
	svc := stubService{feedsErr: errBoom}
	body := getPath(t, newTestServer(t, svc), "/stats").Body.String()
	if strings.Contains(body, "top sources") {
		t.Error("top-sources section should be omitted when ListFeeds fails")
	}
}
```

Add at top of the file (package-level): `var errBoom = errors.New("boom")` with `"errors"` import.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/server/ -run TestStats -v`
Expected: FAIL — no top-sources markup.

- [ ] **Step 3: Implement `handleStats` in `internal/server/handlers.go`**

Add imports `math`, `sort`. Add view struct near `listView`:

```go
// sourceBar is one row of the stats top-sources chart. Bar is a quantized
// width step (5..100 in steps of 5) rendered as a static bar-N CSS class,
// because CSP style-src 'self' forbids inline style widths.
type sourceBar struct {
	Name  string
	Count int
	Bar   int
}

const topSourcesMax = 15

func topSources(feeds []apiclient.Feed) []sourceBar {
	sort.Slice(feeds, func(i, j int) bool { return feeds[i].ArticleCount > feeds[j].ArticleCount })
	if len(feeds) > topSourcesMax {
		feeds = feeds[:topSourcesMax]
	}
	if len(feeds) == 0 || feeds[0].ArticleCount == 0 {
		return nil
	}
	max := float64(feeds[0].ArticleCount)
	out := make([]sourceBar, 0, len(feeds))
	for _, f := range feeds {
		bar := int(math.Round(float64(f.ArticleCount)/max*20)) * 5
		if bar < 5 {
			bar = 5
		}
		out = append(out, sourceBar{Name: sourceName(f.FeedURL), Count: f.ArticleCount, Bar: bar})
	}
	return out
}
```

In `handleStats`, after the stats fetch succeeds:

```go
	var sources []sourceBar
	if feeds, err := s.svc.ListFeeds(r.Context()); err == nil {
		sources = topSources(feeds)
	} // non-fatal: section simply omitted

	setCache(w, cacheStats)
	s.render(w, http.StatusOK, "stats", map[string]any{
		"Title":   "Statistics",
		"Stats":   st,
		"Sources": sources,
	})
```

- [ ] **Step 4: Edit `internal/server/templates/stats.html`**

Change the last-fetch value to `{{ .Stats.LastFetch | relTime }}` with `title="{{ .Stats.LastFetch | formatDate }}"` on its element. Append after the stat-cards grid:

```html
{{ if .Sources }}
<section class="mt-10">
  <p class="font-mono text-[11px] uppercase tracking-[0.25em] text-accent/80 mb-4">// top sources</p>
  <ul class="space-y-2">
    {{ range .Sources }}
    <li class="flex items-center gap-3 font-mono text-xs">
      <span class="w-40 md:w-56 truncate text-zinc-300">{{ .Name }}</span>
      <span class="flex-1 h-2 rounded bg-ink-800 overflow-hidden"><span class="block h-full rounded bg-accent/70 bar-{{ .Bar }}"></span></span>
      <span class="w-14 text-right text-fog">{{ .Count }}</span>
    </li>
    {{ end }}
  </ul>
</section>
{{ end }}
```

- [ ] **Step 5: Add bar classes to `assets/tailwind.input.css`** (after the reveal stagger block)

```css
/* Stats source bars: CSP style-src 'self' forbids inline widths, so widths
   are quantized to 5% steps rendered as static classes. */
.bar-5{width:5%}.bar-10{width:10%}.bar-15{width:15%}.bar-20{width:20%}
.bar-25{width:25%}.bar-30{width:30%}.bar-35{width:35%}.bar-40{width:40%}
.bar-45{width:45%}.bar-50{width:50%}.bar-55{width:55%}.bar-60{width:60%}
.bar-65{width:65%}.bar-70{width:70%}.bar-75{width:75%}.bar-80{width:80%}
.bar-85{width:85%}.bar-90{width:90%}.bar-95{width:95%}.bar-100{width:100%}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/server/ -race`
Expected: PASS. (Template changes render even though app.css isn't rebuilt yet — rebuild happens in Task 4.)

- [ ] **Step 7: Commit**

```bash
git add internal/server/ assets/
git commit -m "feat(frontend): top-sources bar chart on stats page"
```

---

### Task 4: Mobile header, footer, CSS rebuild, verification

**Files:**
- Modify: `internal/server/templates/partials.html` (header wrap/sizing, footer Atom link)
- Regenerate: `internal/server/static/app.css`
- Test: whole repo

**Interfaces:**
- Consumes: everything above; `./scripts/build-css.sh`.

- [ ] **Step 1: Header — make it fit 390px.** In `partials.html`, change the header inner div and sizes:

```html
    <div class="max-w-5xl mx-auto px-4 sm:px-5 min-h-16 py-2 flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
```

Wordmark span: `class="font-mono text-xs sm:text-sm tracking-[0.14em] sm:tracking-[0.22em] text-zinc-100 uppercase"`.
Nav: `class="flex items-center gap-0.5 sm:gap-1 font-mono text-[10px] sm:text-xs uppercase tracking-[0.12em] sm:tracking-[0.18em]"` and each nav link `px-2 sm:px-3 py-1.5 ...` (rest unchanged).

- [ ] **Step 2: Footer — add Atom link.** Replace the footer `<p>` with:

```html
    <div class="flex flex-wrap items-center justify-between gap-2">
      <p class="font-mono text-[11px] uppercase tracking-[0.2em] text-zinc-600">Information Broker · threat intelligence feed</p>
      <a href="/feed.xml" class="font-mono text-[11px] uppercase tracking-[0.2em] text-zinc-600 hover:text-accent transition-colors">Atom feed</a>
    </div>
```

- [ ] **Step 3: Rebuild CSS**

Run: `./scripts/build-css.sh`
Expected: "wrote internal/server/static/app.css (… bytes)". Then `grep -c "bar-100" internal/server/static/app.css` ≥ 1 and `grep -c "line-clamp-3" internal/server/static/app.css` ≥ 1.

- [ ] **Step 4: Full verification**

Run: `gofmt -l . && go vet ./... && go test -race -cover ./...`
Expected: gofmt silent, vet clean, all PASS, `internal/server` coverage ≥ 80%.

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat(frontend): responsive header, footer feed link, rebuilt CSS"
```
