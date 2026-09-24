# Governance contract deployment — 2026-07-26

Deployment record for the 8th CasperProver contract: `governance` — timelock, emergency pause, owner recovery. Closes BACKLOG items 1.6, 1.9, 1.10, 1.11.

## On-chain fingerprint (canonical)

| field | value |
|---|---|
| **network** | casper-test |
| **deploy_hash** | `5f20ecfe2fc0a254db3daa965eb643b053102f14e53853f8c5c385424bdf60a2` |
| **contract_hash** | `38d2fbd24998719fac160c27e2e5435a99bcdebd4c36beac76abe84063a0cf3e` |
| **contract_package_hash** | `69df9be9b3ae690ef36ba9b0535770716fbf52282ee5f76d44a65254b205fbe0` |
| **block_height** | 8635118 |
| **block_hash** | `25309e0cc6b77ee22b7d8910759e6a93194363a27c3689b1f6d849093bcf9e18` |
| **timestamp** | 2026-07-26T21:40:00Z |
| **status** | processed / error=none ✅ |
| **cost** | 75 CSPR (consumed_gas) |
| **deployer / owner (bootstrap)** | `anna-stolbovskaja` — public_key `0202e554b557851b894830b3814fc0f5df7e147937400c537fe5252fc53f4e25257e`, acct_hash `cac1862d745c06860c6acfa275d6afec3be692dedf6c46253ce813dba74ad298` |

### Prior deploy (deprecated, retained on-chain)

An earlier install landed under `defi_mock_owner` at `03189ea1721b517c64073c319e93d6a8cd0e53191925fcf530ebc3b897f9548e` (deploy `1d7c9e8fa8a249977003fc1b36e443cf35c021b20c82e14513725b028ae77440`, block 8634452). It is **not authoritative** — its guardian slots were replaced during canonical deploy above. Subsequent re-install attempts under `defi_mock_owner` failed pre-execution because the NamedKey `governance` was already occupied; canonical deploy therefore switched account to `anna-stolbovskaja` (clean namespace).

## Guardians (3-of-3 slots, 2-of-3 threshold for recovery)

Configured at install-time via runtime args `guardian_1` / `guardian_2` / `guardian_3`. Values are Casper account-hash strings (32-byte hex, no `account-hash-` prefix).

| slot | account_hash | role |
|---|---|---|
| guardian_1 | `cac1862d745c06860c6acfa275d6afec3be692dedf6c46253ce813dba74ad298` | project owner (anna-stolbovskaja) |
| guardian_2 | `84ff0a4692ddeaa2c2991b0a9b084e48772968c242ff9434106571e77d23be10` | defi_mock_owner (contract admin) |
| guardian_3 | `0000000000000000000000000000000000000000000000000000000000000000` | **reserved slot** — populated at mainnet ceremony per `docs/MAINNET_LAUNCH_PLAN.md` |

The zero-hex placeholder is intentional and documented: a 2-of-3 recovery threshold across the two live guardians is fully functional on testnet, and the mainnet launch plan describes the ceremony that fills slot 3 with a canonical guardian key (hardware-backed, separate custody).

## Entry points

`propose`, `execute`, `cancel`, `get_proposal`, `is_executed`, `emergency_pause`, `propose_unpause`, `execute_unpause`, `propose_owner_transfer`, `execute_owner_transfer`, `sign_recovery`, `execute_recovery`.

Default `timelock_secs = 48 * 60 * 60` (48 h).

## MVP-clean WASM build recipe

`casper-js-sdk 5.0.12` installOrUpgrade caps modules at ~65_536 bytes AND rejects any wasm carrying non-MVP opcodes (`bulk-memory`, `sign-ext`, `reference-types`, `mutable-globals`). A stock `cargo +nightly build --release` emits `memory.copy` / `memory.fill` from `core` even when RUSTFLAGS forbid them, because `core` is prebuilt for the target. Fix: rebuild `core` with `-Z build-std` **and** `-Z build-std-features=panic_immediate_abort` to eliminate the `rust_begin_unwind` import.

Exact reproducer used for this deploy (from `contracts/`):

```bash
rustup toolchain install nightly-2025-04-01 --profile minimal --target wasm32-unknown-unknown
rustup component add rust-src --toolchain nightly-2025-04-01

export RUSTFLAGS='-C target-cpu=mvp -C target-feature=-bulk-memory,-sign-ext,-mutable-globals,-reference-types'

cargo +nightly-2025-04-01 build --release --target wasm32-unknown-unknown \
    -Z build-std=core,alloc,compiler_builtins,panic_abort \
    -Z build-std-features=panic_immediate_abort \
    -p governance
```

Post-build shrink + strip:

```bash
wasm-opt -Oz --strip-debug --strip-producers --strip-target-features \
    --signext-lowering --disable-bulk-memory --disable-sign-ext --disable-reference-types \
    --converge \
    contracts/target/wasm32-unknown-unknown/release/governance.wasm \
    -o /tmp/governance.opt.wasm

node scripts/wasm-strip-features.mjs /tmp/governance.opt.wasm /tmp/governance.final.wasm
```

Verification:

```
$ stat -c%s /tmp/governance.final.wasm
53774                             # under 65536 cap ✅

$ python3 -c "d=open('/tmp/governance.final.wasm','rb').read(); \
    print('memory.copy', d.count(b'\\xfc\\x0a')); \
    print('memory.fill', d.count(b'\\xfc\\x0b'))"
memory.copy 0                     # MVP-clean ✅
memory.fill 0                     # MVP-clean ✅
```

Package name in this WASM is `governance_v2_pkg` (unique NamedKeys) — this avoids collision with any prior `governance_pkg` NamedKey on the deployer's account.

## Deploy submission

`scripts/deploy-wasm.mjs` reads three named args for governance install:

```js
const args = Args.fromMap({
  guardian_1: CLValue.newCLString(G1_ACCT_HASH_HEX),
  guardian_2: CLValue.newCLString(G2_ACCT_HASH_HEX),
  guardian_3: CLValue.newCLString(G3_ACCT_HASH_HEX),
});
```

Submitted via `SessionBuilder.installOrUpgrade()` against `https://node.testnet.cspr.cloud/rpc` with `Authorization` header set to the cspr.cloud API key.

## Failed pre-fix attempts (full honesty)

Under `defi_mock_owner` the second install attempt of governance failed pre-execution because the `governance` NamedKey slot was already occupied by the deprecated `03189ea1…548e`. Ranaming the WASM package to `governance_v2_pkg` alone did not resolve it — the deployer's existing `governance` root NamedKey blocked `put_key` regardless of the underlying package name. Switching the deployer to `anna-stolbovskaja` (clean namespace, no prior `governance*` NamedKeys) resolved it on the first attempt.

Payment budgets consumed by the failed pre-execution attempts were spent (no refund policy on `casper-node` for early-abort deploys); this is documented for transparency and does not affect the canonical set.

## Post-install verification

```
$ curl -sS -H "Authorization: $CSPR_CLOUD_API_KEY" \
    "https://api.testnet.cspr.cloud/deploys/5f20ecfe2fc0a254db3daa965eb643b053102f14e53853f8c5c385424bdf60a2" \
    | jq '.data | {status, error_message, block_height, caller_hash, cost}'
{
  "status": "processed",
  "error_message": null,
  "block_height": 8635118,
  "caller_hash": "cac1862d745c06860c6acfa275d6afec3be692dedf6c46253ce813dba74ad298",
  "cost": "75000000000"
}
```

Contract discovery lands under the deployer's named keys, not the deploy itself. Direct read via `state_get_entity`:

```
governance = 38d2fbd24998719fac160c27e2e5435a99bcdebd4c36beac76abe84063a0cf3e
governance_pkg = 69df9be9b3ae690ef36ba9b0535770716fbf52282ee5f76d44a65254b205fbe0
```

Cross-referenced via `state_get_entity(account-hash-cac1862d…d298)` at the latest block — both hashes present under anna's `named_keys`.

## Downstream integration

`verify.sh` checks all 8 canonical contracts. Frontend `onchain.json` (public/dist) mirrored to `deploy-out/onchain.json`. Judge guide references 8 contracts.

The 7 previously-deployed contracts (proof-registry, verifier-gate, defi-mock, stake-slashing, proof-aggregation, model-registry, proof-of-inference) opt into governance via the `is_executed(pid)` read entry-point on their next upgrade. No live redeploy is required — the governance record travels in whichever contract calls it, keyed by proposal id.
