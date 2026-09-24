# On-chain Manifest — Single Source of Truth

CasperProver keeps every contract hash, package hash, and deploy hash in **one canonical file** so nothing drifts between backend, frontend, `verify.sh`, and the README explorer links.

## Files

| Role | Path | Editable by hand? |
|------|------|-------------------|
| **Root canonical** | `deploy-out/onchain.json` | ⚠️ Only when a contract is (re)deployed |
| Frontend copy | `frontend/public/onchain.json` | ❌ Generated — do not edit |
| Generator | `scripts/generate_manifest.py` | ✅ |
| Verifier | `verify.sh` (loads root manifest via `jq`) | ❌ Do not add hashes here |

## Rule

**Never hand-edit a contract hash outside `deploy-out/onchain.json`.** Everything else derives from it.

If you edit the root manifest, run:

```
python scripts/generate_manifest.py
```

This regenerates `frontend/public/onchain.json`, validates each hash against Casper testnet RPC, and stamps a fresh `generated_at` timestamp.

## Schema (root canonical only)

```jsonc
{
  "$schema": "./onchain.schema.json",
  "network": "casper-test",
  "chain_name": "casper-test",
  "project": "CasperProver",
  "deployer": "0203975636c0c327...",
  "explorer": "https://testnet.cspr.live",
  "cspr_cloud": "https://testnet.cspr.cloud",

  // Metadata specific to the root canonical (stripped from frontend copy)
  "manifest_version": "1.0",
  "canonical_source": "deploy-out/onchain.json",
  "regenerate_command": "python scripts/generate_manifest.py",
  "notes": "…",
  "generated_at": "2026-07-19T20:58:24Z",
  "generator_version": "1.0.0",

  "contracts": {
    "proof_registry": {
      "contract_hash": "…",           // 64 hex — hash of the versioned contract
      "contract_package_hash": "…",   // 64 hex — package hash, stable across upgrades
      "deploy_hash": "…",             // 64 hex — deploy that installed this version
      "version": 1,
      "deployed_at": "2026-06-29T09:33:43Z",
      "source": "contracts/proof-registry/src/main.rs",
      "entry_points": ["submit_proof", "get_proof", "revoke_proof", "register_agent", "get_reputation"],
      "_validated": { "ok": true, "detail": "on-chain", "at": "2026-07-19T20:58:24Z" }
    }
    // …verifier_gate, defi_mock, stake_slashing
  },

  "undeployed_contracts": { /* work-in-progress contracts, informational only */ },
  "verification": { /* judge-facing verification checklist */ }
}
```

Frontend copy is identical **minus** these fields: `$schema`, `manifest_version`, `canonical_source`, `regenerate_command`, `notes`, `generated_at`, `generator_version`, and `_validated` inside each contract.

## Generator commands

```
# Regenerate everything, validate against live testnet RPC (default)
python scripts/generate_manifest.py

# Skip the RPC roundtrip (useful in CI without network)
python scripts/generate_manifest.py --no-rpc

# See what would be written without touching files
python scripts/generate_manifest.py --dry-run

# CI drift check: exit 1 if frontend copy diverges from root
python scripts/generate_manifest.py --check
```

## How `verify.sh` uses it

`verify.sh` no longer contains hardcoded hashes. It loads the root manifest at runtime with `jq`, extracts every `contracts.<name>.contract_hash`, and queries Casper testnet RPC for each one. If the manifest is missing or empty, `verify.sh` exits with code 2 and tells the operator to regenerate it. See the "On-chain contracts" section of `verify.sh` for the loader.

## Adding a new contract

1. Deploy the contract to Casper testnet.
2. Append its entry under `contracts.<name>` in `deploy-out/onchain.json` (same shape as existing entries).
3. Run `python scripts/generate_manifest.py`.
4. Confirm the RPC validation prints `✅` for the new contract.
5. Commit both `deploy-out/onchain.json` and `frontend/public/onchain.json` in the same commit.

## Removing a contract

1. Move the entry from `contracts` to `undeployed_contracts` (with a note explaining why).
2. Regenerate. The frontend copy will stop advertising it, and `verify.sh` will stop trying to validate it.

## CI drift guard

The pre-commit hook and CI pipeline both run `python scripts/generate_manifest.py --check`. Any drift between the root manifest and the frontend copy fails the build.

## Why one file

Before this refactor:
- `verify.sh` had 4 hashes hardcoded in a bash array.
- `frontend/public/onchain.json` had them too, sometimes drifting from what was actually deployed.
- README links pointed at hashes copy-pasted from Slack.

Result: three sources of truth, none of which agreed after any redeploy. Now there is one file, one generator, and a drift guard. That is the whole point of the root canonical.
