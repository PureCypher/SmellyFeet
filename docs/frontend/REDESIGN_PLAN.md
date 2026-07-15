# SmellyFeet Redesign Plan — "Signal Lamp"

**Phase 1 deliverable**, owned by Fable per the governing master prompt's model-routing policy. Companion document: `DESIGN_TOKENS.md`. Direction: **Signal Lamp**, chosen by the user from a live 3-option comparison (Amber Phosphor / Signal Lamp / NOC Wallboard) — keeps the shipped amber brand identity fully intact, adds a real severity system only where the Phase 0 audit found a genuine gap (feed health, error states).

**Status: draft for approval.** Per the master prompt, no implementation begins until this plan and `DESIGN_TOKENS.md` are reviewed and approved. This document was produced by Fable-model agents and then compiled and corrected against their own adversarial critique pass — see the "Compilation notes" section at the end for exactly what was fixed and why, so the review isn't starting from an unverified draft.

---

## 1. Design Objectives

Each objective has a manual acceptance check (no CI exists; no analytics exist or ever will — zero JS, no cookies).

**O1 — Time-to-scan the newest item.**
The newest article must be scannable without any interaction. On `/`, the first article card (title, source pill, age) renders fully within the first viewport at 1280×800 with default filters, and within one swipe-length at 390×844. The skip link jumps keyboard users past header and filter form directly to the card list.
*Check:* load `/` cold at both sizes; the first card title is visible without scrolling (desktop) or after at most one scroll gesture (mobile); `Tab` → `Enter` on the skip link lands focus at the content region.

**O2 — Time-to-identify source and age.**
Source and age are identifiable on every card in every route with zero hover, zero taps, on every input modality. Both the relative time and the absolute UTC time are visible text (`<time>`), and the source pill is visible text. Nothing information-bearing lives only in a `title` attribute.
*Check:* `grep -rn 'title="' internal/server/templates/` returns no attribute that is the sole carrier of a timestamp or feed URL.

**O3 — Distinguishable severity, confidence, freshness, and processing state.**
Signal Lamp gives each of these a distinct, non-overlapping vocabulary:

| Dimension | Vocabulary | Where |
|---|---|---|
| Severity (system health) | `--status-ok/warn/critical` dot + mono-caps text label (`OK` / `DEGRADED` / `FAILING`) + a sentence with real counts | Stats health panels, error pages, degraded callouts — the only sanctioned sites (see each route's §4 token-usage table below) |
| Confidence (editorial) | Amber `N SOURCES` cross-feed badge; Important/Other split on `/digest` | Article cards, digest sections |
| Freshness | Relative + absolute time pair on every card; "latest N ago" per source on `/stats` | All data routes |
| Processing state | Summarization panel on `/stats`; "No summary available." per-card state | Stats, cards |

*Check:* no `--status-*` token appears outside the locations enumerated in the four routes' §4 tables; every status dot has an adjacent text label (WCAG 1.4.1 — mandatory because `--status-warn` *is* the brand amber).

**O4 — Mobile usability without losing source attribution.**
At 320px: no horizontal page scroll (WCAG 1.4.10); source pill, CVE badge, N-sources badge, both time forms, and raw feed URLs all remain reachable — visible text or at most one native-disclosure tap away, never hover-gated (hover does not exist on touch). Interactive targets ≥ 24×24 CSS px (WCAG 2.5.8).
*Check:* devtools at 320px on all four routes; no horizontal scrollbar appears; tap every disclosure.

**O5 — Clear separation of upstream content vs. platform-generated LLM text.**
The article page's AI Summary callout already labels the platform-generated summary; that separation extends site-wide: list and digest card summaries are LLM-written and the `/about` copy states this plainly ("summarizes every article with a local LLM"). Upstream RSS content is never rendered unescaped — `html/template` contextual auto-escaping with **zero** `template.HTML` usage is the enforcement mechanism and must not regress.
*Check:* `grep -rn 'template.HTML' internal/` returns nothing.

**O6 — Low visual fatigue for extended analyst use.**
One dark theme (`color-scheme: dark`), no theme toggle. Color is rationed: the amber alpha ladder is fixed at 6/8/10% backgrounds and 25/50% borders (`DESIGN_TOKENS.md` §4), severity color appears only at the audited sites, glows exist only on status/live dots and are static — no pulse, nothing new for `prefers-reduced-motion` to fight. Metadata sits in `--text-muted`/`--text-structural`, keeping the reading surface (titles + summaries) the brightest thing on the page.
*Check:* reduced-motion emulation disables the card reveal entirely (shipped, must not regress); no animation runs longer than `--dur-reveal`; contrast spot-check against the token appendix table.

---

## 2. Information Architecture

### Global navigation model — keep, don't rebuild

The shipped shell already does the right thing: sticky header (`--z-header`), skip-to-content link (`--z-skip`), four-item nav (Feed / Digest / Stats / About) with `aria-label` on both landmarks and `aria-current="page"` on the active item, footer with the Atom link. **This is kept as-is structurally**; Phase 2/3 work on the shell is token/typography restyle only (`partials.html`). No hamburger, no dropdowns, no additional nav levels — four links fit inline at 11px mono down to 320px (confirmed in the Phase 0 mobile captures).

### Where controls and indicators live — one owner per concern

| Concern | Owner route | Rationale |
|---|---|---|
| Search, source filter, sort, filter chips | `/` only | The only route over the paginated articles endpoint; the GET form is the page's secondary action set. No other route grows a search box. |
| Time-range selection | `/digest` only | Single `range` select + Apply. Digest never gains source filtering — its unit is the story cluster, not the feed. |
| All health/status indicators (`--status-ok` anywhere) | `/stats` only | Health data (fetch counts, summarization stats) comes from `/stats`-family endpoints. A green dot on any other route would be invented data — the list and digest payloads carry no health fields. |
| Degraded/critical callouts | Any route, at the point of failure | Error states are local: source-list-unavailable on `/` (warn), digest-unavailable on `/digest` (critical), per-panel UNAVAILABLE on `/stats` (neutral), full error page (critical) as the fallback everywhere. |
| Static trust copy | `/about` only | Zero backend calls, deliberately the page that renders when everything else is down. |

### Digest ↔ Stats cross-linking without duplication

The two routes answer different questions about the same pipeline — *"what matters?"* (digest) vs. *"is it working?"* (stats) — and must not grow copies of each other's widgets:

- **Shared escape hatch to `/`:** both routes link *into* the feed filter rather than reimplementing it — source pills on digest cards and source names on the stats top-sources chart are plain `<a href="/?feed=…">`. One filter implementation, three entry points.
- **Clustering visibility lives on `/stats`, not `/digest`.** When the backend-gated cluster-coverage endpoint ships (see route §5 ledgers below), the coverage widget lands on the stats page. The digest itself only ever says "No cross-feed stories detected in this window" — neutral copy that stays honest while coverage is indistinguishable from absence. Digest never renders health numerals; stats never renders article content.
- **No summary counts on digest, no article lists on stats.** The digest's section counts (`IMPORTANT (5)`) are array lengths of its own payload, not stats-endpoint data; the stats page's numbers never enumerate articles.

---

## 3. Route Wireframes

All four routes below are zero-JS, native-HTML-only (`<details>`, `<select>`, GET forms, plain links), and use only the tokens defined in `DESIGN_TOKENS.md`. Real content shown is from the Phase 0 audit capture except where a wireframe explicitly flags illustrative placeholder values.

### Route `/` — Live Feed

**Primary action:** read an article (stretched-link card title). **Secondary actions:** apply filters (single submit button on the GET form), filter-by-source via pill, remove a chip, toggle upcoming, paginate.

#### Desktop wireframe (≥ `--bp-md`, container `--container` 64rem)

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ [skip to content — visually hidden; on focus: --accent fill, --text-on-accent,   │
│  z: --z-skip]                                                                    │
├──────────────────────────────────────────────────────────────────────────────────┤
│ STICKY HEADER  --surface-header-blur + --blur-header · border-b --border-default │
│  ● INFORMATION_BROKER            [FEED] [DIGEST] [STATS] [ABOUT]                 │
│  ▲ live dot: --accent + --glow-accent   ▲ nav: --font-mono --text-2xs caps,      │
│    (decorative, aria-hidden)              active = aria-current="page" +         │
│                                           --accent text + --surface-overlay pill │
├──────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  // LIVE FEED                       ← eyebrow: --font-mono --text-2xs,           │
│                                       --tracking-label-wide, --accent-dim        │
│  Latest intelligence                ← h1: --text-2xl --text-primary              │
│                                                                                  │
│  ╔══ FILTER FORM (GET → /) ═══ --surface-raised · --border-default · r-xl ════╗  │
│  ║ SEARCH (min 2 chars)      SOURCE                    SORT                   ║  │
│  ║ ┌─────────────────────┐  ┌───────────────────────┐ ┌──────────────┐ ┌────┐ ║  │
│  ║ │ title, summary, or  │  │ All sources         ▾ │ │ Newest first▾│ │APP-│ ║  │
│  ║ │ content…            │  │  cvefeed.io (6,308)   │ │  Oldest first│ │LY  │ ║  │
│  ║ └─────────────────────┘  │  bleepingcomputer.com │ └──────────────┘ └────┘ ║  │
│  ║  ▲ <input minlength=2>   │  (…) — 100+ options,  │  ▲ native      ▲--accent║  │
│  ║    label + "(min 2       │  by article count     │  <select>s     fill,    ║  │
│  ║    chars)" hint visible  └───────────────────────┘                --text-  ║  │
│  ║    in --text-muted, not title-attr-only                           on-accent║  │
│  ╚═══════════════════════════════════════════════════════════════════════════╝  │
│                                                                                  │
│  FILTERED: [source: cvefeed.io ×] [search: "tornado" ×] [oldest first ×]  clear  │
│  ▲ chips: --surface-overlay, --border-accent-faint, each chip is a plain <a>    │
│    that removes its own param. Rendered only when active (backend already       │
│    hides the search chip for <2-char ignored queries — keep that).              │
│  ┌ SOURCE CONTEXT STRIP (only when ?feed= active) ──────────────────────────┐   │
│  │ FEED URL: https://cvefeed.io/rssfeed/latest.xml      ← raw feed_url,     │   │
│  │ --font-mono --text-2xs --text-structural, visible — no longer hover-only │   │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                  │
│  ▸ UPCOMING (7) — FUTURE-DATED WEBINARS & EVENTS                                 │
│  ▲ native <details>/<summary>, --surface-raised-translucent, summary in         │
│    --font-mono --text-2xs caps --text-muted; page 1 + default sort only;        │
│    summary gets the global :focus-visible ring (DESIGN_TOKENS.md §6)            │
│                                                                                  │
│  ┌─ ARTICLE CARD ── --surface-raised · --border-default · --radius-xl ────────┐  │
│  │ [bleepingcomputer.com]                      19m ago · 2026-07-14 13:41 UTC │  │
│  │  ▲ source pill: <a href="/?feed=…">,         ▲ <time>: rel time in         │  │
│  │    --accent-dim mono, --surface-page bg        --text-muted + NEW visible  │  │
│  │                                                absolute UTC in --text-2xs  │  │
│  │                                                --text-structural mono —    │  │
│  │                                                no longer title-attr-only   │  │
│  │ SonicWall warns of SMA1000 flaws exploited in zero-day attacks, patch now  │  │
│  │  ▲ h2 --text-lg --text-primary, stretched link → /article/{id}             │  │
│  │ SonicWall has issued an urgent warning about two critical vulnerabilities  │  │
│  │ (CVE-2026-15409 and CVE-2026-15410) in its SMA1000 security appliance…     │  │
│  │  ▲ summary: --text-sm --text-body, line-clamp-3                            │  │
│  └─────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                  │
│  ┌─ ARTICLE CARD with badges ─────────────────────────────────────────────────┐  │
│  │ [cvefeed.io] [CVE-2026-49855] [3 sources]   58m ago · 2026-07-14 13:02 UTC │  │
│  │               ▲ CVE badge + cross-feed badge: --accent text,               │  │
│  │                 --surface-accent-wash bg, --border-accent-faint —          │  │
│  │                 brand amber, NOT status tokens (they are metadata,         │  │
│  │                 not severity — Signal Lamp leaves them untouched)          │  │
│  │ CVE-2026-49855 - tornado AsyncHTTPClient accumulates decompressed          │  │
│  │ chunks without size limit (gzip bomb)                                      │  │
│  │ CVE-2026-49855 is a high-severity vulnerability in Tornado, an             │  │
│  │ asynchronous networking library for Python. Prior to version 6.5.6…        │  │
│  └─────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                  │
│  (… 20 cards/page, riseIn reveal via nth-child --stagger-step classes,           │
│     disabled under prefers-reduced-motion — shipped, must not regress)           │
│                                                                                  │
│  ┌────────┐                 PAGE 2                              ┌────────┐       │
│  │ ← PREV │        ▲ --font-mono --text-2xs caps.               │ NEXT → │       │
│  └────────┘          NO "of Y" — the articles endpoint          └────────┘       │
│                      returns no total; a page count would                        │
│                      be invented data. Disabled edge =                           │
│                      --text-faint <span>, not a link.                            │
│                                                                                  │
├──────────────────────────────────────────────────────────────────────────────────┤
│ FOOTER  INFORMATION BROKER · THREAT INTELLIGENCE FEED            ATOM FEED       │
└──────────────────────────────────────────────────────────────────────────────────┘
```

#### Mobile wireframe (390px; one column below `--bp-sm`, no horizontal scroll — WCAG 1.4.10 at 320px)

```
┌──────────────────────────────────┐
│ ● INFORMATION_BROKER             │
│ FEED  DIGEST  STATS  ABOUT       │  ← nav wraps under wordmark (shipped);
├──────────────────────────────────┤    tighter --tracking-caps variant
│ // LIVE FEED                     │
│ Latest intelligence              │
│                                  │
│ SEARCH (MIN 2 CHARS)             │  ← form fields stack full-width
│ ┌──────────────────────────────┐ │
│ │ title, summary, or content…  │ │
│ └──────────────────────────────┘ │
│ SOURCE            SORT           │  ← two selects share one row
│ ┌─────────────┐  ┌────────────┐  │    (shipped); native <select> gets
│ │ All sources▾│  │Newest fir▾ │  │    OS-native picker — free on mobile
│ └─────────────┘  └────────────┘  │
│ ┌───────┐                        │
│ │ APPLY │                        │
│ └───────┘                        │
│                                  │
│ FILTERED: [source: cvefeed.io ×] │  ← chips wrap; ≥24px CSS min target
│ [search: "tornado" ×]  clear all │    size incl. padding (WCAG 2.5.8)
│ FEED URL: https://cvefeed.io/    │  ← raw feed_url wraps with
│ rssfeed/latest.xml               │    overflow-wrap:anywhere — stays
│                                  │    visible, never hover-gated
│ ▸ UPCOMING (7) — FUTURE-DATED    │
│   WEBINARS & EVENTS              │
│                                  │
│ ┌──────────────────────────────┐ │
│ │ [cvefeed.io]                 │ │  ← badge row wraps to its own
│ │ [CVE-2026-49855] [3 sources] │ │    lines; nothing truncates away
│ │ 58m ago · 2026-07-14 13:02 UTC│ │  ← time moves to its own line
│ │                              │ │    below badges (was right-aligned
│ │ CVE-2026-49855 - tornado     │ │    same-row on desktop)
│ │ AsyncHTTPClient accumulates  │ │
│ │ decompressed chunks without  │ │
│ │ size limit (gzip bomb)       │ │
│ │ CVE-2026-49855 is a high-    │ │
│ │ severity vulnerability in    │ │
│ │ Tornado, an asynchronous…    │ │  ← summary stays clamped at 3 lines
│ └──────────────────────────────┘ │
│ (… more cards …)                 │
│                                  │
│ ← PREV      PAGE 2      NEXT →   │  ← full-width row; targets ≥24px
├──────────────────────────────────┤
│ INFORMATION BROKER · THREAT      │
│ INTELLIGENCE FEED                │
│ ATOM FEED                        │
└──────────────────────────────────┘
```

**Collapses / reorders:** filter form stacks (search full-width, source+sort share a row, apply below); card meta wraps into badge row + time row; summary clamp stays at 3 lines.
**Abbreviates:** absolute timestamp drops seconds (`2026-07-14 13:02 UTC`); source pill truncates with ellipsis only if a single token exceeds the column — the raw URL in the source-context strip wraps instead of truncating.
**Must never disappear:** source pill, CVE badge, N-sources badge, relative **and** absolute time (both now visible text, not title attributes), the upcoming disclosure, filter chips, pagination, skip link. Nothing on this route may be hover-only — hover does not exist on touch.

#### States

**Loading.** No loading skeleton, deliberately — and it should stay that way. A client-side skeleton requires JS; a streamed-shell CSS skeleton requires flushing `<body>` before data, and `Server.render()` intentionally buffers the full template so a render error yields a clean 500 instead of a torn page. The actual "loading state" is: (1) warm path — Cloudflare full-URL edge cache serves the complete page, the only visible indicator is the browser's native progress bar for tens of milliseconds; (2) cold path — origin blocks on Information-Broker for at most the API client timeout, then renders either the full page or the error state below. No intermediate UI exists or is needed.

**Stale.** Cache-Control on this route is `s-maxage=60, stale-while-revalidate=120` — within that window a visitor may see a page up to ~2 minutes old (new articles, updated cross-feed counts not yet reflected). This is not designed as a visible "stale" indicator: every card's relative timestamp (`19m ago`) is computed at render time and baked into the cached HTML, so it stays honest regardless of when the cache was populated — a visitor never sees a false "just now." No separate staleness UI is needed; the existing timestamp *is* the staleness disclosure.

**Empty search result.** Rendered when the applied filters match nothing. Neutral styling — **no status token**: absence of results is not a fault, and coloring it red would dilute the severity system.

```
│ FILTERED: [search: "quantumblorpX" ×]  clear all      ← chips stay visible
│ ┌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌┐   so escape is one click
│ ┆                                                 ┆
│ ┆        NO ARTICLES MATCH THESE FILTERS          ┆  ← --font-mono --text-sm
│ ┆   Try fewer words, or remove a filter above.    ┆    --text-muted, dashed
│ ┆              [ CLEAR ALL FILTERS ]              ┆    --border-default,
│ ┆                                                 ┆    --pad-empty-y
│ └╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌┘
```

The `<2 characters` case never reaches this state: `minlength="2"` blocks submission natively in the browser, and the backend's existing ignore-behavior plus the already-shipped chip suppression remain as the server-side backstop.

**Full-page error (Information-Broker unreachable).** The `error` template, upgraded from today's ad-hoc reds to the `--status-critical` family. This is one of the two sanctioned severity sites on this route.

```
│ ┌── --status-critical-bg · --status-critical-border · --radius-2xl ──┐
│ │                                                                    │
│ │   // ERROR                       ← eyebrow, --font-mono --text-2xs,│
│ │                                    --status-critical               │
│ │   Something went wrong           ← h1, --status-critical           │
│ │   The intelligence backend is unreachable. Recently cached         │
│ │   pages may still be served at the edge.   ← --text-muted          │
│ │                                                                    │
│ │   ← BACK TO FEED                 ← --accent link (brand action     │
│ │                                    stays amber even on error page) │
│ └────────────────────────────────────────────────────────────────────┘
```

Header, nav, and footer render normally around it — the shell never depends on the API.

**Partial data: source list unavailable (feeds fetch fails, articles succeed).** Today this is silent: `handlers.go:170-172` nils the feed list and the dropdown just shows "All sources". The redesign makes it explicit — the second sanctioned severity site on this route, and it is *degraded*, not failing, so it uses the warn family:

```
│ ║ SOURCE                                                    ║
│ ║ ┌───────────────────────┐                                 ║
│ ║ │ All sources         ▾ │  (only option)                  ║
│ ║ └───────────────────────┘                                 ║
│ ║ ⚠ DEGRADED — source list unavailable; filtering by source ║
│ ║   is temporarily off                                      ║
│ ║   ▲ --status-warn-bg callout, --status-warn-border,       ║
│ ║     "DEGRADED" text label in --text-muted mono caps       ║
```

Articles still render in full below. The inverse partial (articles fail, feeds succeed) has no partial rendering path today and falls through to the full-page error; acceptable — do not build a half-page for it.

#### Severity / status token usage on this route — never color-only

| Location | Tokens | Non-color pairing (WCAG 1.4.1) |
|---|---|---|
| Full-page error | `--status-critical`, `--status-critical-bg`, `--status-critical-border` | "// ERROR" mono eyebrow + "Something went wrong" heading + message body — three text carriers; color is reinforcement only |
| Source-list-unavailable callout | `--status-warn` (= `--accent` by design), `--status-warn-bg`, `--status-warn-border` | Literal "DEGRADED" text label in mono caps + full sentence. Mandatory here: warn *is* the brand hue, so an unlabeled amber box on an amber page carries zero information |
| Header live dot | `--accent` + `--glow-accent` | `aria-hidden` decorative brand mark — explicitly **not** a status indicator; conveys nothing, so needs no label |
| CVE / N-sources badges, chips, buttons, links | `--accent` family only | Metadata, not severity — Signal Lamp deliberately leaves them amber |
| Empty state, disabled pagination ends | No status tokens (`--text-muted` / `--text-faint`) | Not fault states; disabled Prev/Next are `<span>`s, structurally non-interactive, not merely grayed |

No `--status-ok` appears anywhere on `/` — feed health is a stats-page concern and there is no per-feed health data in the list payload. A green dot here would be invented data.

#### Backend work: needed vs. zero-backend

**Buildable today with zero Information-Broker changes — everything in this wireframe:**

- Visible absolute UTC timestamp on every card (`PublishedAt` is already in the payload; today it's rendered into a `title` attribute — this is a template-only change)
- Raw feed URL in the source-context strip (`feed_url` is already in the payload and already used for the filter link)
- `minlength="2"` + visible min-chars hint on search (native constraint validation, mirrors existing backend behavior)
- Explicit "source list unavailable / DEGRADED" partial state (frontend already detects the failure at `handlers.go:172`; it just renders nothing)
- Error-page migration to the `--status-critical` token family (CSS/template only)
- Empty-state upgrade with chips-preserved + clear-all (template only)
- All token work: chips, badges, focus rings, type scale, reveal stagger (CSS build only, CSP-safe)

**Requires new Information-Broker work — each deliberately absent from this wireframe:**

- **"N results" / "Page X of Y"** — articles list endpoint returns no total count. Omitted rather than faked; propose a `total` field only if pagination UX proves painful in Phase 3, not by default.
- **Per-feed health markers on source pills or dropdown options** — no per-feed fetch-health endpoint exists (only aggregate successful/failed counts on stats). Out of scope for `/` under Signal Lamp regardless; if ever wanted, it needs a justified API addition first.

Nothing on this route justifies an API change today.

---

### Route `/digest` — Cross-Feed Importance Digest

Ground truth: template `digest` renders a range `<select>` + Apply GET form, a "since" line, an `Important` list (story cluster shared by ≥2 other feeds — i.e. covered independently by **three or more feeds total**: the source feed plus at least two others), and an `Other` list inside a collapsed `<details>`. The endpoint (`GET /articles/digest?range=daily|weekly|monthly`) is **completely unpaginated** — the monthly screenshot confirms a single very long page.

**Primary action:** range change (select + Apply submit — full page GET navigation, edge-cached per full URL). **Secondary actions:** open article (stretched link), filter by source (pill → `/?feed=…`), expand "everything else" (native details).

#### Desktop wireframe (≥ `--bp-md`, `--container` 64rem)

```
┌────────────────────────────────────────────────────────────────────────────────────────────┐
│ [skip to content]                                     (visually hidden until :focus-visible,│
│                                                        amber fill, --text-on-accent, z-skip)│
│ ● INFORMATION_BROKER            FEED   [DIGEST]   STATS   ABOUT                             │
│   (live dot: --accent +          (nav, --font-mono caps, --tracking-label;                  │
│    --glow-accent, shipped)        DIGEST has aria-current="page", full --accent border)     │
├────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                             │
│  // DIGEST                                  (eyebrow: --font-mono --text-2xs caps,          │
│                                              --tracking-label-wide, --accent-dim)           │
│  Cross-feed importance digest               (h1: --text-2xl, --text-primary)                │
│  SINCE 2026-07-14 00:00 UTC                 (mono --text-2xs, --text-muted — VISIBLE text,  │
│                                              <time datetime="…">, not a title attribute)    │
│                                                                                             │
│  ┌─ form method=GET action=/digest ── --surface-raised, --border-default, --radius-xl ───┐  │
│  │  RANGE                              (label: mono --text-3xs caps, --text-muted)        │  │
│  │  [ Daily          ▾ ]   [ APPLY ]                                                      │  │
│  │   (native <select>:      (submit: --accent fill, --text-on-accent,                     │  │
│  │    daily/weekly/monthly,  hover --accent-bright, --radius-lg)                          │  │
│  │    --surface-page bg,                                                                  │  │
│  │    --border-default)                                                                   │  │
│  └────────────────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                             │
│  IMPORTANT (5)                              (real <h2>: mono --text-2xs caps,               │
│                                              --tracking-label, --text-muted — count is      │
│                                              len(.Important), computed frontend-side)       │
│  Stories reported independently by three or more feeds (the source feed plus at least       │
│  two others).                              (one-line explainer, --text-sm --text-muted —    │
│                                              matches the backend's real ≥2-other-feeds rule) │
│                                                                                             │
│  ┌─ articleCard ── --surface-raised, --border-default, --radius-xl, --pad-card ──────────┐  │
│  │ [bleepingcomputer.com] [CVE-2026-55040] [4 SOURCES]        4h ago · 2026-07-15 06:12 UTC │
│  │  (source pill links      (CVE badge:     (cross-feed badge:  (<time>: relative +       │  │
│  │   /?feed=…, --accent-dim, --accent text,  --accent text,      absolute BOTH visible,   │  │
│  │   --border-default;       --border-accent- --border-accent-   mono --text-2xs,         │  │
│  │   sr-only span carries    faint,           faint,             --text-muted)            │  │
│  │   the full raw feed_url)  --surface-       --surface-                                  │  │
│  │                           accent-wash)     accent-wash)                                │  │
│  │ CVE-2026-55040: Microsoft SharePoint JWT Token Authentication Bypass                   │  │
│  │  (h3 stretched-link title: --text-lg, --text-primary, hover --accent)                  │  │
│  │ Rapid7 has confirmed active exploitation of a JWT authentication bypass in             │  │
│  │ on-premises SharePoint servers. Microsoft shipped an out-of-band fix in the July…      │  │
│  │  (summary: --text-size-prose, --text-body, clamped 3 lines)                            │  │
│  └────────────────────────────────────────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐  │
│  │ [cvefeed.io] [CVE-2026-15409] [3 SOURCES]                  9h ago · 2026-07-15 01:03 UTC │  │
│  │ SonicWall SMA appliances targeted in zero-day attacks (CVE-2026-15409, -15410)         │  │
│  │ SonicWall has identified two critical vulnerabilities in SMA 100 series appliances…    │  │
│  └────────────────────────────────────────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────────────────────────────────────┐  │
│  │ [krebsonsecurity.com] [2 SOURCES]                          1d ago · 2026-07-14 09:41 UTC │  │
│  │ U.S. Treasury Sanctions VPN Provider and Crypto Exchange Behind Ransomware Losses      │  │
│  │ The Treasury Department has imposed sanctions on a virtual private network provider…   │  │
│  └────────────────────────────────────────────────────────────────────────────────────────┘  │
│   … (2 more Important cards) …                                                              │
│                                                                                             │
│  <h2 class="sr-only">Everything else</h2>   (heading kept OUTSIDE <summary> — heading roles │
│                                              inside summary are stripped by some AT)        │
│  ┌─ <details> collapsed by default ── --surface-raised-translucent, --border-default ────┐  │
│  │ ▸ EVERYTHING ELSE (61)                     (<summary>: mono --text-2xs caps,           │  │
│  │                                             --tracking-label-wide, --text-muted,       │  │
│  │                                             hover --accent, focus ring per §6;         │  │
│  │                                             native disclosure triangle = the icon)     │  │
│  └────────────────────────────────────────────────────────────────────────────────────────┘  │
│    (expanded: same articleCard list, single-source items, no cross-feed badge)              │
│                                                                                             │
├────────────────────────────────────────────────────────────────────────────────────────────┤
│  footer: INFORMATION_BROKER · [ATOM FEED]   (mono --text-xs, --text-muted)                  │
└────────────────────────────────────────────────────────────────────────────────────────────┘
```

Note: the three example cards above (SharePoint JWT bypass, SonicWall SMA, Treasury sanctions) are plausible-but-illustrative — they were not individually re-verified against a live Phase 0 capture the way the `/` route's examples were (those reused literal screenshot content). Real headlines will populate the page at implementation time; only the layout, badge semantics, and cross-feed counts (2/3/4 sources, all ≥2-other-feeds-qualifying per the fixed threshold above) are load-bearing.

**Changes vs shipped, all presentational:** visible absolute timestamp next to the relative one (was title-attribute-only — audit flagged this as hover-only/inaccessible); sr-only raw `feed_url` inside the source pill's accessible name (same fix, same reason); `IMPORTANT` count in the heading (derivable from array length — the shipped page already does this for "everything else"); one-line explainer under Important, worded to match the backend's actual threshold; sr-only `<h2>` before the details element.

#### Mobile wireframe (< `--bp-sm`, one column, no horizontal scroll at 320px)

```
┌──────────────────────────────────────┐
│ [skip to content]                    │
│ ● INFORMATION_BROKER                 │
│ FEED  [DIGEST]  STATS  ABOUT         │  nav wraps to its own row; stays 4 plain
│                                      │  links — no hamburger (zero JS)
├──────────────────────────────────────┤
│ // DIGEST                            │
│ Cross-feed importance digest         │
│ SINCE 2026-07-14 00:00 UTC           │  never disappears
│                                      │
│ ┌─ GET /digest ───────────────────┐  │
│ │ RANGE                           │  │
│ │ [ Daily                     ▾ ] │  │  select goes full-width
│ │ [           APPLY             ] │  │  submit goes full-width below it
│ └─────────────────────────────────┘  │
│                                      │
│ IMPORTANT (5)                        │
│ Stories reported independently       │
│ by three or more feeds.              │
│                                      │
│ ┌─────────────────────────────────┐  │
│ │ [bleepingcomputer.com]          │  │  meta row wraps: pill first,
│ │ [CVE-2026-55040] [4 SOURCES]    │  │  badges second — nothing truncates
│ │ CVE-2026-55040: Microsoft       │  │  to invisibility
│ │ SharePoint JWT Token            │  │
│ │ Authentication Bypass           │  │
│ │ Rapid7 has confirmed active     │  │
│ │ exploitation of a JWT           │  │
│ │ authentication bypass in…       │  │  summary clamps to 2 lines on mobile
│ │ 4h ago · 2026-07-15 06:12 UTC   │  │  time moves BELOW summary, own row,
│ └─────────────────────────────────┘  │  both forms still visible
│  … more Important cards …            │
│                                      │
│ ┌─────────────────────────────────┐  │
│ │ ▸ EVERYTHING ELSE (61)          │  │  summary row: full-width tap target,
│ └─────────────────────────────────┘  │  min 24x24px per WCAG 2.5.8 (easily met)
│                                      │
│ footer: [ATOM FEED]                  │
└──────────────────────────────────────┘
```

**What collapses/reorders:** form fields stack full-width; card meta splits into two wrapping rows; timestamp moves to its own row under the summary; summary clamp tightens 3 → 2 lines; card padding steps `--pad-card` → `--pad-card-sm`.

**What must never disappear:** the since line, both time forms (relative *and* absolute — the absolute one is the audit fix and must not retreat back into a title attribute), the CVE and N-sources badges, the source pill, section counts, the Atom link. Nothing on this page is hover-only or title-only after this redesign.

#### States

**Loading.** No client-side loading state exists, by design. Rendering is fully server-side with zero JS: the browser's native navigation indicator is the loading state, and full-URL edge caching (`s-maxage` + `stale-while-revalidate`) makes the common case ~instant. No skeletons — a skeleton requires client fetch, which this architecture forbids. Nothing to wireframe.

**Stale.** Same cache profile as `/` (`s-maxage=60, stale-while-revalidate=120`). The "since {date}" line and each card's relative time remain accurate regardless of cache age since they're computed at render time and baked into the cached HTML. A visitor within the SWR window sees a snapshot that's honestly labeled by its own timestamps — no separate staleness indicator needed.

**Empty window** (`.Important` and `.Other` both empty — e.g. daily range during a quiet ingest gap):

```
│ // DIGEST                                        │
│ Cross-feed importance digest                     │
│ SINCE 2026-07-15 00:00 UTC                       │
│ [ Daily ▾ ] [ APPLY ]                            │
│                                                  │
│ ┌ ─ ─ ─ dashed --border-default, --radius-xl ─ ┐ │
│ │                                              │ │
│ │     No articles found in this window.        │ │   --pad-empty-y (generous space IS
│ │                                              │ │   the explicit-unavailable style)
│ │     Try a wider range: [Weekly] · [Monthly]  │ │   plain links to /digest?range=…
│ │                                              │ │   (frontend-only addition)
│ └ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘ │
```

Neutral styling (`--text-muted` on `--surface-raised`). An empty window is not an error — **no status token here**.

**Error** (broker digest call fails). Today this falls through to the hard error page; redesign renders the page shell + range form with an inline critical callout, so the user can still switch range or navigate away:

```
│ // DIGEST                                        │
│ Cross-feed importance digest                     │
│ [ Daily ▾ ] [ APPLY ]                            │
│                                                  │
│ ┌── --status-critical-bg, --status-critical-border, --radius-xl ──┐
│ │ ● DIGEST UNAVAILABLE                                            │
│ │   (dot: --status-critical + --glow-critical, --status-dot-size; │
│ │    label: mono --text-2xs caps — dot is NEVER the only signal)  │
│ │ The digest backend could not be reached. The rest of the        │
│ │ site may still be available.  [Back to feed →]                  │
│ │   (body: --text-sm --text-body; link: --accent)                 │
│ └─────────────────────────────────────────────────────────────────┘
```

**Partial data** — three real cases:

1. *Important empty, Other populated* (no multi-feed clusters in the window, or the clustering job hasn't covered it yet). Replace the Important list with an honest one-liner, then render Other as normal:
   ```
   │ IMPORTANT (0)                                          │
   │ No cross-feed stories detected in this window.         │  --text-sm --text-muted
   │ 61 single-source articles below.                       │
   │ ▸ EVERYTHING ELSE (61)                                 │
   ```
   The frontend **cannot distinguish** "clustering hasn't run" from "genuinely no shared stories" — see backend item B2 below. Until then the copy stays neutral ("detected"), never claims certainty.
2. *Article without a summary* (nullable `summary`): card shows "No summary available." in italic `--text-muted` (not `--text-faint` — it carries information). Shipped behavior, kept.
3. *Unusually large unpaginated window* (monthly can return hundreds of articles; the payload arrives regardless). The affordance is containment, not truncation: "everything else" stays collapsed by default with its count visible, so the initial paint is bounded even when the DOM is not. **No frontend render cap** — hiding rows with no pagination to reach them fabricates a smaller dataset. The real fix is backend item B1 below; until it lands, the count in the summary row is the honest signal of window size.

#### Severity / status token usage on this route

| Location | Tokens | Non-color pairing |
|---|---|---|
| Error callout ("digest unavailable") | `--status-critical`, `--status-critical-bg`, `--status-critical-border`, `--glow-critical`, `--status-dot-size` | Dot always paired with mono-caps text label "DIGEST UNAVAILABLE" plus a body sentence. Glow is decorative only. |
| Header live-indicator dot | `--accent` + `--glow-accent` (shipped, unchanged) | Sits beside the "INFORMATION_BROKER" wordmark; decorative brand element, conveys no state on this page. |

That is the complete list. **No `--status-ok` and no `--status-warn` anywhere on `/digest`** — importance is an editorial ranking, not a health state, and it stays brand-amber (CVE badge, N-sources badge, active nav). This is the Signal Lamp rule applied: severity color only where real system state exists, which on this route is only the failure case. Nothing on the page is ever color-only: badges are text ("4 SOURCES", "CVE-2026-55040"), the disclosure state is the native triangle + open/closed geometry, and the status dot always carries a text label (WCAG 1.4.1).

#### Backend work: needed vs. zero-change

**Buildable today, zero Information-Broker changes:**
- Visible absolute timestamp beside relative time (`published_at` already in every `ArticleView`).
- sr-only raw `feed_url` in the source pill's accessible name (field already in payload).
- Counts in "IMPORTANT (N)" and "EVERYTHING ELSE (N)" (array lengths; the latter already ships).
- Explainer line under Important, worded to match the real threshold; sr-only `<h2>` before the details element.
- Empty-window state with range-suggestion links.
- Inline error callout replacing the hard error page for digest fetch failure.
- "Important empty but Other populated" partial state.
- All token/typography restyling (pure CSS, Phase 2/3).

**Needs Information-Broker changes — flagged, not assumed, each justified individually:**
- **B1 — digest pagination/limit param** (e.g. `?range=monthly&limit=&offset=`, or a per-section cap with a `truncated` flag). Justification: the endpoint returns every article in the window; monthly volume grows unboundedly with ingest rate (cvefeed.io alone contributes 6,308 articles all-time), so payload size, render time, and edge-cache object size all grow without ceiling. This is the open item flagged in the Phase 0 audit (§9 item 7) as not yet decided — the wireframe deliberately contains it with the collapsed details rather than inventing a pagination UI over a parameter that does not exist. No "load more" affordance is drawn until B1 ships.
- **B2 — cluster-coverage signal** (e.g. a clustering-status field on the digest response, or the missing cluster/embedding-coverage stats endpoint — the `story_cluster_id` / `summary_embedding` columns already exist in Postgres). Justification: lets the frontend distinguish "clustering has not processed this window yet" from "no multi-feed stories exist," so the Important-empty partial state can say which one is true instead of hedging.

Explicitly **not** needed for this route: total counts (derivable client-side from the full payload), a time-series endpoint, and the summarization-stats endpoint (stats-page concern, not digest).

---

### Route `/stats` — Statistics

**Cache:** `s-maxage=30, stale-while-revalidate=60` (unchanged). **Actions on this page** — primary: source-name links → filtered feed (`/?feed=…`); secondary: the `details` disclosure for full URLs/timestamps, nav links, skip link, Atom link. Nothing else is interactive; the page has no forms.

#### Data inventory (drives every widget below)

| Widget | Data source | Status today |
|---|---|---|
| Stat tiles (articles / sources / last fetch) | `GET /stats` | **Shipped** |
| Articles collected (24h / 7d / 30d) | `GET /stats` | **Shipped** (commit `1f06259`) |
| Top sources (top 15 by count) | `GET /feeds` | **Shipped** |
| Ingestion health (success/fail 24h, avg fetch ms) | `GET /stats` — `successful_fetches_24h`, `failed_fetches_24h`, `avg_fetch_time_ms` already returned, not decoded | **Frontend decode only** (`apiclient.go:55-62`) |
| Per-source "latest article" age | `GET /feeds` — `latest_article` already returned, not decoded | **Frontend decode only** |
| Summarization health | `GET /summarization/stats` — exists, never called | **Frontend call only** |
| Story clustering / embedding coverage | nothing aggregates `story_cluster_id` / `summary_embedding` | **Needs new broker endpoint** |
| Ingestion trend (day-by-day series) | only point-in-time window aggregates exist | **Needs new broker endpoint** |
| Per-feed error rates / per-feed status dots | `fetch_logs` rows exist, no per-feed join exposed | **Needs new broker endpoint** |

Numeric values in the wireframes below are the real capture-time values from the Phase 0 screenshots (50,913 articles / 130 sources / cvefeed.io 6,308 …). **Values for the three fetch-health fields, the summarization stats, and the per-source "latest N ago" / last-fetch absolute-timestamp pair are illustrative placeholders** — the underlying fields are real (or, for the last one, already shipped), but the specific numbers shown here were not individually measured against a live response; they illustrate shape and formatting only.

#### Desktop wireframe (≥ 768px, single centered column, `--container` 64rem)

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│ [skip to content — visible on focus, amber fill, --text-on-accent, z: --z-skip]         │
│ ● INFORMATION_BROKER                          FEED    DIGEST    STATS    ABOUT          │
│   (live dot: --accent + --glow-accent)                          ^^^^^ aria-current      │
└────────────────────────────────────────────────────────────────────────────────────────┘

  // SYSTEM                                        (eyebrow: mono --text-2xs, --accent-dim)
  Statistics                                       (h1, --text-2xl, --text-primary)

  ┌─ TOTAL ARTICLES ────────────┐ ┌─ SOURCES ───────────────────┐ ┌─ LAST FETCH ────────────────┐
  │                             │ │                             │ │                             │
  │  50,913                     │ │  130                        │ │  3m ago                     │
  │  (--text-4xl, --accent,     │ │  (--text-4xl,               │ │  2026-07-15 09:41:07 UTC    │
  │   tabular-nums)             │ │   --text-primary)           │ │  (absolute time now VISIBLE │
  │                             │ │                             │ │   mono --text-xs --text-    │
  │                             │ │                             │ │   muted — was title-attr    │
  └─────────────────────────────┘ └─────────────────────────────┘ │   only; audit fix)          │
                                                                  └─────────────────────────────┘

  // INGESTION HEALTH                                    ── NEW · frontend decode only ──
  ┌──────────────────────────────────────────────────────────────────────────────────────┐
  │  ◉ DEGRADED — 26 of 3,130 fetches failed in the last 24h                              │
  │  (dot: --status-warn + --glow-warn, --status-dot-size; label "DEGRADED": mono         │
  │   --text-2xs caps in --text-muted; sentence: --text-sm --text-body. NEVER dot alone.) │
  │  ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄ (--border-subtle) ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄  │
  │  SUCCESSFUL (24H)          FAILED (24H)             AVG FETCH TIME                    │
  │  3,104                     26                       412 ms                            │
  │  (labels: --text-structural mono --text-3xs; numerals: --text-2xl tabular-nums;      │
  │   FAILED numeral in --status-critical ONLY when > 0, and the header sentence          │
  │   always restates the count in words — number is never color-alone)                   │
  └──────────────────────────────────────────────────────────────────────────────────────┘
   Panel border: --status-warn-border (25% warn tint), bg --status-warn-bg when DEGRADED;
   --border-default + no tint when OK. States: OK / DEGRADED / FAILING, derivation below.

  // ARTICLES COLLECTED                                                       (h2, shipped)
  Today        ██████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░       983
  This week    ██████████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░     2,992
  This month   ████████████████████████████████████████████████████████░░    10,488
  (bars: --accent at 70%, track --surface-overlay, widths = quantized bar-N classes;
   counts: mono --text-xs --text-muted, tabular-nums)

  // TOP SOURCES                                             top 15 by article count
  cvefeed.io                  ████████████████████████████████████  6,308 · latest 12m ago
  scworld.com                 ████████████████████████████░░░░░░░░  4,933 · latest 41m ago
  api.msrc.microsoft.com      █████████████████████████░░░░░░░░░░░  4,395 · latest 2h ago
  helpnetsecurity.com         ███████████████████░░░░░░░░░░░░░░░░░  3,288 · latest 1h ago
  bleepingcomputer.com        ████████████████░░░░░░░░░░░░░░░░░░░░  2,743 · latest 26m ago
  feeds.feedburner.com        ████████████░░░░░░░░░░░░░░░░░░░░░░░░  2,082 · latest 3h ago
  csoonline.com               ███████████░░░░░░░░░░░░░░░░░░░░░░░░░  1,940 · latest 55m ago
  theregister.com             ███████████░░░░░░░░░░░░░░░░░░░░░░░░░  1,854 · latest 18m ago
  techradar.com               ██████████░░░░░░░░░░░░░░░░░░░░░░░░░░  1,805 · latest 1h ago
  securityaffairs.com         █████████░░░░░░░░░░░░░░░░░░░░░░░░░░░  1,582 · latest 2h ago
  darkreading.com             █████████░░░░░░░░░░░░░░░░░░░░░░░░░░░  1,501 · latest 47m ago
  infosecurity-magazine.com   ████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░  1,466 · latest 1h ago
  infoworld.com               ████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░  1,431 · latest 5h ago
  rss.nytimes.com             ████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░  1,373 · latest 2h ago
  hackread.com                ████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░  1,365 · latest 33m ago

  (PRIMARY ACTION of this page: each source name is now a plain <a> →
   /?feed=<url-encoded full feed_url> — reuses the shipped list-page feed filter,
   zero backend work. Link color --accent, hover --accent-bright, focus ring per §6.
   "latest Nm ago": mono --text-3xs --text-muted, from /feeds.latest_article —
   decode-only addition. Relative time keeps absolute in visible form via the
   details block below, not via title-attr hover.)

  ▸ FULL FEED URLS & LAST-ARTICLE TIMES (15)          (native <details>/<summary>, no JS)
  ┌ when open ───────────────────────────────────────────────────────────────────────────┐
  │  SOURCE            FEED URL                                  LATEST ARTICLE (UTC)     │
  │  ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄ (header rule: --border-structural) ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄  │
  │  cvefeed.io        https://cvefeed.io/rssfeed/latest.xml     2026-07-15 09:32:11      │
  │  scworld.com       https://www.scworld.com/feed              2026-07-15 09:03:47      │
  │  …13 more rows…                                                                       │
  │  (table headers: --text-structural mono; wraps in overflow-x:auto container)          │
  └───────────────────────────────────────────────────────────────────────────────────────┘
   ^ This is the audit fix for "raw feed URLs exist only in title attributes":
     full URLs + absolute timestamps are now real text, reachable by keyboard,
     screen reader, and touch — not hover-only.

  // SUMMARIZATION                                       ── NEW · frontend call only ──
  ┌──────────────────────────────────────────────────────────────────────────────────────┐
  │  ◉ OK — summarizer keeping up                                                         │
  │  (dot: --status-ok + --glow-ok; label "OK": mono --text-2xs caps --text-muted)        │
  │  ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄  │
  │  TOTAL SUMMARIES           FAILED                   AVG PROCESSING                    │
  │  48,102                    0                        1,840 ms                          │
  │  MOST RECENT ERROR: none                                                              │
  │  (when an error exists: "MOST RECENT ERROR: ollama: context deadline exceeded         │
  │   — 2h ago (2026-07-15 07:38 UTC)" in --status-critical text WITH the literal         │
  │   word "ERROR" in the label — never color-alone)                                      │
  └──────────────────────────────────────────────────────────────────────────────────────┘

  // STORY CLUSTERING                                          ── backend-gated ──
  ┌──────────────────────────────────────────────────────────────────────────────────────┐
  │           UNAVAILABLE                                                                 │
  │           Cluster coverage is not exposed by the broker yet.                          │
  │           (--pad-empty-y; label: mono --text-2xs --text-muted;                        │
  │            sentence: --text-sm --text-muted italic. No status color —                 │
  │            "unavailable" is neutral, not FAILING.)                                    │
  └──────────────────────────────────────────────────────────────────────────────────────┘
   Phase 2 decision: either ship this honest placeholder or omit the section entirely
   until the endpoint exists. Recommend OMIT (a permanent "unavailable" is noise);
   the widget layout is specced here so it can land with the endpoint.

┌────────────────────────────────────────────────────────────────────────────────────────┐
│  INFORMATION BROKER · THREAT INTELLIGENCE FEED                              ATOM FEED  │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

#### Mobile wireframe (390px)

```
┌──────────────────────────────────────┐
│ ● INFORMATION_BROKER                 │
│           FEED DIGEST STATS ABOUT    │  (nav fits inline at 11px mono — confirmed
└──────────────────────────────────────┘   in mobile-390-stats.png; no hamburger, no JS)

  // SYSTEM
  Statistics

  ┌─ TOTAL ARTICLES ──────────────────┐
  │  50,913                           │
  └───────────────────────────────────┘
  ┌─ SOURCES ─────────────────────────┐
  │  130                              │
  └───────────────────────────────────┘
  ┌─ LAST FETCH ──────────────────────┐
  │  3m ago                           │
  │  2026-07-15 09:41:07 UTC          │   <- absolute time stays VISIBLE on mobile;
  └───────────────────────────────────┘      there is no hover on touch, so the old
                                             title-attr was invisible here. Never cut.
  // INGESTION HEALTH
  ┌───────────────────────────────────┐
  │ ◉ DEGRADED                        │
  │ 26 of 3,130 fetches failed (24h)  │
  │ ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄  │
  │ SUCCESSFUL (24H)          3,104   │   <- three stats stack as label/value
  │ FAILED (24H)                 26   │      rows instead of columns
  │ AVG FETCH TIME           412 ms   │
  └───────────────────────────────────┘

  // ARTICLES COLLECTED
  TODAY                          983
  ██████░░░░░░░░░░░░░░░░░░░░░░░░░░░░
  THIS WEEK                    2,992
  ██████████████████░░░░░░░░░░░░░░░░
  THIS MONTH                  10,488
  ██████████████████████████████████
  (label moves ABOVE the bar — kills the
   w-40 truncation the audit flagged)

  // TOP SOURCES
  cvefeed.io          6,308 · 12m ago
  ██████████████████████████████████
  scworld.com         4,933 · 41m ago
  ███████████████████████████░░░░░░░
  api.msrc.microsoft.com
                      4,395 · 2h ago
  █████████████████████████░░░░░░░░░
  helpnetsecurity.com 3,288 · 1h ago
  ███████████████████░░░░░░░░░░░░░░░
  bleepingcomputer.com
                      2,743 · 26m ago
  ████████████████░░░░░░░░░░░░░░░░░░

  ▸ SHOW SOURCES 6–15                    (native <details>; rows 6–15 inside,
                                          identical row anatomy — counts remain
                                          one tap away, never hover-gated)
  ▸ FULL FEED URLS & TIMES (15)          (same details block as desktop; the
                                          inner table scrolls in overflow-x:auto
                                          so long URLs never break 320px reflow)

  // SUMMARIZATION
  ┌───────────────────────────────────┐
  │ ◉ OK — summarizer keeping up      │
  │ ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄  │
  │ TOTAL SUMMARIES          48,102   │
  │ FAILED                        0   │
  │ AVG PROCESSING         1,840 ms   │
  │ MOST RECENT ERROR: none           │
  └───────────────────────────────────┘

  INFORMATION BROKER · THREAT INTEL FEED
  ATOM FEED
```

**Collapses / reorders / abbreviates:** tiles 3-col → 1-col stack (shipped behavior, kept); health-panel stat columns → label/value rows; bar-chart labels move above bars (no more truncated `infosecurity-magazine…`); top sources shows rows 1–5 open, 6–15 behind a `details` disclosure; source full names wrap to their own line rather than truncating.

**Must never disappear:** every numeric count, all status **text labels** (OK/DEGRADED/FAILING — the dot is never the only carrier), the visible absolute last-fetch timestamp, "latest N ago" per source, unavailable-state messages, skip link, all four nav items, the Atom link. Nothing on this page may be hover-only or truncation-lossy on mobile.

#### States

**Loading.** No client-side loading state and none is possible (zero JS, fully server-rendered — the page arrives complete or not at all). No skeletons, no spinners. Perceived latency is handled by the edge: `s-maxage=30, stale-while-revalidate=60` means most visitors get a cached page instantly. Nothing to wireframe.

**Stale.** The 30s/60s SWR window means live intel numbers can lag up to ~90s during peak traffic. Every widget states its own timing where possible ("Last fetch: 3m ago", per-source "latest N ago") — an operator reading a slightly-stale total-articles tile isn't misled, because the surrounding timestamps disclose the actual data age. No separate "this page may be stale" banner — that would be noise on a page whose entire job is showing timestamps.

**Empty (fresh deploy, zero articles).** `/stats` succeeds but everything is zero. `collectionVolume` and `topSources` already return nil at zero — instead of vanishing sections, render explicit empties:

```
  ┌─ TOTAL ARTICLES ─┐ ┌─ SOURCES ─┐ ┌─ LAST FETCH ─┐
  │  0               │ │  0        │ │  —           │   (— for null last_fetch: shipped)
  └──────────────────┘ └───────────┘ └──────────────┘
  // ARTICLES COLLECTED
       NO DATA YET
       Nothing has been collected in the last 30 days.     (--pad-empty-y, --text-muted;
  // TOP SOURCES                                            same anatomy as UNAVAILABLE
       NO DATA YET                                          but different words — empty
       No sources have articles yet.                        is not an error)
  // INGESTION HEALTH: see the status-derivation rule below — a fresh install with
     total_articles == 0 always shows NO DATA YET, never a red FAILING panel.
```

**Error (hard).** `GET /stats` itself fails → existing behavior kept: `renderError` 502 full error page (`no-store`), which is where `--status-critical` already lives today. No partial stats page is attempted — `/stats` is this route's primary call.

```
  ┌───────────────────────────────────────────┐
  │   ◉ ERROR 502                              │  dot --status-critical + --glow-critical,
  │   The broker did not respond.              │  panel --status-critical-bg /
  │   [ ← BACK TO FEED ]                       │  --status-critical-border; "ERROR 502"
  └───────────────────────────────────────────┘  is text — never color-alone
```

**Partial data.** Each secondary call fails independently and degrades to the same one-pattern message, in place, at `--pad-empty-y`:

```
  // TOP SOURCES
       UNAVAILABLE                                   (shipped state, restyled: mono
       Source breakdown unavailable — could not      2xs label + --text-muted sentence;
       reach the feed service.                       keeps its no-color-status neutrality)

  // SUMMARIZATION                                   (new widget, same pattern)
       UNAVAILABLE
       Summarization stats unavailable — could not
       reach the summarizer service.
```

Rules: `/feeds` fails → top sources + the full-URL `details` show UNAVAILABLE; tiles, collection chart, ingestion health still render (they come from `/stats`). `/summarization/stats` fails → only that panel shows UNAVAILABLE; failure is non-fatal exactly like the shipped `/feeds` handling (`handlers.go:270-272` pattern). UNAVAILABLE is always neutral (`--text-muted`) — never a red panel, because "we couldn't ask" is not "it is failing," and a fabricated FAILING would violate the no-invented-data rule just as much as a fabricated green.

**Ingestion-health status derivation** (single unambiguous rule, all from real `/stats` fields — resolves an earlier draft's internal contradiction between "no fetches attempted" and "all fetches failed"):

| Condition | State | Tokens |
|---|---|---|
| `total_articles == 0` | **NO DATA YET** (neutral — fresh install, no history at all) | none — see Empty state above |
| `total_articles > 0 && successful_fetches_24h == 0` | **FAILING** — no successful fetch in the last 24h despite existing history (covers both "every attempt failed" and "the scheduler hasn't attempted a fetch" — both are equally actionable to an operator: no fresh data arrived) | `--status-critical`, `--glow-critical`, `--status-critical-bg/border` |
| `total_articles > 0 && successful_fetches_24h > 0 && failed_fetches_24h > 0` | **DEGRADED** — X of N fetches failed | `--status-warn` (= `--accent`), `--glow-warn`, `--status-warn-bg/border` |
| `total_articles > 0 && successful_fetches_24h > 0 && failed_fetches_24h == 0` | **OK** — all N fetches succeeded | `--status-ok`, `--glow-ok`, `--status-ok-bg/border` |

#### Severity/status token usage — never color-only audit

| # | Location | Token(s) | Non-color pairing |
|---|---|---|---|
| 1 | Ingestion-health dot | `--status-ok/warn/critical` + matching glow, `--status-dot-size` | Always adjacent mono caps label `OK` / `DEGRADED` / `FAILING` **plus** a plain sentence with the actual counts. Critical because `--status-warn` *is* the brand amber — on this amber page a lone amber dot is meaningless by design (`DESIGN_TOKENS.md` §5). |
| 2 | Ingestion-health FAILED numeral when > 0 | `--status-critical` text | The header sentence restates the count in words; the `FAILED (24H)` label is literal text. Color is reinforcement only. |
| 3 | Ingestion-health panel border/bg | `--status-*-border` / `--status-*-bg` | Same panel contains the dot + label + sentence from row 1; border tint carries nothing alone. |
| 4 | Summarization dot | `--status-ok` / `--status-warn` + glow | Paired label `OK` / `DEGRADED` + sentence ("summarizer keeping up" / "N summaries failed"). |
| 5 | Summarization "most recent error" line | `--status-critical` text | Prefixed by the literal words `MOST RECENT ERROR:` and the error string itself. |
| 6 | Hard 502 error page | `--status-critical` family | Literal `ERROR 502` heading + explanatory sentence (formalizes the shipped red). |
| 7 | UNAVAILABLE / NO DATA YET states | **no status token** — `--text-muted` only | Deliberately colorless; the words are the entire signal. |
| 8 | Header live-indicator dot | `--accent` + `--glow-accent` | Decorative brand element, unchanged, conveys no state (reserved per `DESIGN_TOKENS.md` §11). |

Glows are static (no pulse) — nothing new for `prefers-reduced-motion` to disable. WCAG 1.4.1 satisfied everywhere: no state on this page is expressed by hue alone.

#### Backend ledger

**Buildable today, zero Information-Broker changes:**
1. Ingestion-health panel — decode `successful_fetches_24h`, `failed_fetches_24h`, `avg_fetch_time_ms` into the `Stats` struct (`internal/apiclient/apiclient.go:55-62`, three added fields).
2. "latest Nm ago" per source + full-URL/absolute-time `details` table — decode `latest_article` (and optionally `avg_fetch_duration_ms`) from `/feeds` into the `Feed` struct (`apiclient.go:48-52`).
3. Summarization panel — one new `getJSON` call to the existing `GET /summarization/stats`, non-fatal on failure (mirror the `/feeds` pattern at `handlers.go:270-272`).
4. Top-source names become links to `/?feed=<feed_url>` — the list page's feed filter already exists.
5. Visible absolute last-fetch timestamp (move out of `title` attr, `stats.html:18`), thousands separators, mobile label-above-bar layout, `details` disclosures, UNAVAILABLE/NO-DATA states — all template/CSS.

**Needs new Information-Broker work (each justified separately in Phase 2's `ARCHITECTURE_DECISIONS.md`, per the Phase 0 decision that API changes are in scope only case-by-case):**
1. **Per-feed error rates** — justification: per-feed status dots are the natural completion of the Signal Lamp health system; `fetch_logs` has the rows, needs a per-feed join exposed on `/feeds` or `/stats`. Until then, health is aggregate-only.
2. **Cluster / embedding coverage stats** — justification: the digest's "Important" section depends on clustering; operators can't currently see coverage. Columns exist (`story_cluster_id`, `summary_embedding`), nothing aggregates them. Widget specced above but recommended omitted until the endpoint ships.
3. **Day-by-day ingestion time series** — justification: today/week/month windows can't show trend direction. New endpoint or new `/stats` fields; chart would reuse the quantized-bar pattern (a column of per-day bars), no CSP issue.

Nothing in this wireframe renders a number the broker cannot supply today; the three backend-gated features appear only as an explicit UNAVAILABLE placeholder or not at all.

---

### Route `/about` — Trust & Architecture Page

**Scope note.** Lowest-priority route for new design work. `handleAbout` (`internal/server/handlers.go:301-304`) makes **zero backend calls** — the page is fully static copy, edge-cached at 3600s s-maxage / 86400s SWR. The redesign is a copy-and-semantics tidy, not a rebuild.

**Primary action:** `← Back to feed` link (bottom of page). **Secondary actions:** the four nav items (shared shell) and the footer `Atom feed` link. Nothing else on this page is interactive — no forms, no `details`, no selects.

#### Desktop wireframe (≥ 768px, `--container` 64rem)

```
┌────────────────────────────────────────────────────────────────────────────┐
│ [Skip to content — sr-only until :focus-visible, amber fill,               │
│  --text-on-accent, --z-skip]                                               │
│ ● INFORMATION_BROKER              FEED   DIGEST   STATS  [ABOUT]           │ sticky header
│   ^--glow-accent live dot          (mono caps; ABOUT = --accent +          │ --z-header
│    (decorative, brand-only)         bg --surface-overlay + aria-current)   │
├────────────────────────────────────────────────────────────────────────────┤
│                                                                            │
│  // about                          ← eyebrow: --font-mono --text-2xs,      │
│                                      --tracking-label-wide, --accent-dim   │
│  SmellyFeet                        ← h1: --text-2xl, --text-primary,       │
│                                      --tracking-tight                      │
│  the reading room for Information Broker                                   │
│                                    ← --font-mono --text-xs --text-muted    │
│                                                                            │
│  ┌─ intro prose (--container-narrow 42rem, --text-size-prose, ──────────┐ │
│  │  --leading-relaxed)                                                   │ │
│  │                                                                       │ │
│  │  SmellyFeet is the front end for Information Broker — a              │ │
│  │  continuously-running intelligence pipeline that scrapes             │ │
│  │  cybersecurity RSS sources, summarizes every article with a          │ │
│  │  local LLM, and stores them for fast browsing.                       │ │
│  │                                                                       │ │
│  │  This app turns that firehose into something you can actually        │ │
│  │  read: a clean stream of AI-written summaries, a full-text view      │ │
│  │  for each article with a link straight back to the original          │ │
│  │  source, plus keyword search, per-source filtering, and live         │ │
│  │  system statistics.                                                  │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                                                            │
│  ── HOW IT WORKS ──────────────  ← h2: --font-mono --text-3xs caps,        │
│                                    --tracking-label, --text-muted,         │
│                                    leading rule in --border-default        │
│                                                                            │
│  Pipeline rendered as an <ol> (sequence is programmatic, not glyph-only):  │
│                                                                            │
│  ┌─────────────┐   ┌───────────────┐   ┌────────────┐   ┌────────────┐    │
│  │ RSS sources │ → │ LLM summarize │ → │ PostgreSQL │ → │ Broker API │ →  │
│  └─────────────┘   └───────────────┘   └────────────┘   └────────────┘    │
│                                                        ╔════════════╗     │
│                                                        ║ SMELLYFEET ║     │
│                                                        ╚════════════╝     │
│  plain pills: --surface-raised + --border-default, --font-mono --text-xs, │
│               --radius-lg, --text-body                                    │
│  final pill:  --surface-accent-wash + --border-accent-hover, --accent     │
│               (brand emphasis — terminus also named in text + <ol> pos 5) │
│  arrows:      --accent, aria-hidden="true" (decorative; <ol> carries the  │
│               order)                                                       │
│                                                                            │
│  ── WHAT YOU CAN DO ──────────── (h2, same treatment)                      │
│                                                                            │
│  2-col grid (--gap-grid), cards: --surface-raised, --border-default,       │
│  --radius-xl, --pad-card-sm — same surface treatment as the article card   │
│  shell (component #2), just non-interactive static content                │
│  ┌────────────────────────────────┐ ┌────────────────────────────────┐    │
│  │ Browse summaries               │ │ Read the full story            │    │
│  │ Scan the latest intelligence   │ │ Open any article for the full  │    │
│  │ as concise, AI-written         │ │ stored text and a link to the  │    │
│  │ summaries.                     │ │ original source.               │    │
│  └────────────────────────────────┘ └────────────────────────────────┘    │
│  ┌────────────────────────────────┐ ┌────────────────────────────────┐    │
│  │ Search & filter                │ │ Daily digest                    │   │
│  │ Find articles by keyword or    │ │ Important stories — covered by │    │
│  │ narrow the feed to a single    │ │ 3+ feeds — daily, weekly, or   │    │
│  │ source.                        │ │ monthly.                       │    │
│  └────────────────────────────────┘ └────────────────────────────────┘    │
│  ┌────────────────────────────────┐ ┌────────────────────────────────┐    │
│  │ Watch the system               │ │ Follow via Atom                 │   │
│  │ Live stats: total articles     │ │ Subscribe to /feed.xml in any  │    │
│  │ indexed, active sources, and   │ │ feed reader — no account, no   │    │
│  │ last fetch time.               │ │ tracking.                      │    │
│  └────────────────────────────────┘ └────────────────────────────────┘    │
│  card h3: --text-sm semibold --text-secondary; body: --text-sm            │
│  --text-muted --leading-relaxed                                            │
│                                                                            │
│  ── UNDER THE HOOD ──────────── (h2, same treatment)                       │
│                                                                            │
│  <ul>, --font-mono --text-xs --text-muted; › marker in --accent            │
│  (marker is decorative — it's a list item either way)                      │
│  › Server-rendered Go (html/template), styled with Tailwind               │
│  › Zero client-side JavaScript — every control is native HTML             │
│  › No cookies, no sessions, no tracking of any kind                       │
│  › Strict CSP; fonts and assets self-hosted and go:embed-ded              │
│  › No database of its own — talks only to the Information                 │
│    Broker HTTP API                                                         │
│  › Containerized; deploys and restarts independently of the scraper       │
│                                                                            │
│  ────────────────────────────── (hr, --border-default)                     │
│  ← BACK TO FEED                 ← primary action: --font-mono --text-xs    │
│                                   caps --tracking-caps, --accent,          │
│                                   hover --accent-bright                    │
├────────────────────────────────────────────────────────────────────────────┤
│ INFORMATION BROKER · THREAT INTELLIGENCE FEED              ATOM FEED       │
│ (footer, --text-2xs mono caps --text-muted; Atom link hover --accent)      │
└────────────────────────────────────────────────────────────────────────────┘
```

Changes vs. shipped, all copy/semantics only: pipeline becomes an `<ol>` (order was previously conveyed only by visual arrow glyphs — WCAG 1.3.1 fix); feature grid grows from 4 to 6 cards to stop omitting the Digest route and the Atom feed, which both ship today (digest card copy updated to state the real ≥3-feed threshold); "Under the hood" gains three truthful trust bullets (zero-JS, no cookies, CSP/self-hosted) — this is the project's trust page and those are its strongest claims. No new components, no new tokens.

#### Mobile wireframe (< `--bp-sm` 640px)

```
┌──────────────────────────────┐
│ ● INFORMATION_BROKER         │  header wraps to two rows if needed
│ FEED DIGEST STATS [ABOUT]    │  (shipped behavior: 10px mono, tighter
├──────────────────────────────┤   tracking, gap-0.5; keep)
│ // about                     │
│ SmellyFeet                   │
│ the reading room for         │
│ Information Broker           │
│                              │
│ [intro prose, full width,    │
│  both paragraphs — never     │
│  truncated or clamped]       │
│                              │
│ ── HOW IT WORKS ──           │
│ Pipeline goes VERTICAL:      │
│ same <ol>, connectors flip   │
│ from → to ↓ (aria-hidden)    │
│  ┌─────────────────────┐     │
│  │ 1 RSS sources       │     │
│  └─────────┬───────────┘     │
│            ↓                 │
│  ┌─────────────────────┐     │
│  │ 2 LLM summarize     │     │
│  └─────────┬───────────┘     │
│            ↓                 │
│  ┌─────────────────────┐     │
│  │ 3 PostgreSQL        │     │
│  └─────────┬───────────┘     │
│            ↓                 │
│  ┌─────────────────────┐     │
│  │ 4 Broker API        │     │
│  └─────────┬───────────┘     │
│            ↓                 │
│  ╔═════════════════════╗     │
│  ║ 5 SMELLYFEET        ║     │
│  ╚═════════════════════╝     │
│                              │
│ ── WHAT YOU CAN DO ──        │
│ grid collapses to 1 column,  │
│ all 6 cards, same order:     │
│ [Browse summaries       ]    │
│ [Read the full story    ]    │
│ [Search & filter        ]    │
│ [Daily digest           ]    │
│ [Watch the system       ]    │
│ [Follow via Atom        ]    │
│                              │
│ ── UNDER THE HOOD ──         │
│ same 6 bullets, text wraps;  │
│ nothing dropped              │
│                              │
│ ──────────────               │
│ ← BACK TO FEED               │
├──────────────────────────────┤
│ footer stacks: brand line,   │
│ then ATOM FEED link          │
└──────────────────────────────┘
```

- **Collapses/reorders:** pipeline flips horizontal → vertical (pure CSS flex-direction at the breakpoint, wrapping a 5-pill chain with arrows mid-row reads badly at 320px); feature grid 2 → 1 column; footer stacks. Nothing reorders otherwise.
- **Never disappears:** all five pipeline stages in order, all six feature cards, all six under-the-hood bullets, the back-to-feed link, the footer Atom link, the skip link. No `details` collapsing on this page — content is short enough to stand.
- **Title-attribute audit finding:** does **not apply to this route** — `/about` contains no timestamps and no feed URLs, in any attribute. Nothing hover-hidden here to surface. (The fix lands on `/` and `/stats`.)
- Fully usable at 320px, no horizontal scroll (WCAG 1.4.10) — the vertical pipeline is what guarantees this.

#### States

- **Loading:** none exists. The handler makes zero backend calls; the page is a static server render behind a 3600s/86400s edge cache. No skeleton, no spinner (and no JS to drive one). The happy path is the only path.
- **Stale:** N/A in any user-visible sense — the page has no time-sensitive content, so a cache serving a copy from hours ago is indistinguishable from a fresh one.
- **Empty:** N/A — no data-driven content anywhere on the route.
- **Error:** the only failure mode is a template render failure → the shared 500 error page (shell + `--status-critical` heading, `--status-critical-bg`/`--status-critical-border` callout — specified in the `/` route's error section, not repeated here). `/about` can never 502 on broker failure because it never contacts the broker. It is deliberately the page that still works when everything else is down — worth preserving.
- **Partial data:** N/A — no data.

#### Severity/status token usage

**Zero `--status-*` tokens on this route, by design.** Signal Lamp adds severity only where real state needs conveying; `/about` has no state. Explicit inventory of every colored element, confirming none is color-only:

| Element | Token | Meaning carrier besides color |
|---|---|---|
| Header live dot | `--accent` + `--glow-accent` | Purely decorative brand mark; adjacent wordmark text. Not a status indicator — conveys nothing. |
| Active nav "About" | `--accent` + `--surface-overlay` | `aria-current="page"` + background fill (shape) |
| Eyebrow `// about` | `--accent-dim` | It's literal text |
| Pipeline arrows | `--accent` | `aria-hidden`; sequence carried by `<ol>` positions |
| Final "SmellyFeet" pill | `--surface-accent-wash` + `--border-accent-hover` | Labeled "SmellyFeet", position 5 of 5 in the `<ol>`, bolder weight |
| `›` list markers | `--accent` | Decorative; `<li>` carries structure |
| Links (back-to-feed, Atom) | `--accent` / hover `--accent-bright` | Link text + `:focus-visible` ring (`DESIGN_TOKENS.md` §6) |

No green, no red, no warn state appears anywhere on this page.

#### Backend work: none

| Element | Backend change needed? |
|---|---|
| Everything above — copy, `<ol>` pipeline, 6-card grid, trust bullets, mobile vertical pipeline | **None. Entire route ships with zero Information-Broker changes and zero frontend data-fetch changes.** |

**Considered and rejected:** live counts in the intro ("6,300+ articles from 100+ sources"). The stats endpoint already returns the numbers, so it wouldn't even need backend work — but it would give the site's only zero-dependency route a broker dependency and a staleness problem under the 24h SWR cache, in exchange for a vanity number. Keep `/about` the page that renders when everything else is on fire.

---

## 4. Component Inventory

Derived strictly from what the four route wireframes use. Column "Status": **shipped** = exists today and is kept (restyle only), **extended** = exists, gains behavior/markup, **new** = does not exist yet.

| # | Component | Status | Lives in |
|---|---|---|---|
| 1 | Site shell (header, skip link, nav, footer) | shipped | `partials.html` |
| 2 | Article card | extended | `articleCard` partial (`list.html`), reused by digest |
| 3 | Filter/query form (GET form + native selects + Apply submit) | extended | `list.html` (search/source/sort) and `digest.html` (range) — same pattern, two instances, not two components |
| 4 | Filter chips + source-context strip | extended | `list.html` |
| 5 | Pagination | shipped | `list.html` |
| 6 | Native disclosure (`details`/`summary`) | extended | list (upcoming), digest (Other), stats (feed-URL table, mobile sources 6–15) |
| 7 | Stat tile | shipped | `stats.html` |
| 8 | Quantized bar row | shipped | `stats.html` (collection chart + top sources) |
| 9 | Empty / unavailable block | extended | all routes |
| 10 | Status indicator (dot + label + sentence) | **new** | stats, digest error, list degraded callout |
| 11 | Status panel/callout | **new** | wraps #10; stats health + summarization panels, inline error callouts |
| 12 | Error page | extended | `error.html`, `notfound.html` |
| 13 | Pipeline `<ol>` | extended | `about.html` (semantics upgrade of existing pill chain) |

The about-page feature-card grid is **not** a separate component: it reuses component #2's card shell treatment (`--surface-raised`, `--border-default`, `--radius-xl`) minus the interactive/data-bound parts — a static content container, not a new pattern.

**1. Site shell** — *Purpose:* navigation, landmark structure, brand. *Input:* active route name. *Empty/error:* none — renders even when the API is down (the error page keeps the shell). *Keyboard:* skip link first in tab order, visible on `:focus-visible`; nav links get the global focus ring; `aria-current` marks active. *Responsive:* nav wraps under the wordmark below `--bp-sm`, tighter tracking; stays four plain links at all widths.

**2. Article card** — *Purpose:* one scannable unit of intelligence. *Input:* article view (id, title, summary *nullable*, feed_url, published_at, server-extracted CVE id, cross-feed source count). *Empty:* null summary → "No summary available." in italic `--text-muted` (carries information, so not `--text-faint`). *Error:* n/a — a card either has data or isn't rendered. *Keyboard:* stretched-link title is the primary target; source pill is a separate link (must remain reachable — verify the stretched link doesn't occlude it); both get focus rings. *Responsive:* meta row wraps into badge row + time row on mobile; summary clamp 3 lines (list) / 2 lines (digest mobile); badges wrap, never truncate away. *Extension over shipped:* absolute UTC time becomes visible text beside the relative time (`--text-2xs`, not `--text-3xs` — no larger duplicate exists elsewhere on the `/` route); sr-only raw feed_url in the pill's accessible name (digest).

**3. Filter/query form** — *Purpose:* search/source/sort over the articles endpoint (list) or time-range selection over the digest endpoint (digest) — same GET-form-with-native-controls-and-Apply pattern in both places. *Input:* feeds list (may be nil on feeds-fetch failure, list only), current query params. *Empty/error:* feeds nil → dropdown collapses to "All sources" **plus** the new DEGRADED warn callout (list route §"States") — no longer silent. *Keyboard:* all native controls (input, selects, submit); `minlength="2"` on search gives native constraint validation with the browser's own error UI; visible "(min 2 chars)" hint, not title-attr. *Responsive:* stacks below `--bp-sm` — search full-width, selects share a row, Apply below (list); select + Apply stack full-width (digest).

**4. Filter chips + source-context strip** — *Purpose:* show active filters, one-click removal, expose the raw feed URL. *Input:* active params + resolved feed_url. *Empty:* not rendered when no filters active; sub-2-char queries render no chip (shipped backstop, kept). *Keyboard:* each chip is a plain `<a>` removing its own param; "clear all" links to bare `/`. *Responsive:* chips wrap, ≥24px targets; the raw URL wraps with `overflow-wrap: anywhere`, never truncates.

**5. Pagination** — *Purpose:* prev/next traversal. *Input:* current page, has-next flag. **No totals — the endpoint returns none; "page X of Y" stays out until/unless a `total` field is justified (list route backend ledger).** *Empty/error:* disabled edge renders as a `--text-faint` `<span>`, structurally non-interactive (the token's documented exemption — `DESIGN_TOKENS.md` §2). *Keyboard:* two links. *Responsive:* full-width row on mobile.

**6. Native disclosure** — *Purpose:* containment without JS — bounded initial paint over unbounded content. *Input:* section content + a count in the `<summary>` (the honest signal of hidden volume). *Empty:* not rendered when the section is empty. *Keyboard:* `summary` is natively focusable/toggleable; gets the global focus ring (extend the `:focus-visible` selector list to include `summary` — `DESIGN_TOKENS.md` §6). *Responsive:* full-width tap target. *A11y rule:* headings stay **outside** `<summary>` (sr-only `<h2>` before the element — digest route), since some AT strips heading roles inside summary.

**7. Stat tile** — *Purpose:* headline numeral. *Input:* one value from `/stats`. *Empty:* `0` or `—` for null last-fetch (shipped). *Keyboard:* non-interactive. *Responsive:* 3-col → stacked. *Extension:* absolute last-fetch timestamp visible below the relative value.

**8. Quantized bar row** — *Purpose:* magnitude comparison under the no-inline-style CSP rule. *Input:* label, count, precomputed `bar-5`…`bar-100` class (quantization happens in the Go handler/template func — server-side, as today). *Empty:* whole chart replaced by NO DATA YET block (#9). *Error:* whole chart replaced by UNAVAILABLE block (#9). *Keyboard:* bars non-interactive; on top-sources the **label** is the link. *Responsive:* label moves above the bar on mobile — kills the `w-40` truncation the audit flagged.

**9. Empty / unavailable block** — *Purpose:* the no-invented-data rule made visible; generalizes the shipped "source breakdown unavailable" state. Several wordings, one anatomy (mono-caps label + `--text-muted` sentence, `--pad-empty-y`, deliberately **no status token** — "we couldn't ask" is not "it is failing"): `NO ARTICLES MATCH THESE FILTERS` (+ chips preserved + clear-all link), `NO DATA YET` (fresh install), `UNAVAILABLE` (fetch failed / endpoint doesn't exist). *Keyboard:* only the embedded escape links. *Responsive:* full-width at every size.

**10. Status indicator** *(new — the Signal Lamp component)* — *Purpose:* convey real system state. *Anatomy:* `--status-dot-size` dot in `--status-ok/warn/critical` + static matching glow, **always** paired with a mono-caps text label (`OK` / `DEGRADED` / `FAILING`) and a sentence containing the actual counts — never dot-alone, because `--status-warn` is literally the brand amber. *Input:* a state derived from real backend fields only (single unambiguous derivation rule in the stats route's "States" section). *Empty:* if the underlying fetch failed, the indicator is not rendered — its panel shows #9 instead (never a fabricated green **or** red). *Keyboard:* non-interactive, `aria-hidden` on the dot; the text is the accessible content. *Responsive:* unchanged at all sizes; label never drops.

**11. Status panel/callout** *(new)* — *Purpose:* the container that carries #10 plus detail rows; covers the stats ingestion-health and summarization panels, the digest inline critical callout, and the list degraded callout. *Input:* a #10 state + detail stats. *Styling:* `--status-*-bg` at 8% + `--status-*-border` at 25% when non-OK; `--border-default`, no tint, when OK/neutral. *Empty/error:* degrades to #9. *Keyboard:* only embedded links ("Back to feed"). *Responsive:* stat columns become label/value rows on mobile.

**12. Error page** — *Purpose:* hard-failure terminal state (broker unreachable, render failure, 404). *Extension:* migrate the shipped ad-hoc reds to the `--status-critical` family — one red, everywhere; shell renders normally around it; `no-store` stays. *Keyboard:* back-to-feed link. *Responsive:* single column already.

**13. Pipeline `<ol>`** — *Purpose:* the about-page diagram. *Extension:* semantics only — ordered list so sequence is programmatic (WCAG 1.3.1), arrows `aria-hidden`. *Responsive:* flex-direction flips horizontal → vertical below `--bp-sm`.

Nothing else is needed. Explicitly **not** in the inventory: modals, toasts, tabs, dropdown menus, tooltips, carousels, skeletons — zero-JS makes most impossible and the wireframes need none of them.

---

## 5. Responsive Design Rules

### Grid & breakpoints

Two breakpoints total (`--bp-sm` 640px, `--bp-md` 768px), one centered column (`--container` 64rem; `--container-narrow` 42rem for prose). There is no multi-region desktop grid and none is being added — internal grids only (stat tiles 3-col, about feature cards 2-col, cards 1-col always).

**Tablet (640–768px) is not a distinct design target.** It gets the desktop layout minus the `md+` upgrades (hero padding, hero type size). No tablet-only arrangements exist in any wireframe; the collapse strategy is a single fold at `--bp-sm`, which keeps the CSS honest and testable at exactly three widths: 320/390, 700, 1280.

### Mobile single-column order (per the wireframes, binding)

- `/`: shell → eyebrow/h1 → filter form (search, source+sort row, Apply) → chips → source-context strip → upcoming disclosure → cards → pagination → footer.
- `/digest`: shell → eyebrow/h1 → since line → range form → Important heading + explainer → cards → Everything-else disclosure → footer.
- `/stats`: shell → h1 → tiles (stacked) → ingestion health → collection chart → top sources 1–5 → sources 6–15 disclosure → feed-URL disclosure → summarization → footer.
- `/about`: shell → intro prose → vertical pipeline → 6 feature cards → under-the-hood → back link → footer.

### Minimum readable sizes

- Body/summary text: never below `--text-sm` (14px).
- Information-carrying metadata: never below `--text-2xs` (11px mono caps, wide tracking).
- `--text-3xs` (10px) is permitted **only** where the same datum is also available at a larger size on the same page (e.g. "latest 12m ago" beside top-sources bars, repeated in full inside the feed-URL details table on `/stats`). It is *not* used for the `/` card's absolute timestamp, which has no larger duplicate on that route — that value is set at `--text-2xs` instead.
- `--text-faint` color never carries information at any size, except the documented disabled-control exemption (`DESIGN_TOKENS.md` §2).

### Abbreviate vs. never disappear on mobile

This resolves the audit finding that raw feed URLs and absolute timestamps were mobile-inaccessible (title-attribute-only; hover does not exist on touch). Both become real visible text everywhere they matter; the title-attribute pattern is **retired as an information carrier** site-wide.

| May abbreviate on mobile | Rule |
|---|---|
| Absolute timestamps | Drop seconds (`2026-07-14 13:02 UTC`); on `/stats` the full-seconds form remains in the details table |
| Digest card summaries | Clamp 3 → 2 lines |
| Top sources | Rows 6–15 fold behind a `details` disclosure (still one tap, never gone) |
| Source pill text | Ellipsis only when a single unbroken token exceeds the column — legal only because the raw URL is visible elsewhere (context strip / details table) |

| Must never disappear or hide behind hover | Where guaranteed |
|---|---|
| Source pill, CVE badge, N-sources badge | Badge row wraps to extra lines instead of truncating |
| Relative **and** absolute time | Visible `<time>` pair on every card and the last-fetch tile |
| Raw feed URLs | `/` source-context strip; `/stats` feed-URL details table — visible text, keyboard- and touch-reachable |
| Section counts, status text labels (OK/DEGRADED/FAILING), UNAVAILABLE messages | Plain text, no truncation, label never reduced to the dot |
| Skip link, all four nav items, pagination, Atom link | Shell rules |

### Overflow rules

- **Long URLs** (feed_urls in strips/tables): `overflow-wrap: anywhere` — wrap, never truncate, never force page scroll.
- **CVE identifiers** (`CVE-2026-49855`): the badge is an atomic flex item, `white-space: nowrap` inside; the badge **row** wraps. A CVE id never breaks mid-token and never widens the page.
- **Source names**: wrap to their own line on mobile (stats chart) rather than truncating; single-token overflow falls back to the source-pill ellipsis rule above.
- **Tables** (stats feed-URL table): wrapped in an `overflow-x: auto` container — the table may scroll, the page body never does.
- Global invariant: no horizontal page scroll at 320px (WCAG 1.4.10), verified per phase below.

---

## 6. Migration Plan

**Context that shapes every phase:** zero-JS Go `html/template` stdlib server; CSS is Tailwind compiled by `scripts/build-css.sh` (pinned v3.4.17 binary) into the **committed** `internal/server/static/app.css`; table-driven `httptest` suite across ~a dozen `*_test.go` files in `internal/server/` and `internal/apiclient/`; **no CI** — verification is manual and ritualized. The zero-JS and CSP constraints are settled Phase 0 decisions and no phase below revisits them.

**The verification ritual (every phase, no exceptions):**

1. `go test -race ./...`
2. `gofmt -l .` (must print nothing)
3. `go vet ./...`
4. `./scripts/build-css.sh` if templates or theme changed, and commit the regenerated `app.css` in the same commit
5. Live check: run the server against the real broker, load the touched route(s) at 320px and 1280px, confirm CSP headers unchanged (`curl -sI` — the frozen policy string, byte-identical), keyboard-tab the touched route once
6. After deploy: purge the Cloudflare cache for touched routes (full-URL caching means stale HTML can otherwise pair with new CSS for up to the SWR window)

**Rollback approach (every phase):** one phase = one commit (or one tight commit series) touching templates + input CSS + regenerated `app.css` together, so `git revert` restores a coherent HTML/CSS pair. Because `app.css` is committed, a revert needs no rebuild to be deployable. Then purge the edge cache.

### Phase A — Token foundation (CSS only, zero template changes)

- **Do:** declare the `DESIGN_TOKENS.md` custom properties on `:root` in `assets/tailwind.input.css`; point the Tailwind theme values in `tailwind.config.js` at `var(--…)` so every existing utility class (`text-accent`, `bg-ink-900`, …) keeps its name and resolves to a token; add the new severity tokens (`--status-*`, `--text-structural`, glows). Rebuild `app.css`.
- **Files:** `assets/tailwind.input.css`, `tailwind.config.js`, `internal/server/static/app.css` (generated).
- **Risks:** widest visual blast radius of any phase — the zinc-ladder consolidation (zinc-50/100 → one primary, zinc-400 retired) shifts shades on every page; a `var()` typo silently computes to nothing. **Mitigation:** class names don't change, so HTML/CSS edge-cache skew is harmless in this phase; do a route-by-route before/after screenshot compare at the three widths against the Phase 0 captures.
- **Rollback:** revert the single commit; committed `app.css` snaps back.
- **Verify:** ritual + contrast spot-check of the token appendix pairs + confirm `static_test.go` still passes (asset serving unchanged).

### Phase B — Shell and error family

- **Do:** restyle `partials.html` (header/footer/skip link) onto tokens; migrate `error.html` and `notfound.html` to the `--status-critical` family (one red, everywhere) with the "// ERROR" eyebrow anatomy from the `/` route's error state; extend the `:focus-visible` rule to include `summary`.
- **Files:** `internal/server/templates/partials.html`, `error.html`, `notfound.html`, `assets/tailwind.input.css` + regenerated `app.css`; test updates in `internal/server/ui_test.go` / `meta_test.go` where markup is asserted.
- **Risks:** shell markup is on every page — a broken partial breaks everything at once. Error pages are `no-store`, so no cache-skew risk there.
- **Rollback:** revert; shell is self-contained.
- **Verify:** ritual + force an error page (stop the broker or point at a dead port) and confirm the critical styling, the intact shell, and the `no-store` header.

### Phase C — Route `/` frontend-only fixes

- **Do:** everything in the `/` route's "zero backend changes" ledger: visible absolute UTC time on cards, `minlength="2"` + visible hint, source-context strip when `?feed=` is active, empty-state upgrade (chips preserved + clear-all), and the DEGRADED source-list callout. The callout needs the handler to pass an explicit "feeds fetch failed" flag to the view model instead of only nil-ing the list (`handlers.go:170-172`) — a small Go change, still zero broker changes.
- **Files:** `internal/server/templates/list.html`, `internal/server/handlers.go` (one view-model field), `assets/tailwind.input.css` + `app.css`; tests: `filter_ui_test.go`, `ui_test.go`, `filter_test.go` — write the failing table cases first (degraded flag set → callout markup present; empty result → chips retained), per the repo's TDD rule.
- **Risks:** the article card partial is shared with `/digest` — card changes here render there too; run the digest tests and eyeball `/digest` in the live check even though this phase doesn't target it.
- **Rollback:** revert; no API surface touched.
- **Verify:** ritual + live: submit a 1-char query (browser blocks natively; `curl '/?q=a'` confirms the server backstop still ignores it and renders no chip); simulate feeds-failure to see the DEGRADED callout; confirm O2's grep (no title-attr-only timestamps on this route).

### Phase D — Route `/stats` decode-only additions

- **Do:** the stats route's ledger items 1–5: decode `successful_fetches_24h` / `failed_fetches_24h` / `avg_fetch_time_ms` into the Stats struct and `latest_article` into the Feed struct (`internal/apiclient/apiclient.go`); add the non-fatal `GET /summarization/stats` call mirroring the shipped `/feeds` failure pattern; build the ingestion-health and summarization panels (status indicator + panel components), the feed-URL details table, source-name links to `/?feed=…`, visible last-fetch timestamp, mobile label-above-bar layout, NO DATA YET / UNAVAILABLE states.
- **Files:** `internal/apiclient/apiclient.go` + `apiclient_test.go`, `internal/server/handlers.go`, `internal/server/templates/stats.html`, `partials.html` if the status indicator lands as a shared partial, CSS pair; tests: new table-driven cases for the status-derivation rule **including every boundary in the truth table** (`total_articles==0`, `successful==0` with articles present, `successful>0 && failed>0`, `successful>0 && failed==0`), plus each partial-failure combination (`/feeds` down, `/summarization/stats` down).
- **Risks:** largest phase. The status-derivation logic is the one place a bug fabricates state — a wrong branch shows a green dot over failing fetches, violating the no-invented-data rule. That's why its truth table is tested exhaustively before the template work, and why the rule was tightened to a single unambiguous condition during plan review (see Compilation notes). The summarization call adds one upstream request per cache miss; it must be non-fatal and must not extend the page's failure surface (hard-fail only on `/stats` itself, as today).
- **Rollback:** revert commits; struct-field additions are backward-compatible with the broker (extra JSON fields were already being sent and ignored).
- **Verify:** ritual + live checks per state: healthy broker (OK panel), induced feeds-failure (top-sources UNAVAILABLE, tiles intact), summarization endpoint blocked (only that panel UNAVAILABLE). Confirm `s-maxage=30, swr=60` unchanged.

### Phase E — Route `/digest`

- **Do:** the digest route's zero-backend ledger: `IMPORTANT (N)` count + explainer worded to match the real ≥2-other-feeds threshold, sr-only `<h2>` before the details element, visible absolute timestamps (mostly inherited from Phase C's card work), empty-window state with range-suggestion links, Important-empty-but-Other-populated partial state, and — the one handler-flow change — the inline critical callout (shell + range form + status callout) replacing the hard error page on digest-fetch failure.
- **Files:** `internal/server/templates/digest.html`, `internal/server/handlers.go` (digest handler error branch), CSS pair; tests: `digest_test.go` — failing cases first for the three states (empty window, Important-empty, fetch-failure callout with form still present).
- **Risks:** the error-branch change alters a failure path — keep `renderError` as the fallback for template-render failure so a bug here can't produce a torn page. The unpaginated-endpoint containment strategy (collapsed details + honest count, **no** render cap, **no** invented pagination) is deliberate; do not "improve" it ahead of backend item B1.
- **Rollback:** revert; handler change is isolated to one branch.
- **Verify:** ritual + live: all three ranges, an induced broker failure showing the callout with a working range form and nav, monthly range at 320px (long page, no horizontal scroll).

### Phase F — Route `/about`

- **Do:** copy/semantics only per the about route's wireframe: pipeline pill chain becomes an `<ol>` with `aria-hidden` arrows and the CSS vertical flip below `--bp-sm`; feature grid 4 → 6 cards (adds Digest and Atom, which ship today); three truthful trust bullets (zero-JS, no cookies, strict CSP / self-hosted). No data, no new tokens, no live counts (considered and rejected — keeps this the zero-dependency route).
- **Files:** `internal/server/templates/about.html`, CSS pair; `ui_test.go` if it asserts about markup.
- **Risks:** near-none — the lowest-risk phase, sequenced last on purpose.
- **Rollback:** revert one commit.
- **Verify:** ritual + 320px vertical-pipeline check + heading-order pass (h1 → h2s, `<ol>` announced as a five-item list).

### Phase G — Backend-gated features (separate track, not part of this migration)

Each item already has its individual justification recorded in the relevant route's backend ledger above and gets a full write-up in Phase 2's `ARCHITECTURE_DECISIONS.md` before any Information-Broker code is written; **no frontend work in this plan depends on any of them**:

| Item | Justified by | Frontend behavior until it ships |
|---|---|---|
| B1 — digest pagination/limit | Unbounded monthly payload growth (`/digest` backend ledger); this is the open item from Phase 0 audit §9 item 7 | Collapsed details + honest count; no load-more UI drawn |
| B2 — cluster-coverage signal / stats endpoint | Distinguish "not clustered yet" from "no shared stories" (`/digest` ledger); operator visibility (`/stats` ledger) | Neutral "detected" copy on digest; clustering widget omitted from stats (recommended) rather than a permanent UNAVAILABLE |
| Per-feed error rates | Completes the Signal Lamp health system (`/stats` ledger) | Health stays aggregate-only |
| Day-by-day ingestion series | Trend direction (`/stats` ledger) | Window bars only; no trend chart |
| Articles `total` count | Only if pagination UX proves painful in Phase 3 (`/` ledger) | Prev/next with no totals |

### Sequencing rationale

CSS-only first (A) because it has the widest blast radius but the cheapest rollback and zero cache-skew exposure; shell + error family next (B) so every later phase's failure states already look right; then routes in ascending risk of the *Go* changes they carry — `/` (one view-model field) → `/stats` (decode + new call + derivation logic) → `/digest` (error-flow change) → `/about` (none). Every phase leaves the site shippable; there is no big-bang cutover and no point where old HTML meets incompatible CSS beyond a purgeable cache window.

---

## Compilation notes

This plan was produced by four parallel Fable-model wireframe agents plus a synthesis agent, then checked by a Fable-model critique agent against the master prompt's Phase 1 exit criteria. The critique found the deliverable's repo-fact claims all checked out (CSP string, quantized bar classes, struct field names, feeds nil-ing behavior, zero `template.HTML` usage, existing `details` elements, the focus-visible rule — all verified against the live source). It also found one exit-criterion gap and eight internal inconsistencies, all fixed in the text above during compilation:

1. **Stale state** was undispositioned on every route — added a `Stale` entry to each route's States section, derived from the actual per-route `Cache-Control` values already documented in `AUDIT.md`.
2. **`--text-prose` name collision** between a color token (§2) and a size token (§8) in `DESIGN_TOKENS.md` — the size token renamed to `--text-size-prose`, cross-referenced in both places.
3. **Status-border naming drift** (`--status-*-border` vs. an undefined `--border-status-*`) — standardized on `--status-*-border` everywhere, including the stats route's panel-border note.
4. **The "Important" threshold** was stated three incompatible ways (≥2 other feeds / "two or more feeds" / "3+ sources") — verified against the actual backend rule (`digest.go`: cross_feed_count ≥ 2 other feeds) and standardized to "three or more feeds total (the source feed plus at least two others)" everywhere, including the `/about` and `/digest` copy.
5. **Ingestion-health FAILING truth table** contradicted itself on the zero-fetches-attempted case — replaced with a single unambiguous four-branch rule (see the `/stats` route's "Ingestion-health status derivation" table) that a fresh install with no history shows NO DATA YET, and any history with zero successful fetches in 24h shows FAILING regardless of whether fetches were attempted.
6. **`--text-faint`'s "never carries information" rule** contradicted its own use on disabled Prev/Next controls — added the WCAG inactive-component exemption directly to the token definition.
7. **A `--text-3xs` sizing rule violation** on the `/` route's absolute timestamp (no larger duplicate existed on that route, violating the rule that 10px text must have one) — bumped that specific instance to `--text-2xs`.
8. **Component Inventory gaps** for the digest range form and the about feature-card grid — folded into existing components #3 and #2 respectively with an explanatory note, rather than inventing new component types the wireframes don't actually need.
9. **Two data-labeling gaps**: extended the stats route's "illustrative placeholder" disclaimer to cover the per-source "latest N ago" and last-fetch timestamp values (previously only the fetch-health/summarization numbers were flagged); added a note to the digest route's example cards clarifying their headlines are illustrative, not individually re-verified against a Phase 0 capture the way the `/` route's examples are.

No Phase 0 decision was reversed anywhere in the draft (zero-JS, the frozen CSP, public/anonymous, WCAG 2.2 AA, and amber-as-brand-accent all hold throughout) — the critique confirmed this explicitly and it remains true after the fixes above.
