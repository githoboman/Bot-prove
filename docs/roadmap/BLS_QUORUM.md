# Threshold BLS Quorum Registry — Design

> **Status:** PARTIALLY SHIPPED. Off-chain layer (`engine/internal/quorum/` + `/v1/quorum/*` API) is live — see `docs/BLS_QUORUM.md` for the honest contract of what shipped. On-chain contract, DKG-based TSS, and rogue-key PoP challenge protocol remain planning-doc scope.

Ref: `handoff/CP_FINAL_TASKS_V2.md` §D.

## Problem

The current `verifier-gate` contract validates a single signer's Merkle
inclusion proof. A production evaluator committee needs threshold quorum
(k-of-n) with aggregate signatures, so that the on-chain gate can verify
_the committee_ rather than a designated signer.

## Design overview

Use BLS12-381 aggregate signatures with a threshold t = ⌈2n/3⌉ + 1 for
Byzantine-tolerant quorum (matches the aggregator design in
`engine/internal/decision/`).

```
   ┌─────────────┐   ┌─────────────────┐   ┌──────────────────┐
   │ Committee   │──▶│ SignerRegistry  │──▶│ verifier-gate    │
   │ members (n) │   │ (on-chain)      │   │ (on-chain)       │
   └─────────────┘   └─────────────────┘   └──────────────────┘
        │                    │                      │
        ▼                    ▼                      ▼
   BLS keypair          registration        aggregate signature
                        + slashing bond     verified against
                                            aggregated pubkey
```

## On-chain surface

New contract `signer-registry` with entrypoints:

- `register_signer(bls_pubkey, bond)` — adds a signer, escrows a bond.
- `remove_signer(signer_id)` — admin-only; releases bond after cooldown.
- `rotate_key(signer_id, new_pubkey, sig_over_new_pubkey_by_old_key)` —
  online key rotation; the signature proves the rotation is authorised.
- `slash_signer(signer_id, evidence_hash, severity)` — admin-only (or
  challenge-gated); burns a fraction of the bond and marks the signer as
  slashed.

Extend `verifier-gate` with entrypoint:

- `verify_quorum(evidence_root, aggregate_sig, signer_bitset)` —
  reconstructs the aggregate pubkey from `signer_bitset ∩ active
  signers`, checks `t = ⌈2n/3⌉ + 1`, verifies the pairing off-chain via a
  Casper-friendly path (see [Verification path](#verification-path)) and
  writes the resulting commitment.

## Verification path

Casper Condor 2.x has no BLS12-381 pairing precompile. Two paths:

1. **Off-chain verifier + on-chain commitment.** The engine verifies the
   aggregate signature with `github.com/consensys/gnark-crypto/ecc/bls12-381`
   and writes `{aggregate_hash, signer_bitset, verdict}` on-chain. The
   contract enforces that the committing key is the registry admin, not
   the signers themselves.
2. **BLS-signature-over-Groth16 (out-of-scope for the 30-day slice).**
   Generate a Groth16 proof that "there exists a valid k-of-n BLS
   aggregate over this evidence root", and let the existing off-chain
   Groth16 pipeline verify it. Documented for the 90-day slice.

Path 1 is the 30-day target. It trades pure trustlessness for a small
trust root (the registry admin key) with slashing accountability, and
it is honest about that trade in `docs/PQ_HONESTY.md` terms.

## Slashing economics

- Bond size: TBD; parameterise per-signer.
- Slash triggers:
  - Equivocation (same-signer conflicting verdicts on the same evidence).
  - Provable unavailability (missed quorum participation over N rounds).
  - Admin-issued severity level (0..3) with corresponding fractions
    (1/16, 1/4, 1/2, 1).
- Slashed bonds: burned or routed to a treasury; design partner input needed.

## RWA-Sentinel port note

RWA-Sentinel ships a `blsThreshold.ts` implementation. Do NOT port it
verbatim into Go — see `docs/roadmap/RWA_SALVAGE.md` for the boundaries.
The design here uses `gnark-crypto` on the Go side; only the challenge
lifecycle and severity policy from RWA-Sentinel's `oracle-slashing` are
directly reusable.

## Milestones

1. **Interface skeleton (3 days).** Rust interfaces for `signer-registry`
   with `checked_*` arithmetic; Go interfaces for the aggregate-signature
   engine layer. No live contract deploy yet.
2. **Off-chain aggregate verification (5 days).** `engine/internal/quorum/`
   with `Aggregate`, `Verify`, `RotateKey`, PBT for the aggregation
   properties.
3. **Contract prototype (7 days).** Build the WASM under the 65 KiB limit;
   verify size after `wasm-opt -Oz --strip-debug`; testnet deploy.
4. **End-to-end (5 days).** `scripts/mass-runner-quorum.mjs`; reconcile per
   entrypoint; ≥ 95% pass rate.

## Non-goals

- On-chain pairing verification. Blocked by Casper Condor 2.x.
- Automatic quorum-size renegotiation. Manual admin-only for the 30-day
  slice.
- Cross-chain BLS quorum. Roadmap.

## Acceptance criteria

- [ ] `engine/internal/quorum/` with property-based tests.
- [ ] `contracts/signer-registry/` Rust crate that compiles under the WASM
      size limit.
- [ ] `docs/roadmap/BLS_QUORUM.md` cross-linked from `30-DAY.md`.
- [ ] `handoff/GAP_AUDIT_REPORT_*.md` follow-up notes matched.
