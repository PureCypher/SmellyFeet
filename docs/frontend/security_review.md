# SmellyFeet Frontend — Security Review (Phase 2)

**Owner:** Opus (Phase 2 — architecture & security decisions)
**Redesign direction:** Signal Lamp (Phase 1, Approved)
**Scope:** the `smellyfeet` frontend service only — a zero-dependency Go 1.22 `net/http` + `html/template` server. The `Information-Broker` backend, its Postgres store, and its Ollama sidecar are cross-referenced where relevant but are out of scope for this frontend review.
**Status:** Approved with three carry-forward items (two MEDIUM-severity deployment/backend cross-references, one INFO-level note) and one open item requiring human sign-off (B1). See §7.

---

## 0. Scope & Method

This review formalizes the security posture of the frontend redesign. It is a *review of settled inputs*, not a re-litigation of decisions already Approved in Phases 0 and 1. Every mitigation claimed as "shipped" below was verified against the working tree at review time; load-bearing claims are anchored to `file:line`.

The governing security thesis of this frontend is unusually strong and worth stating up front, because every finding follows from it:

> **The site ships zero JavaScript, stores zero HTML, holds zero per-user state, and makes zero third-party requests.**

Three of the four classic web-app attack surfaces (client-side script execution, third-party supply chain, session/auth handling) are not *mitigated* here — they are *structurally absent*. The redesign preserves that absence. Signal Lamp adds only server-rendered CSS classes and server-rendered text; it introduces no new script, no new network origin, and no new rendering path for untrusted content.

Verified invariants underpinning this review:

| Invariant | Verified at |
|---|---|
| Frozen CSP string, enforced and asserted in tests | `internal/server/middleware.go:10`, `internal/server/middleware_test.go:9` |
| `X-Content-Type-Options: nosniff` + `Referrer-Policy: strict-origin-when-cross-origin` sent on every response | `internal/server/middleware.go:39-40` |
| Zero uses of `template.HTML` / `template.URL` / `template.JS` / `template.CSS` anywhere | codebase-wide grep — no matches |
| External article link carries `target="_blank" rel="noopener noreferrer"` | `internal/server/templates/article.html:23` |
| Zero inline `style="…"` attributes in templates | template grep — no matches |
| Zero third-party origins referenced (no `<script>`, no CDN `<link>`, no `@import`, no web-font host) | template grep — no matches |
| All static assets (`app.css`, fonts) `go:embed`-ded and served from origin | `internal/server/server.go:24-27,178` |
| Error page renders only a fixed server-side constant, never a raw error | `internal/server/templates/error.html`, `internal/server/server.go:216` |

---

## 1. Threat Model

Assessed against the STRIDE-adjacent surfaces named in the master prompt. Each row states the threat, the pre-existing mitigation, and whether Signal Lamp changes anything.

### 1.1 Malicious RSS `title` / `description` / `author` / `enclosure` / `link` fields

**Status: Mitigated (defense in depth, two independent layers). No new exposure from Signal Lamp.**

Untrusted feed content is neutralized *twice*, at two different trust boundaries:

1. **At ingest (Information-Broker).** `monitor.go` `extractMainContent` runs feed content through goquery's `.Text()` extraction, which discards *all* HTML markup. Postgres never stores HTML — only plain text. UTF-8 validity and rune-safe truncation are enforced separately by `sanitize.go` (this is a text-normalization step, *not* an HTML allowlist — there is deliberately no `bluemonday`-style dependency, because there is no HTML to allow-list). By the time any feed field reaches the frontend, it is already plain text.
2. **At render (frontend).** Every field is emitted through Go `html/template`, whose contextual auto-escaping is the second, independent layer. A title, author, or description is HTML-escaped for its element context regardless of what the backend stored. `enclosure` is not rendered as active content; a feed `link` is rendered only as an `href` (see §1.4). Confirmed: **zero** `template.HTML`/`template.URL` escape-hatches exist anywhere, so no field can bypass contextual escaping.

The two layers are independent: even if the ingest strip regressed and HTML reached the DB, `html/template` would still escape it into inert text on the way out (and vice-versa). Signal Lamp adds no field that is rendered as markup and adds no `template.HTML` call — every new value it introduces (severity class names, health counts) is server-computed, not feed-derived.

### 1.2 Stored XSS through archived feed content

**Status: Mitigated at ingest. No frontend exposure.**

Because HTML is stripped *before* storage (§1.1, layer 1), the archive contains no HTML to replay. There is no "render the stored article body as HTML" path anywhere in the frontend — article bodies are plain text emitted through `html/template`. A poisoned feed cannot plant a stored payload that later executes, because the payload never survives ingest as markup and would be escaped even if it did.

### 1.3 DOM-based XSS through client-side filtering/rendering

**Status: Not applicable — no client-side execution surface exists.**

There is no JavaScript in this application. All filter/sort/range state lives in URL query parameters (`q`, `feed`, `sort`, `page` on the list route; `range` on the digest route) and is consumed **server-side**; the server re-renders HTML on every navigation. There is no client-side templating, no `innerHTML`, no `eval`, no `document.write`, no DOM manipulation of any kind — because there is no script to do it. DOM-based XSS requires a JavaScript sink; the frozen CSP forbids script entirely (§3), so this class of bug has no surface to land on. Signal Lamp does not add JavaScript and does not change this.

### 1.4 Unsafe URL schemes (`javascript:`, `data:`, malformed URLs)

**Status: Mitigated by `html/template`'s URL-context filter. Explicitly covers both href families.**

There are two distinct href families in the frontend, and both are safe by different mechanisms:

- **The article external link** (`article.html:23`, `href="{{ .URL }}"`) is the only place an attacker-controlled *absolute* URL is rendered directly into an `href`. Because `{{ .URL }}` sits in URL-attribute context, `html/template` applies its `urlFilter`. That filter allows only URLs whose scheme is `http`, `https`, or `mailto` (plus scheme-relative and relative URLs); **any other scheme — including `javascript:`, `data:`, `vbscript:`, and unknown/malformed schemes — is replaced with the inert sentinel `#ZgotmplZ`.** A feed advertising `href="javascript:alert(1)"` therefore renders as a dead `#ZgotmplZ` link, not an executable one. This is automatic and requires no code from us; it is confirmed present because no `template.URL` escape-hatch disables it.
- **The list/digest source pills** (`list.html:7`, `href="/?feed={{ .FeedURL | urlquery }}"`) are *internal, same-origin* navigations. The untrusted portion (the feed URL) is confined to a **query parameter** and passed through the `urlquery` escaper, so it is percent-encoded into the query string and can never present itself as an href *scheme* at all. This path is strictly stronger than the `urlFilter` path — there is no attacker-controlled scheme position to abuse. Pagination and active-filter links follow the same pattern (`list.html:56-58,83-85`).

**Verdict:** every untrusted URL either passes through `urlFilter` (external article link) or is confined to a query parameter and `urlquery`-escaped (all internal navigations). No unsafe scheme can reach a live `href`. Signal Lamp introduces no new URL-rendering path.

### 1.5 External-link tabnabbing & referrer leakage

**Status: Mitigated by `rel="noopener noreferrer"` + `Referrer-Policy` header.**

The single external link (`article.html:23`) carries `rel="noopener noreferrer"`:

- `noopener` severs `window.opener`, defeating reverse-tabnabbing (the destination page cannot navigate our tab).
- `noreferrer` sends **no** `Referer` header at all for that specific click, so the destination learns nothing about the article path the visitor came from.

For every *other* navigation (internal links, and any future external link that forgets `noreferrer`), the site-wide `Referrer-Policy: strict-origin-when-cross-origin` header (`middleware.go:40`) is the backstop: cross-origin requests leak only the bare origin (`https://smellyfeet.example`), never the path or query. The per-link `noreferrer` and the header are complementary — the link is maximally private and the header catches everything the link attribute does not. Signal Lamp adds no new external links.

### 1.6 Third-party chart / font / CDN / analytics / script supply-chain risk

**Status: None. Zero third-party requests. Confirmed to hold for Signal Lamp.**

Everything the browser loads originates from our own domain:

- **CSS:** Tailwind is compiled **at build time** into a static `static/app.css` that is `go:embed`-ded into the binary and served from `/static/` on our origin (`server.go:24-27,178`). This is the compiled-stylesheet path, **not** the runtime Tailwind Play CDN — the Play CDN ships a JavaScript runtime and would require `script-src` plus a CDN origin, both of which the frozen CSP forbids, so its absence is structurally guaranteed, not merely policy.
- **Fonts:** served from the embedded `static` FS under `font-src 'self'`. No Google Fonts, no external font host.
- **Charts:** server-rendered (CSS/SVG bars driven by precomputed quantized classes — see §2, inline-style avoidance). No client charting library, hence no library supply chain.
- **Analytics:** none. No trackers, no beacons, no pixels.

A codebase-wide template grep for `<script>`, external `<link>`, `@import`, `googleapis`, and `fonts.g*` returns nothing but the expected article external link. Signal Lamp is **CSS-only** (severity color classes) and server-rendered text; it adds no asset host, no font, no script, no analytics. The zero-third-party-request property holds unchanged.

### 1.7 UI redress / clickjacking

**Status: Mitigated by CSP `frame-ancestors 'none'`.**

The frozen CSP includes `frame-ancestors 'none'` (§3), which forbids *any* origin — including our own — from embedding these pages in a `<frame>`, `<iframe>`, `<object>`, or `<embed>`. This is the modern, CSP-native successor to `X-Frame-Options: DENY` and is respected by all current browsers. There is no clickjacking surface because the pages cannot be framed at all.

### 1.8 Information leakage through metrics or error messages

**Status: Mitigated for shipped copy; one Signal-Lamp addition needs a genericization rule (see §1.10).**

- **Error pages.** The error template renders only `{{ .Message }}` (`error.html`), and `Message` is a **fixed server-side constant** — currently `"The article service is currently unavailable. Please try again later."` (`server.go:216`). The raw `error` value is never interpolated into the response. Other error paths use static strings too (`http.Error(w, "Internal Server Error", …)` at `server.go:203`; `"upstream unavailable"` at `feed.go:57`). No stack trace, no hostname, no port, no DB error, no upstream HTTP status reaches the client.
- **Phase 1 planned copy** (`"The intelligence backend is unreachable."`) was reviewed for topology leakage. It names no host, IP, port, technology, or path. It *does* mildly telegraph a two-tier architecture (a frontend fronting a separate "backend") — but that is low-value to an attacker and arguably self-evident for an RSS aggregator. **Verdict: acceptable.** Recommendation: keep the phrasing generic and user-facing, and treat "`{{ .Message }}` must remain a fixed constant, never `err.Error()` or an upstream status code" as a **MUST-PRESERVE invariant** for Phase 3 implementation. This is the single most important error-handling control in the app and it is cheap to break by accident.
- **Metrics.** The frontend exposes no Prometheus endpoint of its own; Information-Broker's metrics are not proxied through the public frontend.

### 1.9 Abuse of polling or real-time endpoints

**Status: Not applicable.**

There are no polling, streaming, WebSocket, SSE, or long-poll endpoints. All four public routes (`/`, `/digest`, `/stats`, `/about`) are GET-only, read-heavy, fully server-rendered request/response cycles, and edge-cached (§4). There is no real-time channel to abuse, and none is planned. The Cloudflare edge cache (60s–1y `s-maxage` per route) further absorbs repeat load for the tunnel-fronted path; see §4 for the one path that bypasses it.

### 1.10 Excessive retention / exposure of article content and LLM metadata

**Status: Largely a backend concern; ONE Signal-Lamp addition is a genuine frontend leakage risk and gets a concrete rule.**

Retention of article bodies, `summary_embedding` vectors, and LLM metadata is an Information-Broker/Postgres concern outside this frontend's control — cross-referenced to that service's own posture. What *this* redesign changes is **what the frontend newly displays publicly**. The new Signal Lamp / stats surfaces expose backend operational data that was previously internal:

| Newly-displayed field | Leakage risk | Disposition |
|---|---|---|
| Per-feed fetch counts / health dots (Signal Lamp) | **Low.** Reveals which feeds are configured (already public via the source filter) and rough health. No topology. | Safe to display as-is. |
| Cluster/embedding coverage signal (B2) | **Low.** An aggregate count/percentage. | Safe. |
| Day-by-day ingestion counts (B4) | **Low.** Aggregate volume already implied by the public article list. | Safe. |
| **Summarization panel "most-recent-error" string** | **HIGH if rendered raw.** A raw backend/Ollama error can carry internal hostnames, the Ollama endpoint URL, model names, filesystem paths, DB connection fragments, or stack text. | **Must be genericized before display — see recommendation.** |

**Concrete recommendation (frontend, Phase 3):** never render a raw backend or LLM error string to an unauthenticated public visitor. Choose one of, in order of preference:
1. **Do not surface a free-form error string at all.** Show only an enumerated health state computed server-side (`healthy` / `degraded` / `recovering`), driven by a boolean or small enum from the backend. This is the Signal Lamp pattern already — extend it, don't bolt a raw string onto it.
2. If a human-readable hint is genuinely wanted, **map** the backend error to a fixed allowlist of known-safe phrases server-side; render the mapped phrase, never the source string.
3. As a last resort, **truncate and scrub** before render: reject/strip any candidate string containing a URL, filesystem path, IP address, port, or `:` scheme separator, and cap length hard.

Per-feed error *rates* and *counts* (B3) are fine as numbers. Free-form error *strings* are the only real risk here, and rule (1) removes it cleanly.

---

## 2. Mandatory Controls

Status of each control the master prompt requires. "Shipped" = present and verified in the working tree; "Action" = recommended change for Phase 3.

| # | Control | Status | Notes / evidence |
|---|---|---|---|
| 2.1 | Context-appropriate output encoding | **Shipped** | `html/template` contextual auto-escaping on every field; zero `template.HTML`/`template.URL`/`template.JS`/`template.CSS`. |
| 2.2 | Sanitized plain-text rendering by default | **Shipped** | HTML stripped at ingest via goquery `.Text()` (`monitor.go extractMainContent`); Postgres stores plain text only. UTF-8/rune-safe truncation in `sanitize.go`. |
| 2.3 | Explicit URL validation & allowlisted schemes | **Partial → recommend explicit check** | Today relies on `html/template`'s built-in `urlFilter` (allowlist: `http`/`https`/`mailto` + relative; everything else → `#ZgotmplZ`). This is *sufficient and correct*. Because Phase 3 will touch these templates anyway, **recommend adding a defensive, documented scheme allowlist at the data boundary** (validate `article.URL` scheme ∈ {`http`,`https`} when it is read from the backend, before it ever reaches the template) so the guarantee is asserted in code we own and does not silently depend on an implicit stdlib behavior. LOW severity — defense in depth, not a gap. |
| 2.4 | `rel="noopener noreferrer"` on external links | **Shipped** | `article.html:23`. |
| 2.5 | Deliberate `Referrer-Policy` | **Shipped** | `strict-origin-when-cross-origin` (`middleware.go:40`). |
| 2.6 | Clickjacking protection via CSP `frame-ancestors` | **Shipped** | `frame-ancestors 'none'` in the frozen CSP. |
| 2.7 | Restrictive CSP compatible with the stack | **Shipped & frozen** | Full string reproduced in §3. |
| 2.8 | Avoidance of inline scripts / inline event handlers | **Shipped** | Zero JavaScript in the app; no `on*=` handlers, no `<script>`. |
| 2.9 | Avoidance of `unsafe-inline` | **Shipped — and a hard constraint on all future work** | The CSP contains **no** `unsafe-inline` anywhere. Enforcement mechanism: `style-src 'self'` with **zero** inline `style="…"` attributes in any template (verified). **This constrains ALL future component work, including the new Signal Lamp severity-indicator components: they MUST use the quantized-class pattern (`bar-5`…`bar-100`, `nth-child` stagger-delay classes) or CSS custom properties defined in the embedded stylesheet — never an inline `style=` attribute.** A single inline style would force `style-src 'unsafe-inline'` and break the whole no-inline guarantee. This is a Phase 3 review gate. |
| 2.10 | SRI for third-party static assets | **N/A** | There are no third-party assets. All CSS/fonts are `go:embed`-ded and served from `'self'`; SRI applies only to cross-origin subresources, of which there are none. |
| 2.11 | Security headers appropriate to deployment | **Shipped (CSP, Referrer-Policy, nosniff) + ACTION: add `Permissions-Policy`** | See recommendation below. Phase 0 flagged the absence of `Permissions-Policy`. |
| 2.12 | Dependency audit & lockfile integrity | **N/A for this service (frontend has zero deps) → cross-reference backend** | `go.mod` is `module smellyfeet / go 1.22` with an **empty require block** — nothing to audit, no third-party supply chain, no lockfile drift possible. Information-Broker's dependencies (`goquery`, `gofeed`, `lib/pq`, Prometheus client, Ollama client) **are out of scope for this frontend review** but should be audited under Information-Broker's own posture (`govulncheck` + pinned `go.sum`). Flagged as cross-reference, not owned here. |
| 2.13 | Safe error handling (no stack traces / internal topology) | **Shipped** | Error template renders a fixed constant (`error.html` + `server.go:216`); other error paths use static strings (`server.go:203`, `feed.go:57`). Raw errors never reach the client. See §1.8 for the MUST-PRESERVE invariant and §1.10 for the one new field that needs genericization. |

### 2.11 Recommendation — add `Permissions-Policy`

Phase 0 correctly flagged the absence of a `Permissions-Policy` header. Because the site uses zero JavaScript and no powerful browser features, the correct policy is to **deny everything** — pure defense in depth, zero functional cost, set once alongside the other static headers in `middleware.go`. Recommended concrete value:

```
Permissions-Policy: accelerometer=(), autoplay=(), camera=(), display-capture=(), encrypted-media=(), fullscreen=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), midi=(), payment=(), picture-in-picture=(), publickey-credentials-get=(), screen-wake-lock=(), usb=(), xr-spatial-tracking=()
```

Every feature is denied to all origins (`=()`). This is safe precisely *because* the app requests none of these features; if a future dependency or embed tried to, the browser would refuse it. **Severity: MEDIUM / nice-to-have** — recommend adding in Phase 3 when `middleware.go` is next touched. It should also be covered by an assertion in `middleware_test.go`, matching how the CSP and existing headers are already tested.

---

## 3. CSP Baseline

The Content-Security-Policy is **frozen** — verified live and asserted in `middleware_test.go`. It must not change. Exact string (`middleware.go:10`):

```
default-src 'none'; style-src 'self'; font-src 'self'; img-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'
```

### 3.1 Directive-by-directive

| Directive | Value | Why |
|---|---|---|
| `default-src` | `'none'` | The keystone. Deny-by-default for **every** fetch directive not otherwise specified. Any resource type without its own directive inherits `'none'` and is blocked outright. This is what makes the policy allowlist-shaped rather than blocklist-shaped. |
| `script-src` | *(not set → inherits `'none'`)* | **No `script-src` directive exists**, so it inherits `default-src 'none'` — the browser will execute no script from any source, inline or external. An explicit `script-src 'none'` would be *exactly equivalent*; the inherited form is correct and idiomatic. The site ships zero JavaScript (a decision made twice in prior Approved specs and reaffirmed in Phase 0/1), so this is the enforced browser-level guarantee behind that decision: even if a script tag were somehow injected, the browser refuses to run it. |
| `style-src` | `'self'` | Stylesheets load only from our own origin (the embedded, build-time-compiled `app.css`). Note there is **no `'unsafe-inline'`**, which is what forbids inline `style="…"` attributes and inline `<style>` blocks — the enforcement mechanism behind §2.9. This is why data-driven visuals use quantized CSS classes, not inline styles. |
| `img-src` | `'self'` | Images load only from our origin. No remote images, no tracking pixels, no `data:` image URIs. |
| `font-src` | `'self'` | Fonts load only from our origin (embedded). No Google Fonts / external font CDNs. |
| `connect-src` | *(not set → inherits `'none'`)* | Inherits `default-src 'none'`, so `fetch`/XHR/WebSocket/EventSource are all blocked. This is **correct and costless**: with zero JavaScript there is no code that could issue a `fetch`, open a WebSocket, or start an `EventSource` in the first place. The directive formalizes what the no-JS decision already guarantees. |
| `frame-ancestors` | `'none'` | No origin may frame these pages — the clickjacking control (§1.7). Supersedes `X-Frame-Options`. |
| `base-uri` | `'self'` | Restricts `<base href>` to our origin, preventing an injected `<base>` tag from re-pointing all relative URLs at an attacker origin. |
| `form-action` | `'self'` | Form submissions may target only our origin. The only forms are the GET-only search/filter controls on the list route; there is no POST/PUT/DELETE anywhere in the frontend. Constrains where any form can send data even so. |
| `object-src` | *(not set → inherits `'none'`)* | Inherits `default-src 'none'`, blocking `<object>`/`<embed>`/`<applet>` (legacy plugin XSS vectors). Correct — the site uses none. |

### 3.2 Signal Lamp requires ZERO CSP changes

**Explicitly stated for the record:** the Signal Lamp redesign requires **no change to this policy**. It adds only (a) server-rendered CSS **class names** (the additive `status-ok` / `status-warn` / `status-critical` severity system, plus the existing quantized bar/stagger classes), all defined in the same embedded `app.css` already served under `style-src 'self'`, and (b) server-rendered **text content**. It introduces no new script source, no new style source, no new connect target, no new image origin, no new font host. The severity colors are additive CSS on the existing single-origin stylesheet; the amber brand accent (`#f5b13d`) is untouched. Nothing Signal Lamp does touches any CSP directive. The frozen policy stays frozen.

---

## 4. Deployment-Topology Risks (carry-forward)

Two topology risks identified in Phase 0 are carried forward here with recommendations. Both sit at the deployment boundary, not in the frontend code, but this review must not drop them.

### 4.1 LAN-exposed port 3000 bypasses Cloudflare's edge

**Risk.** In addition to the Cloudflare-Tunnel-fronted path (`smellyfeet` + `cloudflared` sidecar), the service is also published LAN-only on port `3000` at `192.168.1.135`. That path **bypasses Cloudflare's edge entirely**: no edge rate limiting, no WAF, and any `CF-Connecting-IP` value appearing in logs from that path is **spoofable** (it is a plain header on a direct connection, not one the edge stamped). Full-URL edge caching (`Cache-Control` `s-maxage`/SWR per route — list 60/120s, article 300/3600s, stats 30/60s, about 3600/86400s, static immutable 1y, errors `no-store`) also does not apply to direct-to-origin traffic, so the LAN path can drive uncached load straight at Postgres via the backend.

**Recommendation (carry-forward, deployment owner):**
- Treat the port-3000 listener as **trusted-LAN-only** and ensure it is not reachable from any untrusted network segment (firewall / network segmentation, not application logic).
- **Never trust `CF-Connecting-IP` (or any `X-Forwarded-*`) in logs or logic on the direct path** — only values stamped by the Cloudflare edge are authentic. If per-client attribution is ever needed, derive it from the actual TCP peer on the LAN path, not from a spoofable header.
- If the LAN path is not actually required, the simplest hardening is to remove the direct publish and route even LAN clients through the tunnel hostname. Prefer deletion over a second ingress to secure.
- Severity: **MEDIUM** — the frontend is fully public and anonymous by design (no cookies, no sessions, no per-user state, no mutating endpoints), so the blast radius of an un-rate-limited direct path is *load*, not data exposure. It is a DoS/observability concern, not a confidentiality one.

### 4.2 Information-Broker outbound page-fetch — theoretical SSRF surface

**Risk.** Information-Broker fetches remote feed/article pages (goquery/gofeed) as part of ingest. Any service that fetches attacker-influenceable URLs has a theoretical **SSRF** surface (e.g., a feed pointing `link`/`enclosure` at `http://169.254.169.254/…` or an internal RFC-1918 address).

**Disposition.** This is a **backend concern, outside this frontend security review's direct scope** — the frontend never performs outbound fetches; it only reads already-ingested plain text from Postgres. **Flagged here for cross-reference into Information-Broker's own security posture**, where it should be reviewed: outbound fetches should enforce an egress allowlist / block link-local and private ranges, cap redirects, and time out. No action for the frontend; recorded so the item is not lost at the seam between the two services.

---

## 5. Backend API Extension Security Review (ADR-style)

Phase 1 (`REDESIGN_PLAN.md`, "Phase G") identified five backend API extensions the frontend redesign wants. Each is written up here ADR-style — **data source, meaning, type, null behavior, authorization needs, backward-compatibility plan** — from a security/privacy standpoint. All five are **read-only, GET, unauthenticated** additions to an already fully-public, anonymous, no-cookie surface; the shared authorization posture is: *no authorization is added or needed — the entire dataset is already public. The security questions for each are therefore about data-exposure shape and leakage, not access control.*

### B1 — Digest pagination / limit parameter  ⚠️ OPEN — needs human sign-off

- **Data source:** the `/articles/digest` endpoint, which today returns **every** article in the selected window with no cap.
- **Meaning:** a caller-supplied bound on how many digest items are returned (and/or an offset/cursor), so payload size, render time, and edge-cache-object size stop growing unbounded as ingest volume grows.
- **Type / null behavior (proposed, NOT decided):** an integer limit (e.g. `limit`), optionally with a cursor/offset. When omitted, the backend must apply a **safe default cap** rather than the current unbounded behavior. Out-of-range values must be clamped server-side (reject/clamp negatives and values above a hard maximum) — this is the one input-validation obligation of the whole set.
- **Authorization:** none (public).
- **Backward-compatibility plan:** additive query parameter; existing callers that omit it keep working *but should now receive the safe default cap instead of the full window* — this is a deliberate behavior change (unbounded → bounded) and must be called out in the changelog.
- **Security angle:** the security value is **availability** — capping response size defends the origin (and especially the un-rate-limited LAN path, §4.1) against an unbounded, uncached, expensive response as data grows. Clamping the parameter server-side prevents a caller from requesting a pathologically large page to force the same unbounded work back.
- **STATUS — FLAG FOR HUMAN SIGN-OFF:** this is explicitly still an **open, undecided item** (Phase 0 audit §9 item 7). Opus does **not** silently decide the exact parameter shape (`limit` vs `limit`+`offset` vs cursor; default value; hard max). **The exact param shape requires final human sign-off.** Recommendation to the decider: prefer a `limit` with a conservative default and a hard maximum; add a cursor only if deep pagination of the digest is actually needed. Whatever is chosen, server-side clamping is mandatory.

### B2 — Cluster / embedding-coverage signal

- **Data source:** the existing `story_cluster_id` and `summary_embedding` columns in Postgres (they already exist; nothing currently aggregates them).
- **Meaning:** lets the frontend distinguish *"clustering hasn't processed this window yet"* from *"no shared stories exist in this window."* Today it cannot tell these apart.
- **Type / null behavior:** an aggregate — e.g. a coverage ratio (`clustered / total` for the window) or a boolean/enum "clustering current." Null/absent should be rendered by the frontend as an explicit "coverage unknown / pending" state, never silently as "no clusters."
- **Authorization:** none (public).
- **Backward-compatibility plan:** new field or new endpoint; purely additive; no existing response shape changes.
- **Security angle:** **low.** It exposes an aggregate processing-coverage number, not content and not the raw embedding vectors. **Do not expose raw `summary_embedding` vectors to the frontend** — the frontend needs a coverage *signal*, not the vectors themselves; keep vectors backend-internal. With that constraint, no leakage concern.

### B3 — Per-feed error rates

- **Data source:** the existing `fetch_logs` rows, joined per feed (the rows exist; no per-feed join is exposed today).
- **Meaning:** completes the Signal Lamp health system by giving each feed a status dot (ok/warn/critical) driven by its recent fetch success/failure rate.
- **Type / null behavior:** per-feed numeric rate and/or an enumerated status. A feed with no recent logs should surface as "unknown," not "healthy."
- **Authorization:** none (public).
- **Backward-compatibility plan:** additive endpoint/fields; no change to existing shapes.
- **Security angle:** per-feed **counts and rates are low-sensitivity** (they reveal configured feeds — already public via the source filter — and rough health). **The one hard rule: do not include a raw free-form error string in this payload** for public display; expose the *rate* and an *enumerated status* only. If a diagnostic string is ever attached, it must be genericized per §1.10 before it reaches the template. This is the field most likely to tempt a raw-error leak, so the constraint is stated at the source.

### B4 — Day-by-day ingestion time-series

- **Data source:** ingestion timestamps aggregated by day (today `/stats` offers only point-in-time window aggregates — today/week/month — with no trend direction).
- **Meaning:** a per-day count series so `/stats` can show trend, not just a snapshot.
- **Type / null behavior:** an ordered array of `{day, count}`. Days with no ingest should appear as explicit zeros, not be omitted (so the frontend chart doesn't imply missing days were busy).
- **Authorization:** none (public).
- **Backward-compatibility plan:** additive field/endpoint; existing `/stats` aggregates unchanged.
- **Security angle:** **low.** Aggregate volume counts, already implied by the public article list. No content, no per-user data (there are no users). No concern.

### B5 — Articles-list total-count field  (conditional — do NOT add by default)

- **Data source:** a `COUNT(*)` over the current list query's result set.
- **Meaning:** would enable a "page X of Y" pagination affordance.
- **Type / null behavior:** an integer total for the active filter/query.
- **Authorization:** none (public).
- **Backward-compatibility plan:** additive; add **only if** pagination UX proves painful during Phase 3 implementation.
- **Security angle / decision:** **conditional — omit by default.** The current design **deliberately** omits "page X of Y" rather than fabricate a total, and adding a `COUNT(*)` per list request also adds query cost on the un-rate-limited LAN path (§4.1). **Recommendation: do not add B5 unless Phase 3 demonstrates concrete pagination pain.** If added later, prefer an approximate/`COUNT` with a cap over an exact unbounded count. No security *risk* either way — this is a YAGNI/performance call, recorded here so the "don't add by default" decision is explicit.

---

## 6. Signal Lamp — Net Security Delta

For the record, the complete security delta introduced by adopting Signal Lamp:

- **New rendering paths for untrusted content:** none.
- **New scripts / inline scripts / event handlers:** none.
- **New network origins (script/style/img/font/connect):** none.
- **New CSP requirements:** none — the frozen policy is unchanged (§3.2).
- **New publicly-displayed backend fields:** four (per-feed health, cluster coverage, day-by-day ingest, and — only if a string is attached — a summarization error). Three are low-sensitivity aggregates; the fourth (free-form error string) is the **only** new leakage risk and is closed by the genericization rule in §1.10.
- **New constraint on future work:** the severity-indicator components must use quantized classes / CSS custom properties, never inline `style=` (§2.9).

Signal Lamp is, from a security standpoint, **additive CSS and additive server-rendered text over an already-hardened, zero-JS, zero-dependency surface.** It does not weaken any existing control.

---

## 7. Verdict & Open Items

**Overall: Approved.** No CRITICAL or HIGH findings. The frontend's security posture is strong by construction: zero JavaScript, zero stored HTML, zero third-party requests, zero per-user state, and a frozen deny-by-default CSP. The redesign preserves all of it.

**Action items for Phase 3 (none blocking):**

| Item | Severity | Owner |
|---|---|---|
| B1 digest pagination — **exact param shape requires human sign-off** (Phase 0 §9.7 open item); do not silently decide; server-side clamping mandatory | needs sign-off | Human + backend |
| Genericize the summarization "most-recent-error" string before public display (§1.10) — prefer enumerated health state, never raw error | MEDIUM (closes what would be a HIGH-severity raw-error leak if implemented naively — see §1.10; tracked here as MEDIUM because the field hasn't shipped and the fix is a design constraint, not a live vulnerability) | Frontend + backend |
| Add `Permissions-Policy: …=()` deny-all header, with a `middleware_test.go` assertion (§2.11) | MEDIUM | Frontend |
| Add a defensive, documented `http/https` scheme allowlist at the article-URL data boundary (§2.3) — defense in depth over the implicit `urlFilter` | LOW | Frontend |
| Severity-indicator components must use quantized classes / CSS custom properties, never inline `style=` — Phase 3 review gate (§2.9) | LOW (gate) | Frontend reviewer |

**Carry-forward flags (deployment / other service):**

| Item | Severity | Owner |
|---|---|---|
| LAN port 3000 bypasses Cloudflare edge (no rate limit; spoofable `CF-Connecting-IP`) — segment it, don't trust the header, or remove it (§4.1) | MEDIUM | Deployment |
| Information-Broker outbound page-fetch SSRF surface — cross-reference into that service's posture (§4.2) | MEDIUM | Information-Broker |
| Information-Broker dependency audit (`govulncheck`, pinned `go.sum`) — out of scope for this frontend review, flagged (§2.12) | INFO | Information-Broker |

**MUST-PRESERVE invariants for Phase 3 (breaking any is a regression):**

1. No `template.HTML` / `template.URL` / `template.JS` / `template.CSS` anywhere.
2. No inline `style="…"` attribute or inline `<style>` — the enforcement behind `style-src 'self'` with no `unsafe-inline`.
3. Error/`{{ .Message }}` values remain fixed server-side constants — never `err.Error()`, never an upstream status code.
4. The frozen CSP string is unchanged.
5. No JavaScript. No third-party request. No cookie/session/per-user state.
