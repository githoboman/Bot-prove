# How to Judge CasperProver — Casper Hackathon 2026

> A **2-minute** path for judges. If you have 20 minutes, follow the
> deep-dive links; if you have 2, the *Fast path* is what you need.

## Fast path (2 minutes)

1. Open the live console: **https://casperprover.xyz**
2. Click **Lab → ZK Proofs**.
   - Prove knowledge of preimage `42`. Real gnark BN254/MiMC Groth16.
   - Verify. Then change one byte and verify again → fails. That's
     real cryptography, not simulation.
3. Open **Lab → Contracts**. Four deployed testnet contracts with
   hashes; every hash links to `testnet.cspr.live`.

That's it. Everything below is depth on how each of the 8 judging
criteria is satisfied.

## 8-criteria map — Casper Hackathon 2026

| # | Criterion | Where to look |
|---|-----------|---------------|
| 1 | **Casper-native architecture** | `contracts/` — 9 deployed Rust WASM contracts on testnet (proof-registry, verifier-gate, defi-mock, stake-slashing, proof-of-inference, model-registry, proof-aggregation, governance, zk-verifier). Cross-contract Merkle-inclusion verification. Contract hashes at `/lab/contracts` and in `README.md`. |
| 2 | **Working demo on testnet** | `/lab/zk-proofs` — real gnark BN254/MiMC Groth16 prove + verify. `/lab/proofs` — deterministic proof engine with optional on-chain anchoring. 248+ testnet txs to date across all 9 contracts. |
| 3 | **Technical correctness** | Engine tests: `engine/zk_gate4_test.go` (real Groth16 round-trip + sim banner regression). Contract tests: `contracts/tests/` including 28 tests for the proof-aggregation guard suite. Independent smoke suite: `./verify.sh`. |
| 4 | **Novelty / originality** | Real gnark BN254/MiMC Groth16 as the primary ZK path (hash-based conceptual endpoints kept only as `[sim]` for comparison). Hybrid Ed25519 + ML-DSA-65 (FIPS 204) signatures. Merkle-inclusion cross-contract verification. Stake-slashing economic-security layer on-chain. |
| 5 | **Documentation quality** | `README.md`, `docs/ARCHITECTURE.md`, `docs/JUDGE_GUIDE.md`, `docs/KNOWN_LIMITATIONS.md`, `docs/SDK.md`, `docs/TX_MANIFEST.md`. |
| 6 | **Security posture** | `docs/RED_TEAM.tmp`; `docs/SECURITY_AUDIT.md` (owner/admin/reentrancy audit); create_batch revert guards (empty batch_id / zero max_proofs / duplicate batch_id) — Gate 1 P1. Rate limiting, API key auth on mutating endpoints, KYC whitelist with DB persistence. |
| 7 | **Developer experience** | Go SDK — 32 methods, 1:1 with API. MCP server — 32 tools with full input schemas. `/lab/playground` — every API operation with request/response and errors visible. `scripts/judge_demo.py` — one-command read-only replay. |
| 8 | **Business viability** | ZK-as-a-service for proof anchoring + slashing-backed reputation. Target ICP: AI providers who need verifiable inference receipts + auditors who need proof-of-training-data provenance. Full model in `docs/KNOWN_LIMITATIONS.md` (roadmap section). |

## Real vs sim — what's on-chain, what's in-process

| Component | Status | Notes |
|-----------|--------|-------|
| 4 Casper testnet contracts (proof-registry, verifier-gate, defi-mock, stake-slashing) | **Real** (on-chain) | Hashes in README + `/lab/contracts`. |
| Cross-contract Merkle inclusion verification | **Real** (on-chain) | verifier-gate consumes proof-registry state. |
| Stake slashing on revoked/invalid proofs | **Real** (on-chain) | Hardened redeploy 2026-07-19. |
| Real gnark BN254 Groth16 (MiMC preimage) | **Real** (off-chain crypto) | `/zk/groth16-real/*` — primary ZK path. |
| Hybrid Ed25519 + ML-DSA-65 signatures | **Real** (off-chain crypto) | FIPS 204. |
| SPHINCS+ post-quantum signing | **Real** (off-chain crypto) | NIST PQC finalist. |
| Lamport OTS (education path) | **Real** (off-chain crypto) | For teaching, not production. |
| Legacy hash-based Groth16-style endpoints (`/zk/verify-groth16`, `/zk/batch-verify`) | **Sim** | Every response carries `simulation: true`, `deprecated: true`, `use: "/zk/groth16-real/verify"`, plus `Warning` / `Deprecation` / `Sunset` HTTP headers. Kept only for comparison; deletion path in roadmap. |
| Full-circuit ZK proof of arbitrary ML inference | **Not yet** | Explicit roadmap item; do not claim. Current: MiMC preimage. |
| Production trusted setup ceremony | **Not yet** | Current: per-start generation. Roadmap item. |

## One-command evidence

```bash
python3 scripts/judge_demo.py
```

Default is **read-only**: queries all four testnet contracts, API
health/proofs, and the frontend. For the real Groth16 round-trip on
the mutating endpoints, obtain the temporary judge key from the
submission notes and run:

```bash
CP_JUDGE_API_KEY='provided-out-of-band' python3 scripts/judge_demo.py
```

The key is never embedded in the script, URL, repository, or shell
history examples. Revoke it after judging.

## Functional walkthrough (existing)

See `docs/JUDGE_GUIDE.md` for the 9-step table (contracts → ZK proofs
→ negative security tests → PQ crypto → proof engine → playground →
verify.sh). This document is the higher-level 2-minute path; that one
is the deeper walkthrough.

## Failure behaviour

A failed check exits non-zero and prints a bounded HTTP/network error
**without secrets**. The script never retries mutating calls, never
invents a transaction hash, and skips write-based crypto checks unless
a judge key is explicitly supplied.

## Originality (2-line pitch per angle)

- **Real Groth16 as the primary ZK path.** Hash-based conceptual
  endpoints are labeled `[sim]` at every layer (response body, HTTP
  headers, docs); the real gnark BN254/MiMC circuit is the first-class
  citizen.
- **Cryptography-per-layer honesty.** Every claim carries its own
  boundary label — `REAL CRYPTO`, `ON-CHAIN`, `SIMULATION`. Nothing
  is left implied.
- **Economic-security layer on-chain.** The stake-slashing contract
  ties reputation to real testnet stake; revoked proofs cost the
  proof-submitter, not just a database row.

## Limitations we admit up front

- The real Groth16 circuit proves **knowledge of a MiMC preimage**,
  not arbitrary ML inference. Full-circuit inference is a stated
  roadmap item.
- **Per-start** trusted setup; production would use an MPC ceremony.
- Live deployment targets **testnet**. Mainnet migration is a
  separate hardening pass.

## Regulatory posture (30-second read)

CasperProver is a **compliance enabler**, not a regulated entity. It
produces ZK-proofs and Merkle-committed evidence for downstream
systems that ARE regulated. Full analysis in `docs/COMPLIANCE.md`.

- **EU MiCA** — out of CASP scope. CP does not custody assets, does
  not exchange crypto-assets, does not operate a trading platform.
  It is a utility layer under Art. 3 that produces cryptographic
  attestations. Not a Crypto-Asset Service Provider.
- **EU AI Act** — CP is **not itself an AI system**. It is a
  compliance enabler for high-risk AI deployers (e.g. AgentEscrow402
  operating under Annex III 8(a) ADR classification): ZK-proofs
  provide Art. 12/13/14 evidence for logging, transparency and
  human-oversight without exposing raw prompts or business secrets.
- **GDPR** — ZK-proofs are the canonical **Art. 5(1)(c) data
  minimisation** mechanism. Verifiers learn a statement is true
  without seeing the underlying personal data; Merkle roots commit
  to a set without publishing its members.
- **US FinCEN (2019 guidance)** — CP does not transmit value.
  It publishes hashes and proofs. Not a money transmitter under
  31 CFR § 1010.100(ff)(5).
- **NIST AI RMF 1.0 + GenAI Profile (July 2024)** — CP maps to
  Govern/Map/Measure/Manage as an evidence-production layer for
  downstream deployers. Concrete subcategory mapping is in
  `docs/COMPLIANCE.md`.

Honest gap-list is in `docs/COMPLIANCE.md` (formal legal opinion
per commercial deployer, salting guidance for low-entropy pre-images,
per-jurisdiction deployer runbooks).

## Anti-linking pass — this project is CasperProver

CasperProver and AgentEscrow402 are **independent submissions** by
different owners. They share cryptographic primitives (Merkle
provenance math) because both are open-source and portable across
languages, not because one is a fork of the other. No shared
wallets, no shared branding, no shared demo story.

## Contact

- Repo: https://github.com/anna-stolbovskaja/CasperProver
- Live: https://casperprover.xyz
- SDK: Go module (see `sdk/`) + MCP server (32 tools)
