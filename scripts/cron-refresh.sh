#!/usr/bin/env bash
# Weekly, fully-autonomous refresh of the tracked security events.
# Installed as a host cron (see README "Autonomous refresh"). No human in the loop.
#
# Each run: reset to origin/main (the committed baseline), re-scrape every source via
# scripts/import-events.py --apply, and rebuild the container so the fresh events go live.
# It does NOT push (the host has no git credentials) — the built container carries the
# fresh events; origin/main stays the committed baseline. A safety floor aborts the
# deploy if the scrape collapses (< MIN events), leaving the current site untouched.
set -euo pipefail

REPO=/home/pure/smellyfeet-frontend
LOG=/home/pure/smellyfeet-events-refresh.log
MIN=30

# single-instance lock so a slow run never overlaps the next week's
exec 9>/tmp/smellyfeet-refresh.lock
flock -n 9 || { echo "$(date -Is) already running, skipping" >>"$LOG"; exit 0; }
exec >>"$LOG" 2>&1

echo "===== $(date -Is) refresh start ====="
cd "$REPO"
git fetch --quiet origin
git reset --hard --quiet origin/main

python3 scripts/import-events.py --apply
n=$(python3 -c 'import json;print(len(json.load(open("internal/server/meetups_seed.json"))["meetups"]))')
echo "scraped $n events"

if [ "$n" -lt "$MIN" ]; then
  echo "ABORT: only $n events (< $MIN) — reverting, not deploying"
  git checkout -- internal/server/meetups_seed.json
  exit 1
fi

docker compose --project-directory deploy up -d --build
git checkout -- internal/server/meetups_seed.json   # keep the working tree clean for manual deploys
echo "===== $(date -Is) done: $n events live ====="
