# Smoke verification — 3 MVP-clean contracts, read-only entrypoints

**Date:** 2026-07-26
**Network:** casper-test (`https://node.testnet.casper.network/rpc`)
**Signer:** `anna-stolbovskaja` (public key
`0202e554b557851b894830b3814fc0f5df7e147937400c537fe5252fc53f4e25257e`,
SECP256K1). No other keys involved — Anna is the only account we use
for CP contract callers.

Purpose: prove the three MVP-clean contracts promoted in
`c9d3879` (`proof_aggregation`, `model_registry`,
`proof_of_inference`) accept an on-chain call end-to-end from a
non-installer signer.

## Method

Each contract has a documented read-only entrypoint that requires no
arguments:

| Contract | Entrypoint | Notes |
|---|---|---|
| `proof_aggregation` | `get_stats` | returns 4-tuple `(u64,u64,u64,u64)` via `runtime::ret` |
| `model_registry`    | `get_price_bps` | returns `u64` |
| `proof_of_inference`| `get_stats` | returns 4-tuple `(u64,u64,u64,u64)` |

The Casper 2.0 storage model requires an on-chain deploy even for
read-only entrypoints (they run `runtime::ret` and consume gas). We
send the minimum viable transaction from Anna, wait for finality on
the public RPC, and read `execution_result.Version2.error_message`
to confirm success.

Driver:
`scripts/smoke-call.mjs <secret.pem> <contract-hash-hex> <entry_point> {}`
(uses `casper-js-sdk@5.0.12`, 3 CSPR payment cap, `WAIT_TIMEOUT_MS`
env default 7 min).

## Results

Log: `reports/smoke-anna-2026-07-26T16-15-33Z.jsonl`.

All three calls were dispatched from Anna. The script's 180 s
finality-poll (old default) timed out on each — the RPC then
confirmed all three were `processed` with `error_message == None`,
which is what we care about. Script default raised to 420 s so future
runs will see finality inline.

| Contract | Entrypoint | tx hash | block hash | cost (motes) | consumed (motes) | error |
|---|---|---|---|---|---|---|
| `proof_aggregation` | `get_stats` | `0c531dff…c85c` | `48451cd0…54bc` | 3 000 000 000 | 15 847 010 | none |
| `model_registry` | `get_price_bps` | `412afa00…6c16` | `ca414721…7ffb` | 3 000 000 000 | 15 594 875 | none |
| `proof_of_inference` | `get_stats` | `4f2df881…6935` | `14e7d874…fb85` | 3 000 000 000 | 399 874 340 | none |

`cost` is the 3 CSPR fixed payment cap set by the driver; `consumed`
is the actual gas the entrypoint spent. `proof_of_inference.get_stats`
spends ~25× more than the other two because it aggregates over the
proofs & verifiers dictionaries (see `contracts/proof-of-inference/src/main.rs:329`).

Full tx hashes:
- `0c531dffa257ba4a3f892fee31f0229f215a6bc91c51deca31c90e8b07ddc85c`
- `412afa00e568c9f1439c4a73492749a6d75b289041db4657f5d63dacae676c16`
- `4f2df881f92d97d9965767ddf4010111db775afb2584a98d78659f87e7926935`

Explorer:
- <https://testnet.cspr.live/deploy/0c531dffa257ba4a3f892fee31f0229f215a6bc91c51deca31c90e8b07ddc85c>
- <https://testnet.cspr.live/deploy/412afa00e568c9f1439c4a73492749a6d75b289041db4657f5d63dacae676c16>
- <https://testnet.cspr.live/deploy/4f2df881f92d97d9965767ddf4010111db775afb2584a98d78659f87e7926935>

## What this proves

- All three contracts are callable on Casper testnet from a
  non-installer signer.
- Their WASM (MVP-clean, non-installer entrypoints) accepts calls,
  runs, and returns without error.
- `get_stats` on both contracts survives cross-account access (the
  installer/admin gate does not block read-only entrypoints — as
  intended).

## What this does NOT prove

- Write-path entrypoints (`create_batch`, `register_model`,
  `register_proof`, …). Those need per-contract fixtures + arg-typed
  smoke calls, tracked separately.
- End-to-end proof-anchoring flow (engine → contract). That belongs
  in an integration-test slot, not this verification.
