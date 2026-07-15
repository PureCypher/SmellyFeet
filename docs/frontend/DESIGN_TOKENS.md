# SmellyFeet Design Tokens — "Signal Lamp"

**Phase 1 deliverable.** Direction chosen from the Phase 0 three-way comparison
(Amber Phosphor / **Signal Lamp** / NOC Wallboard). Signal Lamp is the
smallest-lift option: the existing amber brand identity is untouched (favicon,
nav, buttons, links, CVE and cross-feed badges), and a genuine severity system
is added **only** where real state needs conveying — feed health (currently
invisible) and error states (currently red-only and undifferentiated). No cyan,
no indigo, no new favicon, no new social preview images, no brand re-theme.

## Ground rules

- **Zero client-side JavaScript.** Tokens must never assume JS (no theme
  toggle, no runtime theming). One dark theme, `color-scheme: dark`.
- **CSP is frozen:** `style-src 'self'`, no inline `style=""` attributes.
  Anything data-driven (bar widths, stagger delays) stays quantized CSS
  classes (`bar-5`…`bar-100`, `nth-child` delays).
- **Self-hosted everything.** Fonts are `go:embed`-ded woff2; no CDNs.
- **Naming is semantic only.** Tokens are named for role
  (`--status-critical`), never for raw color (`red-400`). Raw hex values
  appear exactly once — here, as the token's value.
- **Source of truth.** Every value below is derived from what ships today in
  `tailwind.config.js` / `assets/tailwind.input.css` / `internal/server/static/app.css`,
  formalized into named tokens. New values are marked **NEW** and exist only
  because the severity system needs them.
- **Implementation shape (Phase 2):** declare these as CSS custom properties
  on `:root` in `assets/tailwind.input.css`, and point the Tailwind theme at
  `var(--…)` so existing utility names (`text-accent`, `bg-ink-900`, …) keep
  working. Pure build-time change; no CSP or runtime impact.
- **No invented data.** Tokens describe presentation only. Any metric the
  backend does not return today renders an explicit unavailable state — a
  token never papers over missing data with a fabricated number.

---

## 1. Surface & background layers

Elevation is expressed by **surface step + border**, not by drop shadows
(see §11). Four ink steps, darkest = page.

| Token | Value | Usage |
|---|---|---|
| `--surface-page` | `#0a0a0c` | Page background (ink-950); also scrollbar track and the border ring around scrollbar thumbs. |
| `--surface-raised` | `#0f0f13` | Cards, panels, article containers (ink-900) — the default component surface. |
| `--surface-raised-translucent` | `rgba(15,15,19,0.60)` | Inset panels that let the page gradient breathe through (digest sections, stat sub-panels). |
| `--surface-hover` | `#15151b` | Hover state of raised surfaces (ink-850); one visible step, never a color change. |
| `--surface-overlay` | `#1c1c24` | Highest opaque layer (ink-800): chips, badges, form fields, hovered list rows. |
| `--surface-header` | `rgba(10,10,12,0.80)` | Sticky header fallback where `backdrop-filter` is unsupported. |
| `--surface-header-blur` | `rgba(10,10,12,0.55)` | Sticky header when `backdrop-filter` is supported, paired with `--blur-header` (§11). |
| `--surface-accent-wash` | `rgba(245,177,61,0.06)` | Faint amber wash on hero/active panels; `0.08` and `0.10` steps allowed for active-filter chips — never above `0.10`. |
| `--surface-scrollbar` | `#2a2a33` | Scrollbar thumb; hover `#3a3a46`. Decorative only, exempt from contrast targets. |

**Fixed background recipes (not tokens, documented so they don't drift):**
page gradient = `radial-gradient(1100px 520px at 50% -8%, rgba(245,177,61,0.08), transparent 62%)`
over `linear-gradient(180deg, #0c0c10, #0a0a0c 55%)`, `background-attachment: fixed`;
grid overlay = 46px × 46px lines at `rgba(255,255,255,0.022)`, radial-masked from top center, `z: var(--z-decor)`.

## 2. Text levels

Consolidates today's seven-step zinc ladder (zinc-50/100/200/300/400 +
`fog` + article gray) into six roles. Contrast checked against the darkest
surface each role legally sits on.

| Token | Value | Usage |
|---|---|---|
| `--text-primary` | `#fafafa` | Headings, hero numerals, article titles. (Absorbs shipped zinc-50 **and** zinc-100 — one primary, not two.) |
| `--text-secondary` | `#e4e4e7` | Subheadings, emphasized inline text, button labels on dark fills. |
| `--text-body` | `#d4d4d8` | Default UI body text (card summaries, form labels). |
| `--text-prose` | `#c4c7d0` | Long-form article body only (`.article-body`); tuned for 1.85 line-height at 72ch measure. Color role — see `--text-size-prose` (§8) for the paired type-scale size; the two are distinct tokens despite the shared name root. |
| `--text-muted` | `#8b8b97` | Metadata, timestamps, captions (`fog`). ≥ 5.4:1 on all surfaces — the AA floor fixed in the interim a11y batch; do not darken. |
| `--text-structural` | `#8891a3` | **NEW.** Secondary labels where the neutral must read as *chrome*, not *content* (stats table headers, axis labels). A hair warmer than the zinc it replaces. Used sparingly — if in doubt, use `--text-muted`. |
| `--text-faint` | `#52525b` | Placeholders and decorative glyphs, plus disabled/inactive controls where WCAG's inactive-component exemption applies (e.g. the disabled Prev/Next `<span>`s — structurally non-interactive, not merely grayed). Fails AA by design; never used for content that could be enabled, relevant, or actionable. |
| `--text-on-accent` | `#0a0a0c` | Text/icons on amber fills (primary buttons, skip-link focus state, active badges). |

Shipped `zinc-400 (#a1a1aa)` occurrences migrate to `--text-muted` or
`--text-body`, whichever role fits — the in-between step is retired.

## 3. Border & divider colors

| Token | Value | Usage |
|---|---|---|
| `--border-default` | `#26262f` | Card and section borders, `<hr>`, table rules (`line`). |
| `--border-subtle` | `rgba(38,38,47,0.50)` | Internal dividers inside a card. (Folds shipped `line/50` and `line/70` into one step.) |
| `--border-structural` | `rgba(136,145,163,0.35)` | **NEW.** Emphasis divider from the structural neutral — e.g. the rule under a stats table header. Sparingly; most dividers stay `--border-default`. |
| `--border-accent-faint` | `rgba(245,177,61,0.25)` | Resting border on amber-tinted chips/badges. |
| `--border-accent-hover` | `rgba(245,177,61,0.50)` | Hover/active border on interactive amber elements. (Folds shipped 0.40/0.50/0.60 ladder to one resting + one hover step; full-strength `--accent` allowed for the active nav item.) |
| `--status-ok-border` / `--status-warn-border` / `--status-critical-border` | see §5 | Status chips/callouts use the 25 %-alpha tint of their status color, defined alongside the status colors in §5 — not a separate `--border-status-*` family. |

## 4. Accent colors

The brand. Unchanged, formalized.

| Token | Value | Usage |
|---|---|---|
| `--accent` | `#f5b13d` | Brand amber: nav wordmark, links, primary buttons, CVE badges, cross-feed badges, favicon, selection tint, focus ring, chart bars. |
| `--accent-bright` | `#ffc964` | Hover state of any amber element (link hover, button hover). |
| `--accent-dim` | `#c08a33` | Pressed/secondary amber, small-caps mono labels. Still ≥ 6:1 on `--surface-page`; do not use below 14px on `--surface-overlay`. |
| `--accent-selection` | `rgba(245,177,61,0.32)` | `::selection` background (with `#fff` text). |

Alpha ladder for amber tints is fixed at **6/8/10 % backgrounds, 25/50 %
borders** (§1, §3). New alpha steps require a design decision, not a one-off.

## 5. Status & severity colors

The point of Signal Lamp. Three states, used **only** where real state exists:
feed health on the stats page, and error states. Everything else stays amber.

| Token | Value | Usage |
|---|---|---|
| `--status-ok` | `#4ade80` | **NEW.** Healthy: feed fetched successfully in the current window. ~11:1 on ink surfaces. |
| `--status-warn` | `var(--accent)` (`#f5b13d`) | Degraded: fetch failures present but at least one successful fetch in the current window (content still served). Deliberately the brand hue — warning and accent are literally the same color by design. |
| `--status-critical` | `#f87171` | Failing: no successful feed fetch in the current window despite existing article history (see the status-derivation rule in `REDESIGN_PLAN.md`'s stats route section), plus hard error pages. Formalizes the red already shipped on the error page; ~7:1 on ink surfaces. |
| `--status-ok-bg` | `rgba(74,222,128,0.08)` | **NEW.** Tint background for healthy chips/callouts. |
| `--status-warn-bg` | `rgba(245,177,61,0.08)` | Tint background for degraded chips (same value as the accent wash — intentional). |
| `--status-critical-bg` | `rgba(248,113,113,0.08)` | Tint background for error callouts. (Replaces shipped `rgba(239,68,68,0.05)`.) |
| `--status-ok-border` / `--status-warn-border` / `--status-critical-border` | 25 % alpha of each status color | Chip/callout borders, matching the accent 25 % pattern. (`--status-critical-border` replaces shipped `rgba(239,68,68,0.25)`.) This is the canonical name for these three tokens — do not use `--border-status-*` anywhere. |
| `--status-dot-size` | `0.5rem` | **NEW.** Diameter of the status dot (matches the existing `h-2 w-2` live-indicator dot). |

**Consolidation:** today's ad-hoc error reds — `#fca5a5` text, `rgba(248,113,113,0.8)`
text, `#ef4444`-based tints — all collapse into the `--status-critical` family.
One red, everywhere.

**Never color alone (WCAG 1.4.1).** Because `--status-warn` *is* the brand
color, a lone amber dot is ambiguous on an amber-branded page. Every status
dot is therefore always paired with a text label (`OK` / `DEGRADED` /
`FAILING`) in `--text-muted` mono caps. This also covers color-blind users
for the green/red pair.

**Data honesty.** Feed-health state derives from the per-feed
successful/failed fetch counts the stats endpoint already returns (frontend
decode is a known trivial addition). If the feeds fetch itself fails, the
stats page shows the existing explicit "source breakdown unavailable" state —
never a fabricated green.

## 6. Focus ring

Shipped in the interim a11y batch; formalized, must not regress.

| Token | Value | Usage |
|---|---|---|
| `--focus-ring-color` | `var(--accent)` | Global `:focus-visible` outline on `a, button, input, select`. |
| `--focus-ring-width` | `2px` | Outline width. |
| `--focus-ring-offset` | `2px` | Outline offset — the gap shows the page surface, keeping the ring visible even on amber-filled controls. |
| `--focus-ring-radius` | `4px` | Border-radius applied with the outline so the ring hugs rounded controls. |

Keyboard-only (`:focus-visible`); mouse focus is unstyled. `details`/`summary`
already ship today (list-page "Upcoming", digest "Other"); this redesign
extends the `:focus-visible` selector list to also cover `summary`, which is
not currently included, so the disclosure toggle gets a visible ring like
every other interactive element. The skip-to-content link keeps its dedicated
focused presentation (amber fill, `--text-on-accent`, `z: var(--z-skip)`).

## 7. Font stacks

Self-hosted IBM Plex (latin subset), `font-display: swap`, served from
`/static/fonts/` via `go:embed`. No new weights.

| Token | Value | Usage |
|---|---|---|
| `--font-sans` | `"IBM Plex Sans", ui-sans-serif, system-ui, sans-serif` | Prose, headings, UI copy. Weights hosted: 400, 500, 600, 700. |
| `--font-mono` | `"IBM Plex Mono", ui-monospace, SFMono-Regular, monospace` | Small-caps labels, timestamps, numerals, badges, status labels. Weights hosted: 400, 500, 600. |

Numerals in stat tiles and tables use `font-variant-numeric: tabular-nums`
(already shipped).

## 8. Type scale

Formalizes exactly what ships (10px/11px mono micro-labels; prose from `sm`
through `2xl` plus `3xl`/`4xl` display; the one-off 15px and `.article-body`
0.94rem merge into a single prose size). No new sizes.

| Token | Value | Usage |
|---|---|---|
| `--text-3xs` | `10px` | Mono micro-labels: chart annotations, badge fine print. Always caps + wide tracking; never body copy; never the sole carrier of a datum not shown at a larger size elsewhere on the same route. |
| `--text-2xs` | `11px` | Mono small-caps section labels, nav meta, status labels. |
| `--text-xs` | `0.75rem` | Badges, timestamps, footer text. |
| `--text-sm` | `0.875rem` | Secondary UI text, buttons, card summaries. |
| `--text-size-prose` | `0.9375rem` | Article body and list-item summaries. (Merges shipped `text-[15px]` and `.article-body` 0.94rem — a 0.04px difference nobody can see.) Size-scale token — pairs with the `--text-prose` **color** token in §2; the two are separate properties despite the shared name root, do not conflate them. |
| `--text-base` | `1rem` | Default body size. |
| `--text-lg` | `1.125rem` | Card titles, list-item headlines. |
| `--text-xl` | `1.25rem` | Section headings. |
| `--text-2xl` | `1.5rem` | Page headings, article titles. |
| `--text-3xl` | `1.875rem` | Stat-tile numerals. |
| `--text-hero` | `1.95rem` | Masthead at `md+` (shipped responsive one-off, kept). |
| `--text-4xl` | `2.25rem` | Hero numerals / largest display size. |

**Line heights:** `--leading-tight: 1.25` (display), `--leading-snug: 1.375`
(headings), `--leading-normal: 1.5` (UI), `--leading-relaxed: 1.625`
(summaries), `--leading-prose: 1.85` (article body).

**Letter spacing** (consolidates seven shipped values to five):
`--tracking-tight: -0.025em` (display), `--tracking-caps: 0.1em` (generic
caps), `--tracking-label: 0.14em` (mono section labels; absorbs 0.12em),
`--tracking-label-wide: 0.2em` (hero eyebrows; absorbs 0.18/0.22em),
`--tracking-brand: 0.25em` (wordmark only).

## 9. Spacing scale

Base unit `0.25rem` (Tailwind's). The shipped page uses steps 0.5–10 plus
16/20 for empty states; formalized as semantic aliases where a value has a
job. Everything else uses the raw scale.

| Token | Value | Usage |
|---|---|---|
| `--space-unit` | `0.25rem` | Base unit; raw steps ½, 1, 1½, 2, 2½, 3, 4, 5, 6, 7, 8, 9, 10, 16, 20 remain valid. |
| `--pad-card-sm` | `1rem` | Compact cards, chips-with-padding, mobile card padding (p-4). |
| `--pad-card` | `1.5rem` | Default card padding (p-5/p-6 shipped; standardize on 6, allow 5 for dense lists). |
| `--pad-card-lg` | `1.75rem` | Featured/digest cards (p-7). |
| `--pad-hero` | `2.25rem` | Hero panel at `md+` (md:p-9). |
| `--gap-grid` | `1rem` | Card grid gap. |
| `--gap-inline` | `0.5rem` | Badge rows, icon–label pairs (gap-1.5/2 territory). |
| `--pad-section-y` | `2.5rem` | Vertical rhythm between page sections (py-10). |
| `--pad-empty-y` | `4rem`–`5rem` | Empty/unavailable-state blocks (py-16/py-20) — generous space is part of the explicit "unavailable" presentation. |

## 10. Border radius

Formalizes the shipped `rounded` → `rounded-2xl` range. No new radii.

| Token | Value | Usage |
|---|---|---|
| `--radius-sm` | `0.25rem` | Inline code, tiny chips, the focus-ring radius (§6). |
| `--radius-md` | `0.375rem` | Small badges, mono labels. |
| `--radius-lg` | `0.5rem` | Buttons, inputs, selects, nav pills. |
| `--radius-xl` | `0.75rem` | Standard cards, list rows. |
| `--radius-2xl` | `1rem` | Hero panel, featured cards, top-level page containers. |
| `--radius-full` | `9999px` | Pills, status dots, the live-indicator dot. |

## 11. Shadow & elevation rules

**Rule: elevation = surface step + border, never drop shadows.** The page has
no box-shadow elevation today and gains none. The only shadows are *glows* —
that is the lamp in Signal Lamp.

| Token | Value | Usage |
|---|---|---|
| `--glow-accent` | `0 0 12px rgba(245,177,61,0.75)` | The live-indicator dot in the header (shipped). Reserved for exactly that. |
| `--glow-ok` | `0 0 8px rgba(74,222,128,0.45)` | **NEW.** Halo on a healthy status dot. |
| `--glow-warn` | `0 0 8px rgba(245,177,61,0.45)` | **NEW.** Halo on a degraded status dot. |
| `--glow-critical` | `0 0 8px rgba(248,113,113,0.45)` | **NEW.** Halo on a failing status dot. |
| `--blur-header` | `8px` | `backdrop-filter: blur()` on the sticky header, paired with `--surface-header-blur`. |

Glows are static (no pulse animation — nothing new to disable under
reduced-motion) and purely decorative: the dot color + text label carry the
information; the glow is ambience.

## 12. Z-index layers

Five layers, fixed. New stacking contexts must claim one of these — no
magic numbers.

| Token | Value | Usage |
|---|---|---|
| `--z-decor` | `0` | Fixed grid overlay (`body::before`) and any background decoration. |
| `--z-content` | `1` | `header`, `main`, `footer` — lifts content above decor. |
| `--z-raised` | `10` | Locally raised elements inside content (sticky sub-elements, hover affordances). |
| `--z-header` | `20` | The sticky site header. |
| `--z-skip` | `50` | Skip-to-content link when focused. Nothing ever sits above it. |

No modals, dropdowns, or toasts exist (zero-JS); no layers are reserved
for them.

## 13. Animation durations

Formalizes shipped motion. No new animation for the severity system —
status dots are static.

| Token | Value | Usage |
|---|---|---|
| `--dur-fast` | `150ms` | Color, opacity, and small transform transitions (link/button/border hovers). |
| `--dur-medium` | `300ms` | Larger transforms: chart-bar scale-in, group-hover reveals. |
| `--dur-reveal` | `500ms` | The `riseIn` entrance animation on list cards. |
| `--stagger-step` | `35ms` | Per-card reveal delay step, applied via `nth-child(2..20)` classes (35ms → 665ms). Matches the list page size of 20; CSP forbids inline delays, so the classes stay. |
| `--ease-standard` | `cubic-bezier(0.4, 0, 0.2, 1)` | Hover/color transitions. |
| `--ease-rise` | `cubic-bezier(0.21, 0.7, 0.2, 1)` | The `riseIn` entrance. |

**Reduced motion:** `@media (prefers-reduced-motion: reduce)` disables the
reveal animation entirely (`animation: none; opacity: 1`) — shipped, must not
regress. New motion of any kind must ship with its reduced-motion branch in
the same commit.

## 14. Breakpoints & container strategy

The layout is a single centered column; two breakpoints cover everything
shipped. **Container queries are deliberately not adopted**: they would buy
nothing at one column with two media queries, and every component currently
renders in exactly one container width per breakpoint. Revisit only if a
component must live in two differently-sized containers at once (e.g. a
future dashboard grid on the stats page).

| Token | Value | Usage |
|---|---|---|
| `--bp-sm` | `640px` | Card grids go 1 → 2/3 columns; tighter tracking variants engage. |
| `--bp-md` | `768px` | Hero padding steps to `--pad-hero`; hero type steps to `--text-hero`; fixed sidebar widths engage. |
| `--container` | `64rem` | Main content max-width (`max-w-5xl`), centered. |
| `--container-narrow` | `42rem` | Article/reading column (`max-w-2xl`). |
| `--measure-prose` | `72ch` | `.article-body` line-length cap — the readability ceiling for RSS full text. |

Below `--bp-sm` everything is one column; the design must be fully usable
there without horizontal scroll (WCAG 1.4.10 reflow at 320px).

---

## Appendix: contrast reference (WCAG 2.2 AA)

Measured against `--surface-page` (`#0a0a0c`); ratios are slightly lower but
still passing on `--surface-overlay` (`#1c1c24`).

| Foreground | Ratio | Verdict |
|---|---|---|
| `--text-primary` `#fafafa` | ~18.9:1 | Pass |
| `--text-body` `#d4d4d8` | ~13.3:1 | Pass |
| `--text-prose` `#c4c7d0` | ~11.4:1 | Pass |
| `--text-muted` `#8b8b97` | ~5.9:1 | Pass (normal text) |
| `--text-structural` `#8891a3` | ~6.2:1 | Pass (normal text) |
| `--accent` `#f5b13d` | ~9.4:1 | Pass |
| `--accent-dim` `#c08a33` | ~6.4:1 | Pass (avoid < 14px on `--surface-overlay`) |
| `--status-ok` `#4ade80` | ~11.2:1 | Pass |
| `--status-critical` `#f87171` | ~7.0:1 | Pass |
| `--text-on-accent` on `--accent` | ~9.4:1 | Pass |
| `--text-faint` `#52525b` | ~2.6:1 | **Fail — decorative/placeholder/disabled-control only, by design (§2)** |

Non-text UI (borders on interactive elements, status dots) targets 3:1
against adjacent colors per WCAG 1.4.11; status dots always carry a text
label regardless (§5).
