# Product

<!-- impeccable:product-schema 1 -->
<!-- Written from the governing brief and repository evidence while the owner
     was away; every inferred fact below is labeled. Re-confirm with the
     owner when they are available. -->

## Platform

web

## Stack

- **Frontend (this repo, SmellyFeet):** Go 1.22, zero-dependency (stdlib + golang.org/x/net for HTML sanitization only). Server-rendered `html/template` with Tailwind 3.4 CSS built from `assets/tailwind.input.css` into `internal/server/static/app.css` via a cached standalone Tailwind binary. **Zero client-side JavaScript; frozen CSP** (`default-src 'none'; style-src 'self'; img-src 'self'; font-src 'self'`). Fonts self-hosted (IBM Plex Sans/Mono). Data fetched from the backend API at request time; no caching layer in the frontend.
- **Backend (Information-Broker, sibling repo):** Go + PostgreSQL. RSS/monitor ingestion with full-content extraction, Ollama summarization, story clustering, maintenance `backfill` command.
- Inferred from repo structure and docs/frontend/ARCHITECTURE.md. Deployment target is a self-hosted machine (192.168.1.135), public URL https://feed.purecypher.com.

## Users

**Primary user [INFERRED]:** a technically fluent reader (security analyst, researcher, curious engineer) checking what's new in cybersecurity, wanting a fast scan of fresh articles plus the ability to read an article properly on the platform itself. Public, anonymous access — no accounts, no cookies, no JS.

**Job:** scan fresh security news → judge relevance quickly → read the AI summary and/or the stored article text → open the original source when needed → return to scanning without losing context.

## What it does

Ingests cybersecurity RSS feeds, extracts article content (plain text + structured HTML with images), generates short AI summaries with a local LLM, and serves a dark, restrained "security intelligence newsroom" reading experience. The distinctive mechanism, verifiable in the code: **structured article capture (`content_html`) rendered through a strict whitelist sanitizer with a same-origin image proxy**, so long-form reading works on-platform without weakening CSP or trusting publisher markup.

## Constraints that must hold

- Zero client-side JavaScript; CSP frozen; no inline styles (data-driven values use quantized classes).
- No new third-party fonts, CDNs, analytics, or trackers.
- Honest states: summary pending/failed/unavailable and missing full text are labeled as such; never fabricate content, model names, or severity scores.
- Content is hostile input: every HTML path goes through the sanitizer; no raw-HTML bypasses.
- Publisher images load only through the SSRF-guarded `/img` proxy; publishers that block the proxy degrade to a framed slot with alt text.
- The amber "Signal Lamp" visual identity (user-chosen from live comparison) is the committed world; severity/status color is rationed to audited sites.

## Terminology

- **Feed / source:** an RSS source; "N sources" on a digest story = cross-feed corroboration.
- **Summary state:** `available` / `pending` / `failed` (backend-computed; older articles have no state and render their raw summary or "No summary available").
- **Full text:** extracted article body — structured (`content_html`, preferred) or plain (`full_content` fallback).
