# VRF & Range Proofs — engine/internal/crypto

Backlog items **2.13 (VRF sortition)** and **2.14 (range proofs)**.

Both primitives live in `engine/internal/crypto/` alongside the existing PQ
(ML-DSA-65 / Lamport / Ed25519) code. They are honest real cryptography —
sign-then-verify actually works, tampering breaks verification, no
placeholders — and they carry an explicit **REAL CRYPTO** badge in the
docstring rather than a `simulation` marker.

Neither is used by any hot path yet; both are library-level building blocks
for follow-up commits (sortition-based challenge selection for the challenge
game, range-proven attestation stakes, private-input KYC bands).

## 1. VRF (`vrf.go`)

**Construction:** Schnorr-style ECVRF over the ed25519 prime-order subgroup
(cofactor-8 cleared). Similar in shape to RFC 9381 §5 but **not
byte-identical** — do not use it to interop with external ECVRF
implementations (Algorand, Chainlink, IETF test vectors). Everyone that has
to verify a proof runs this same Go code.

Scheme (informal):

```
Keygen: sk in Z_q, pk = sk * B
Prove(sk, alpha):
  H     = HashToCurve(pk, alpha)     # RFC 9381 TAI, cofactor-cleared
  Gamma = sk * H
  k     = HashToScalar(suite || seed || H || alpha)   # deterministic nonce
  c     = HashPoints(H, Gamma, k*B, k*H)
  s     = k + c*sk  mod q
  proof = (Gamma || c || s)          # 96 bytes
  beta  = SHA-512(suite || "beta" || Gamma)   # 64 bytes
Verify(pk, alpha, proof):
  H  = HashToCurve(pk, alpha)
  U  = s*B - c*pk
  V  = s*H - c*Gamma
  accept iff c == HashPoints(H, Gamma, U, V)
```

Sizes: seed 32B · pk 32B · proof 96B · beta 64B.

Properties:

| Property | Status |
|---|---|
| Completeness (honest prover verifies) | ✅ tested |
| Determinism (same seed+alpha → same proof) | ✅ tested |
| Different alpha → different beta | ✅ tested |
| Tampered proof rejected | ✅ tested |
| Tampered message rejected | ✅ tested |
| Wrong pk rejected | ✅ tested |
| RFC 9381 bit-compatibility | ❌ not claimed |

Tests: `TestVRF_*` in `vrf_test.go`.

## 2. Range proofs (`rangeproof.go`)

**Construction:** Pedersen commitment + per-bit disjunctive Sigma proof
(Chaum-Pedersen OR, Fiat-Shamir'd) + homomorphic reconstruction.

- Group: ed25519 prime-order subgroup (cofactor-8 cleared).
- Generators: `G` = 8·basepoint; `H` = NUMS point (deterministic
  hash-to-curve of a fixed domain string — anyone can recompute it and
  confirm it is not a hidden multiple of `G`).
- Commitment: `C(x, r) = x·G + r·H`.
- Bit commitments: `C_i = b_i·G + r_i·H`, with
  `x = Σ b_i·2^i` and `r = Σ r_i·2^i`.
- Per-bit proof: OR-proof that `C_i` opens to `0` or `1` — real HVZK Sigma
  protocol, not a placeholder.
- Homomorphic tie: verifier recomputes `Σ 2^i · C_i` and asserts equality
  with the aggregate `C`.

**This is NOT Bulletproofs.** Proof size is `O(n)`, not `O(log n)`. Same
soundness guarantee for the range statement `x ∈ [0, 2^n)`, worse
succinctness. Upgrading to Bulletproofs (inner-product argument
recursion) is a follow-up ticket (`BP-succinct`).

Sizes for `n = 32` (typical unsigned-32 amount):

- 1 aggregate commitment (32B)
- 32 bit commitments (32B each = 1024B)
- 32 OR proofs (160B each = 5120B)
- **Total: ≈ 6.1 KB per range proof**

For a Bulletproofs `n = 32` range proof this would drop to ≈ 675B — that is
the goal for `BP-succinct`.

Security (informal):

- **Completeness:** honest `x ∈ [0, 2^n)` always verifies.
- **Soundness:** forging accepts requires breaking Schnorr soundness of the
  OR proof (equivalent to solving DL of `H` w.r.t. `G` in the ed25519
  prime-order subgroup) — same assumption as any Pedersen-commitment
  system on this curve.
- **Zero-knowledge:** per-bit blindings hide bit values; the OR proof is a
  standard honest-verifier-ZK Sigma protocol with Fiat-Shamir.

Tests (`rangeproof_test.go`):

- `TestRange_ProveVerify_Roundtrip` — {0, 1, 2, 42, 1023, 65535} for n=16
- `TestRange_OutOfRange_Rejected` — x=256, n=8 → error at prove-time
- `TestRange_TamperedBitCommitment_Rejected`
- `TestRange_TamperedORProof_Rejected`
- `TestRange_TamperedAggregate_Rejected`
- `TestRange_RandomizedFuzz` — 20 random 32-bit values
- `TestRange_PedersenCommit_Consistency`

## Honest-badge summary

Both primitives are **REAL CRYPTO** — they are not simulations, mocks, or
"looks-like" placeholders. They are also **not on-chain**: neither has a
Casper Rust contract shipping today. On-chain verification of a VRF proof
or a range proof would require either a pairing precompile (blocked on
Condor 2.x) or bit-serial reimplementation inside a Casper contract — both
are follow-up tickets.

Where the honest label lives:

- `vrf.go` file docstring: "REAL cryptography … NOT a bit-identical RFC
  9381 implementation".
- `rangeproof.go` file docstring: "This is NOT Bulletproofs. Proof size
  is O(n), not O(log n)."
- This document, linked from README (follow-up commit).
