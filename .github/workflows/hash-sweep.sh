#!/usr/bin/env bash
#
# hash-sweep.sh — CI-friendly scan for superseded Casper contract hashes.
#
# Runnable both in GitHub Actions and locally:
#   bash .github/workflows/hash-sweep.sh
#
# Exit 0 = PASS (no active references to superseded hashes).
# Exit 1 = FAIL (a stale hash appears outside a superseded / historical context).

set -euo pipefail

# 8-char prefixes of hashes that are NO LONGER active on testnet.
# Append newly-superseded prefixes after each redeploy.
STALE_HASHES=(
  "cf70e1fe"   # old stake-slashing (superseded 2026-07-18 by 1ad1b3d9)
)

# Files where a mention of a stale hash is always OK (change log / lessons doc).
ALLOWED_HISTORICAL_PATHS=(
  "CHANGELOG.md"
  "docs/DEPLOYMENT_LESSONS.md"
  ".github/workflows/hash-sweep.sh"
  ".github/workflows/hash-sweep.yml"
)

# Marker words that make an ACTIVE mention safe (line is describing what the
# hash USED to be, not what it currently is).
CONTEXT_MARKERS='old|previous|superseded|superseding|replaced|replacing|hardened|deprecated|before the redeploy|no longer|used to be|earlier'

FAIL=0

for hash in "${STALE_HASHES[@]}"; do
  # Collect files that contain the hash, excluding vendored / built dirs.
  mapfile -t hits < <(
    grep -rln -I "$hash" \
      --exclude-dir=node_modules \
      --exclude-dir=target \
      --exclude-dir=dist \
      --exclude-dir=build \
      --exclude-dir=.git \
      --exclude-dir=__pycache__ \
      --exclude="*.pyc" \
      --exclude="*.wasm" \
      --exclude="*.tmp" \
      . 2>/dev/null || true
  )

  if [ "${#hits[@]}" -eq 0 ]; then
    echo "hash $hash: clean"
    continue
  fi

  for file in "${hits[@]}"; do
    rel="${file#./}"

    allowed_path=0
    for allow in "${ALLOWED_HISTORICAL_PATHS[@]}"; do
      if [ "$rel" = "$allow" ]; then
        allowed_path=1
        break
      fi
    done
    if [ "$allowed_path" -eq 1 ]; then
      echo "hash $hash: allow-listed in $rel (OK)"
      continue
    fi

    bad_lines=$(grep -nE "$hash" "$rel" | grep -Ev "$CONTEXT_MARKERS" || true)

    if [ -z "$bad_lines" ]; then
      echo "hash $hash: found in $rel but every line has a historical marker (OK)"
    else
      echo "::error file=$rel::superseded hash $hash used as an ACTIVE reference:"
      while IFS= read -r line; do
        echo "::error file=$rel::  $line"
      done <<< "$bad_lines"
      FAIL=1
    fi
  done
done

if [ "$FAIL" -eq 1 ]; then
  echo ""
  echo "hash-sweep FAILED: superseded contract hash(es) still referenced as active."
  echo "Fix each error above by either:"
  echo "  * Replacing the hash with the current one from the onchain.json manifest, OR"
  echo "  * Rewriting the line to explicitly describe it as old/superseded/replaced."
  exit 1
fi

echo ""
echo "hash-sweep PASSED: no active references to superseded contract hashes."
