#!/usr/bin/env python3
"""
generate_manifest.py — regenerate deploy-out/onchain.json (root canonical)

Reads contract hashes from the current root manifest and optionally validates
them against Casper testnet RPC. Writes deploy-out/onchain.json with fresh
generation metadata, then mirrors the contract data (without metadata) to
frontend/public/onchain.json so the frontend stays in sync.

Usage:
  python scripts/generate_manifest.py              # validate + write
  python scripts/generate_manifest.py --no-rpc     # skip live RPC check
  python scripts/generate_manifest.py --dry-run    # show diff, do not write
  python scripts/generate_manifest.py --check      # verify frontend matches root; exit 1 on drift

Root canonical:      deploy-out/onchain.json
Frontend generated:  frontend/public/onchain.json
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parent.parent
ROOT_MANIFEST = REPO_ROOT / "deploy-out" / "onchain.json"
FRONTEND_MANIFEST = REPO_ROOT / "frontend" / "public" / "onchain.json"
RPC_ENDPOINT = "https://node.testnet.casper.network/rpc"

# Fields that live on the root manifest but SHOULD NOT propagate to the
# frontend copy (they are meta about how the manifest was produced, not
# on-chain data the UI needs).
ROOT_ONLY_KEYS = {
    "$schema",
    "manifest_version",
    "canonical_source",
    "regenerate_command",
    "notes",
    "generated_at",
    "generator_version",
}

GENERATOR_VERSION = "1.0.0"


def load_json(path: Path) -> dict[str, Any]:
    with path.open() as f:
        return json.load(f)


def dump_json(path: Path, data: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w") as f:
        json.dump(data, f, indent=2, sort_keys=False)
        f.write("\n")


def validate_hash_on_chain(contract_hash: str) -> tuple[bool, str]:
    """Query Casper testnet RPC for contract hash. Return (ok, detail)."""
    payload = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "query_global_state",
        "params": {
            "state_identifier": None,
            "key": f"hash-{contract_hash}",
            "path": [],
        },
    }
    try:
        result = subprocess.run(
            [
                "curl", "-sf", "--max-time", "10",
                RPC_ENDPOINT,
                "-H", "Content-Type: application/json",
                "-d", json.dumps(payload),
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        if result.returncode != 0:
            return False, f"curl failed: rc={result.returncode}"
        resp = json.loads(result.stdout)
        if "result" in resp:
            return True, "on-chain"
        if "error" in resp:
            return False, f"rpc error: {resp['error'].get('message', 'unknown')}"
        return False, "unexpected response shape"
    except (json.JSONDecodeError, subprocess.SubprocessError) as e:
        return False, f"exception: {e}"


def build_root_manifest(existing: dict[str, Any], validate_rpc: bool) -> dict[str, Any]:
    """Build a fresh root manifest from the current one, preserving all data."""
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    root = {
        "$schema": "./onchain.schema.json",
        "network": existing.get("network", "casper-test"),
        "chain_name": existing.get("chain_name", "casper-test"),
        "project": existing.get("project", "CasperProver"),
        "deployer": existing["deployer"],
        "explorer": existing.get("explorer", "https://testnet.cspr.live"),
        "cspr_cloud": existing.get("cspr_cloud", "https://testnet.cspr.cloud"),
        "manifest_version": "1.0",
        "canonical_source": "deploy-out/onchain.json",
        "regenerate_command": "python scripts/generate_manifest.py",
        "notes": (
            "This is the SINGLE SOURCE OF TRUTH for contract deployments. "
            "Do not hand-edit hashes anywhere else in the repo. verify.sh, "
            "frontend/public/onchain.json, README explorer links, and API "
            "responses must be derived from this file. See "
            "scripts/generate_manifest.py for the generator and "
            "docs/MANIFEST.md for the schema."
        ),
        "generated_at": now,
        "generator_version": GENERATOR_VERSION,
        "contracts": {},
    }

    # Copy contracts, optionally validating each hash against live RPC.
    for name, meta in existing.get("contracts", {}).items():
        entry = dict(meta)
        if validate_rpc:
            ok, detail = validate_hash_on_chain(entry["contract_hash"])
            entry["_validated"] = {"ok": ok, "detail": detail, "at": now}
            status = "✅" if ok else "❌"
            print(f"  {status} {name}: {entry['contract_hash'][:16]}... — {detail}")
            if not ok:
                print(f"    WARNING: {name} did not validate on-chain", file=sys.stderr)
        root["contracts"][name] = entry

    # Preserve any extra sections (undeployed_contracts, verification, etc.)
    for key, value in existing.items():
        if key in {"contracts"} or key in ROOT_ONLY_KEYS or key in root:
            continue
        root[key] = value

    return root


def derive_frontend_manifest(root: dict[str, Any]) -> dict[str, Any]:
    """Strip generator metadata from root to produce frontend-facing copy."""
    fe: dict[str, Any] = {}
    for key, value in root.items():
        if key in ROOT_ONLY_KEYS:
            continue
        if key == "contracts":
            # Strip _validated from each contract entry for frontend
            fe["contracts"] = {
                name: {k: v for k, v in meta.items() if k != "_validated"}
                for name, meta in value.items()
            }
        else:
            fe[key] = value
    return fe


def check_drift(root: dict[str, Any], frontend: dict[str, Any]) -> list[str]:
    """Return list of drift messages. Empty list = in sync."""
    drift: list[str] = []
    root_derived = derive_frontend_manifest(root)
    root_contracts = root_derived.get("contracts", {})
    fe_contracts = frontend.get("contracts", {})
    if set(root_contracts.keys()) != set(fe_contracts.keys()):
        drift.append(
            f"contract set differs: root={sorted(root_contracts)} "
            f"frontend={sorted(fe_contracts)}"
        )
    for name, root_entry in root_contracts.items():
        fe_entry = fe_contracts.get(name)
        if fe_entry is None:
            continue
        for field in ("contract_hash", "contract_package_hash", "deploy_hash"):
            if root_entry.get(field) != fe_entry.get(field):
                drift.append(
                    f"{name}.{field} differs: root={root_entry.get(field)} "
                    f"frontend={fe_entry.get(field)}"
                )
    return drift


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--no-rpc", action="store_true",
                        help="Skip live Casper RPC validation")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print what would be written, do not modify files")
    parser.add_argument("--check", action="store_true",
                        help="Verify frontend/public/onchain.json matches root. Exit 1 on drift.")
    args = parser.parse_args()

    if not ROOT_MANIFEST.exists():
        print(f"ERROR: root manifest not found at {ROOT_MANIFEST}", file=sys.stderr)
        print("This script requires an existing root manifest to regenerate.",
              file=sys.stderr)
        return 2

    existing = load_json(ROOT_MANIFEST)

    if args.check:
        if not FRONTEND_MANIFEST.exists():
            print(f"ERROR: frontend manifest missing at {FRONTEND_MANIFEST}",
                  file=sys.stderr)
            return 1
        fe = load_json(FRONTEND_MANIFEST)
        drift = check_drift(existing, fe)
        if drift:
            print("DRIFT DETECTED between root and frontend manifest:",
                  file=sys.stderr)
            for msg in drift:
                print(f"  - {msg}", file=sys.stderr)
            return 1
        print("OK: root and frontend manifest are in sync")
        return 0

    if not args.no_rpc:
        print("Validating contract hashes against Casper testnet RPC...")

    root = build_root_manifest(existing, validate_rpc=not args.no_rpc)
    frontend = derive_frontend_manifest(root)

    if args.dry_run:
        print("=== ROOT MANIFEST (dry-run) ===")
        print(json.dumps(root, indent=2))
        print("=== FRONTEND MANIFEST (dry-run) ===")
        print(json.dumps(frontend, indent=2))
        return 0

    dump_json(ROOT_MANIFEST, root)
    print(f"✅ Wrote root manifest: {ROOT_MANIFEST}")
    dump_json(FRONTEND_MANIFEST, frontend)
    print(f"✅ Wrote frontend manifest: {FRONTEND_MANIFEST}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
