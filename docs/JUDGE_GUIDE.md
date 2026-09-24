# Judge Verification Guide

Shortest reproducible path through CasperProver, mapped to the eight judging criteria. Every claim is labeled **REAL CRYPTO**, **ON-CHAIN**, or **SIMULATION** so it can be verified rather than trusted.

## 0. One-command evidence

```bash
python3 scripts/judge_demo.py
```

Read-only by default: queries all **nine** Casper testnet contracts, API health/proofs, and the frontend. For the real Groth16 write round-trip:

```bash
CP_JUDGE_API_KEY='request-from-team' python3 scripts/judge_demo.py
```

**This round's submission form has no free-text field for accompanying notes, so the key isn't pre-delivered anywhere.** If you want to run the authenticated write round-trip, open a GitHub issue on this repo (or message us via the DoraHacks submission page) and we'll hand you the key directly — it's never committed, logged, or embedded in the repo/URL/shell history, and gets revoked after judging. Every read-only check above works with zero setup regardless.

Second lane, if you prefer a shell-only path:

```bash
./verify.sh
```

## 1. Canonical on-chain surface (`deploy-out/onchain.json`)

Eight contracts, all Casper testnet, all cross-linked in the frontend `/lab/contracts`:

| Contract | Hash (first…last) | Deployer | Purpose |
|---|---|---|---|
| `proof_registry` | `e11088f1…7bc5` | `defi_mock_owner` | Proof metadata + reputation store |
| `verifier_gate` | `06d69182…9b66` | `defi_mock_owner` | Whitelist check over `proof_registry` |
| `defi_mock` | `fe0c45f6…39ef` | `defi_mock_owner` | Lending gate consumer (KYC + whitelist) |
| `stake_slashing` | `1ad1b3d9…3d52` | `defi_mock_owner` | Stake + slashing bookkeeping |
| `proof_aggregation` | `b29f32ab…d2bb` | `defi_mock_owner` | Batched proof anchoring |
| `model_registry` | `b3cdd1df…340a` | `defi_mock_owner` | Model provenance registry |
| `proof_of_inference` | `3d772fe1…b318` | `defi_mock_owner` | Inference attestation ledger |
| `governance` | `38d2fbd2…cf3e` | `anna-stolbovskaja` | Timelock, guardians, recovery |

Anchor: `deploy-out/onchain.json` is the single source of truth (SDK, frontend, judge_demo all read it).

## 2. Functional walkthrough

| # | Action | Expected evidence | Boundary |
|---|---|---|---|
| 1 | Open `https://casperprover.xyz/lab/contracts` | All eight deployed contracts + explorer links | **ON-CHAIN** |
| 2 | Run `python3 scripts/judge_demo.py` | Contract queries + API/frontend checks pass | **ON-CHAIN / LIVE SERVICE** |
| 3 | Open `/lab/zk-proofs`, prove preimage `42`, then verify | Valid gnark BN254/MiMC Groth16 proof | **REAL CRYPTO, OFF-CHAIN** |
| 4 | Flip one byte of `proof_hex`, verify again | Verification fails | **NEGATIVE SECURITY TEST** |
| 5 | Open `/lab/pq-crypto`, hybrid-sign, verify | Ed25519 + ML-DSA-65 both valid | **REAL CRYPTO, OFF-CHAIN** |
| 6 | Change message, verify again | Signature verification fails | **NEGATIVE SECURITY TEST** |
| 7 | Open `/lab/proofs`, create+verify a proof | Deterministic hash/Merkle record; optional wallet anchor | **ENGINE; ON-CHAIN ONLY WHEN TX HASH EXISTS** |
| 8 | Open `/lab/playground` | Full API surface with request/response and errors | **DEVELOPER UX** |
| 9 | Run `./verify.sh` | Independent smoke suite passes | **REGRESSION EVIDENCE** |

## 3. Eight-criteria map

| # | Criterion | Where to look | Evidence |
|---|---|---|---|
| 1 | **Technical execution** | `scripts/judge_demo.py`, `./verify.sh`, contracts in `contracts/*/src/main.rs` | 9/9 contracts deployed on testnet, verify.sh 9/9 pass rate, gitleaks clean, no `unsafe` in engine, Rust nightly MVP-clean WASM (sign-ext lowered, mutable-globals off) |
| 2 | **Innovation / originality** | `docs/architecture.md`, `docs/originality.md`, `contracts/proof-aggregation/`, `contracts/proof-of-inference/` | Merkle-anchored agent-decision attestations on Casper; hybrid PQ signatures (Ed25519 + ML-DSA-65); batched proof aggregation; governance timelock with 2-of-3 guardian recovery |
| 3 | **Casper Network fit** | `deploy-out/onchain.json`, `deploy/scripts/*.sh`, `sdk/casper-native/` | Purse-backed staking, NamedKey addressing, contract packages (versioned), native `casper_client` deploys, no external L1 dependency |
| 4 | **Real-world use case** | `docs/use-cases.md`, `/lab/defi-mock` frontend, `contracts/defi-mock/` | Regulated lending gate (KYC + on-chain whitelist), healthcare provenance path, agent audit for HITL policy |
| 5 | **Business model** | `docs/business-model.md`, `docs/pricing.md`, `docs/MAINNET_LAUNCH_PLAN.md` | SDK subscription tier, per-proof anchor fee, enterprise governance seat pricing, launch plan documented |
| 6 | **Security & honest claims** | `README.md` badges, `docs/threat-model.md`, `docs/hackathon/CP_STRICT_MODE.md` | REAL / ON-CHAIN / SIMULATION labels on every surface; no "on-chain Groth16", no "ZK proof of ML inference"; reentrancy tests, invariant tests, gitleaks in CI |
| 7 | **Documentation & DX** | `README.md`, `docs/quickstart.md`, `sdk/*/examples/`, `docs/JUDGE_GUIDE.md` | One-command demo, Go SDK quickstart, MCP server quickstart, architecture doc, data-room `/data-room` |
| 8 | **Presentation** | `README.md`, submission video (link in DoraHacks), `docs/pitch.md` | Video walkthrough of `/lab/*`, honest scope in `CP_STRICT_MODE.md`, live deployed frontend `casperprover.xyz` |

## 4. Claim boundary

| Label | What CasperProver actually does |
|---|---|
| **REAL CRYPTO** | gnark/BN254 Groth16 for a MiMC preimage circuit; ML-DSA-65 + Ed25519 hybrid signatures; Lamport OTS education path |
| **ON-CHAIN** | Eight Casper testnet contracts store/validate proof metadata, hashes, access state, stake/slashing state, model provenance, inference attestations, and governance timelock |
| **SIMULATION** | Legacy conceptual Groth16/STARK-style hash flows, kept for comparison and explicitly labeled in UI + code |

CasperProver does **not** claim a Casper-native pairing verifier or a ZK proof of arbitrary ML inference. The current real Groth16 circuit proves knowledge of a MiMC preimage.

## 5. Governance contract — full disclosure

- **Canonical:** `38d2fbd24998719fac160c27e2e5435a99bcdebd4c36beac76abe84063a0cf3e` (deployer: `anna-stolbovskaja`, tx `5f20ecfe…4bdf60a2`, block `8635118`)
- **Guardians (2-of-3 for recovery, 48h timelock for parameter changes):**
  - `guardian_1` = `cac1862d…d298` (anna-stolbovskaja, contract owner bootstrap)
  - `guardian_2` = `84ff0a46…be10` (defi_mock_owner, second live signer)
  - `guardian_3` = `0000…0000` **RESERVED PLACEHOLDER** — populated at mainnet key ceremony per `docs/MAINNET_LAUNCH_PLAN.md`. On testnet this means the 2-of-3 threshold is currently satisfiable by anna+defi_mock_owner alone; documented rather than hidden.
- **Prior deployment** `03189ea1…548e` (under defi_mock_owner) is deprecated but remains on-chain (Casper testnet cannot delete contract packages). Not referenced by SDK/frontend/verify.

## 6. Failure behavior

A failed check exits non-zero and prints a bounded HTTP/network error without secrets. `judge_demo.py` never retries mutating calls, never invents a transaction hash, and skips write-based crypto checks unless a judge key is explicitly supplied. `verify.sh` runs read-only.

## 7. Key artifacts for reviewers

- **Reproducible demo:** `scripts/judge_demo.py`, `verify.sh`
- **On-chain manifest:** `deploy-out/onchain.json`
- **Frontend:** `https://casperprover.xyz` (routes `/lab/contracts`, `/lab/zk-proofs`, `/lab/pq-crypto`, `/lab/decisions`, `/lab/playground`)
- **API health:** `https://casperprover-api-ylsh.onrender.com/health`
- **SDK entry:** `sdk/go/README.md`, `sdk/mcp/README.md`
- **Architecture:** `docs/architecture.md`
- **Honesty gates:** `docs/hackathon/CP_STRICT_MODE.md`, README claim badges
- **Post-hackathon plan:** `docs/MAINNET_LAUNCH_PLAN.md`
