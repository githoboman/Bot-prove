# CasperProver — On-chain Transaction Manifest

> Every claim of "on-chain" the project makes is backed by a concrete
> testnet transaction. This document is the single source of truth.
> Update it whenever a contract is deployed or a new evidence tx is
> emitted; never invent a hash.

**Network**: `casper-test` (testnet).
**Explorer base**: https://testnet.cspr.live/

---

## 1. Deployed contracts (verified live)

| Contract | Package hash | First deploy | Last deploy | Status |
|----------|--------------|--------------|-------------|--------|
| **proof-registry** | [`e11088f1...5fa7bc5`](https://testnet.cspr.live/contract/e11088f1f15a719f21c0c318d1f34d0b96419a22d60ac8fa384ecf5285fa7bc5) | 2026-06-29 | 2026-07-26 | ✅ Live (redeployed under canonical `defi_mock_owner`, deploy `c9e0fee7...`, verified via CSPR.cloud) |
| **verifier-gate**  | [`06d69182...cc0c79b66`](https://testnet.cspr.live/contract/06d69182b13c4d041613fe7e6e0805cdb06f099eff4291b40154d78cc0c79b66) | 2026-06-29 | 2026-07-26 | ✅ Live (redeployed under canonical `defi_mock_owner`, deploy `43ca8966...`, verified via CSPR.cloud) |
| **defi-mock**      | [`fe0c45f6...0b39ef`](https://testnet.cspr.live/contract/fe0c45f67c8cd99f0bda0047399a113588870ec0d79d9102f44107303f0b39ef) | 2026-07-07 | 2026-07-07 | ✅ Live |
| **stake-slashing** | [`1ad1b3d9...983d52`](https://testnet.cspr.live/contract/1ad1b3d94be631532d6daf3a195fafc9dfe8a16504e87d87784d51089b983d52) | (earlier) | 2026-07-19 | ✅ Live (hardened redeploy — arithmetic + self-verifying record_stake) |
| **proof-aggregation** | [`b29f32ab...63d2bb`](https://testnet.cspr.live/contract/b29f32abcc029d523de212bd7c87993f2f1bf96ba1523091c7b01adf6d63d2bb) | 2026-07-25 | 2026-07-25 | ✅ Live (deploy `35c003e5...`, verified via CSPR.cloud, `create_batch` guard suite — 28 tests) |
| **model-registry** | [`b3cdd1df...5e64a340a`](https://testnet.cspr.live/contract/b3cdd1df25714b341e34f6bb29f6c7900267e44c7742c81221e1eab5e64a340a) | 2026-07-25 | 2026-07-25 | ✅ Live (deploy `fd21b26e...`, verified via CSPR.cloud) |
| **proof-of-inference** | [`3d772fe1...12498b318`](https://testnet.cspr.live/contract/3d772fe1618fde438c4ffdaec22d83ffd9b4a1d769d6da32a38d56f12498b318) | 2026-07-25 | 2026-07-25 | ✅ Live (deploy `bde5cfb7...`, verified via CSPR.cloud) |
| **governance** | [`38d2fbd2...84063a0cf3e`](https://testnet.cspr.live/contract/38d2fbd24998719fac160c27e2e5435a99bcdebd4c36beac76abe84063a0cf3e) | 2026-07-26 | 2026-07-26 | ✅ Live (deploy `5f20ecfe...`, block 8635118, deployer anna-stolbovskaja, verified via CSPR.cloud; timelock 48h + 2-of-3 guardian recovery: anna/defi_mock_owner/reserved-slot. Prior install `03189ea1...548e` under defi_mock_owner is deprecated — retained on-chain but not authoritative.) |
| **zk-verifier** | [`1c13c999...8c807`](https://testnet.cspr.live/contract-package/1b4faa048f9d5a0366c42ec9a432dd9b3d8128368b889eb125f84d1047564f1a) | 2026-07-27 | 2026-07-27 | ✅ Live (deploy `1cbae7c8...`, block 8640738, deployer anna-stolbovskaja, verified via CSPR.cloud. On-chain vk registry + off-chain Groth16 verdict recorder — see contract source doc comment for the exact trust model, it is not a pairing verifier. Compiled since the hackathon deadline but only deployed 2026-07-27; was the one remaining gap flagged during a judge-key access review.) |

Contracts through `governance` were deployed 2026-07-25/26 from the
secondary account (`0202da6cfba1...` / `anna-stolbovskaja`) using an
MVP-clean WASM toolchain fix (Rust nightly `-Z build-std`,
bulk-memory-opt/sign-ext/reference-types disabled, `wasm-opt
--signext-lowering`, stripped `target_features` section — 0 non-MVP
opcodes); `zk-verifier` used the identical recipe on 2026-07-27. Deploy
hashes confirmed `processed` via `api.testnet.cspr.cloud/deploys/<hash>`
with matching timestamps and caller public key. No contracts remain
undeployed — see [`deploy-out/onchain.json`](../deploy-out/onchain.json)
for the full machine-readable manifest (9 contracts total).

## 3. Evidence transactions (representative)

A handful of representative txs judges can inspect directly. This is
not exhaustive — 248+ total testnet transactions across the nine live
contracts. The rows below are each contract's *install* deploy hash
(from `deploy-out/onchain.json`, the generated source of truth) —
guaranteed real and resolvable on cspr.live; entry-point-call hashes
(submit_proof, verify, etc.) are a follow-up, not yet backfilled here.

| Purpose | Tx hash | Notes |
|---------|---------|-------|
| proof-registry: install deploy | [`c9e0fee7...`](https://testnet.cspr.live/deploy/c9e0fee7cdb75abbf2a66d893d43bdf9d8d24d6ff7a103b841ad81f4a50d1c84) | Canonical redeploy under `defi_mock_owner`, 2026-07-26. |
| verifier-gate: install deploy | [`43ca8966...`](https://testnet.cspr.live/deploy/43ca896633e77e1ed87d43f7cd76e1fb1f57ccbdd60a249e92b496a7ee5986b2) | Canonical redeploy under `defi_mock_owner`, 2026-07-26. |
| defi-mock: install deploy | [`7e590fb9...`](https://testnet.cspr.live/deploy/7e590fb94fb0c3e41cd01e44e14157c3e537f4766a546d1dedbe5b137210625e) | 2026-07-07. |
| stake-slashing: install deploy | [`ac4712a3...`](https://testnet.cspr.live/deploy/ac4712a3ecc29c058330df88781d488f61c3993b7ee2720c2024fc2a231d2532) | Hardened redeploy, 2026-07-19. |
| proof-aggregation: install deploy | [`35c003e5...`](https://testnet.cspr.live/deploy/35c003e59ef5c335b3758445013b34a86c411cfc3be64da87ed958096d5b5646) | 2026-07-25. |
| model-registry: install deploy | [`fd21b26e...`](https://testnet.cspr.live/deploy/fd21b26ec69023aefd6d44d07963f3586b9084addc5ef810422acd6bed07c267) | 2026-07-25. |
| proof-of-inference: install deploy | [`bde5cfb7...`](https://testnet.cspr.live/deploy/bde5cfb70715f01b1fc7f6bfeb6f331113082ede7dd7973a8fafffb9937da95e) | 2026-07-25. |
| governance: install deploy | [`5f20ecfe...`](https://testnet.cspr.live/deploy/5f20ecfe2fc0a254db3daa965eb643b053102f14e53853f8c5c385424bdf60a2) | Canonical install under anna-stolbovskaja, block 8635118, 2026-07-26. |
| zk-verifier: install deploy | [`1cbae7c8...`](https://testnet.cspr.live/deploy/0c5f87ec45f1c51390203ea09210a2db517784f33f2d5bb9a4419deead9eaacc) | Canonical install under anna-stolbovskaja, block 8640738, 2026-07-27. |

**Adding an evidence tx**: open a deploy on cspr.live, copy the deploy
hash (starts with 64 hex chars, no `deploy-` prefix), and add a row
with `[hash-prefix...suffix](https://testnet.cspr.live/deploy/<full-hash>)`.

## 4. How to reproduce a hash

Every row in section 1 is reproducible without trust:

```bash
casper-client query-global-state \
  --node-address https://rpc.testnet.casperlabs.io \
  --state-root-hash "$(casper-client get-state-root-hash \
      --node-address https://rpc.testnet.casperlabs.io \
      | jq -r .result.state_root_hash)" \
  --key hash-<PACKAGE_HASH> \
  --path '' | jq
```

Replace `<PACKAGE_HASH>` with the hash from the manifest row. A
successful query returns the stored contract package with entry-point
list and named-key map — the same shape cspr.live renders.

## 5. Anti-linking pass

**CasperProver contracts are not shared with any other Casper hackathon
submission.** All eight package hashes above resolve to accounts under
the CasperProver deployer key. No shared deploys with AgentEscrow402
or any other submission.

## 6. Change log

| Date | Change | Author |
|------|--------|--------|
| 2026-06-29 | Initial deploy: proof-registry, verifier-gate | anna-stolbovskaja |
| 2026-07-07 | defi-mock deployed | anna-stolbovskaja |
| 2026-07-18 | stake-slashing arithmetic + record_stake self-verifying fix (found in in-house audit) | anna-stolbovskaja |
| 2026-07-19 | stake-slashing hardened redeploy | anna-stolbovskaja |
| 2026-07-25 | proof-of-inference, model-registry, proof-aggregation deployed (MVP-clean WASM toolchain fix) | anna-stolbovskaja |

## 7. Non-goals of this document

- **No secrets.** Wallet keys, deployer secrets, RPC tokens are never
  in this file. Only public hashes.
- **No test fixture hashes.** Only real testnet deploys; local nctl
  hashes belong in `docs/DEV_ENVIRONMENT.md` if anywhere.
- **No promises.** A row without a hash is *pending*, not on-chain.
  Do not link to `docs/TX_MANIFEST.md#pending` as if it were evidence.
