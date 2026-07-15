# Architecture Decision Records — SmellyFeet Frontend

> **Phase 2 deliverable.** These ADRs formalize architecture and security decisions for the SmellyFeet frontend redesign (Phase 1 direction: **Signal Lamp**). Phase 0 (Audit) and Phase 1 (Redesign Plan) are settled inputs; this document records the *why* behind decisions that are already effectively made, evaluates the genuine alternatives that were rejected, and states the rollback path for each.
>
> **Scope boundary.** SmellyFeet is the presentation frontend (Go module `smellyfeet`, zero third-party dependencies, `net/http` + `html/template`). Information-Broker is a separate backend service (goquery, gofeed, lib/pq, Prometheus client) over PostgreSQL 15 with an Ollama sidecar. Decisions here bind the frontend; ADR-006 proposes — but does not unilaterally decide — extensions to the Information-Broker API.
>
> **Related docs:** `docs/frontend/AUDIT.md` (Phase 0), `docs/frontend/REDESIGN_PLAN.md` (Phase 1, incl. the "Phase G" API-extensions table), `docs/frontend/DESIGN_TOKENS.md` (token layer referenced by ADR-002).

| ADR | Decision | Status |
|-----|----------|--------|
| [ADR-001](#adr-001--rendering-architecture) | Rendering architecture: stay zero-JS Go `html/template` SSR | Accepted |
| [ADR-002](#adr-002--styling-approach) | Styling: keep Tailwind CLI, add a CSS custom-property token layer underneath | Accepted |
| [ADR-003](#adr-003--live-update-mechanism) | Live updates: none; rely on Cloudflare edge caching + SWR | Accepted |
| [ADR-004](#adr-004--state-management) | State: server + URL query params only; no client state, no cookies | Accepted |
| [ADR-005](#adr-005--charts-and-data-visualization) | Charts: quantized CSS-class bars, numeric labels always present | Accepted |
| [ADR-006](#adr-006--backend-api-extensions) | API extensions: five additive backend items (B1 needs human sign-off) | Proposed |

---

## ADR-001 — Rendering Architecture

**Status:** Accepted

### Context

SmellyFeet is a public, anonymous, read-only threat-intelligence reader with four content routes (`/`, `/digest`, `/stats`, `/about`) plus `/article/{id}`, `/feed.xml`, `/robots.txt`, and `/healthz`. Every route is `GET`-only and fully server-rendered per the Phase 1 wireframes. There are no forms that mutate state and no `POST`/`PUT`/`DELETE` anywhere in the frontend.

The current implementation is a `net/http` server (Go 1.22, `mux.HandleFunc("GET /{$}", …)` style routing in `internal/server/server.go`) rendering `html/template`. Its `go.mod` `require` block is empty — the frontend ships **zero** third-party Go dependencies and **zero** JavaScript. The Content-Security-Policy is frozen and verified live in `internal/server/middleware.go` and asserted byte-for-byte in `middleware_test.go`:

```
default-src 'none'; style-src 'self'; font-src 'self'; img-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'
```

There is no `script-src`, so it inherits `'none'`: the browser will refuse to execute any script, inline or external. The zero-JavaScript posture has been decided twice in prior Approved specs and reaffirmed in Phase 0 and Phase 1.

The question this ADR settles is not "should we rewrite" — it is: **for the redesign, do we stay on plain SSR, adopt HTMX to enhance SSR, or move to an SPA?** We evaluate all three honestly against SEO, feed freshness, JavaScript-disabled degradation, caching, deployment complexity, bundle size, and compatibility with the confirmed Go stdlib backend.

### Options Considered

**Option A — Server-rendered `html/template` (status quo).**
The server produces complete HTML per request. No client runtime. Filter/sort/range state travels in URL query parameters and is resolved server-side.
- *SEO:* Fully crawlable; content is in the initial HTML payload. Best-in-class.
- *Feed freshness:* Freshness is a function of edge-cache TTL (ADR-003), not client polling. A hard refresh always yields current server state.
- *JS-disabled degradation:* Not "degradation" — it is the baseline. The site is fully functional with scripting disabled, which is table stakes for a security-audience tool where readers often browse hardened.
- *Caching:* Ideal. Every route is a cacheable full-URL HTML object; Cloudflare edge caching works without cache-key gymnastics.
- *Deployment complexity:* Single static Go binary, one container. No build step for JS, no asset-hash manifest, no hydration contract to keep in sync.
- *Bundle size:* Zero JS bytes shipped. Only HTML + one compiled CSS file (ADR-002).
- *Backend compatibility:* Perfect — Go template rendering consumes the Information-Broker JSON via `internal/apiclient` and emits HTML directly.

**Option B — HTMX-enhanced SSR.**
Keep server rendering, add HTMX (~14 KB gzipped) to swap page fragments (e.g. re-render the article list on filter change without a full navigation) via `hx-get`/`hx-target` attributes.
- *SEO:* Preserved (initial render is still server HTML).
- *Feed freshness:* Marginally better UX for in-place refresh, but the underlying data staleness is still governed by the edge cache; HTMX does not make the backend fresher.
- *JS-disabled degradation:* HTMX itself degrades if links/forms are authored to work without it — but that requires maintaining *two* correct code paths (the `hx-*` path and the plain-navigation fallback) for every interaction, forever.
- *Caching:* **This is where it breaks.** HTMX fragment responses are partial HTML at the same or query-varied URLs; they fragment the edge-cache object model. We would either serve partials from distinct URLs (doubling the cacheable surface) or vary on an `HX-Request` header (a cache-key split that halves hit rate and invites the classic "cached fragment served as a full page" bug).
- *Deployment complexity:* Adds a vendored JS asset, a CSP change, and a second rendering contract.
- *Bundle size:* +~14 KB JS where today there is zero.
- *Backend compatibility:* Fine, but irrelevant given the CSP conflict below.

**Option C — Single-Page Application (React/Svelte/Vue + client router).**
Ship a JS bundle that fetches JSON from an API and renders client-side.
- *SEO:* Requires SSR/prerendering to be crawlable — reintroducing a server-render path *and* a client-render path (the worst of both).
- *Feed freshness:* Client can poll, but that reopens the transport question (ADR-003) the project has deliberately closed.
- *JS-disabled degradation:* Blank page. Total failure for the hardened-browser audience.
- *Caching:* Static bundle caches well, but content becomes uncacheable-at-edge JSON + client render; we lose full-URL HTML edge caching entirely.
- *Deployment complexity:* Bundler, asset pipeline, hydration, source maps, a second deployable — a large step up.
- *Bundle size:* Tens to hundreds of KB.
- *Backend compatibility:* Would push presentation concerns into the API and couple the frontend to a JSON contract far more tightly than today's server-side consumption.

### Decision

**Stay with zero-JavaScript Go `html/template` server-side rendering (Option A). No HTMX. No SPA.**

### Rationale

1. **The CSP forbids B and C outright, and the CSP is frozen.** Both HTMX and any SPA require executing JavaScript, which needs a `script-src` directive. Adding one is a change to the frozen, test-locked CSP (`middleware_test.go` asserts the exact string) and a reversal of a zero-JS decision made twice in prior Approved specs. Option A is the *only* option that does not require reopening that decision.
2. **HTMX does not earn its complexity here.** Its payoff is in-place partial updates for interaction-heavy pages. SmellyFeet has no mutations, no multi-step flows, no live regions the user drives — only navigation between cacheable read views. Adopting HTMX would trade our clean, uniformly-cacheable full-URL HTML model for a fragment/cache-key-splitting problem, in exchange for eliminating a full-page reload that the edge already serves in single-digit milliseconds. That is negative ROI.
3. **An SPA would violate the two hard constraints simultaneously** — the frozen CSP and the zero-JS decision — while regressing SEO, JS-disabled degradation, edge cacheability, and deployment simplicity. It is disqualified on the merits, not merely on inertia.
4. **The audience.** Threat-intel readers disproportionately browse with scripting restricted. A tool that goes blank without JS fails its own users. Option A is functional in the most locked-down browser.
5. **Backend fit.** The confirmed Go stdlib posture on both sides means SSR consumes backend JSON in-process and emits HTML with no serialization boundary leaking into the client. This keeps the frontend/backend contract narrow and server-owned.

### Consequences

- **Positive:** Zero client JS to maintain, audit, or CVE-patch. Every route stays a first-class edge-cacheable HTML object. Full crawlability and JS-disabled operability come for free. One binary, one container, one rendering path.
- **Positive (security):** The zero-JS + `script-src 'none'` posture eliminates the entire XSS execution surface at the browser. Combined with the ingest-time HTML stripping and `html/template` contextual auto-escaping (see cross-cutting note), untrusted RSS content has no path to script execution.
- **Negative / accepted:** Any interaction that a JS app would do in-place (filter, sort, paginate) costs a full navigation. Given edge caching, this is imperceptible and is accepted.
- **Negative / accepted:** Rich client-only affordances (drag-reorder, live-typeahead) are off the table by construction. None are in the Phase 1 scope.

### Rollback Path

Rolling *back* from this decision means adopting client JavaScript, which is gated on **first reopening the zero-JS decision and amending the frozen CSP** — explicitly out of scope for Phase 2 and requiring human sign-off. Mechanically, the smallest reversal would be:
1. Re-open the CSP decision; add a scoped `script-src` (e.g. `'self'` with per-asset hashes — never `'unsafe-inline'`).
2. Update `middleware.go` **and** the `wantCSP` assertion in `middleware_test.go` together.
3. Introduce HTMX (Option B) first as the least-invasive step, vendored and self-hosted (no CDN, to satisfy `default-src 'none'`), before ever considering an SPA.

Because the current architecture emits complete HTML, this rollback is purely additive and non-destructive: SSR remains the substrate even if progressive enhancement is layered on later.

---

## ADR-002 — Styling Approach

**Status:** Accepted

### Context

The project already styles with **Tailwind CSS**, compiled via a **pinned CLI binary** (cached at `.cache/tailwindcss-v3.4.17`), driven by `tailwind.config.js` and `assets/tailwind.input.css`, emitting a single committed stylesheet at `internal/server/static/app.css`. That file is served under `/static/` with `Cache-Control: public, max-age=31536000, immutable`. The frozen CSP allows `style-src 'self'` and `font-src 'self'` but — critically — there is **no** `'unsafe-inline'` for styles, and there are **zero** inline `style=""` attributes anywhere in the templates. Data-driven visual values (chart bar widths, animation stagger delays) are expressed as **precomputed quantized CSS classes** (`bar-5` … `bar-100`, nth-child delay classes), never inline styles (see ADR-005).

The Phase 1 "Signal Lamp" direction keeps the single amber brand accent (`#f5b13d`) intact and adds a small additive severity palette (`status-ok` green, `status-warn` reusing the amber hue on purpose, `status-critical` red) for feed-health and error states only. Phase 1 also introduced `docs/frontend/DESIGN_TOKENS.md`, a design-token specification to sit underneath the styling layer.

The question: **custom hand-written CSS, adopt Tailwind (already adopted), or keep Tailwind and formalize a token layer?** — evaluated against bundle size, design-token implementation, maintainability, CSP compatibility, and velocity.

### Options Considered

**Option A — Rip out Tailwind, hand-write custom CSS.**
- *Bundle size:* Potentially smaller if hand-optimized, but only marginally versus Tailwind's purged output, which already tree-shakes to the classes actually used.
- *Tokens:* Native — CSS custom properties are the idiomatic home for tokens.
- *Maintainability:* Regresses. Discards a working, pinned, reproducible build the team already knows.
- *CSP:* Neutral (external stylesheet either way).
- *Velocity:* Slowest — a full restyle rewrite for no functional gain. Pure churn.

**Option B — Keep Tailwind as-is, no token layer.**
- *Bundle size:* Good (purged single file).
- *Tokens:* Weak. Tailwind's `theme` config holds design constants, but the Signal Lamp severity semantics (`ok`/`warn`/`critical`) live better as named CSS custom properties that both Tailwind utilities and hand-authored component classes can reference.
- *Maintainability:* Fine, but severity colors risk being duplicated as literal hex across utilities and the `bar-*` classes.
- *CSP:* Compatible today.
- *Velocity:* Fastest short-term, but leaves a token gap Phase 1 explicitly wants closed.

**Option C — Keep Tailwind, add a CSS custom-property token layer underneath (per `DESIGN_TOKENS.md`).**
Define brand and severity tokens as `:root { --accent: #f5b13d; --status-ok: …; --status-warn: var(--accent); --status-critical: … }` in the input CSS, and have both Tailwind theme values and component classes consume them.
- *Bundle size:* Effectively unchanged; custom properties add negligible bytes and the file stays a single purged stylesheet.
- *Tokens:* First-class. One source of truth for the accent and severity palette; `status-warn` literally aliases the accent (`var(--accent)`), encoding the Phase 1 intent in code.
- *Maintainability:* Best. Changing a severity hue is a one-line token edit; `bar-*` and utilities both inherit it.
- *CSP:* Fully compatible — see the JIT caveat below.
- *Velocity:* High. Additive to the existing pipeline; no rewrite.

### Decision

**Keep Tailwind (pinned CLI, committed `app.css`) and add the `DESIGN_TOKENS.md` CSS custom-property layer underneath it (Option C).** Do **not** enable or use Tailwind's JIT arbitrary-value syntax that would emit inline styles.

### Rationale

1. **It is already Tailwind, and it works.** The pinned-CLI + committed-output setup is reproducible, self-hosted (satisfying `style-src 'self'`), and requires no Node runtime at serve time. Ripping it out (Option A) is churn with no payoff; not formalizing tokens (Option B) leaves the Signal Lamp palette scattered.
2. **Tokens encode intent.** Making `--status-warn: var(--accent)` in the token layer captures the deliberate Phase 1 decision that the "warn" severity *is* the brand amber — not a coincidental hue match. Future edits can't accidentally desynchronize them.
3. **CSP compatibility is preserved by refusing JIT arbitrary values that inline.** This is the load-bearing constraint and deserves an explicit rule:

   > **Rule: no Tailwind arbitrary values that produce `style` attributes, and no inline styles at all.**
   >
   > Tailwind arbitrary-*class* utilities (e.g. `w-[73%]`) compile to a generated **class** in the stylesheet, not an inline `style=""` — those are CSP-safe in principle. The danger is twofold: (a) any pattern or plugin that emits an inline `style` attribute would be blocked by the frozen CSP (no `'unsafe-inline'` in `style-src`) and would silently break the layout; and (b) unbounded arbitrary values applied to *data-driven* dimensions (like a bar width computed per request) would explode the generated CSS with one class per distinct value and defeat purging. For data-driven values we therefore use the **quantized `bar-5…bar-100` classes** (ADR-005), authored once in the token/component layer, not Tailwind arbitrary values and never inline styles. Arbitrary values remain acceptable only for **static, design-time** one-offs that purge cleanly.

4. **Single cacheable asset.** The output stays one `immutable`, 1-year-cached `app.css`, which is optimal for the edge-caching strategy (ADR-003).

### Consequences

- **Positive:** One source of truth for brand + severity color; trivial theming edits; no build-pipeline change; CSP posture unchanged.
- **Positive:** The token layer gives the `bar-*` and `status-*` classes (ADR-005, Signal Lamp) a stable, named color contract.
- **Negative / accepted:** Contributors must know the "no inline styles, quantize data-driven values" rule. This is documented here and enforced structurally by the CSP (violations fail visibly in the browser) and by `middleware_test.go` guarding the policy.
- **Negative / accepted:** Continued dependency on the pinned Tailwind CLI version. Mitigated by committing both the binary cache reference and the compiled output, so a serve-time build is never required.

### Rollback Path

- **Off the token layer, back to Option B:** the custom properties are additive; removing them means inlining literal hex back into utilities/`bar-*` classes and recompiling. Non-destructive.
- **Off Tailwind entirely, to Option A:** extract the purged `app.css` as the starting point for hand-authored CSS, drop the CLI, keep serving the same single stylesheet path so the CSP and cache headers are unaffected. The `/static/app.css` URL contract and its `immutable` caching remain the stable interface regardless of how the bytes are produced.

---

## ADR-003 — Live-Update Mechanism

**Status:** Accepted

### Context

Content originates in Information-Broker's ingest pipeline and lands in PostgreSQL. SmellyFeet renders that data on read. The product is a threat-intel reader, not a trading terminal or a chat app: **sub-minute freshness is not a real requirement.** A reader who reloads a few minutes later seeing content that is up to a couple minutes stale is fully acceptable and, for this domain, expected.

Deployment routes production traffic through a **Cloudflare Tunnel** (the `smellyfeet` container + a `cloudflared` sidecar), with full-URL edge caching configured per route via `Cache-Control` in `internal/server/server.go`:

| Route | `Cache-Control` |
|-------|-----------------|
| List `/` | `public, max-age=60, s-maxage=120, stale-while-revalidate=300` |
| Article `/article/{id}` | `public, max-age=300, s-maxage=3600` |
| Stats `/stats` | `public, max-age=30, s-maxage=60` |
| About `/about` | `public, max-age=3600, s-maxage=86400` |
| Feed `/feed.xml` | `public, max-age=300, s-maxage=300` |
| Static `/static/…` | `public, max-age=31536000, immutable` |
| Errors | `no-store` |

There is also a **LAN-only** publish on `192.168.1.135:3000` that bypasses the Cloudflare edge entirely (no edge caching, no edge rate limiting there; `CF-Connecting-IP` in logs is spoofable on that path).

The question: for this read-heavy app, do we implement **no live updates**, **conditional REST polling**, **Server-Sent Events (SSE)**, or **WebSockets**?

### Options Considered

**Option A — No live updates (rely on navigation + edge cache).**
The page is fresh as of its (possibly edge-cached) render. Users get new content on their next navigation/reload; `stale-while-revalidate` lets the edge serve instantly while refreshing in the background.
- *Freshness:* Bounded by `s-maxage` + SWR window per route (e.g. list: served fresh ≤120 s, then stale-served up to 300 s while revalidating). Well inside the domain's tolerance.
- *Transport cost:* Zero. No open connections, no client runtime.
- *CSP/JS:* Requires nothing; compatible with zero-JS.

**Option B — Conditional REST polling.**
Client JS periodically re-fetches with `If-None-Match`/`If-Modified-Since`, expecting `304`s.
- Requires JavaScript → requires `script-src` → **violates the frozen CSP and the zero-JS decision.**
- Interval/backoff/jitter/failure-handling all become live concerns.
- Polling would also punch through the edge cache or fight it, undermining ADR-003's own caching premise.

**Option C — Server-Sent Events.**
Server holds a long-lived `text/event-stream` per client, pushing updates.
- Requires client JS (`EventSource`) → CSP/zero-JS violation.
- Long-lived connections do not sit well behind edge caching and add server connection-state.

**Option D — WebSockets.**
Full-duplex persistent connection.
- Requires client JS → CSP/zero-JS violation.
- Full-duplex is gratuitous for a read-only feed; adds the most operational surface (connection lifecycle, backpressure, reconnect) for the least product need.

### Decision

**No live-update mechanism (Option A).** Freshness is delivered by full-URL Cloudflare edge caching with per-route `s-maxage` and `stale-while-revalidate`. Recommended transport: **none.**

### Rationale

1. **Every push/poll option requires JavaScript, and the zero-JS + frozen-CSP decision (ADR-001) forecloses all of them.** This decision is therefore *downstream* of ADR-001: **it cannot be revisited without first reopening the zero-JS decision, which is explicitly out of scope for Phase 2.**
2. **Interval / backoff / failure-handling are moot.** Because the decision is "none," there is no polling cadence to tune, no exponential backoff to design, no reconnect storm to defend against, and no failure mode where a hung stream shows stale data with a false "live" indicator. The simplest correct behavior is the absence of the mechanism.
3. **Edge caching is operationally sufficient for this domain.** `stale-while-revalidate` gives the best of both: the edge serves a cached page instantly (fast) and refreshes it out-of-band, so the *next* visitor sees new content without any visitor ever waiting on origin. For threat-intel reading, a freshness bound measured in tens-to-hundreds of seconds is not a limitation — it is appropriate, and it protects the origin from load spikes.
4. **Least surface, most cacheable.** No open connections means the entire site remains a set of independently cacheable HTML objects, which is exactly what makes the edge strategy work. Any live transport would erode cacheability (the very property this ADR depends on).

### Consequences

- **Positive:** No client runtime, no connection state on origin, no push infrastructure. Freshness is a declarative header concern, tunable per route without code changes to any client.
- **Positive:** No "phantom liveness" bug class (a UI claiming real-time while a dead socket silently serves stale data).
- **Negative / accepted:** Content can be up to the per-route SWR bound stale. Accepted by the domain's freshness tolerance. If a specific route ever needs tighter freshness, the correct lever is **lowering that route's `s-maxage`**, not adding a transport.
- **Operational note (LAN path):** The `192.168.1.135:3000` publish bypasses the edge, so it neither benefits from edge caching nor is bounded by it — every LAN request hits origin and renders live. This is acceptable for the low-volume LAN use case but should be remembered when reasoning about origin load and about the fact that edge rate-limiting and trustworthy client IPs do not exist on that path.

### Rollback Path

Adding any live-update transport is gated, exactly as in ADR-001, on **reopening the zero-JS decision and amending the frozen CSP** — out of scope for Phase 2. If that gate is ever passed, the recommended escalation order is:
1. **Tighten `s-maxage`** on the affected route first — often this alone resolves a freshness complaint with zero new mechanism.
2. If genuine push is required, prefer **SSE over WebSockets** (read-only, one-directional, simpler), self-hosted so `default-src`/`connect-src` can be scoped to `'self'`, and add a `connect-src` directive rather than loosening anything else.
3. WebSockets only if bidirectional interaction is ever introduced (not foreseen). Each step updates `middleware.go` and `middleware_test.go` in lockstep.

---

## ADR-004 — State Management

**Status:** Accepted

### Context

The frontend has no cookies, no sessions, and no per-user state — Phase 0 decided, final, that the site stays fully public and anonymous. All filter/sort/range state already lives in **URL query parameters**: `q`, `feed`, `sort`, `page` on the list route (`/`), and `range` (`daily`/`weekly`/`monthly`) on the digest route (`/digest`). There is no JavaScript, so there is no client-side store to hold state in, and none is planned.

The question is not "which state library" but "what is the state model, and is it coherent and shareable?"

### Options Considered

We frame the four canonical state categories and where each lives (or does not) in SmellyFeet:

**Option A — Formalize the current model: server state + URL query state only; no client state; no persisted preferences.**

- **Server state:** The authoritative content — articles, digest groupings, per-feed health, stats — owned by Information-Broker/PostgreSQL, fetched per request by `internal/apiclient`, rendered by templates. The frontend holds no long-lived server-state cache of its own beyond the edge cache (ADR-003).
- **Client-only UI state:** **None.** With zero JavaScript there is no ephemeral client state (open/closed panels, unsent form input, optimistic UI). Any such affordance would require JS and is out of scope by ADR-001.
- **URL query state:** All filter/sort/range/pagination selections — `q`, `feed`, `sort`, `page`, `range`. This is the *entire* user-controllable view state, and it is in the URL by design.
- **Persisted preferences:** **None.** No cookies, no `localStorage` (no JS), no server-side per-user store. The site is anonymous; there is nothing to persist and nowhere compliant to persist it.

**Option B — Introduce a global client state library (Redux/Zustand/signals/etc.).**
Disqualified at the premise: there is no client JavaScript runtime to host a store, and adding one violates ADR-001. A state library manages state that lives in the client; SmellyFeet has no such state and no runtime for it.

**Option C — Introduce cookies/server sessions for persisted preferences.**
Disqualified by the Phase 0 final decision that the site is fully public and anonymous with no per-user state. Cookies would also complicate edge caching (cache-key on cookie) for no product benefit.

### Decision

**Adopt Option A: server state + URL query state only. No client-only UI state, no global state library, no cookies, no persisted preferences.** Confirm that filter and sort selections are already fully shareable via URL.

### Rationale

1. **There is no client to hold state in.** A global state library manages *client* state; with zero JavaScript (ADR-001) that category is empty and the library would have nothing to manage. YAGNI in its purest form.
2. **URL-as-state is already correct and has properties a client store lacks.** Because `q`/`feed`/`sort`/`page`/`range` live in the URL: every view is **bookmarkable, shareable, and linkable**; the browser Back/Forward buttons work as navigation through view states for free; and the edge cache keys naturally on the full URL, so each filter combination is independently cacheable (ADR-003). A client store would *lose* all of these.
3. **Anonymity is a feature, not a gap.** No cookies/sessions means no consent banner, no per-user cache fragmentation, no PII, and a clean threat model. The absence of persisted preferences is a deliberate simplification, not a missing feature.
4. **Server owns truth.** All content state is resolved server-side per request against the backend, so there is exactly one source of truth and no client/server reconciliation problem.

### Consequences

- **Positive:** Trivially shareable views; native Back/Forward; edge-cacheable per URL; zero state-sync bugs; zero client state code to maintain; clean anonymous threat model.
- **Positive (security/privacy):** No cookies means no CSRF surface for state, no session fixation, no tracking, and no per-user cache-poisoning vector.
- **Negative / accepted:** No server-remembered preferences (e.g. "always sort newest"). A user re-specifies via URL or a bookmark. Acceptable and arguably preferable for an anonymous public tool.
- **Constraint for implementers:** New view options must be added as URL query parameters (validated server-side; see the cross-cutting input-validation note), never as hidden client state, to preserve shareability and cacheability.

### Rollback Path

- **Persisted preferences later:** would require reopening the anonymous/no-cookie Phase 0 decision. If ever done, prefer encoding preferences in the URL or a shareable path first; only fall back to a cookie if truly per-device persistence is required, and then set `Vary`/cache-key implications explicitly so the edge strategy is not silently broken.
- **Client UI state later:** gated on ADR-001 (reopening zero-JS). Even then, the URL-as-source-of-truth model should remain primary, with any client store hydrated *from* the URL, not replacing it.

---

## ADR-005 — Charts and Data Visualization

**Status:** Accepted

### Context

The `/stats` route (and any future visualized surface) needs simple bar-style charts — e.g. collection volume per feed/day. Two hard constraints shape the solution: (1) the frozen CSP forbids inline styles (no `'unsafe-inline'` in `style-src`, and a project rule of **zero** inline `style=""` attributes), so a bar's width cannot be set with `style="width:73%"`; and (2) there is **no JavaScript**, so no charting library (Chart.js, D3, etc.) is available — they are disqualified by ADR-001 regardless.

Phase 1 already specified the approach: **precomputed, quantized CSS classes** `bar-5` through `bar-100` (widths in fixed increments), authored once in the stylesheet, applied to bar elements based on the server-computed value bucketed to the nearest step. Phase 1 also specified the **narrow-screen fallback** (labels move above bars) and that **every bar carries a visible numeric label** alongside it.

### Options Considered

**Option A — Quantized CSS-class bars (`bar-5 … bar-100`) + always-visible numeric labels (Phase 1 spec).**
Server computes each value, buckets it to the nearest quantized step, and emits `class="bar bar-70"`. The width lives in the stylesheet class; the exact number is rendered as text next to the bar.
- *CSP:* Fully compliant — width comes from a stylesheet class, not an inline style.
- *Bytes:* Adds ~20 small width rules to `app.css` once; **zero** per-render JS and no per-value generated CSS.
- *Accessibility:* The precise value is always present as readable text, so the chart is not the only representation of the data.
- *Fallback:* Pure CSS media query moves labels above bars on narrow screens; no JS.

**Option B — Inline-style bars (`style="width:73%"`).**
- *CSP:* **Blocked.** No `'unsafe-inline'` in `style-src`; the width would be dropped and every bar would collapse. Disqualified by the frozen CSP.

**Option C — SVG-rendered charts (server-generated `<svg>`).**
- *CSP:* Inline `<svg>` with geometry attributes (not `style`) is technically renderable, but bar sizing via presentational attributes/`<rect width>` works while inline `style` on SVG would again hit the CSP. Feasible but heavier: server-side SVG generation is more template complexity and larger markup than a handful of `<div>`s with width classes, for charts this simple.
- *Accessibility:* Requires deliberate `<title>`/`<desc>`/text or the data is locked in vector geometry.

**Option D — JavaScript charting library.**
- Disqualified by ADR-001 (no JS, frozen CSP). Not considered further.

**Option E — Continuous per-value generated CSS (one class per exact width, e.g. via Tailwind arbitrary values `w-[73%]`).**
- *CSP:* Compliant (generates classes, not inline styles).
- *But:* data-driven values are unbounded, so this generates a new class per distinct value, bloating `app.css` and defeating purging (see ADR-002's JIT rule). Quantizing (Option A) caps the class count at ~20 regardless of data cardinality.

### Decision

**Adopt Option A: quantized CSS-class bars (`bar-5` … `bar-100`) with a visible numeric label beside every bar, and a CSS-only narrow-screen fallback that moves labels above bars.**

### Rationale

1. **It is the lightest approach that satisfies both hard constraints.** No JS (respects ADR-001/CSP), no inline styles (respects the frozen CSP and ADR-002), and a bounded ~20-rule CSS footprint added once — versus SVG's heavier markup or continuous generated CSS's purge-defeating bloat.
2. **Quantization is the key idea.** Bucketing values to fixed steps means the stylesheet needs a *fixed, small* set of width classes no matter how many bars or how varied the data. This is what makes the CSP-safe, pre-authored-class approach scale.
3. **The data is never chart-only.** Because a visible numeric label accompanies every bar, the exact figure is available as text to everyone — screen-reader users, users on the narrow-screen fallback, and anyone for whom the visual bar is ambiguous. The bar is an *at-a-glance* aid layered on top of the authoritative number, satisfying WCAG 2.2 AA (the accessibility conformance target fixed in Phase 0) and the general rule that no data point in this redesign is chart-only. Quantization affects only the visual bar length, never the printed number.
4. **The fallback is free.** Moving labels above bars on narrow screens is a CSS media-query concern, requiring no JavaScript and no second code path.

### Consequences

- **Positive:** CSP-compliant charts with zero JS and a tiny, cache-friendly CSS cost. Fully accessible by construction (numbers always visible). Responsive via pure CSS.
- **Negative / accepted:** Visual bar length is quantized to the nearest step, so two close-but-distinct values can render the same bar width. This is a *visual* approximation only — the exact values remain distinguishable via their labels. Accepted; the bar is a glanceable aid, not the data of record.
- **Constraint for implementers:** Bucketing happens **server-side** from backend-computed values; templates emit the chosen `bar-N` class. Never compute widths from user input, and never fall back to inline styles.

### Rollback Path

- **Finer resolution:** add more quantization steps (e.g. `bar-2.5` increments) — a stylesheet-only change, still bounded and CSP-safe.
- **Richer charts (line/area/stacked):** move to **server-generated SVG** (Option C), sizing via geometry attributes (never inline `style`), with `<title>`/`<desc>` and retained text labels. This stays within the frozen CSP and zero-JS posture. A JS charting library remains gated on ADR-001 and is the last resort.

---

## ADR-006 — Backend API Extensions

**Status:** Proposed (B1 requires explicit human sign-off before implementation)

### Context

Phase 1 identified and individually justified a set of Information-Broker API extensions in the `REDESIGN_PLAN.md` "Phase G" table. This ADR gives each one an ADR-style writeup: **data source, meaning, type, null behavior, authorization needs, and backward-compatibility plan.** Two cross-cutting constraints bind all of them:

- **Authorization:** The frontend is fully public and anonymous, and **no authentication or authorization mechanism exists** anywhere in the stack (ADR-004, Phase 0). Every one of these endpoints/fields is therefore **public, unauthenticated read data**. The only relevant access controls are the environment ones already in place: production traffic through the Cloudflare edge (edge rate limiting available), and the LAN-only `192.168.1.135:3000` path that bypasses the edge (no edge rate limiting, spoofable `CF-Connecting-IP`). New read endpoints inherit that posture; none should ever return per-user or sensitive data because there is no per-user data and no auth to gate it.
- **Backward compatibility:** All extensions are **additive only** — new optional query parameters, new response *fields*, or new *endpoints*. **No existing response shape changes and no existing field is removed or repurposed.** A frontend built against today's responses keeps working unchanged; new behavior is opt-in.

The five items follow, labeled **B1–B5** consistently with `security_review.md` §5: **B1 is a genuinely open Phase 0 item (audit §9, item 7) and is presented with options for human sign-off, not silently decided. B5 is conditional.**

---

### ADR-006.1 — B1: Digest pagination / limit parameter

**Status:** **Proposed — needs final human sign-off on the exact parameter shape before Information-Broker implementation.** (Open item from Phase 0 audit §9, item 7.)

**Context / justification.** The digest endpoint (backing the frontend `/digest` route, whose handler calls `svc.GetDigest` for `range` ∈ {`daily`,`weekly`,`monthly`}) currently returns **every** article in the selected window with **no cap**. As ingest volume grows, the payload, the server render time, and — importantly — the **edge-cache object size** all grow unbounded. Large cached objects degrade edge efficiency and can approach object-size limits. A bound is needed.

- **Data source:** The same digest query in Information-Broker over the articles in the selected `range` window in PostgreSQL.
- **Meaning:** A limit on how many articles (overall or per digest section/day) are returned, so payload size is bounded independent of ingest volume.
- **Type:** Integer parameter(s) on the request; response gains a small boolean/int signal indicating truncation (shape depends on the option chosen below).
- **Null behavior:** Parameter **omitted → preserve today's behavior** (no cap) for strict backward compatibility, *or* apply a safe server-side default cap. Which of these is the default is part of what needs sign-off (a silent default cap is technically a behavior change for existing callers, even if additive in spirit).
- **Authorization:** Public, none.
- **Backward compatibility:** New optional query param + new optional response field. Existing callers that omit the param must keep working; the truncation-signal field is additive.

**Options for the parameter shape (for human decision):**

- **Option 1 — `limit` / `offset` pagination.** Classic. Flexible, supports "load more" style paging. *Cost:* offset paging fragments the edge cache into many URL variants and invites deep-offset queries; also implies a notion of total/pages the digest UX deliberately avoids. Heavier than the digest UX needs.
- **Option 2 — Per-section cap + `truncated` flag *(recommended)*.** Cap each digest section (e.g. per day) at *N* articles server-side; when a section is trimmed, set a `truncated: true` (and optionally `omittedCount`) field on that section. *Benefit:* bounds payload and cache-object size with **one** canonical URL per `range` (no offset explosion, preserves clean edge caching from ADR-003), and matches the digest's "most important, grouped" intent rather than exhaustive paging. Backward compatible: omit the cap param → uncapped; the `truncated` field is additive and simply absent/false today.
- **Option 3 — Hard window-size ceiling.** A single global max article count per digest response, applied server-side. Simplest to implement and reason about, single cache object. *Cost:* coarser — can drop a whole tail of a busy window without the per-section fairness of Option 2.

**Recommendation:** **Option 2 (per-section cap + `truncated` flag)** best preserves the single-cacheable-URL-per-`range` property that ADR-003 depends on, bounds the object size, and fits the digest's grouped-importance UX without introducing a page count. **However, per Phase 0 audit §9 item 7, the exact shape — including whether the default is "uncapped" or a safe server-side cap, and the precise field names — must receive final human sign-off before Information-Broker implements it. This ADR flags it as open, not settled.**

---

### ADR-006.2 — B2: Cluster / embedding-coverage signal

**Status:** Proposed

**Context / justification.** The `story_cluster_id` and `summary_embedding` columns already exist in PostgreSQL, populated asynchronously by the Ollama-backed story-clustering pipeline. But nothing aggregates them for the frontend, so the UI **cannot distinguish "clustering hasn't processed this window yet" from "there genuinely are no shared/cross-source stories."** These are very different messages to show a reader, and today they look identical.

- **Data source:** Aggregate over `story_cluster_id` / `summary_embedding` for the articles in the relevant window (e.g. count of articles with a non-null `summary_embedding` and/or assigned `story_cluster_id` versus total).
- **Meaning:** A coverage signal — how much of the current window has been embedded/clustered — so the frontend can render "clustering in progress / not yet processed" distinctly from "no shared stories found."
- **Type:** A small additive object or fields — e.g. `embeddedCount` (int), `totalCount` (int), and/or a derived `coverage` ratio or a `clusteringComplete` boolean — on the digest/relevant response, or a dedicated lightweight endpoint.
- **Null behavior:** If the pipeline has processed nothing for the window, the signal reflects zero coverage (not null-as-error). Consumers treat "coverage low/zero" as "in progress," not "no stories."
- **Authorization:** Public, none.
- **Backward compatibility:** Additive fields or a new endpoint. Existing digest consumers ignore the new fields and behave exactly as today.

---

### ADR-006.3 — B3: Per-feed error rates

**Status:** Proposed

**Context / justification.** The Signal Lamp direction (ADR-002 / Phase 1) introduces **per-feed status dots** for feed health. `fetch_logs` already holds the rows to compute per-feed fetch success/failure, but **no per-feed join is exposed** today, so the health dots have no data source. This item completes the Signal Lamp health system.

- **Data source:** Aggregation of `fetch_logs` grouped by feed (e.g. recent success/failure counts or last-fetch outcome per feed), joined to the feed list.
- **Meaning:** A per-feed health/error-rate figure driving the `status-ok` / `status-warn` / `status-critical` dot for each feed — all **server-rendered from this backend-computed field, never from user input** (consistent with the Signal Lamp rule that severity is computed server-side).
- **Type:** Additive per-feed fields — e.g. `errorRate` (float) or `recentFailures`/`recentAttempts` (ints), and/or a precomputed `status` enum (`ok`/`warn`/`critical`) so severity thresholds live in one authoritative place (backend) rather than being re-derived in the template.
- **Null behavior:** A feed with no recent fetch attempts → status "unknown"/neutral rather than a misleading "ok" or "critical"; the frontend renders a neutral dot. Null must not be coerced into a false-healthy signal.
- **Authorization:** Public, none. (Feed health is not sensitive.)
- **Backward compatibility:** Additive fields on the feed listing (or a small companion endpoint). Existing consumers ignore them.

**Note:** Prefer the backend to emit the **precomputed severity enum**, so the ok/warn/critical thresholds are defined once server-side; the frontend then maps enum → `status-*` CSS class (ADR-002) with no threshold logic of its own.

---

### ADR-006.4 — B4: Day-by-day ingestion time-series

**Status:** Proposed

**Context / justification.** `/stats` today offers only **point-in-time window aggregates** (today / week / month) with **no trend direction** — a reader cannot see whether ingestion is rising, falling, or steady. A day-by-day series enables the collection-volume trend chart (rendered via the ADR-005 quantized bars).

- **Data source:** Article counts grouped by ingest day over a bounded range in PostgreSQL (e.g. last N days).
- **Meaning:** An ordered series of `{ date, count }` points describing ingestion volume over time, for a trend chart.
- **Type:** Additive array of `{ date: string (ISO date), count: int }` on the stats response, or a dedicated endpoint. Range should be **bounded** (a fixed lookback window) so the payload and its edge-cache object stay bounded (consistent with the ADR-003/B1 concern about unbounded responses).
- **Null behavior:** Days with zero ingestion should appear explicitly as `count: 0` (dense series) rather than being omitted, so the chart's time axis has no silent gaps; an empty overall range returns `[]`, not null.
- **Authorization:** Public, none.
- **Backward compatibility:** Additive field/endpoint; existing point-in-time stats fields are untouched and continue to render.

---

### ADR-006.5 — B5: Articles-list total-count field (conditional)

**Status:** Proposed — **conditional; add only if pagination UX proves painful in Phase 3, not by default.**

**Context / justification.** The list route (`/`) paginates via the `page` query param but the current design **deliberately omits "page X of Y"** rather than fabricate a total — a considered choice, not an oversight. A total-count field is justified **only conditionally**: if, during Phase 3 implementation, the absence of a total makes the pagination UX genuinely painful (e.g. users can't tell they've reached the end). It is **not** added by default.

- **Data source:** A `COUNT(*)` over the same filtered article query (respecting the active `q`/`feed`/`sort` filters) in PostgreSQL.
- **Meaning:** Total number of articles matching the current filter set, to support "page X of Y" or an end-of-results indicator **if** adopted.
- **Type:** Additive optional integer field (e.g. `totalCount`) on the list response.
- **Null behavior:** If not implemented, the field is simply absent and the frontend behaves exactly as today (no total shown). If implemented, it reflects the filtered total; an empty result set is `0`, not null.
- **Authorization:** Public, none.
- **Backward compatibility:** Additive optional field. Because the current design shows no total, its absence is already the supported state; adding it later breaks nothing.
- **Cost note:** A filtered `COUNT(*)` is a second query per list render and can be non-trivial on large tables. This cost is another reason to add it **only when the UX pain is demonstrated in Phase 3**, and to consider caching it alongside the already edge-cached list route rather than counting on every request.

---

### Decision (ADR-006 overall)

Propose all five extensions as **additive, public, unauthenticated** API changes to Information-Broker, to be implemented as new optional parameters, new response fields, or new endpoints — **never** as breaking changes to existing responses. **B1 (digest pagination) is explicitly gated on human sign-off of its parameter shape** (recommended: Option 2, per-section cap + `truncated` flag) before implementation. **B5 (articles total count) is gated on demonstrated Phase 3 pagination-UX pain** and is otherwise not built.

### Rationale

1. **Additive-only keeps the frontend/backend contract safe.** New fields/endpoints let the frontend adopt at its own pace; a frontend built against today's responses keeps working, satisfying the KISS/backward-compatibility bar.
2. **Public-by-default matches reality.** There is no auth and no per-user data (ADR-004); every item is non-sensitive read data. The relevant protections are environmental (edge rate limiting on the Cloudflare path; awareness that the LAN path bypasses it) — not per-endpoint authz, which does not exist and is not being introduced.
3. **Each item closes a specific, justified gap:** B1 bounds an unbounded response (payload/render/cache-size); B2 disambiguates "in progress" from "empty"; B3 (per-feed error rates) supplies the Signal Lamp health dots; B4 (the day-by-day series) adds trend where only point aggregates exist; B5 (the total-count) is held back precisely to avoid fabricating data the current design intentionally omits.
4. **Bounding is a recurring theme.** B1's cap and B4's fixed lookback both exist to keep response and edge-cache object sizes bounded as data grows — the same operational concern ADR-003 relies on.

### Consequences

- **Positive:** The frontend can render feed health, clustering coverage, ingestion trend, and bounded digests without any breaking change; each rollout is independent and opt-in.
- **Positive (security posture):** Emitting **precomputed severity/status enums** from the backend (B3, per-feed) keeps threshold logic server-side and the frontend free of data-derived branching, reinforcing "severity is server-computed, never from user input."
- **Negative / accepted:** B1's final shape is a **known open decision** blocking its implementation until signed off — deliberately surfaced here rather than decided unilaterally. B5 adds a second `COUNT(*)` query if adopted, hence its conditional gating.
- **Coordination:** These are Information-Broker changes; the frontend cannot implement them alone. They should be tracked against the `REDESIGN_PLAN.md` Phase G table and sequenced with backend work.

### Rollback Path

Because every item is additive, rollback is clean per item:
- **New field:** stop emitting it; consumers already tolerate its absence (that is today's state). No frontend break.
- **New endpoint:** remove the route; the frontend feature that consumed it degrades to its pre-extension rendering.
- **B1 specifically:** if a chosen cap shape proves wrong, revert to uncapped (today's behavior) by omitting/ignoring the parameter, then re-decide the shape — no existing caller is affected because the param is optional. This reversibility is itself an argument for not over-committing before sign-off.

---

## Cross-Cutting Note — Untrusted-Content Handling (preserve, do not rebuild)

The RSS/untrusted-content defenses are **already shipped and must be preserved, not re-litigated or rebuilt.** They are recorded here because several ADRs above depend on them:

- **Ingest-time HTML stripping (Information-Broker):** `monitor.go`'s `extractMainContent` uses goquery's `.Text()` to strip **all** HTML markup at ingest. PostgreSQL stores **plain text only — never HTML.**
- **UTF-8 / rune-safe truncation (`sanitize.go`):** handled separately from escaping — this is **not** an HTML allowlist, and there is **no** `bluemonday`-style dependency.
- **Template auto-escaping (SmellyFeet):** Go `html/template` contextual auto-escaping is the second layer; grep confirms **zero** uses of `template.HTML` or `template.URL` anywhere in the codebase. External links use `rel="noopener noreferrer" target="_blank"`, and `html/template`'s URL-context filter neutralizes dangerous schemes (e.g. `javascript:`) automatically.
- **Atom feed (`feed.go`):** uses `encoding/xml` marshaling, which XML-escapes independently.
- **CSP as backstop:** with `script-src` inheriting `'none'` (ADR-001), even a hypothetical escaping miss has no script-execution path in the browser.

This defense-in-depth (plain-text-at-rest → contextual escaping → `script-src 'none'`) is why the ADRs above can treat rendered content as safe. **Any change touching rendering, templates, or the CSP must keep all four layers intact.**

---

## Input-Validation Note (trust boundaries)

All URL query parameters (`q`, `feed`, `sort`, `page`, `range`) and any future ADR-006 parameters are **untrusted input at a system boundary** and must be validated server-side before use — e.g. `range` is already whitelisted against `{daily, weekly, monthly}` in the digest handler (`validDigestRanges`). New parameters (notably B1's limit/cap) must be range-checked and defaulted safely, and must never be interpolated into SQL except via parameterized queries in Information-Broker. This is a non-negotiable trust-boundary requirement and is not simplified away by any decision above.

---

*End of ADRs. ADR-001 through ADR-005 are Accepted. ADR-006 is Proposed; ADR-006.1 (B1) additionally requires human sign-off on its parameter shape, and ADR-006.5 (B5) is conditional on demonstrated Phase 3 need.*
