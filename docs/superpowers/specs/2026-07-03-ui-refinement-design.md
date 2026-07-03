# SmellyFeet UI Refinement — Design

**Date:** 2026-07-03
**Status:** Approved
**Goal:** Refine the dark intelligence-terminal theme for scanability and mobile correctness —
zero JS, strict CSP untouched, no new Go dependencies, no backend changes.

## Context

Live at feed.purecypher.com. Screenshot review found: raw feed URLs dominate every list card;
absolute timestamps; identical card weight with a redundant "READ FULL ARTICLE →" row; header
clips "ABOUT" and the page overflows horizontally at 390px; stats page is three cards in an
empty page; article page prints the original URL twice. Direction chosen: refine the existing
theme (not a redesign). Approach chosen: zero-JS overhaul (rejected: adding a small JS file for
auto-submit/keyboard nav — breaks the zero-JS/CSP guarantee).

## 1. Template funcs (internal/server, funcMap)

- `sourceName(feedURL string) string` — `url.Parse` → `Hostname()` → strip leading `www.`;
  returns the input unchanged on parse failure or empty hostname.
- `relTime(t any) string` — accepts `time.Time`/`*time.Time` like `formatDate`; "—" for
  nil/zero; "just now" < 1 min; "Nm ago" < 60 min; "Nh ago" < 24 h; "Nd ago" < 7 d; else
  `2006-01-02`. Core logic `relTimeAt(t time.Time, now time.Time) string` for testable clock;
  the absolute value stays available via `<time datetime="..." title="...">` markup.
- `cveID(title string) string` — first match of `CVE-\d{4}-\d{4,}` or "".

## 2. List cards (list.html)

Meta line: `[source pill] [CVE badge?] …spacer… [relTime]`. Source pill = compact bordered
mono badge showing `sourceName`, raw feed URL in `title` tooltip. CVE badge = accent-colored
mono badge showing the extracted ID, only when non-empty. Title remains the visual anchor;
summary keeps `line-clamp-3`. The "READ FULL ARTICLE →" row is removed; a small `→` appears
on card hover (CSS only). Source `<select>` options display `sourceName (count)`; option
values remain the raw feed URL so the `feed` query param is unchanged.

## 3. Article page (article.html)

Meta row uses the source pill + `relTime` (absolute in tooltip). The plain-text URL printed
beside the "Read original" button is removed (the button alone carries the link).

## 4. Stats page (stats.html + handlers)

`handleStats` additionally calls `ListFeeds`; on error the new section is omitted (stat cards
unchanged). New "// top sources" section: top 15 feeds sorted by ArticleCount descending —
each row: sourceName, count, horizontal amber bar. Bar widths are quantized server-side to
5% steps rendered as classes `bar-5` … `bar-100` (20 rules in assets/tailwind.input.css),
because CSP `style-src 'self'` forbids inline `style=""` widths. Last-fetch stat uses
`relTime`.

## 5. Mobile & chrome (partials.html, input CSS)

Header wraps/tightens below `sm:` (smaller wordmark + nav text/tracking) so all three nav
items fit at 390px with no horizontal overflow; `overflow-wrap: anywhere` safety on card meta.
Footer gains an Atom feed link (`/feed.xml`).

## 6. Testing & pipeline

Table tests for `sourceName`, `relTimeAt`, `cveID`. Render tests: list card shows the pill
text and not the raw URL; CVE badge appears for a CVE title and not otherwise; stats page
renders the top-sources section (and omits it when ListFeeds errors); article page no longer
contains the duplicate URL text. Existing suite stays green (`go test -race ./...` ≥ 80%
coverage). CSS rebuilt once via `./scripts/build-css.sh`; committed output updated.

## Out of scope

- True total pagination (broker API `count` is page size, not total).
- Backend/API changes, content dedup, JS interactivity, theme change.
