#!/usr/bin/env bash
# Copy the canonical on-chain manifest into the frontend static asset
# tree so `vite build` picks it up. Run automatically by `npm run build`
# via the "prebuild" hook (see frontend/package.json).
#
# Canonical location:  deploy-out/onchain.json  (single source of truth)
# Frontend consumer:   frontend/public/onchain.json  (served at /onchain.json)
#
# Also validates the JSON so a corrupt manifest fails the build early
# instead of silently shipping.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
src="$repo_root/deploy-out/onchain.json"
dst="$repo_root/frontend/public/onchain.json"

if [[ ! -f "$src" ]]; then
  echo "sync-onchain: canonical manifest missing at $src" >&2
  exit 1
fi

if ! python3 -c 'import json,sys; json.load(open(sys.argv[1]))' "$src" >/dev/null 2>&1; then
  echo "sync-onchain: $src is not valid JSON" >&2
  exit 1
fi

mkdir -p "$(dirname "$dst")"
cp "$src" "$dst"
echo "sync-onchain: $src -> $dst"
