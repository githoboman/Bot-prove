# Hash Algorithm Analysis

**Status**: `DRAFT — design analysis`. This file catalogues the hash
functions used in CasperProver, the property each one is asked to
provide, the honesty label each one currently carries, and the migration
posture (both defensive and post-quantum). **No code is changed by this
document. No dependency is added. No paid service is authorised. No
vendor is selected.**

Cross-refs:
- `docs/KNOWN_LIMITATIONS.md` — honesty ladder (`REAL` / `ON-CHAIN` /
  `SIMULATION`).
- `docs/HSM_PLAN.md` — key custody, which binds signature choices to
  hash choices (SLH-DSA parameterisation, KDF, HKDF-Expand-Label).
- `docs/MAINNET_LAUNCH_PLAN.md` §3 G2 — independent security audit gate
  under which any hash-family swap must be re-scoped.
- `docs/ZKML_RESEARCH_SPIKE.md` — the STARK/FRI candidate line depends on
  arithmetic-friendly hashes (Poseidon/Rescue-Prime), which live in this
  document's classification.

---

## 1. Framing — what a hash is *actually asked to do*

A single hash function name (SHA-256, Keccak-256, Poseidon, BLAKE3, …)
is not a security posture. The same primitive can be safe in one usage
and catastrophically wrong in another. This document classifies every
hash usage in CasperProver by **the security property it is asked to
provide**, because relabelling a bad choice as "REAL" without pinning
the property is exactly the failure mode the honesty ladder exists to
prevent.

The property axes are:

- **Preimage / second-preimage resistance** — given `H(x)` recover `x`.
  Required by every attestation.
- **Collision resistance** — find `x ≠ y` with `H(x) = H(y)`. Required
  by the Merkle tree; a collision here breaks provenance.
- **Domain separation** — `H_a(x) ≠ H_b(x)` when the *use* differs.
  Required so a receipt hash cannot be re-interpreted as a KDF output
  or a signature-input hash. This is *not* a property of the primitive;
  it is a property of the framing.
- **Length-extension safety** — Merkle-Damgård functions (SHA-256) are
  vulnerable when misused. Fixed by prefix-MAC, HMAC, or by using a
  sponge (Keccak/SHA-3, BLAKE3).
- **Post-quantum posture** — Grover halves the effective security
  bit-count for preimage; classical collision bounds are unchanged
  under BHT (birthday) but degraded in the quantum walk model.
  Practical impact: 256-bit output is the honest minimum.
- **Arithmetic-friendliness** — is the function efficient inside a
  SNARK/STARK circuit? Poseidon, Rescue-Prime, Griffin, Anemoi — yes.
  SHA-256, Keccak, BLAKE3 — no (feasible but expensive; measured in
  the surveyed benchmarks).
- **Side-channel exposure** — timing/cache leaks on secret-dependent
  branches. Any signing pathway (SLH-DSA seed, HMAC key) must use a
  constant-time implementation.

**Every hash usage in the tree must state which of the above properties
it needs. A hash usage that does not name the property is a defect and
is enumerated in §4.**

---

## 2. Registry — hash usages in CasperProver (as of `main`)

### 2.1 Merkle tree over attestation records (existing)

- **Where**: attestation batches → Merkle root → chain anchor.
- **Property required**: collision resistance ≥ 128-bit classical,
  ≥ 64-bit BHT-quantum → 256-bit output minimum.
- **Current primitive**: SHA-256.
- **Label**: `REAL`.
- **Analysis**: SHA-256 is a Merkle-Damgård construction. In the Merkle
  tree it is used only as `H(left || right)` at each level; there is no
  length-extension surface (both children are fixed-length hashes).
  Collision resistance for the batch sizes CasperProver targets is not
  at risk. Domain separation is achieved by prefixing internal nodes
  differently from leaf nodes; **the current implementation must be
  audited to confirm this prefix exists** (open question, §4-Q1).
- **Migration**: no urgency. If G2 recommends a sponge, dual-hash the
  root (SHA-256 || BLAKE3) for one release window and switch. Never
  swap primitives silently.

### 2.2 Attestation-receipt hash (existing)

- **Where**: canonical serialisation of `{model_id, inputs_hash,
  outputs_hash, timestamp, nonce}` → SHA-256 → this is the value that
  gets signed and anchored.
- **Property required**: preimage + second-preimage + domain separation
  from Merkle-node hashes.
- **Current primitive**: SHA-256.
- **Label**: `REAL`.
- **Analysis**: same primitive as §2.1, different use → **domain
  separation is critical**. The receipt hash and a Merkle internal node
  must be provably distinguishable so an adversary cannot substitute
  one for the other. The canonical serialisation must include a
  version tag and a purpose tag (`"cp:receipt:v1"`) before the payload.
  Open question §4-Q2 tracks whether this is implemented today or is
  a `SIMULATION`-style claim.
- **Migration**: same as §2.1.

### 2.3 KDF for symmetric envelope encryption (planned, `docs/CONFIDENTIAL_STORAGE.md`)

- **Where**: per-object DEK derivation from Operator KEK.
- **Property required**: preimage, key-derivation soundness, domain
  separation from receipt hashes.
- **Planned primitive**: HKDF-SHA-256 (RFC 5869) with a labelled info
  string.
- **Label**: `SIMULATION` (not implemented; on AL's confidential storage
  roadmap).
- **Analysis**: HKDF's Extract-Expand structure provides the required
  domain separation as long as the `info` label is well-chosen
  (`"cp:csl:v1:dek"` proposed). Never share a `salt` across purposes.
- **Migration**: HKDF-SHA-256 → HKDF-SHA-384 or HKDF-BLAKE3 gated on
  G2 (or on any downstream sponge switch).

### 2.4 HMAC over API request bodies (existing)

- **Where**: authentication of mutating API endpoints.
- **Property required**: MAC unforgeability under CMA; not a hash
  property per se — but the primitive underneath is a hash, and the
  choice matters.
- **Current primitive**: HMAC-SHA-256.
- **Label**: `REAL`.
- **Analysis**: HMAC's design covers length-extension issues for
  SHA-256; the primitive choice is not a concern. Key rotation posture
  is bound to `docs/HSM_PLAN.md`.
- **Migration**: no urgency. Any move to a sponge would be for
  ergonomics, not security.

### 2.5 SLH-DSA (SPHINCS+, FIPS-205) internal hashing (existing, AD)

- **Where**: hypertree, WOTS+, FORS — all internal hashing inside the
  post-quantum signature.
- **Property required**: preimage-resistance under quantum access to
  the hash oracle (Grover) — 256-bit output is the honest minimum.
- **Current primitive**: SHA-256 in the FIPS-205 parameter sets
  currently shipped (`SHA2-128f`, `SHA2-128s`, `SHA2-192f`, `SHA2-256s`).
- **Label**: `REAL`.
- **Analysis**: parameter-set selection determines the security level.
  For hackathon-scope demo, the smallest fast set (`SHA2-128f`) is
  documented in AD as sufficient; production posture (Gate G2)
  requires either `SHA2-192s` or `SHA2-256s` depending on the target
  security level. Never mix parameter sets across signing operations.
- **Migration**: parameter-set upgrade is a **schema-breaking event**
  (public-key size changes). Must be scheduled as such.

### 2.6 Arithmetic-friendly hash for STARK circuits (planned, `docs/ZKML_RESEARCH_SPIKE.md`)

- **Where**: hypothetical STARK-based ZK-ML prover. Not implemented.
- **Property required**: collision + preimage + very-low-degree
  arithmetisation over the STARK field.
- **Planned primitive family**: Poseidon, Rescue-Prime, Griffin, Anemoi.
- **Label**: `SIMULATION` (spike-only).
- **Analysis**: these primitives are **immature by the standards of
  SHA-2 and SHA-3**. Published cryptanalysis has caused parameter
  changes multiple times in the past 24 months. Any use in
  CasperProver must ship behind a `SIMULATION` label and cannot be
  relabelled `REAL` until the primitive itself is under independent
  audit (routed through G2). Non-negotiable.
- **Migration**: parameterisation is field-dependent; the choice of
  proving system fixes the choice of hash. See ZKML_RESEARCH_SPIKE §3.

### 2.7 VRF / Range-proof internal hashes (existing, AC)

- **Where**: VRF output derivation and range-proof commitments.
- **Property required**: pseudo-randomness of output + collision
  resistance for the commitment.
- **Current primitive**: SHA-256.
- **Label**: `REAL` for the primitive; `SIMULATION` for the higher-level
  VRF construction until an independent-implementation review is
  attached (routed through G2).
- **Analysis**: primitive is not the concern; construction is.
- **Migration**: none required at the primitive layer.

### 2.8 Merkle-provenance vectors (existing, AE)

- **Where**: cross-batch continuity proofs across chained receipts.
- **Property required**: same as §2.1 plus a strict linkage property
  (any tampering with a prior batch invalidates all subsequent ones).
- **Current primitive**: SHA-256.
- **Label**: `REAL`.
- **Analysis**: linkage is achieved by including the previous root in
  the leaf schema; the domain-separation prefix must distinguish
  linkage nodes from ordinary leaves (open question §4-Q1).
- **Migration**: same as §2.1.

### 2.9 Ceremony transcript hashing (existing, AF)

- **Where**: multi-party trusted-setup transcript (upgraded from
  single-coordinator in the current AF ceremony to multi-party in
  KEY_CEREMONY_PLAN).
- **Property required**: collision resistance + public-verifiability of
  the transcript hash + beacon-binding.
- **Current primitive**: SHA-256.
- **Label**: `REAL` for the primitive; `SIMULATION` for the multi-party
  extension until KEY_CEREMONY_PLAN executes.
- **Analysis**: primitive is fine. The beacon-binding property depends
  on the beacon source's own transparency, not the hash.
- **Migration**: none at the primitive layer.

### 2.10 Observability request-id hashing (existing, AG)

- **Where**: trace/span id derivation; NOT security-critical.
- **Property required**: pseudo-uniqueness only.
- **Current primitive**: (random 128-bit; no hash needed).
- **Label**: `REAL`.
- **Analysis**: not a hash usage; catalogued for completeness so no
  reviewer confuses it for one.
- **Migration**: N/A.

---

## 3. Classification summary

| Usage                                     | Primitive        | Label         | Post-quantum posture                    | Migration urgency |
|-------------------------------------------|------------------|---------------|-----------------------------------------|-------------------|
| 2.1 Merkle over attestations              | SHA-256          | REAL          | 256-bit output, 128-bit Grover-safe     | Low (dual-hash before swap) |
| 2.2 Receipt canonical hash                | SHA-256          | REAL (see Q2) | 128-bit Grover-safe                     | Low (fix domain-sep first) |
| 2.3 KDF for envelope encryption           | HKDF-SHA-256     | SIMULATION    | 128-bit Grover-safe                     | Blocked on AL   |
| 2.4 API HMAC                              | HMAC-SHA-256     | REAL          | MAC not affected by Grover              | None            |
| 2.5 SLH-DSA internal hashing              | SHA-256          | REAL          | Whole point of AD; PQ-safe by design    | Parameter-set upgrade for G2 |
| 2.6 STARK-arithmetic hash (planned)       | Poseidon-family  | SIMULATION    | Field-dependent; immature cryptanalysis | Blocked on G2   |
| 2.7 VRF / range-proof internals           | SHA-256          | REAL / SIM    | 128-bit Grover-safe                     | Construction audit, not primitive |
| 2.8 Merkle-provenance vectors             | SHA-256          | REAL          | Same as 2.1                             | Same as 2.1     |
| 2.9 Ceremony transcript                   | SHA-256          | REAL / SIM    | 128-bit Grover-safe                     | KEY_CEREMONY_PLAN |
| 2.10 Observability request-id             | random 128-bit   | REAL          | N/A                                     | None            |

**Consistency observation.** Every `REAL` label in the table is
underwritten by SHA-256 (or HMAC/HKDF built on it). One primitive
family carries the whole honesty posture. This is not necessarily
wrong — SHA-256 is the most-scrutinised hash in production
cryptography — but it is a **single point of primitive risk** and is
called out in §4 as an open question.

---

## 4. Open questions (routed to `docs/KNOWN_LIMITATIONS.md`)

**Q1 — Domain separation prefixes.** Confirm (audit against current
code, not assume) that Merkle internal nodes and receipt hashes use
distinct domain-separation prefixes (`"cp:merkle:node:v1"`,
`"cp:merkle:leaf:v1"`, `"cp:receipt:v1"`). If any prefix is missing,
that is a real defect — not a documentation gap — and is on the
critical path.

**Q2 — Canonical serialisation.** Confirm the receipt canonical
serialisation includes a version tag and purpose tag *before* the
payload. If it does not, receipts across versions could hash-collide
in a way that laundered them into each other.

**Q3 — SHA-256 monoculture.** Is the single-primitive-family posture
acceptable, or should CasperProver introduce a secondary primitive
(BLAKE3, SHA-3) in one high-value chain (e.g. the Merkle root) to
provide diversity? This is a G2 conversation, not a hackathon
decision. Defensible answer today: **no diversity change until G2**,
because introducing a second primitive without a formal soundness
analysis is worse than staying with one well-scrutinised primitive.

**Q4 — SLH-DSA parameter set for production.** Which FIPS-205
parameter set is the honest production target — `SHA2-192s` (small
signatures, slower signing) or `SHA2-256s` (larger signatures, higher
security margin)? Decision is deferred to G2; recorded here so the
default cannot silently drift.

**Q5 — Arithmetic-friendly hash immaturity.** Any STARK-based ZK-ML
prototype must ship behind a `SIMULATION` label, non-negotiable, until
the chosen arithmetic-friendly hash has an independent cryptanalytic
review. Poseidon has changed parameters multiple times in recent years;
CasperProver must not silently ride those changes.

**Q6 — HKDF `info` labels.** Every KDF call site must be catalogued
with its `info` label before it ships. Missing labels = missing
domain separation = a real defect.

**Q7 — Length-extension attack surface.** Confirm there is no code
path where a SHA-256 result is used as an implicit MAC (i.e. `H(key ||
message)` without HMAC framing). If there is, that is a real defect
and pre-dates this document.

---

## 5. Post-quantum posture summary

- **Signatures**: covered by SLH-DSA (AD). PQ-safe by construction.
- **Hashes**: 256-bit outputs everywhere → Grover halves to 128-bit
  effective. Acceptable under current NIST guidance.
- **Symmetric encryption** (planned for CSL): AES-256-GCM or
  XChaCha20-Poly1305 (AL). Grover halves to 128-bit effective.
  Acceptable.
- **Public-key encryption / key exchange**: not currently in the tree;
  if introduced, must use a PQ-hardened primitive (ML-KEM / Kyber for
  KEM) and route through G2. Explicitly out of scope for this
  document.

**Consequence.** CasperProver's PQ posture is already dominated by
SLH-DSA. The hash layer is *not the weakest link* today; the ZK-ML
label (`SIMULATION`) and the trusted-setup ceremony (`SIMULATION` until
KEY_CEREMONY_PLAN executes) are.

---

## 6. What this document does not do

- It does not swap any primitive.
- It does not name a vendor.
- It does not authorise a dependency addition.
- It does not commit to a schedule.
- It does not relabel any existing `SIMULATION` claim to `REAL`.
- It does not treat SHA-256 monoculture as a resolved risk.
- It does not treat Poseidon-family primitives as production-ready.

The single deliverable is a per-usage classification with property,
label, PQ posture, and migration urgency so that any future primitive
change can be discussed against an existing baseline instead of
against implicit assumptions.

---

*This is a design analysis. It ships no code and commits to no
migration. Its only purpose is to make CasperProver's hash-primitive
posture auditable, per usage, per property.*
