# Post-Quantum & Research Claims — Honesty Audit

**Scope:** cryptographic claims made anywhere in the CasperProver repo — README, frontend, engine, SDK, docs. This document is the reference for what the codebase _actually_ delivers versus what remains research/roadmap. Any contributor adding a new PQ / quantum / ZK claim MUST reconcile it with this file first.

Authoritative for §F of `handoff/CP_FINAL_TASKS_V2.md`.

## Table of contents
1. [What is real today](#what-is-real-today)
2. [What is educational-only](#what-is-educational-only)
3. [What is banned wording](#what-is-banned-wording)
4. [SHA-256 → SHAKE-256 disclaimer](#sha-256--shake-256-disclaimer)
5. [Simulated annealing / "quantum-inspired" disclaimer](#simulated-annealing--quantum-inspired-disclaimer)
6. [Recursive ZK, folding, VRF, on-chain pairings](#recursive-zk-folding-vrf-on-chain-pairings)
7. [Patents / prior art / FTO disclaimer](#patents--prior-art--fto-disclaimer)
8. [Contributor checklist](#contributor-checklist)

---

## What is real today

| Component | Standard | Library | Notes |
|-----------|----------|---------|-------|
| ML-DSA-65 signing | FIPS 204 (August 2024) | `github.com/cloudflare/circl` | NIST-standardised lattice signature. Real cryptographic security under LWE/SIS hardness assumptions. |
| Hybrid Ed25519 + ML-DSA-65 | draft-ietf-hybrid-signatures (transitional) | `circl` + `crypto/ed25519` | Provides `Ed25519 AND ML-DSA` — an attacker must break both to forge. Compatible transition path. |
| Lamport one-time signature | Lamport 1979 | in-tree | Hash-based OTS. Genuinely PQ, but each keypair signs exactly one message. |
| BN254 Groth16 (off-chain) | Groth 2016 | `github.com/consensys/gnark` | Real Groth16 over MiMC preimage. Not "ZK proof of ML inference" — the circuit encodes only the MiMC relation. |
| PQ keyring rotation + versioning | in-tree | `internal/crypto/keyring.go` | Monotone versions per algo, retired keys stay verify-only, migrate primitive. Private keys in process memory only by default — see keystore row. |
| Keystore backends | in-tree | `internal/crypto/keystore/` | `memory` (default), `file` (ChaCha20-Poly1305 + Argon2id at rest), `remote` HSM/KMS gateway stub with a documented HTTP contract. A real HSM driver lives per-deployment, not in this repo. |

## What is educational-only

- **Lamport OTS as a general-purpose signature.** In this repo it demonstrates hash-based PQ signatures. It is NOT SPHINCS+ / SLH-DSA. Where "SPHINCS+ family" appears in the UI or docs, it means "the hash-based slot"; the code path is Lamport, not SPHINCS+. A production deployment must swap Lamport for SLH-DSA (FIPS 205) when a mature Go implementation ships.
- **Conceptual Groth16 verifier** (`/zk/verify-groth16` non-real endpoint). Simulation for rapid testing. The real path is `/zk/groth16-real/*`.
- **`ProofSystemSpec.tla`** — TLC-checked on a small model. Not "formal verification of the pipeline"; small-model model-checking is a partial invariant check.

## What is banned wording

The following claims MUST NOT appear in README, marketing copy, UI text, comments, or commit messages unless the referenced deliverable is genuinely present and verifiable. If any grep finds them again, treat as a regression and open a follow-up.

- ❌ "on-chain Groth16 verifier" / "on-chain pairing verification"
- ❌ "ZK proof of ML inference" / "proved ML inference"
- ❌ "quantum speedup" / "quantum-inspired" (as a performance claim)
- ❌ "quantum-resistant SHA-256" / "SHAKE-256 is post-quantum"
- ❌ "formal verification completed" (small-model TLC is not full verification)
- ❌ "audited by Halborn" / "Allium dashboard integrated" / "NowNodes integrated" (unless the vendor has publicly confirmed the specific integration/audit and the report/dashboard is linked)
- ❌ "recursive ZK aggregation" / "STARK aggregation" (as a shipped feature)
- ❌ "future-proof" (unqualified — always pair with the specific scheme and its assumption set)

Existing sentences that are close-to-banned but currently accurate — e.g. "Post-Quantum Ready" on the Features tile — are acceptable ONLY because the tile enumerates the exact primitives underneath. Removing the enumeration turns the tile into an overclaim.

## SHA-256 → SHAKE-256 disclaimer

If a future PR proposes swapping SHA-256 for SHAKE-256 (same 256-bit output) and marketing it as a PQ uplift:

- **Do not accept the framing.** Grover's algorithm gives at best a quadratic speedup against generic hash preimage/collision search. To retain ≥128-bit collision resistance against a quantum adversary the output must be ≥256 bits, and to retain ≥128-bit *preimage* resistance under Grover the output must be ≥256 bits. **SHA-256 already meets that bar; a same-output-length SHAKE-256 does not add PQ security.**
- A meaningful PQ uplift requires a *parameter change* — e.g. SHAKE-256 at 512-bit output, or moving to a scheme whose security does not reduce to hash preimage/collision at all.
- Any commit that swaps hash functions MUST cite the parameter analysis and the target security level (classical + quantum) in the message.

## Simulated annealing / "quantum-inspired" disclaimer

Quantum-inspired simulated annealing (QISA) is a **classical heuristic**. If any future PR proposes shipping QISA:

- Required: seeded benchmark against random baseline, greedy baseline, and standard SA. Numbers reported per random seed, not cherry-picked.
- Forbidden: any claim of "quantum speedup", "near-quantum performance", or "post-classical" for QISA. It is not quantum, it is inspired by quantum annealing dynamics.
- Preferred wording: "classical annealing heuristic modelled after quantum annealing dynamics."

## Recursive ZK, folding, VRF, on-chain pairings

All of the following are **roadmap**, not shipped:

- Nova / SuperNova / folding schemes for proof aggregation (a **harness** with a hash-chain stand-in labelled `hash-fold-v1` ships in `internal/aggregator/nova.go` — see `docs/roadmap/NOVA_HARNESS.md`. It is explicitly NOT a folding scheme; it stabilises the API contract while the Pallas/Vesta curve cycle in Go matures.)
- Recursive Groth16 / Halo-2-style accumulation
- Full bisection games (interactive verification) beyond the current single-shot challenge/slash path
- VRF-based sortition for verifier committees
- Range proofs (Bulletproofs / Bulletproofs++)
- On-chain pairing verification on Casper (blocked by the lack of a mature BN254/BLS12-381 pairing precompile in Casper Condor 2.x)

Any PR shipping one of these MUST update this file's "What is real today" table and remove the corresponding row from the roadmap.

## Patents / prior art / FTO disclaimer

Publications, prior art, and academic literature cited in this repo are **inspiration and prior art**, not evidence of patentability or freedom-to-operate.

- Citing a paper does NOT imply we own or have licensed the technique.
- Before any commercialisation step (SaaS launch, exclusive licensing, patent filing), a professional patent landscape / FTO review by qualified counsel is required.
- The repo's Apache-2.0 licence covers our code; it does not override any third-party patent claims.

## Contributor checklist

Before opening a PR that touches PQ / ZK / cryptographic claims:

1. Grep the diff for any bullet in the [banned wording](#what-is-banned-wording) list.
2. If a claim is new, add or update the row in [What is real today](#what-is-real-today).
3. If a claim is aspirational, add or update the row in the appropriate roadmap doc under `docs/roadmap/`.
4. Reviewers: reject any PR whose commit message or diff contains banned wording without an accompanying parameter/assumption analysis.

_Last audit: 2026-07-26 — matches HEAD `edb94e3` on branch `fix/mass-runner-errors`._
