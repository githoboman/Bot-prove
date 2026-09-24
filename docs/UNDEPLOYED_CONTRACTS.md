# Undeployed contracts — resolved (all four now live)

> **Update (2026-07-26):** the three contracts this doc used to track as
> "not yet deployed" (`proof-of-inference`, `model-registry`,
> `proof-aggregation`) were deployed to Casper testnet on 2026-07-25 from
> the secondary deployer account (`0202da6cfba1...`), using an MVP-clean
> WASM build (Rust nightly `-Z build-std=core,alloc,compiler_builtins,panic_abort`,
> bulk-memory-opt/sign-ext/reference-types disabled, `wasm-opt
> --signext-lowering`, stripped `target_features` section — 0 non-MVP
> opcodes) that fixed the size/opcode issues this doc previously
> documented. All three deploys are recorded in
> [`deploy-out/onchain.json`](../deploy-out/onchain.json) and confirmed
> `processed` via `api.testnet.cspr.cloud/deploys/<hash>`.
>
> **Update (2026-07-27):** `zk-verifier` (`contracts/zk-verifier/`) was
> written post-hackathon-deadline as roadmap work (BACKLOG 1.8) and
> compiled/tested from day one, but nobody had actually run the deploy —
> it sat compiled-only while `deploy-out/onchain.json` and every doc
> referencing "8 contracts" quietly excluded it. Deployed to testnet
> 2026-07-27 using the identical MVP-clean recipe (53107 B, under the
> 65536 B installOrUpgrade cap), deploy `1cbae7c8...`, verified via
> `api.testnet.cspr.cloud/deploys/<hash>`.
>
> Canonical status now lives in [`TX_MANIFEST.md`](TX_MANIFEST.md) —
> CasperProver has **9 contracts live** on testnet, none pending.

| Contract | Contract hash | Deploy hash | Deployed |
|---|---|---|---|
| `proof-of-inference` | `3d772fe1618fde438c4ffdaec22d83ffd9b4a1d769d6da32a38d56f12498b318` | `bde5cfb70715f01b1fc7f6bfeb6f331113082ede7dd7973a8fafffb9937da95e` | 2026-07-25 |
| `model-registry` | `b3cdd1df25714b341e34f6bb29f6c7900267e44c7742c81221e1eab5e64a340a` | `fd21b26ec69023aefd6d44d07963f3586b9084addc5ef810422acd6bed07c267` | 2026-07-25 |
| `proof-aggregation` | `b29f32abcc029d523de212bd7c87993f2f1bf96ba1523091c7b01adf6d63d2bb` | `35c003e59ef5c335b3758445013b34a86c411cfc3be64da87ed958096d5b5646` | 2026-07-25 |
| `zk-verifier` | `4500da5d314f11f8a535b9c70fb67729da6b452efc6c302abf292b42b6dc96a1` | `0c5f87ec45f1c51390203ea09210a2db517784f33f2d5bb9a4419deead9eaacc` | 2026-07-27 |

This file is kept only as historical context for the old
size/opcode-fix work; the active source of truth for deploy status is
`TX_MANIFEST.md` and `deploy-out/onchain.json`.
