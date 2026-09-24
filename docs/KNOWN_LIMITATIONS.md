# Status & Roadmap

> Current state as of 2026-07-26. All systems live on Casper testnet.

## ✅ What's Live

### Smart Contracts (8 on testnet — see `docs/TX_MANIFEST.md` for hashes)
- **proof-registry** — immutable proof store with on-chain anchoring
- **verifier-gate** — Merkle inclusion proof checker, cross-contract verification
- **defi-mock** — KYC-gated DeFi vault with admin-controlled whitelisting
- **stake-slashing** — economic penalty system for revoked/invalid proofs
- **proof-of-inference** — on-chain verification of ZK inference proofs (deployed 2026-07-25)
- **model-registry** — canonical model-hash registry with owner controls (deployed 2026-07-25)
- **proof-aggregation** — batch anchor of aggregated proof roots (deployed 2026-07-25)
- **governance** — timelock (48h) + 2-of-3 guardian recovery (anna-stolbovskaja / defi_mock_owner / reserved slot for mainnet ceremony), downstream contracts opt in via `is_executed(pid)`. Canonical deploy `38d2fbd2...84063a0cf3e` under anna-stolbovskaja (deployed 2026-07-26); a prior install `03189ea1...548e` under defi_mock_owner is deprecated and not referenced anywhere live.

All contract WASM is built in CI (`.github/workflows/check.yml →
build-contracts`) with a size gate: `scripts/contract-size-report.sh`
fails CI if any `.wasm` exceeds 200KB (hard gas ceiling); a warning
fires above 65KB (historical `installOrUpgrade` limit under
casper-js-sdk 5.0.12). The last three contracts above needed an
MVP-clean WASM toolchain fix (Rust nightly `-Z build-std`, disabled
bulk-memory-opt/sign-ext/reference-types, `wasm-opt
--signext-lowering`, stripped `target_features`) to pass that gate and
deploy cleanly — see `docs/UNDEPLOYED_CONTRACTS.md` for the historical
record of that fix.

### Proof Engine (32 API endpoints)
- SHA-256 Merkle proof generation & verification (<50ms)
- **Real BN254 Groth16 ZK proofs via gnark (`/zk/groth16-real/*`)** — the primary ZK path. Real R1CS circuit, real (session-local) trusted setup, real pairing-based verification
- `[sim]` Conceptual hash-based Groth16 flow (`/zk/verify-groth16-sim`, alias `/zk/verify-groth16`; batch `/zk/batch-verify-sim`, alias `/zk/batch-verify`) — kept for legacy demo/comparison, NOT real BN254 pairing math. Responses carry `simulation:true, deprecated:true, use:"/zk/groth16-real/verify"` plus `Warning`/`Deprecation`/`Sunset` headers
- Post-quantum signing: SPHINCS+ (NIST PQC), hybrid Ed25519 + ML-DSA-65 (FIPS 204)
- Hash-chain aggregation with Postgres persistence
- Proof-chain DAG validation (Phase 2: cycle detection, input continuity)
- KYC whitelist with database persistence
- API key authentication on mutating endpoints
- Rate limiting (60 req/min) and input validation

### SDK & MCP
- Go SDK: 32 methods, 1:1 mapping to all API endpoints
- MCP server: 32 tools with full InputSchema definitions
- All tools backed by real API — no stubs

### Frontend (11 interactive tabs)
- Overview, Proofs, Models, Aggregation, ZK Proofs, PQ Crypto, Contracts, Agent Demo, Playground, KYC

## Legal & policy (drafts)

Draft, self-authored, not-counsel-reviewed policy documents live under
`LEGAL/`:

- `LEGAL/TOS.md` — Terms of Service draft (testnet-only status, warranty
  disclaimer, indemnity).
- `LEGAL/AUP.md` — Acceptable Use Policy draft (encouraged uses,
  prohibited uses, enforcement).
- `LEGAL/DATA_PROTECTION.md` — GDPR-adjacent data protection notice
  draft (hash-only architectural boundary, retention schedule,
  data-subject rights template).

All three are labelled **DRAFT — not reviewed by counsel** and will be
replaced with counsel-reviewed versions before commercial launch
(`docs/MAINNET_LAUNCH_PLAN.md`, Pack AK).

## 🔜 Roadmap

### Operations (added by Pack AH)

- Ops runbook shipped as `docs/OPS_RUNBOOKS.md` — REAL playbook but DRAFT until it is exercised in an actual incident. Blue/green playbook is validated locally via `scripts/lb_flip_test.sh` (5 offline tests) but has not yet been exercised on a real cloud load balancer.
- SLO alert rules under `deploy/observability/alerts/` are `promtool check`-clean and have 4 `promtool test rules` unit-tests. They are NOT wired to a paid pager — Alertmanager routes to a null receiver until MAINNET_LAUNCH_PLAN.md unlocks paid on-call.
- No on-call rotation. Single-maintainer mode until investment.

### Near-term
- Real Groth16 circuit for full model-inference verification (current: MiMC preimage)
- Production trusted setup ceremony (current: per-start generation) — see `docs/KEY_CEREMONY_PLAN.md`
- HSM-resident anchor and attestation signing — see `docs/HSM_PLAN.md`
- Multi-party (public) Groth16 ceremony (current: real gnark Phase 1 + Phase 2 with single-coordinator N-contribution chain — see `zk/ceremony/README.md` for the honesty label and multi-party upgrade path; upgrade requires only running `Contribute()` on independent machines, no code change)
- Deploy `proof-of-inference`, `model-registry`, `proof-aggregation` contracts
- Demo/Real data toggle in lab

### Observability (2026-07)
- Local `/metrics` + Grafana stack shipped in `deploy/observability/`.
- **In-process** metrics + tracing library is a zero-dependency,
  purpose-built impl (Prometheus 0.0.4 text exposition; OTel-shaped NDJSON
  spans). Not `prometheus/client_golang`, no OTLP/gRPC exporter.
  Upgrade path is call-site-preserving (see `docs/OBSERVABILITY.md`).

### Medium-term
- STARK recursive aggregation (pending mature Go library; winterfell is Rust-only)
- Multi-chain anchoring (EVM, Solana)
- Hardware attestation (TPM 2.0 / SGX / SEV — interfaces defined in Phase 2)
- Distributed prover network with MPC threshold support

### Long-term
- Full-circuit ZK proofs for arbitrary ML models
- Formal verification of proof pipeline (current TLC pass is *small-model* model-checking, not a full verification)
- Mainnet deployment with gas optimization

## ⚠️ Cryptographic honesty audit

Before claiming anything PQ / ZK / quantum related, read `docs/PQ_HONESTY.md`. In particular:

- SHA-256 → SHAKE-256 with the same output length is **not** an automatic PQ uplift. Grover gives at most a quadratic speedup against generic preimage/collision search; SHA-256 already meets ≥128-bit post-quantum resistance for its output length.
- "Quantum-inspired simulated annealing" is a classical heuristic. Never claim "quantum speedup" for QISA.
- Recursive ZK / folding / on-chain pairings / VRF sortition / range proofs are **roadmap**, not shipped.
- Citing a patent or academic publication is prior-art context, not evidence of patentability or freedom-to-operate. A professional FTO review is required before any commercialisation step.

### Off-repo design plans (DRAFT — not counsel-reviewed, no code shipped)
- `docs/CONFIDENTIAL_STORAGE.md` — optional per-Operator confidential
  payload store, sits **beside** the Service (never in the hot path);
  hash-only boundary preserved for Operators who do not opt in.
- `docs/REPUTATION_ECONOMICS.md` — economic model on top of the
  existing `stake-slashing` structural stub; bonds, challenges,
  Adjudicator quorum, Governance boundary. All `SIMULATION` until
  Gate G7 of `docs/MAINNET_LAUNCH_PLAN.md`.
## Hash primitive posture

`docs/HASH_ALGORITHM_ANALYSIS.md` catalogues every hash usage in the tree with
its security property, honesty label, post-quantum posture, and migration
urgency. Key open questions routed from that document (do not close without
reviewing them):

- **Q1 domain-sep prefixes** — confirm Merkle internal nodes and receipt hashes
  use distinct domain-separation prefixes. Missing prefix = real defect.
- **Q2 canonical serialisation** — confirm receipt canonical serialisation
  carries version + purpose tags before payload.
- **Q3 SHA-256 monoculture** — acceptable under current review; any diversity
  change deferred to G2.
- **Q4 SLH-DSA parameter set** — production target (`SHA2-192s` vs
  `SHA2-256s`) deferred to G2.
- **Q5 Poseidon-family immaturity** — any STARK-based ZK-ML prototype stays
  SIMULATION until independent cryptanalytic review.
- **Q6 HKDF info labels** — every KDF call site must be catalogued with its
  label. Missing labels = defect.
- **Q7 length-extension surface** — confirm no `H(key || message)` MAC
  substitute exists anywhere in the tree.
## Integrations roadmap
`docs/INTEGRATIONS_ROADMAP.md` classifies integration surfaces by category
(SDK-side, sinks, sources, chains, verifier-side, standards conformance),
labels each honestly (REAL / SIMULATION), and gates each on the correct
mainnet-plan gate. Bridge-based cross-chain anchoring is **permanently
rejected** in the current design (bridges are the least-audited surface in
the ecosystem; adopting one imports its risk profile wholesale). Multi-chain
is reachable only via §5.2 per-chain writes with per-chain G2 audit.
---
## Mainnet — gated, not scheduled
Any mainnet surface is gated by the eight-gate ledger in
[`docs/MAINNET_LAUNCH_PLAN.md`](./MAINNET_LAUNCH_PLAN.md). Until every
gate is closed and the launch review in Gate 8 is signed, the current
system remains **testnet-only** and the honesty labels
(`REAL / ON-CHAIN / SIMULATION`) must not be changed to imply a live
mainnet surface. That plan is a map, not a schedule — no dates, no
vendors, no spend are authorized by it.
## Metadata privacy posture
`docs/METADATA_PRIVACY.md` catalogues seven metadata classes (ingress, timing,
chain-side, verifier-side, observability, confidential-storage access,
external-observer) with per-class mitigation ladder priced honestly.
**Payload privacy ≠ metadata privacy.** The hash-only architectural boundary
from `LEGAL/DATA_PROTECTION.md` solves *what* the Service sees; it does not
solve *how much can be inferred from the fact that the Service saw anything*.
Anchor addresses on Casper Network are pseudonyms, not anonymity primitives;
traffic patterns leak business rhythm; verifier-side calls fingerprint the
Verifier. Any commit that promises "anonymous attestation" or "unlinkable
proofs" without a hard REAL/SIMULATION qualifier is a defect.
## SDK versioning policy
`docs/SDK_VERSIONING.md` specifies the SemVer contract for API, receipt
schema, and SDKs. Highlights:
- **Receipt schemas support forever**. A receipt signed under `cp:receipt:v1`
  must remain verifiable after `v2` ships. Non-negotiable.
- **Honesty-ladder downgrade** (`REAL` → `SIMULATION`) is ALWAYS a MAJOR bump
  and requires a `LEGAL/TOS.md` amendment. Forbidden as a silent change.
- **No public publish until G6** (ops readiness) unless tagged `-alpha`/`-beta`
  and documented as SIMULATION.
- **Deprecation flow**: 6–12 months notice + WARNING headers + changelog
  entry before deletion. Deletion of a receipt schema is forbidden.
## `slash_equivocation` entrypoint (spec, non-deployed)
`contracts/stake-slashing/SLASH_EQUIVOCATION_DRAFT.md` specifies the
minimum-viable equivocation-slashing extension to the existing `stake-slashing`
contract. Ships no code and authorises no redeploy — per invariant "не
редеплоим до аудита" a live contract change requires G2 sign-off.
Highlights: two conflicting proofs (same input_hash + model_id, different
output_hash) are self-witnessing evidence; entrypoint is permissionless
(anyone can call); slash is 50% of current stake (stricter than the 20% of
`report_and_slash`, because equivocation is the strictest failure mode);
composite key `min(a,b)|max(a,b)` prevents order-swap replay; optional
`evidence_hash` lane is reserved for future dispute-resolver, NOT in MVP.
## Off-repo design plans (`SIMULATION` / DRAFT)
These plans ship no code and change no runtime behaviour. They exist so the
honesty ladder is discoverable from the front door.
- `docs/ZKML_RESEARCH_SPIKE.md` — full landscape survey and feasibility matrix
  for ZK-ML proving approaches (Groth16/PLONK-family, STARK/FRI, zkVM,
  lookup+PLONK, recursion). Non-endorsement, non-procurement.
- `docs/ZKML_HONEST_VERDICT.md` — single-page decision record explaining why
  every ML-inference claim in the tree is currently labelled `SIMULATION` and
  what four conditions would all have to hold before any of them could
  honestly be relabelled `REAL (ZK-ML)`. Bound to G2 in the mainnet launch
  plan.
### `SIMULATION` label — not negotiable until conditions met
Any claim that implies a cryptographic proof of a model's inference (as
opposed to an attestation of inputs, outputs, and a model identifier) is
labelled `SIMULATION`. Relabelling to `REAL (ZK-ML)` requires ALL of:
1. A named model with a compiled circuit and both **weights hash** and
   **circuit hash** published and anchored.
2. Independent third-party audit sign-off on both the circuit and the
   underlying IOP/lookup argument (reserved for mainnet-plan gate G2).
3. Per-inference proving cost that fits under the Challenger dispute cost
   ceiling from the reputation economics draft.
4. Receipt-format extension carrying circuit hash, verifying-key hash,
   weights hash, and toolchain version (breaking schema change; must be
   scheduled as such).
Skipping any one turns `REAL` into laundered `SIMULATION`. Explicitly rejected.