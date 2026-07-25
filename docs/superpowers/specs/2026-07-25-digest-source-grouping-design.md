# Digest Source Grouping — Design

**Date:** 2026-07-25
**Status:** Approved

**Goal:** On `/digest`, collapse the multiple articles that cover the same story into a single
expandable topic group, so one story occupies one row instead of one row per feed that ran it.

## Context

The digest page renders `Important` and `Other` as flat lists of `articleCard`. The
embedding-based clustering job already knows which articles are the same story — that is what
populates `cross_feed_count` — but the page never uses that grouping. A story covered by four
feeds renders as four separate cards, each badged "3 sources", stacked next to each other.
The redundancy is worst exactly where the page is meant to be most useful: the top of the
`Important` list, which is sorted by cross-feed coverage and therefore front-loads the most
duplicated stories.

The clustering itself is unchanged by this design. `story_cluster_id` (see
`2026-07-14-story-clustering-design.md`) is assigned by `ClusteringScheduler` from summary
embeddings and is already the exact grouping key needed; it is simply not exposed to the
frontend today. The digest response carries only the derived `cross_feed_count`, which tells
you *how many* feeds covered a story but not *which articles* those were.

Backend source of truth: `/home/pure/Documents/github/Information-Broker`. Scope: both repos.

## Approaches considered

**A — Expose `story_cluster_id`, group in the frontend (chosen).** One additive column in the
digest SELECT, one nullable JSON field, and a pure grouping function in SmellyFeet. The API
response shape is unchanged for existing consumers, so the backend deploys independently and
first. Presentation-order decisions stay in the repo that owns presentation.

**B — Backend returns nested clusters.** `DigestResult` becomes `[]Cluster{Lead, Members}`.
Cleaner semantics on the wire, but it is a breaking response-shape change requiring lock-step
deploys of both repos, and it moves lead-selection and group-ordering — both purely
presentational — into the API.

**C — Frontend-only, group by title similarity.** Requires no backend change, and is rejected:
this is the exact-match-after-normalization approach that this feature already failed with
twice (`pg_trgm` self-join, then `GROUP BY` on normalized title — both documented in
`2026-07-14-story-clustering-design.md`). It cannot catch the reworded-headline case that the
embedding clustering exists to catch, which is most of the real cross-outlet coverage.

## 1. Backend (Information-Broker)

Additive only — no schema change, no new index, no behaviour change for any other endpoint.

- `ArticleView` gains one field:

  ```go
  StoryClusterID *int64 `json:"story_cluster_id,omitempty"`
  ```

  A pointer because the column is NULL for articles the clustering job has not reached yet
  (its ticker interval, gated further by summarization activity — see the `ponytail:` comment
  at the digest query site).

- `buildDigestQuery` adds `a.story_cluster_id` to the SELECT list. The `LEFT JOIN` subquery,
  the `WHERE`, and the `ORDER BY` are untouched. `getArticlesDigest` scans it through a
  `sql.NullInt64` and assigns the pointer only when `Valid`.

- `buildArticlesQuery` does **not** select the column, so `/articles` responses leave the field
  nil and `omitempty` drops it from the JSON entirely. No existing consumer sees a difference.

## 2. Frontend API client (SmellyFeet)

`apiclient.Article` gains the mirroring field:

```go
StoryClusterID *int64 `json:"story_cluster_id,omitempty"`
```

Same pattern as `CrossFeedCount`: one shared `Article` type across every page, populated only
by the endpoint that returns it.

## 3. Grouping (SmellyFeet, internal/server)

One type and one pure function, in a new `digest.go` alongside the existing `digest_test.go`:

```go
// articleGroup is one story: the newest article covering it, plus every
// other article in the same cluster.
type articleGroup struct {
    Lead    apiclient.Article   // newest article in the cluster
    Members []apiclient.Article // the rest, newest-first; excludes Lead
}

func groupByCluster(articles []apiclient.Article) []articleGroup
```

A single pass over the incoming slice, which the backend already orders
`cross_feed_count DESC, publish_date DESC`:

- The first article seen for a given `StoryClusterID` becomes that group's `Lead`; subsequent
  articles with the same ID append to `Members`. Every member of a cluster shares the same
  `cross_feed_count`, so within a cluster the incoming order is purely `publish_date DESC` —
  first-seen is therefore the newest, which is the intended lead.
- Group order is order of first appearance, so the backend's ranking is preserved exactly.
- `StoryClusterID == nil` yields a singleton group. Unclustered articles are never merged with
  each other — a nil cluster ID means "not yet classified", not "same story".
- Same-feed duplicates within one cluster stay as ordinary members; no special-casing.

`handleDigest` calls `groupByCluster` on both slices returned by `GetDigest`.
`digestView.Important` and `.Other` change type from `[]apiclient.Article` to
`[]articleGroup`. Nothing else in the handler changes — range whitelisting, the `FetchFailed`
path, and cache headers are untouched.

## 4. Templates

**`list.html` refactor.** `articleCard` currently emits `<li class="reveal">` wrapping a card
`<div>`. The inner `<div>` is extracted into `{{ define "articleCardBody" }}`, leaving:

```
{{ define "articleCard" }}<li class="reveal">{{ template "articleCardBody" . }}</li>{{ end }}
```

Every existing caller (list, upcoming, article) renders byte-identically. This exists so the
digest group partial can reuse the card body while controlling its own `<li>`.

**New `articleGroup` partial in `digest.html`:**

```
<li class="reveal{{ if .Members }} grouped{{ end }}">
  {{ template "articleCardBody" .Lead }}               <- unmodified card div
  {{ if .Members }}
  <details>                                            <- no `open`: collapsed by default
    <summary>+{{ len .Members }} more sources</summary>
    <ul> one compact row per member: source pill · relTime · title → /article/{{ .ID }} </ul>
  </details>
  {{ end }}
</li>
```

The lead card must lose its bottom radius so it reads as one unit with the disclosure below
it, but `articleCardBody` receives an `Article` and has no knowledge of the group. Rather than
plumb a flag through the shared partial, the `<li>` carries a `grouped` class and
`tailwind.input.css` gains one rule joining the two children:

```css
.grouped > div:first-child { border-bottom-left-radius: 0; border-bottom-right-radius: 0; border-bottom-width: 0; }
.grouped > details          { border-top-left-radius: 0; border-top-right-radius: 0; }
```

This keeps `articleCardBody` byte-identical for its other four call sites, and the styling
concern stays in the stylesheet. It follows the existing precedent of hand-written rules in
`tailwind.input.css` for things Tailwind utilities cannot express under the CSP (the `.reveal`
`nth-child` stagger is there for the same reason).

- The `<details>` is a **sibling** of the card `<div>`, not a child. The card body's title link
  carries `after:absolute after:inset-0`, a stretched link covering the whole card; nesting the
  disclosure inside it would put that overlay on top of the summary and swallow the clicks.
- Native `<details>`/`<summary>` gives keyboard operation and screen-reader disclosure
  semantics with no JavaScript, matching the existing "everything else" disclosure on the same
  page and the "upcoming" one in `list.html`.
- A group with no members renders exactly as today: the card alone, full border radius, no
  disclosure element.
- `.reveal`'s staggered entrance is pure CSS `nth-child` on the `<li>`. One `<li>` per group
  keeps the stagger correct.

**Badge fix.** `articleCardBody`'s source badge becomes `{{ inc .CrossFeedCount }} sources`.
`cross_feed_count` is the count of *other* feeds (`distinct_feeds - 1`), so the current label
under-reports by one — a three-feed story badges "2 sources". `inc` is already in the funcmap.
The `{{ if gt .CrossFeedCount 1 }}` guard is unchanged, so the badge still renders only on
`/digest`, where `CrossFeedCount` is non-zero.

**Counts.** The `Important (N)` heading and the `everything else (N)` summary count groups
rather than articles — N distinct stories, not N rows. The empty-state condition
(`and (not .Important) (not .Other)`) works unchanged on the grouped slices.

## 5. Testing

- **`groupByCluster` table tests** (pure, no server): nil cluster IDs each become their own
  singleton group; articles sharing an ID merge into one group with `Lead` = first-seen;
  group order matches first-appearance order; a mixed nil/non-nil slice groups correctly;
  empty input returns an empty slice.
- **Handler render tests** (existing `digest_test.go` style, stub API + canned `DigestResult`):
  a multi-article cluster renders a `<details>` with `+N more sources`; a singleton renders no
  `<details>`; the rendered `<details>` carries no `open` attribute; headings report group
  counts; the `FetchFailed` path is unaffected.
- **Backend:** extend the existing `TestBuildDigestQuery` SQL-shape assertion to expect
  `a.story_cluster_id` in the SELECT list.
- One CSS rebuild (`./scripts/build-css.sh`) after the template edits.
- **Deploy order:** backend first — it is purely additive and the frontend tolerates the field
  being absent (every article becomes a singleton group, i.e. today's behaviour). Frontend
  once the field is live.

## Out of scope

- An expand-all / collapse-all control.
- Persisting expansion state across page loads or ranges.
- Grouping on `/` (live feed) or `/upcoming` — digest only.
- Any change to the clustering algorithm, its similarity threshold, or its cadence.
- Changing which articles land in `Important` vs `Other`; the split still keys off
  `cross_feed_count` exactly as today.
- Deduplicating the summary text shown across members (each member keeps its own).
