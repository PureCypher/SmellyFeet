# DEPLOYMENT — session-2 repairs (pending approval)

Target: 192.168.1.135 (host), production at https://feed.purecypher.com/ via Cloudflare Tunnel.
Current deployed builds: backend Information-Broker `d3a1d7c` (container), frontend SmellyFeet `ac4df86` (container).

## What's queued for deployment

| Repo | Commits | Contents |
|---|---|---|
| Information-Broker | `545c199` | `/articles/latest` fix (500 → working), `summary_state`, `total`, rejected-CVE ingest skip, cvefeed description extraction, 24h future-date clamp, AlertManager mutex |
| SmellyFeet | `320cccb`, `e41c87a` | summary-state rendering, "Page N of M" pager, source-dropdown dedupe/wrapper labels, docs |

## Compatibility

Additive contract changes only (`summary_state`, `total` new fields; frontend falls back gracefully when they're absent). **Deploy order is safe either way.** Recommended order: backend first so the frontend immediately has real state data.

## Steps (after approval)

1. **Backup (backend changes touch ingest + API only; no schema changes):**
   `docker exec information-broker-postgres pg_dump -U postgres information_broker | gzip > ~/backup-$(date +%Y%m%d-%H%M).sql.gz`
2. Backend:
   `cd /home/pure/Documents/github/Information-Broker && git pull && docker compose up -d --build rss-monitor`
   Watch: `docker compose logs -f rss-monitor` for ~2 min — confirm "Successfully summarized" lines resume and no `pq:` errors appear.
3. Frontend:
   `cd /home/pure/Documents/github/SmellyFeet && git pull && docker compose --project-directory deploy up -d --build`
4. Smoke tests via the real domain:
   - `curl -s https://feed.purecypher.com/ | grep -c "summary_state"` — page HTML should show new states (grep for "Summary queued").
   - `curl -s "https://feed.purecypher.com/?page=2" | grep -oE "Page 2 of [0-9]+"` — pager with total.
   - `curl -s http://localhost:8080/articles/latest?limit=1` on the host — 200 with `summary_state` (was 500).
   - Check `/stats` renders; check browser console clean on `/` and `/article/<id>`.

## Rollback

Both stacks rebuild from git: `git checkout ac4df86` (SmellyFeet) / `git checkout d3a1d7c` (Information-Broker) then the same `docker compose up -d --build`. No schema rollback needed (no migrations shipped). Note: ingest filters mean articles ingested post-deploy differ; DB backup above covers data-level rollback of articles/summaries if ever needed.
