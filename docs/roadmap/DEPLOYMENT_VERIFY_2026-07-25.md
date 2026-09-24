# Deployment verification — three MVP-clean contracts

**Date:** 2026-07-25 → verified 2026-07-26
**Network:** casper-test
**Deployer account:** secondary signer (redacted — non-Anna).
Anna's account has a `storage::new_dictionary` name-collision on prior
`stakes` / `slashed_proofs` named keys, so the three new contracts were
deployed from the alternate account. Old `stake_slashing` contract was
already redeployed from the same alternate account on 2026-07-19.

The three previously-`undeployed` contracts are now on-chain and observable
on the public RPC + explorer indexer:

## proof_aggregation

- `contract_hash`  = `b29f32abcc029d523de212bd7c87993f2f1bf96ba1523091c7b01adf6d63d2bb`
- `contract_package_hash` = `4d891784b5874b5c1b0f707b318702898e32deb527d42fc236e7da6adabf04b9`
- `deploy_hash`    = `35c003e59ef5c335b3758445013b34a86c411cfc3be64da87ed958096d5b5646`
- block           = `bab1abf3b9a376f258d262bcecb89aae3d0f360311893db8676c100d228210ee`
- gas cost        = `55` CSPR
- WASM size       = 40 928 B
- entry_points (state-verified via `state_get_item`): `add_proof`,
  `create_batch`, `finalize_batch`, `get_stats`

## model_registry

- `contract_hash`  = `b3cdd1df25714b341e34f6bb29f6c7900267e44c7742c81221e1eab5e64a340a`
- `contract_package_hash` = `7dc3db96942659492b6d14097f3a8c5d596bfb86e06e4fb58a6628f820c3d2c1`
- `deploy_hash`    = `fd21b26ec69023aefd6d44d07963f3586b9084addc5ef810422acd6bed07c267`
- block           = `3fec945ba555c14f4132b6e2f47fc8aa9d2f30d40e52ecd65ca5b2cd9c1e540b`
- gas cost        = `65` CSPR
- WASM size       = 46 540 B
- entry_points (state-verified): `configure_price_bps`, `deprecate_model`,
  `get_model`, `get_price_bps`, `register_model`, `transfer_ownership`,
  `update_model`, `verify_model`

## proof_of_inference

- `contract_hash`  = `3d772fe1618fde438c4ffdaec22d83ffd9b4a1d769d6da32a38d56f12498b318`
- `contract_package_hash` = `69053cda288aa30649edb9fbfe17b4e0dfe6d6ec3f9afcd465574a3c885f3dc3`
- `deploy_hash`    = `bde5cfb70715f01b1fc7f6bfeb6f331113082ede7dd7973a8fafffb9937da95e`
- block           = `6cabc3e713eae0c22693ec512fc1bf124b3a1cc66726a51adac906a722887477`
- gas cost        = `75` CSPR
- WASM size       = 50 563 B
- entry_points (state-verified): `challenge_proof`, `get_proof`, `get_stats`,
  `register_proof`, `register_verifier`, `resolve_challenge`, `revoke_verifier`,
  `verify_proof`

## MVP-clean WASM toolchain used

Rust nightly with
`-Z build-std=core,alloc,compiler_builtins,panic_abort` plus:

- `bulk-memory-opt`, `sign-ext`, `reference-types` — disabled
- post-build `wasm-opt --signext-lowering`
- `target_features` custom section stripped

Result: 0 non-MVP opcodes, 0 `rust_begin_unwind` imports — all three WASMs
pass the Casper 1.5.x MVP validator (which was the previous blocker that had
these three sitting in `undeployed_contracts` since the hackathon).

## Verification commands (reproducible)

```bash
# 1. Confirm on-chain execution success + deployer + cost + block
for h in 35c003e5…d5b5646 fd21b26e…bed07c267 bde5cfb7…7da95e; do
  curl -s https://api.testnet.cspr.live/deploys/$h | jq .data
done

# 2. Confirm entry_points live in global state (indexer-independent)
STATE=$(curl -s -X POST https://node.testnet.casper.network/rpc \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"chain_get_state_root_hash"}' \
  | jq -r .result.state_root_hash)
curl -s -X POST https://node.testnet.casper.network/rpc \
  -H 'Content-Type: application/json' \
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"state_get_item\",\"params\":{\"state_root_hash\":\"$STATE\",\"key\":\"hash-<contract_hash>\",\"path\":[]}}" \
  | jq '.result.stored_value.Contract | {entry_points: [.entry_points[].name], named_keys_count: (.named_keys | length)}'
```

## `onchain.json` change summary

- Moved three contracts out of `undeployed_contracts` into `contracts`,
  each with `contract_hash`, `contract_package_hash`, `deploy_hash`,
  `deployer`, `version`, `deployed_at`, `notes`.
- Removed the top-level `deployer` field (it did not match any actual
  on-chain signer for the deployed set) and replaced it with a
  `deployer_note` documenting that contracts are split between the two
  accounts (`0202e554…` for pre-existing `proof_registry` and
  `verifier_gate`; `0202da6c…` for `defi_mock`, `stake_slashing` and the
  three new ones).
- Existing `stake_slashing_session` remains in `undeployed_contracts`
  (still awaiting deploy).

Backup of the pre-change file: `deployments/backups/onchain-20260725T212307Z.json`
(untracked — not committed by design).

## Follow-ups

- End-to-end smoke calls of at least one read-only entrypoint per new
  contract (e.g. `get_stats` / `get_price_bps` / `get_proof`) from a
  wrapper script — deferred to the parallel agent that owns
  `scripts/smoke-call.mjs`.
- FE `onchain.json` consumer already picks the new hashes up because it
  reads `contracts` (not `undeployed_contracts`); confirmation with the
  admin dashboard belongs to the FE slot (backlog 9.6).
