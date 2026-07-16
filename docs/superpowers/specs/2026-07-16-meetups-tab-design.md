# Meetups Tab — Design Spec

**Date:** 2026-07-16
**Status:** Approved (brainstorming) — pending implementation plan
**Feature:** A first-class "Meetups" navigation tab for SmellyFeet listing BSides-spirit
community infosec meetups, with detail, propose, chapters, ICS export, and a read-only JSON API.

---

## 1. Context & constraints

SmellyFeet is a **deliberately zero-dependency, stateless, GET-only** Go frontend that proxies
the Information-Broker API and renders `html/template` pages. There is **no database**, no auth,
no sessions, no CSRF, no external Go deps (even `commas` is hand-rolled to avoid `golang.org/x/text`).
It deploys as a container behind a Cloudflare tunnel; all content ships by embedding + redeploy.

The original master prompt assumed a DB + admin auth + a propose→pending→publish moderation
queue. That is an architectural departure from a stateless proxy. **Decision (user-approved):**
implement the feature in the grain of the existing app — static embedded seed data + a
webhook-relay propose form — adding **zero new dependencies**.

## 2. Goals

- New primary nav tab **"Meetups"**, framed as a community meetup tracker "in the BSides spirit"
  (no implied official BSides affiliation; attribution + link to `bsides.org/chapters/`).
- Discover meetups (list), view details, propose a meetup, browse a chapter directory.
- Add-to-calendar (ICS), external RSVP CTA, OSM map link, shareable URLs.
- Read-only `GET /api/meetups` JSON for future Discord/Matrix bots.
- Visually and architecturally consistent with the existing site shell.

## 3. Non-goals

- No SQLite/DB, no migrations, no admin CMS, no in-app moderation queue.
- No ticketing/payments, no attendee PII storage (RSVP is always an external link).
- No markdown rendering / sanitizer library (see §7).
- Not an official mirror of bsides.org; never asserts "approved chapter" status.

## 4. Architecture

New files in the existing `package server` (flat, matching `feed.go`/`handlers.go`):

- `meetups.go` — `Meetup`/`Chapter` types, seed load (`//go:embed meetups_seed.json`),
  filtering, URL-scheme validation, timezone display, ICS generation.
- `meetups_handlers.go` — HTTP handlers (list, detail, chapters, propose GET/POST, ICS, API).
- `meetups_seed.json` — embedded seed: `{ "meetups": [...], "chapters": [...] }`.
- Templates: `meetups_list.html`, `meetup_detail.html`, `meetups_propose.html`, `chapters.html`.

Publishing a meetup = edit `meetups_seed.json` + redeploy (identical to how all site content
already ships). Git history is the audit log.

## 5. Routes

Stdlib `net/http.ServeMux`, Go 1.22 method+pattern routes. Literal paths win over `{slug}` by
ServeMux specificity, so `/meetups/propose` and `/meetups/chapters` resolve correctly.

| Method + pattern | Purpose |
|---|---|
| `GET /meetups` | List. Upcoming first; past events in a separate collapsed section. Filters `?city=`, `?online=1`, `?tag=`, `?chapter=` (server-side, whitelisted). Empty state → "propose one" + official-chapters link. |
| `GET /meetups/chapters` | Chapter directory from seed; each row links to `?chapter=` filter. |
| `GET /meetups/propose` | Render propose form. |
| `POST /meetups/propose` | Validate → honeypot → rate-limit → relay to webhook → re-render with flash. |
| `GET /meetups/{slug}` | Detail. Unknown slug → existing `notfound` template (404). |
| `GET /meetups/{slug}/ics` | `text/calendar` VEVENT download (hand-rolled, zero-dep). |
| `GET /api/meetups` | Read-only JSON. Published-only (all seed). Optional `?from=`/`?to=` (RFC3339) + `?city=`/`?tag=` filters. No organizer PII (none stored). |

All new routes/nav are gated by `MEETUPS_ENABLED` (default true). When disabled: tab hidden,
routes return 404.

## 6. Data model

`meetups_seed.json` → `[]Meetup`. Trimmed from the master-prompt model because there is no DB
(everything in the file *is* published; git history is the audit trail):

**Meetup:** `Slug` (unique key), `Title`, `Summary`, `Description` (plain multi-line text, **not
markdown**), `StartsAt`/`EndsAt` (RFC3339 with offset), `LocationType` (`in_person|online|hybrid`),
`VenueName`, `VenueAddress`, `City`, `Region`, `Country`, `OnlineURL`, `RSVPURL`, `ChapterName`,
`ChapterURL`, `Tags[]`.

**Chapter:** `Name`, `City`, `Country`, `Website`, `Email` (optional).

**Dropped (YAGNI without a DB):** `id` (slug is the key), separate `timezone` (offset baked into
`StartsAt`), `geo_lat/lon`, organizer PII, `status`/`draft`/`pending`, `created/updated/published_at`,
`source`.

Derived at request time: `IsPast` (`EndsAt`—or `StartsAt` if no end—before now).

## 7. Security

Adapted to a stateless, session-less app (GET + one relay-only POST):

- **URL-scheme allowlist (the one real control):** every rendered `href`
  (`OnlineURL`/`RSVPURL`/`ChapterURL`) must be `http`/`https`. Enforced at **seed-load** (a bad
  scheme fails the load, caught by a test) **and** at render. Blocks `javascript:`/`data:` XSS and
  open redirects. (`html/template` also neutralizes bad `href` schemes; we validate explicitly too.)
- **No CSRF token — justified deviation:** there is no session or cookie to ride, and the POST
  performs no state change on our side (it relays to a webhook). CSRF's threat model does not apply.
- **Anti-spam on propose:** hidden **honeypot** field (filled → silent 200, no webhook) + an
  **in-memory per-IP fixed-window rate limit** (mutex-guarded map). `// ponytail:` ceiling noted —
  upgrade to a shared store only if multi-instance.
- **Proposed content is never rendered on-site** → no markdown lib, no bluemonday. User input is only
  escaped into the webhook JSON body.
- **Server-side validation** of the propose form: required fields, field length caps, URL schemes;
  invalid → re-render form with a field-level error (mirrors the `/digest` inline-error pattern).
- **No secret leakage:** `MEETUPS_NOTIFY_WEBHOOK` is env-only, never in a template. If empty (local
  dev), the form still validates + thanks the user and logs the proposal server-side (log a redacted
  summary, not the raw contact).
- Trusted seed content is auto-escaped by `html/template`; multi-line `Description` uses CSS
  `white-space: pre-line` (same approach as `cleanContent` for articles).

## 8. Timezone

Display in `MEETUPS_DEFAULT_TZ` (default `Europe/London`), `time.LoadLocation` once at startup.
`import _ "time/tzdata"` (stdlib) embeds the tz database so `Europe/London` resolves in a scratch
container — zero external dep. Each meetup's `StartsAt`/`EndsAt` is an unambiguous instant
(RFC3339 + offset); rendered in the display TZ with the zone label. Past/upcoming split uses the
same instants vs `time.Now()`.

## 9. Config additions (`internal/config`)

| Env var | Default | Meaning |
|---|---|---|
| `MEETUPS_ENABLED` | `true` | Gate nav tab + all `/meetups*` and `/api/meetups` routes. |
| `MEETUPS_DEFAULT_TZ` | `Europe/London` | Display timezone. |
| `MEETUPS_NOTIFY_WEBHOOK` | `` (empty) | Propose-form relay target (e.g. Discord webhook). Empty → log-only. |

Nav visibility uses a `meetupsEnabled` `funcMap` entry (like `assetHash`) so the shared header
partial gates the tab with `{{ if meetupsEnabled }}` — no per-handler data changes.

## 10. Seed honesty (canonical-reference constraint)

- Sample **meetups** are clearly-marked **examples** — no fabricated real events/dates.
- **Chapters** verified against `bsides.org/chapters/` (WebFetch) at implementation time; listed as
  "discovery only — not an official mirror," with **no invented "approved" status**.
- Every list/detail/chapters view carries the attribution line + link to `bsides.org/chapters/`
  and a short inclusive code-of-conduct blurb.

## 11. Testing (table-driven `go test`, `-race`)

- Seed: loads, slugs unique, all URLs `http`/`https`, dates parse.
- Timezone display formatting (`Europe/London`).
- Filters: city / online / tag / chapter.
- ICS: valid `VEVENT`, correct escaping, `Content-Type`/`Content-Disposition`.
- Handlers: list renders; detail 200 + 404 for unknown slug; propose GET renders; propose POST
  valid → webhook fired (injected sink), invalid → re-render with error, honeypot → silent success
  (no webhook), rate-limit trips after N; bad URL scheme rejected at load.
- API: `/api/meetups` returns published JSON, `from`/`to` filter works, no PII fields present.
- Nav: tab present when enabled, absent + routes 404 when `MEETUPS_ENABLED=false`.

## 12. Docs

README: routes, env vars, how to add a meetup (edit seed + redeploy), moderation flow
(webhook → curate → commit → redeploy), attribution note. Include 2–3 example meetups + a few
chapter seeds.

## 13. Deliverables

Code (types/seed/handlers/templates/config), embedded seed JSON, tests, README update, PR summary
with routes + env + screenshots (list / detail / nav).

## 14. Follow-ups (out of scope)

- In-app moderation / SQLite — add when edit-JSON-and-redeploy is outgrown.
- Markdown descriptions — add when non-authors author descriptions (then add a sanitizer).
- Discord bot / Matrix room consuming `/api/meetups`.
- Official BSides chapter approval pitch (email `jack@securitybsides.org`).
