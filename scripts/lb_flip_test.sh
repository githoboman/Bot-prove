#!/usr/bin/env bash
# lb_flip_test.sh — offline tests for lb_flip.sh.
#
# These tests do not require a running engine — they stub curl to always
# succeed and drive the sed rewrite logic on a scratch copy of
# prometheus.yml. Guards against the file-mutation regression classes.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LB_FLIP="$SCRIPT_DIR/lb_flip.sh"

# --- Test fixtures ---------------------------------------------------------
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

STUB_BIN="$TMPDIR/bin"
mkdir -p "$STUB_BIN"
cat > "$STUB_BIN/curl" <<'STUB'
#!/usr/bin/env bash
# stub: succeed on every probe, ignore POST reload.
exit 0
STUB
chmod +x "$STUB_BIN/curl"

# Copy of prometheus.yml with engine-blue as the current scrape target.
mkdir -p "$TMPDIR/deploy/observability"
cat > "$TMPDIR/deploy/observability/prometheus.yml" <<'YAML'
global:
  scrape_interval: 15s
scrape_configs:
  - job_name: cp-engine
    static_configs:
      - targets: ["engine-blue:8080"]
YAML

# Overlay the scratch repo root by symlinking scripts + deploy trees.
mkdir -p "$TMPDIR/scripts"
cp "$LB_FLIP" "$TMPDIR/scripts/lb_flip.sh"

PATH="$STUB_BIN:$PATH"
export PATH

pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1" >&2; exit 1; }

# --- Test 1: reject invalid args -------------------------------------------
if bash "$TMPDIR/scripts/lb_flip.sh" 2>/dev/null; then
  fail "no args should reject"
fi
if bash "$TMPDIR/scripts/lb_flip.sh" blue blue 2>/dev/null; then
  fail "same slots should reject"
fi
if bash "$TMPDIR/scripts/lb_flip.sh" red green 2>/dev/null; then
  fail "unknown slot should reject"
fi
pass "arg validation"

# --- Test 2: successful cutover blue -> green ------------------------------
( cd "$TMPDIR" && bash "$TMPDIR/scripts/lb_flip.sh" blue green >/dev/null )
if ! grep -q "engine-green:8080" "$TMPDIR/deploy/observability/prometheus.yml"; then
  fail "green not present after cutover"
fi
if grep -q "engine-blue:8080" "$TMPDIR/deploy/observability/prometheus.yml"; then
  fail "blue still present after cutover"
fi
pass "cutover blue -> green"

# --- Test 3: idempotency — running the same cutover twice is a no-op -------
before_hash="$(sha256sum "$TMPDIR/deploy/observability/prometheus.yml")"
( cd "$TMPDIR" && bash "$TMPDIR/scripts/lb_flip.sh" blue green >/dev/null )
after_hash="$(sha256sum "$TMPDIR/deploy/observability/prometheus.yml")"
[ "$before_hash" = "$after_hash" ] || fail "second run should be a no-op"
pass "idempotency"

# --- Test 4: cutover back green -> blue ------------------------------------
( cd "$TMPDIR" && bash "$TMPDIR/scripts/lb_flip.sh" green blue >/dev/null )
grep -q "engine-blue:8080" "$TMPDIR/deploy/observability/prometheus.yml" \
  || fail "blue not restored"
grep -q "engine-green:8080" "$TMPDIR/deploy/observability/prometheus.yml" \
  && fail "green still present after rollback"
pass "rollback green -> blue"

# --- Test 5: unhealthy target aborts (curl stub returns 1) -----------------
cat > "$STUB_BIN/curl" <<'STUB'
#!/usr/bin/env bash
# stub: always fail probes.
exit 1
STUB
chmod +x "$STUB_BIN/curl"
if ( cd "$TMPDIR" && bash "$TMPDIR/scripts/lb_flip.sh" blue green 2>/dev/null ); then
  fail "unhealthy target should abort"
fi
pass "unhealthy target aborts"

echo "All lb_flip.sh offline tests passed."
