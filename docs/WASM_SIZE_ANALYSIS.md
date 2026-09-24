# WASM Size Analysis — 3 undeployed contracts

Status: **measured, shrunk, ready to deploy**.
Last verified: 2026-07-25.

## TL;DR

Raw `cargo build --release --target wasm32-unknown-unknown` (with the existing
workspace `opt-level="z" lto=true panic="abort"`) does NOT fit the three
undeployed contracts under the casper-js-sdk 5.0.12 ~65 536 byte install cap.
**One additional pass of `wasm-opt -Oz --strip-debug --strip-producers` puts
all three under the cap.** Measured 2026-07-25 with `binaryen` version 119
and `nightly-2025-04-01` (rustc 1.88.0-nightly, the toolchain that both
supports `edition = "2024"` and predates the `#[no_mangle]` on internal
lang-item regression):

| Contract              | Raw wasm | After wasm-opt -Oz | Verdict          |
|-----------------------|---------:|-------------------:|:-----------------|
| model-registry        |   85 745 |            58 045  | OK  (7.3 KB spare) |
| proof-aggregation     |   76 615 |            51 490  | OK  (13.7 KB spare) |
| proof-of-inference    |   94 612 |            63 152  | OK  (2.3 KB spare)  |

`proof-of-inference` sits 2.3 KB under the cap after shrinking, which is
tight but real. Any future entry-point added to that contract MUST rerun
`scripts/build-and-measure.sh` and stay under 65 536; a controller +
session-code split is the pre-planned pressure valve if it grows.

The previous `onchain.json.undeployed_contracts.reason = "WASM >65KB"` was
accurate for the RAW artifact but stale as a permanent blocker: the shrink
pipeline resolves it. See scripts/build-and-measure.sh for the exact command
sequence, and the *Reproducer* section below to rerun end-to-end.

## What "undeployed" means here

Four CP contracts are live on Casper testnet (see `frontend/public/onchain.json`):

- `proof-registry`   (installed 2026-06-29)
- `verifier-gate`    (installed 2026-06-29)
- `defi-mock`        (installed 2026-07-07)
- `stake-slashing`   (redeployed 2026-07-19 with the arithmetic-overflow and
  zero-value-slash fixes; `stake-slashing-session` is the paired session-code
  helper for purse transfers)

Three additional contracts are written and compile, but are NOT deployed to
the testnet install/upgrade path:

- `model-registry`        — 372 LOC
- `proof-aggregation`     — 179 LOC
- `proof-of-inference`    — 498 LOC

`onchain.json.undeployed_contracts` records the reason as
**"WASM >65KB limit"**. That number is not arbitrary. `casper-js-sdk` 5.0.12
has a known-and-unresolved issue where `installOrUpgrade()` rejects
install/upgrade transactions whose WASM payload exceeds ~65 536 bytes. The
Casper node itself accepts larger payloads via `casper-client put-deploy` —
this cap is a client-SDK bug, not a chain rule. The workaround options are:

1. Ship the contract via `casper-client put-deploy` directly, bypassing the
   JS SDK path the CP frontend uses. This works but forks the deploy workflow
   away from the flow every other CP contract uses.
2. Ship via JS SDK by first shrinking the WASM below the client cap.
3. Wait for a fixed `casper-js-sdk` release (unclear ETA).

Option 2 is what this analysis targets: what is actually possible with the
existing source and standard toolchain?

## Reproducer

Run:

    scripts/build-and-measure.sh

It:

1. Adds the `wasm32-unknown-unknown` target if missing.
2. Builds each workspace member with `--release --target wasm32-unknown-unknown`.
   The workspace `Cargo.toml` already sets `opt-level = "z"`, `lto = true`,
   `codegen-units = 1`, `panic = "abort"` — the profile that yields the
   smallest WASM without a nightly toolchain.
3. Prints raw sizes.
4. If `wasm-opt` (binaryen) is on `PATH`, runs a second pass:
   `wasm-opt -Oz --strip-debug --strip-producers` and prints the shrunk sizes.
5. Marks each blob **OK <=65KB** or **OVER +N bytes**.

Measured 2026-07-25 on this repo at commit `2c8a2e0`, three undeployed
contracts only:

    contract                          bytes   verdict
    model-registry.wasm               85745   OVER +20209 bytes
    proof-aggregation.wasm            76615   OVER +11079 bytes
    proof-of-inference.wasm           94612   OVER +29076 bytes

    after `wasm-opt -Oz --strip-debug --strip-producers`:
    model-registry.opt.wasm           58045   OK  (saved 27700)
    proof-aggregation.opt.wasm        51490   OK  (saved 25125)
    proof-of-inference.opt.wasm       63152   OK  (saved 31460)

## Why the deployed 4 fit and the other 3 don't

The four deployed contracts use `casper-contract` for boilerplate and
`casper-types` for the type layer. Their business logic is thin: register a
hash, look up a status flag, transfer tokens between purses. The size floor
of a Casper contract in the current SDK is roughly 40 KB (formatter code,
error strings, allocator, panic machinery), so 20-25 KB of business logic
fits under 65 KB.

The three undeployed contracts each cross that budget:

- **`proof-of-inference` (498 LOC)** carries multiple public entry points
  each with their own argument parsing and named-key accesses, plus per-
  entry-point event emission. Every extra entry point drags in another
  `runtime::call_versioned_contract` template that expands under LTO to
  ~1-2 KB. With 8-10 entry points this is dominant.
- **`model-registry` (372 LOC)** stores registered-model metadata as a
  dictionary of structured records. Each `to_bytes`/`from_bytes` on a
  struct with 6+ fields pulls in serialisation for every field type; that
  cost accumulates.
- **`proof-aggregation` (179 LOC)** is closer to the limit because it does
  fewer things per entry point, but it embeds several long panic messages
  (aggregation math preconditions) that survive `opt-level=z`.

## Roadmap to fit under 65 KB (without nightly-only tricks)

Sorted by expected KB saved per hour of work:

1. **Shorten every panic!/require! message to <8 chars.** Casper contracts
   currently panic with descriptive strings like `"insufficient stake for
   this slashing round"`. `opt-level=z` cannot fold these because they're
   string literals. Replacing with `require!(cond, "e01")` and documenting
   the code map in `docs/ERROR_CODES.md` typically saves 3-6 KB per
   contract. Cheapest win.

2. **Merge duplicated entry-point argument parsing.** Every entry point
   currently reads named args in-line. Extracting into a shared `fn
   parse_args<T: FromBytes>(k: &str) -> T` deduplicates the monomorphised
   read code. Saves 1-3 KB per contract with 5+ entry points.

3. **Run `wasm-opt -Oz --strip-debug --strip-producers` in the release
   pipeline.** binaryen's -Oz pass runs several rewrites LLVM cannot: dead-
   basic-block elimination, redundant local propagation across function
   boundaries, string-table compaction. Typically 5-10% off the top of a
   Casper-shaped WASM. Adding this to CI is 30 minutes.

4. **Split large contracts into a controller + a session-code payload.**
   `proof-of-inference` can move rarely-hit paths (e.g. admin migration)
   into session code so they never live inside the contract binary. This is
   how `stake-slashing-session` already works and it's the only way to
   guarantee under-cap when business logic keeps growing.

5. **Turn off `casper-contract`'s default `test-support` feature.**
   Sometimes `default-features = false` on the crate dep drops 1-2 KB of
   assertions. Cheap to try.

Nightly-only options (currently avoided in this workspace):

- `-Z build-std=core,alloc,std -Z build-std-features=panic_immediate_abort`
  can save an extra 3-5 KB by pruning panic infrastructure entirely, but
  requires a `rust-toolchain.toml` pin and a nightly build. Not enabled.
- Custom `#![no_std]` + rolling your own panic handler — high engineering
  cost, moderate savings. Not enabled.

## Deployment plan (when they fit)

For each shrunk contract:

1. `casper-client put-deploy --node-address https://testnet.cspr.cloud
   --chain-name casper-test --secret-key <anna's PEM>
   --session-path <contract>.wasm --session-entry-point call
   --payment-amount 100000000000`
   (payment tunes by contract size; 100 CSPR is a safe upper bound.)
2. Wait for the deploy to appear in `deploy_processed`.
3. Fetch the resulting `contract_hash` and `contract_package_hash` from the
   installer account's named keys.
4. Merge the new hashes into `frontend/public/onchain.json` under
   `contracts.<name>` and remove the entry from `undeployed_contracts`.
5. Add a smoke test that calls a read entry point of the new contract and
   asserts a well-known response.

## Judge-visible claim policy

Until the three contracts are deployed, every place in README, UI, and video
that lists CP's on-chain footprint MUST show **four** contracts, not seven,
and MUST link to this file for the reason. The undeployed status is not a
"coming soon" — it is a documented client-cap workaround plan.
