# DEPLOYMENT — production runbook

Target: `llm-server` (192.168.1.135), production at https://feed.purecypher.com/ via Cloudflare Tunnel.

## Topology (verified 2026-10-27)

| Component | Where | How deployed |
|---|---|---|
| Frontend (SmellyFeet repo, brand "Information Broker") | `~/smellyfeet-frontend` on host, git clone of GitHub main | `deploy/docker-compose.yml` → containers `deploy-smellyfeet-1` + `deploy-cloudflared-1` |
| Backend (Information-Broker repo) | `~/Information-Broker` on host, git clone | `docker-compose.yml` → `information-broker-app` (rss-monitor), postgres, flaresolverr, prometheus, grafana, alertmanager |
| Public ingress | Cloudflare Tunnel (`deploy-cloudflared-1`) → frontend `:3000` | Tunnel token from `deploy/.env` |

Note: the host clone of the backend may sit on local feature branches (e.g.
`feat/requeue-catchup`). Before deploying, reconcile those into GitHub main
(fetch → merge → test → push) so the host can `git pull --ff-only`.

## Deploy order

Backend first (frontend tolerates missing new API fields), then frontend.

1. **Backup** (before any backend deploy; ~400 MB):
   `ssh 192.168.1.135 'docker exec information-broker-postgres pg_dump -U postgres information_broker | gzip > ~/backup-$(date +%Y%m%d-%H%M).sql.gz'`
2. **Reconcile + push** both repos from the dev box; host pulls from GitHub.
3. **Backend**: `ssh … 'cd ~/Information-Broker && git checkout main && git pull --ff-only && docker compose up -d --build rss-monitor'`
   (Rebuild `alertmanager` too only if its code changed.)
   Watch: `docker logs information-broker-app --since 2m` — summarization scheduler lines resume, no `pq:` errors. Per-feed `Status: error` lines are routine upstream failures, not regressions.
4. **Frontend**: `ssh … 'cd ~/smellyfeet-frontend && git pull --ff-only && docker compose --project-directory deploy up -d --build'`

## Smoke tests (via the real domain)

- `curl -s https://feed.purecypher.com/ | grep -c 'Information Broker'` — new brand present.
- `curl -s https://feed.purecypher.com/feed.xml | head -3` — Atom title matches brand.
- `curl -s "https://feed.purecypher.com/?page=2" | grep -oE 'Page 2 of [0-9]+'`
- Article page: `curl -s "https://feed.purecypher.com/article/<id>?cb=x"` — title, summary, content states.
- Edge cache caveat: `cache-control` includes `stale-while-revalidate=300`, so
  changed pages can serve stale copies for up to ~420 s post-deploy. Verify
  changed routes with a query string (`?cb=$RANDOM`) immediately, plain URL
  after the SWR window.

## Rollback

Both stacks rebuild from git: `git checkout <previous-commit>` in the host
clone, then the same `docker compose up -d --build`. No DB migrations ship
(schema changes are additive `ALTER TABLE ... IF NOT EXISTS` at startup); the
pg_dump backup covers data-level rollback.

## Current state (2026-10-27)

- Backend: GitHub main `15c37f5` (merge of host-only `feat/requeue-catchup` 7d81845 + dev content_html/summary_state work); deployed.
- Frontend: GitHub main `e1ecd03` (Information Broker rebrand); deployed.
- Backup taken pre-deploy: `~/backup-20261027-0919.sql.gz`.
