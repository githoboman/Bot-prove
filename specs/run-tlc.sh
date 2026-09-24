#!/usr/bin/env bash
# specs/run-tlc.sh — run every TLA+ spec in this directory under TLC.
#
# Usage:
#   bash specs/run-tlc.sh                # run everything
#   bash specs/run-tlc.sh QuorumSpec     # run just one spec
#
# Env:
#   TLA_TOOLS_JAR   path to tla2tools.jar (default /data/opt/tla2tools.jar,
#                   with a fallback download to /tmp/tla2tools.jar).
#   TLC_WORKERS     TLC -workers value (default: auto)
#   TLC_TIMEOUT     per-spec timeout in seconds (default: 900)
#
# Exit code: 0 if every model check completed with no error, non-zero
# otherwise. Suitable for CI.
set -euo pipefail

SPECS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SPECS_DIR}"

# --- Locate tla2tools.jar --------------------------------------------------
TLA_TOOLS_JAR="${TLA_TOOLS_JAR:-/data/opt/tla2tools.jar}"
if [[ ! -f "${TLA_TOOLS_JAR}" ]]; then
    TLA_TOOLS_JAR="/tmp/tla2tools.jar"
    if [[ ! -f "${TLA_TOOLS_JAR}" ]]; then
        echo ">>> Downloading tla2tools.jar to ${TLA_TOOLS_JAR}"
        curl -fsSL \
            https://github.com/tlaplus/tlaplus/releases/download/v1.8.0/tla2tools.jar \
            -o "${TLA_TOOLS_JAR}"
    fi
fi
echo ">>> Using TLA tools jar: ${TLA_TOOLS_JAR}"

# --- Java ------------------------------------------------------------------
if ! command -v java >/dev/null 2>&1; then
    if [[ -x /data/opt/jdk-21.0.4+7-jre/bin/java ]]; then
        export PATH="/data/opt/jdk-21.0.4+7-jre/bin:${PATH}"
    else
        echo "ERROR: java not found on PATH" >&2
        exit 2
    fi
fi

TLC_WORKERS="${TLC_WORKERS:-auto}"
TLC_TIMEOUT="${TLC_TIMEOUT:-900}"

# --- Select specs ----------------------------------------------------------
if (( $# > 0 )); then
    specs=("$@")
else
    mapfile -t specs < <(ls -1 *.tla | sed 's/\.tla$//')
fi

if (( ${#specs[@]} == 0 )); then
    echo "No .tla specs found in ${SPECS_DIR}" >&2
    exit 1
fi

# --- Run each spec ---------------------------------------------------------
overall=0
declare -a failed=()

for spec in "${specs[@]}"; do
    tla="${spec}.tla"
    cfg="${spec}.cfg"
    if [[ ! -f "${tla}" || ! -f "${cfg}" ]]; then
        echo ">>> SKIP ${spec} (missing ${tla} or ${cfg})"
        continue
    fi
    echo
    echo "========================================================"
    echo ">>> TLC ${spec}"
    echo "========================================================"
    if timeout "${TLC_TIMEOUT}" java -XX:+UseParallelGC \
        -cp "${TLA_TOOLS_JAR}" tlc2.TLC \
        -workers "${TLC_WORKERS}" \
        -config "${cfg}" -deadlock "${tla}"; then
        echo ">>> ${spec}: PASS"
    else
        rc=$?
        echo ">>> ${spec}: FAIL (exit ${rc})"
        overall=$((overall + 1))
        failed+=("${spec}")
    fi
done

# --- Clean up TLC state directories ---------------------------------------
find . -maxdepth 2 -type d -name 'states' -prune -exec rm -rf {} + 2>/dev/null || true
find . -maxdepth 1 -type f -name '*_TTrace_*.tla' -delete 2>/dev/null || true

echo
if (( overall == 0 )); then
    echo ">>> All ${#specs[@]} specs passed"
else
    echo ">>> ${#failed[@]} spec(s) FAILED: ${failed[*]}"
fi
exit "${overall}"
