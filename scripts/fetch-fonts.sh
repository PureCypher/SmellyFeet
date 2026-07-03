#!/usr/bin/env bash
# One-time: download latin-subset IBM Plex woff2 files (committed to the repo).
set -euo pipefail
cd "$(dirname "$0")/.."
dir=internal/server/static/fonts
mkdir -p "$dir"
base=https://cdn.jsdelivr.net/fontsource/fonts
for w in 400 500 600 700; do
  curl -fsSL -o "$dir/ibm-plex-sans-$w.woff2" "$base/ibm-plex-sans@latest/latin-$w-normal.woff2"
done
for w in 400 500 600; do
  curl -fsSL -o "$dir/ibm-plex-mono-$w.woff2" "$base/ibm-plex-mono@latest/latin-$w-normal.woff2"
done
ls -l "$dir"
