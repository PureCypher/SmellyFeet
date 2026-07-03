#!/usr/bin/env bash
# Rebuild internal/server/static/app.css from templates + assets/tailwind.input.css.
# The output is committed; re-run only when templates or the theme change.
set -euo pipefail
cd "$(dirname "$0")/.."
TW_VERSION=v3.4.17
TW_BIN=".cache/tailwindcss-$TW_VERSION"
if [ ! -x "$TW_BIN" ]; then
  mkdir -p .cache
  curl -fsSL -o "$TW_BIN" "https://github.com/tailwindlabs/tailwindcss/releases/download/$TW_VERSION/tailwindcss-linux-x64"
  chmod +x "$TW_BIN"
fi
"$TW_BIN" -c tailwind.config.js -i assets/tailwind.input.css -o internal/server/static/app.css --minify
echo "wrote internal/server/static/app.css ($(wc -c < internal/server/static/app.css) bytes)"
