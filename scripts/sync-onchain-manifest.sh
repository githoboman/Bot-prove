#!/usr/bin/env bash
# Sync the canonical on-chain contract manifest into consumer surfaces.
#
# Canonical source:  ./deploy-out/onchain.json  (root, git-tracked)
# Consumers:
#   - ./frontend/public/onchain.json  (bundled into the SPA at /onchain.json)
#
# Run this whenever contracts are (re)deployed and deploy-out/onchain.json is
# updated. `make sync-onchain` and the frontend `npm run prebuild` script both
# call this. Idempotent — safe to run repeatedly.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/deploy-out/onchain.json"
DST_FRONTEND="$ROOT/frontend/public/onchain.json"

if [ ! -f "$SRC" ]; then
  echo "[sync-onchain] ERROR: canonical manifest not found: $SRC" >&2
  echo "[sync-onchain] Regenerate it after a deploy (see docs/deploy.md) before syncing." >&2
  exit 1
fi

# Optional strict validation against the schema if it's present + ajv/jsonschema
# is available. We don't hard-require it here — the schema check is a CI job.
if command -v jq >/dev/null 2>&1; then
  if ! jq -e '.contracts | length > 0' "$SRC" >/dev/null; then
    echo "[sync-onchain] ERROR: canonical manifest has no contracts" >&2
    exit 1
  fi
fi

mkdir -p "$(dirname "$DST_FRONTEND")"
cp "$SRC" "$DST_FRONTEND"
echo "[sync-onchain] OK: $SRC -> $DST_FRONTEND"
