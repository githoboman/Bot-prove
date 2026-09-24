# Distributed Prover — MPC Threshold Signing (AU / 6.5)

> **Status: `[SPEC / DEFERRED / POST-AUDIT]`**
> This document defines the *design* for a t-of-n distributed prover based on
> threshold signing. **No runtime code is shipped for this component in the
> hackathon build.** The interface hooks in `attestor/` and the multi-verifier
> gossip design (`MULTI_VERIFIER_GOSSIP.md`) are the compile-time seams this
> module will plug into when work resumes post-audit.
>
> **Honesty ladder:**
> - REAL — nothing runtime yet.
> - ON-CHAIN — no on-chain touchpoint added by this document.
> - SIMULATION — no simulation shipped.
> - SPEC — this document + threat model + protocol sketch.

---

## 1 · Motivation

The single-signer proof-attestation path today is:

```
inputs → engine → hash → sign(sk) → anchor(Casper) → receipt
```

The signer holds a single private key. That key is the trust bottleneck of the
whole *Verifiable Agent Decision & Attestation Layer*:

- **Key compromise** breaks non-repudiation for all proofs signed by the key.
- **Signer censorship** allows a single operator to refuse valid decisions.
- **Signer equivocation** cannot be detected without an out-of-band gossip
  layer (see `MULTI_VERIFIER_GOSSIP.md`).

A t-of-n *threshold* prover replaces the single key with a set of n signer
shares `sk_1 … sk_n` such that any subset of size ≥ t can jointly produce a
signature indistinguishable from a single-key signature — but no subset of
size < t learns any information about the aggregate private key.

The distributed prover is the natural upgrade of the *attestor* interface
introduced in `HARDWARE_ATTESTOR_INTERFACES.md` (AT / 6.1–6.4): each attestor
becomes a shareholder rather than a full signer.

---

## 2 · Non-goals (deliberately)

- **Not a consensus protocol.** MPC signing is *not* a Byzantine agreement
  layer. Consensus over *which* decisions to sign lives in
  `MULTI_VERIFIER_GOSSIP.md`.
- **Not a mixnet.** This document does not address metadata privacy of the
  signing set. See `docs/METADATA_PRIVACY.md`.
- **Not zk-of-signing.** Threshold signing yields a normal signature; there is
  no zero-knowledge property over the identity of participating signers.
- **Not honest-majority-only.** The design targets a Byzantine adversary that
  may corrupt up to `t − 1` shareholders.

---

## 3 · Cryptographic Primitives Considered

Three families of threshold signature schemes are considered; the table
captures the design tradeoffs. **The recommendation is FROST/Ed25519 for
production, BLS-threshold for on-chain aggregation.**

| Scheme         | Signature type | DKG needed | On-chain verify cost | Async-friendly | Notes |
|----------------|---------------|------------|----------------------|----------------|-------|
| Shamir + BLS   | BLS12-381     | Trusted or verifiable-DKG | O(1) pairing | Fully async | Native aggregation with `MULTI_VERIFIER_GOSSIP.md` BLS quorum sigs; requires pairing. |
| FROST (Ed25519)| Schnorr-Ed25519 | DKG (Pedersen-VSS or FROST-DKG) | O(1) Ed25519 verify (30k gas) | 2-round or 1-round preprocessed | Rewinding-free, no random oracle beyond hash. Recommended for hackathon evolution. |
| GG20 (ECDSA-secp256k1) | ECDSA-secp256k1 | Interactive DKG | O(1) ECDSA verify | 6-round classic | Compatible with existing Casper ECDSA verifier if pursued; heavier round complexity. |

The reference implementation planned post-audit is **FROST** because:

1. It emits a **standard Schnorr/Ed25519 signature** — verifiable by anyone
   with a stock Ed25519 verifier. No custom on-chain verifier needed.
2. Signing rounds can be **preprocessed** (1-round online phase) — critical
   for the decision-attestation latency budget.
3. Public reference implementations exist (Zcash foundation).

BLS-threshold remains an option for the *aggregator* path — if the
per-decision proof needs to be aggregated across many decisions on-chain,
BLS12-381 aggregate signatures reduce N sigs to 1.

---

## 4 · Threat Model

Actors:

- **Signers** `S₁ … Sₙ` — hold key shares.
- **Coordinator** `C` — driver of the signing session (may be one of the
  signers or a separate node). *Not trusted.*
- **Requestor** `R` — engine caller that wants a decision signed.
- **Adversary** `A` — can corrupt up to `t − 1` signers *and* the coordinator
  simultaneously. Can drop, reorder, and delay messages. Cannot break Ed25519
  or the underlying hash function.

Assumptions:

- Each signer runs in an attested execution environment (see AT). At minimum
  the *audit-canary attestor* records every signing session — providing
  detection even when the environment cannot be fully attested.
- The share-refresh (proactive secret sharing) cadence is at most 90 days —
  see § 8.
- Network is partially synchronous during signing (rounds complete within a
  bounded interval); safety survives asynchrony, only liveness may pause.

Adversary goals we defeat:

- **Forgery** — trivially defeated: `A` holds < t shares.
- **Silent equivocation** — one honest signer + gossip layer sees the divergent
  signature and reports it (§ 7.3).
- **Denial-of-service by absent signers** — surviving majority ≥ t can still
  sign; § 7.4 covers rejoins.

Adversary goals we *do not* defeat (out of scope for this document):

- Colluding threshold `≥ t` signers producing valid but incorrect signatures.
  The *decision layer* (`decision/`) plus the multi-verifier gossip design
  catch the *content* being wrong; the MPC protocol only guarantees signature
  authenticity.

---

## 5 · Data Flow

```
                          coordinator C
                              │
     ┌────────────────────────┼────────────────────────┐
     ▼                        ▼                        ▼
   S₁ (share sk₁)           S₂ (share sk₂)          Sₙ (share skₙ)
     │                        │                        │
     ├── round1: nonces ──────┼── round1: nonces ──────┤
     ├── round2: partial ─────┼── round2: partial ─────┤
     │      sig share σ₁      │      sig share σ₂      │
     ▼                        ▼                        ▼
                          coordinator C
                              │
                              ▼
                    aggregate σ = Σ σᵢ (mod q)
                              │
                              ▼
                    verify(pk_group, msg, σ)
                              │
                              ▼
                      publish + anchor
```

At publish time:

- The **aggregate signature** is what the SDK returns to the caller — same
  wire format as today's single-key path. Consumers do not know signing is
  distributed.
- The **coordinator log** (signed session transcript + participant list) is
  emitted to the audit-canary attestor and, when the gossip layer is enabled,
  broadcast to peer verifiers.

---

## 6 · DKG (Distributed Key Generation)

Two DKG modes are supported by the design:

1. **Trusted setup** — one-time ceremony (§ 8), acceptable for the initial
   hackathon-successor deployment while auditing threshold-DKG code paths.
2. **FROST-DKG** — verifiable secret sharing via Pedersen commitments; no
   trusted dealer. Interactive but only once per group lifetime.

DKG output:

```
group_public_key : Ed25519 point PK such that PK = Σ pkᵢ · λᵢ
share_i          : scalar skᵢ held only by signer Sᵢ
lagrange_coefs   : {λᵢ}  interpolation coefficients for the chosen subset
commitments      : {Cᵢ}  public commitments to each share
```

The DKG artifact is anchored on Casper as a **group public key registration**
(new contract entry point `register_group_key(group_id, PK, threshold, n)`
to be added post-audit).

---

## 7 · Signing Protocol (FROST, 2-round or preprocessed)

### 7.1 · Round 1 — nonce commitment

Each signer `Sᵢ` picks two random scalars `dᵢ, eᵢ ∈ Z_q` and sends
`(Dᵢ, Eᵢ) := (dᵢ·G, eᵢ·G)` to the coordinator. The pair `(Dᵢ, Eᵢ)` may be
precomputed **before** the message is known — enabling a 1-round online
phase.

### 7.2 · Round 2 — signature share

Coordinator picks the signing subset `T ⊆ {1..n}` with `|T| = t`, computes
the binding factor `ρᵢ = H₁(i, msg, {(Dⱼ, Eⱼ)}ⱼ∈T)`, publishes the group
commitment `R = Σⱼ∈T (Dⱼ + ρⱼ·Eⱼ)`, then broadcasts `(msg, R)` to `T`.

Each `Sᵢ ∈ T` computes:

```
c   = H₂(R, PK, msg)                      -- challenge (Ed25519 standard)
zᵢ  = dᵢ + eᵢ·ρᵢ + λᵢ · skᵢ · c  (mod q)  -- signature share
```

and returns `zᵢ` to the coordinator.

### 7.3 · Aggregation + verification

```
z = Σⱼ∈T zⱼ  (mod q)
σ = (R, z)
```

Coordinator (and any observer with `PK`) verifies:

```
σ is valid iff  z·G  ==  R + c·PK
```

If verification fails, the coordinator identifies the malicious share by
recomputing each signer's expected share commitment
`Aⱼ = Dⱼ + ρⱼ·Eⱼ + λⱼ·c·pkⱼ` and comparing to the broadcast partial. This
gives **provable attribution** — the misbehaving signer's `zⱼ` is a proof of
misbehavior that can be published on-chain to slash their stake (see
`docs/SLASH_EQUIVOCATION_SPEC.md`).

### 7.4 · Rejoin / abort

- **Missing signer during round 1**: coordinator picks a different subset
  `T'` and restarts (only round 1 nonces of `T'` are used).
- **Missing signer during round 2**: session aborts, coordinator publishes
  the abort record to the audit canary; a new session begins.
- **Malicious partial in round 2**: attribution proof (§ 7.3) is broadcast
  to gossip peers, and the session restarts with a subset excluding the
  malicious signer.

Abort records include:

- Session id (uuidv7).
- Round reached (1 or 2).
- Reason (`timeout | invalid_partial | subset_change`).
- Attribution proof if `invalid_partial`.

Aborts are **not fatal** — the requestor sees a `503 SigningInProgress` from
the engine, the SDK retries with exponential backoff, and a fresh session
completes with a new subset.

---

## 8 · Proactive Share Refresh

Compromise of `t − 1` shares over the lifetime of the group is a certainty
without refresh. Every 90 days the group runs a **proactive secret sharing
(PSS) refresh**:

1. Each `Sᵢ` samples a random polynomial `pᵢ(x)` of degree `t − 1` with
   `pᵢ(0) = 0`.
2. Each `Sᵢ` distributes `pᵢ(j)` to each other `Sⱼ`.
3. New share `sk'ᵢ = skᵢ + Σⱼ pⱼ(i)`.
4. Old shares are securely erased.

The group public key `PK` is **unchanged** — refresh is transparent to
downstream verifiers. Casper anchoring of the refresh transcript is optional
(same group_id, incremented refresh_epoch).

---

## 9 · Interface Sketch (post-audit Go package)

**Not implemented in this document.** Sketch only, to be filled in by a
future audited PR:

```go
// engine/internal/prover/mpc/mpc.go (POST-AUDIT, NOT SHIPPED)

package mpc

type Session struct {
    ID          string            // uuidv7
    GroupID     string
    Threshold   int
    Members     []MemberID
    Coordinator MemberID
    Round       int               // 1 or 2
    State       SessionState      // Pending, Round1Done, Round2Done, Aborted, Committed
    Nonces      map[MemberID]Nonce
    Partials    map[MemberID]Scalar
    Message     []byte
}

type Prover interface {
    // NewSession initiates a signing session on the caller (coordinator role).
    // Returns SessionInProgress error if a session for the same message is
    // already active.
    NewSession(ctx context.Context, groupID string, msg []byte) (*Session, error)

    // ContributeNonce is called by a member on receipt of a Round1 message
    // from the coordinator.
    ContributeNonce(ctx context.Context, sessionID string) (Nonce, error)

    // ContributePartial is called by a member on receipt of a Round2 message.
    ContributePartial(ctx context.Context, sessionID string, msg []byte, R Point) (Scalar, error)

    // Finalize is called by the coordinator once t partials are collected.
    // Verifies the aggregate signature and returns it in canonical Ed25519 wire format.
    Finalize(ctx context.Context, sessionID string) (Signature, error)

    // Abort marks the session as aborted with the given reason and, if applicable,
    // an attribution proof. Emitted to the audit canary.
    Abort(ctx context.Context, sessionID string, reason AbortReason, proof *AttributionProof) error
}

type Attestor interface {
    // Reused from HARDWARE_ATTESTOR_INTERFACES.md; each MPC member is also
    // an attestor, recording the session_id and its own contribution digest.
    AttestSession(sessionID string, digest []byte) (Attestation, error)
}
```

Hooks into the current codebase:

- `attestor.Interface` — MPC members are attestors; audit canary records
  every session.
- `MULTI_VERIFIER_GOSSIP.md` — the group's public key `PK` is published via
  gossip; peers verify aggregate sigs the same way they verify single-key
  sigs today.
- `SLASH_EQUIVOCATION_SPEC.md` — attribution proofs from § 7.3 are the
  slashing evidence format.

---

## 10 · Migration Path

1. **Phase 0 (today, hackathon):** single-key signer. Interface hooks in
   place (`attestor.Interface`). No MPC code.
2. **Phase 1 (post-audit, ~3 months):** implement `mpc/frost.go` package;
   run FROST-DKG once with `n = 5, t = 3` on the operator team. Signing key
   for `verify.sh` demo remains single-key; MPC runs in shadow mode,
   producing sigs but not being used as the source of truth.
3. **Phase 2 (~6 months):** flip the signer path in the engine to MPC; single-
   key path kept as fallback behind a feature flag for two release cycles.
4. **Phase 3:** retire single-key path. All group keys rotated via PSS every
   90 days. Attestor sessions logged to audit canary + gossip.

---

## 11 · Test Vectors (spec-only)

FROST test vectors from RFC 9591 will be embedded in the eventual runtime
package. **This document specifies the vector shape**, not the values:

```
{
  "curve":        "Ed25519",
  "group_public": "<64-hex>",
  "shares":       [{"id":1,"share":"<64-hex>"}, ...],
  "message":      "<hex>",
  "round1": {
    "member_nonces": [
      {"id":1,"D":"<64-hex>","E":"<64-hex>"}, ...
    ]
  },
  "round2": {
    "binding":       [{"id":1,"rho":"<64-hex>"}, ...],
    "group_commit":  "<64-hex>",
    "partials":      [{"id":1,"z":"<64-hex>"}, ...]
  },
  "aggregate": {"R":"<64-hex>", "z":"<64-hex>"},
  "verifies": true
}
```

Vector files land under `testdata/mpc/frost/` in the runtime package. **Not
committed in this branch.**

---

## 12 · Open Questions

- **Casper on-chain verifier**: Ed25519 verification precompile availability
  on Casper Condor 2.x is tracked in `docs/MAINNET_LAUNCH_PLAN.md`; if
  absent, sig verification stays off-chain (proof-of-signing anchored, sig
  verified by relayer).
- **HSM integration**: signer shares should live in HSMs. Interface mapping
  to `docs/HSM_AND_KEY_CEREMONY_PLAN.md` (AJ) to be reconciled in Phase 1.
- **Formal proof**: FROST security is proven in the ROM; a machine-checked
  proof (per `docs/AV_FORMAL_VERIFICATION_PLAN.md` — deferred) would target
  the actual Go implementation.

---

## 13 · References

- Komlo & Goldberg — *FROST: Flexible Round-Optimized Schnorr Threshold
  Signatures* (2020).
- IRTF CFRG — RFC 9591, *The FROST Threshold Signature Scheme*.
- Herzberg et al. — *Proactive Secret Sharing* (1995).
- Gennaro & Goldfeder — *One Round Threshold ECDSA* (GG20).
- Shoup — *Practical Threshold Signatures* (2000).
- CasperProver internal:
  - `docs/HARDWARE_ATTESTOR_INTERFACES.md` (AT)
  - `docs/MULTI_VERIFIER_GOSSIP.md` (BC)
  - `docs/SLASH_EQUIVOCATION_SPEC.md` (BD)
  - `docs/HSM_AND_KEY_CEREMONY_PLAN.md` (AJ)
  - `docs/PHASE2_CEREMONY.md` (AF)

---

## 14 · Ladder Statement

| Property                     | Ladder                    |
|------------------------------|---------------------------|
| Threshold prover runtime     | **[SPEC / DEFERRED]** — no code shipped in this branch. |
| Attestor interface hooks     | REAL — landed in AT/6.1-6.4. |
| On-chain group-key registration | **[POST-AUDIT]** — contract entry point deferred. |
| Test vectors                 | **[SPEC-ONLY]** — vector shape defined; values not committed. |
| DKG ceremony                 | **[POST-AUDIT]** — Phase 1 milestone. |

_This document is deliberately code-free for the hackathon submission. Any
future PR that adds runtime MPC code is a security-critical change and MUST
be gated on an external audit of both the design and the implementation._
