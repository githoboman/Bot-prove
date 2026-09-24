# Integrations Roadmap

**Status**: `DRAFT — design plan`. This document sketches the
integration surfaces CasperProver plausibly grows into (SDK-side,
Operator-side, Verifier-side, ecosystem-side) and, for each, what
must be true before the integration can ship under a `REAL` label
instead of `SIMULATION`. **No code is shipped. No vendor is
selected. No partnership is announced. No paid service is
authorised.**

Cross-refs:
- `docs/KNOWN_LIMITATIONS.md` — honesty ladder.
- `LEGAL/TOS.md`, `LEGAL/AUP.md`, `LEGAL/DATA_PROTECTION.md` — every
  integration inherits these; a partner integration cannot silently
  loosen them.
- `docs/MAINNET_LAUNCH_PLAN.md` (AK) — G2 (independent audit) and
  G6 (ops readiness) gate any integration that touches mainnet.
- `docs/METADATA_PRIVACY.md` (AO) — every integration adds a metadata
  surface; must be evaluated against the seven metadata classes.
- `docs/HSM_PLAN.md` (AJ) — any integration that signs on behalf of
  CasperProver must route through the `Signer` interface, not
  side-channel a soft-key.
- `docs/REPUTATION_ECONOMICS.md` (AL) — third-party integrations that
  attest on behalf of others become Attesters in the reputation model
  and inherit its bond/challenge dynamics.

---

## 1. Framing — what an "integration" is here

An integration is any interface across which CasperProver exchanges
work or trust with a system it does not own. That includes:

- **SDKs** — code the Operator embeds; runs under Operator control;
  the honesty question is *whether the SDK enforces the same
  contracts the Service does*.
- **Sinks** — where CasperProver receipts land: object storage,
  logging pipelines, SIEM, analytics tools.
- **Sources** — where the raw work CasperProver attests comes from:
  agent frameworks, model-serving stacks, decision-logging harnesses.
- **Chains** — Casper Network (target), and any multi-chain anchor
  stubs from AA. Each chain is an integration with its own
  operational model.
- **Verifier tooling** — receipt viewers, block explorers, dispute
  UIs that live outside the Service.
- **Standards** — cryptographic and audit standards that
  CasperProver claims to conform to (FIPS-204/205, RFC-5869, etc.);
  conformance is an integration commitment.

Every category has different failure modes; a single "integrations
roadmap" that treats them uniformly is a category error. This
document classifies them separately.

---

## 2. SDK-side integrations (Operator embedding)

### 2.1 Go SDK (existing, `REAL`)

- **Status**: shipped. 32 methods, 1:1 with API. `REAL`.
- **Honesty invariants**:
  - SDK never bypasses domain-separation tags (HASH_ALGORITHM_ANALYSIS §2.2).
  - SDK never logs raw payload; only hashes (LEGAL/DATA_PROTECTION.md).
  - SDK exposes the honesty ladder to consumers (a method call that
    returns a `SIMULATION` claim must return it typed, not stringified).
- **Roadmap**: keep in lock-step with API contract. No breaking
  changes without a schema version bump (HASH_ALGORITHM_ANALYSIS §4-Q2).

### 2.2 MCP server (existing, `REAL`)

- **Status**: shipped. 32 tools with schemas. `REAL`.
- **Honesty invariants**: same as 2.1.
- **Roadmap**: consumer tooling (LangChain, LlamaIndex, ADK) is a
  *reference*, not an endorsement. Documented as such.

### 2.3 Language-native SDKs (planned, `SIMULATION` until shipped)

- **Candidates (informational only, no vendor commitment)**: Python,
  TypeScript, Rust.
- **Preconditions before shipping**:
  1. Contract-driven: generated from OpenAPI or from a canonical schema
     file, not hand-ported (to prevent divergence).
  2. Coverage-parity with Go SDK on the same test vectors
     (`test/cp-merkle-provenance-vectors` from AE).
  3. Independent packaging (no lock-in to a single package manager's
     policies).
  4. Explicit honesty-ladder types in the language's type system
     (Python: `Literal["REAL"] | Literal["SIMULATION"] | ...`).
- **Migration urgency**: low. Existing Go + MCP surface covers
  target ecosystems today.

### 2.4 CLI (existing, `REAL`)

- **Status**: `verify.sh` exists (referenced from AK G1 exit criterion).
  `REAL` for the verify path.
- **Roadmap**: no new CLI surface until G6 (ops readiness).

---

## 3. Sink integrations (where receipts and metadata land)

### 3.1 Object storage (planned, blocked on `docs/CONFIDENTIAL_STORAGE.md`)

- **Status**: `SIMULATION` until CSL ships.
- **Preconditions**: CSL §6 Store interface exists; three candidate
  categories catalogued in CSL §6 are qualified under six selection
  gates. **No vendor named.**
- **Metadata implication**: object-storage access logs are themselves
  metadata (METADATA_PRIVACY §2.6). Any storage vendor with default
  access-log opacity is disqualified.

### 3.2 Logging / SIEM (planned, `SIMULATION`)

- **Status**: `SIMULATION`. Not shipped.
- **Preconditions**:
  1. SIEM ingest schema does not require field-level PII (fits the
     hash-only boundary).
  2. Log format is OpenTelemetry-compatible or clearly-documented plain
     JSON, so no vendor lock-in.
  3. Retention aligned with `LEGAL/DATA_PROTECTION.md`.
- **Migration urgency**: none for hackathon; medium for post-G6 GA.

### 3.3 Analytics / dashboards (existing local stack, `REAL / LOCAL-STACK`)

- **Status**: local Prometheus + Grafana dashboards from AG. `REAL`
  as a local operational tool; not a hosted analytics product.
- **Preconditions for hosted analytics**: none authorised. Any hosted
  analytics ships behind `SIMULATION` label first.

---

## 4. Source integrations (where the attested work comes from)

### 4.1 Agent frameworks

- **Candidates (informational only)**: LangChain, LlamaIndex, ADK,
  CrewAI, AutoGen — each has different decision-logging surfaces.
- **Status**: `SIMULATION` for framework-specific adapters.
  Reference example (MCP server) exists.
- **Preconditions**:
  1. Adapter cannot leak raw payloads to the Service (hash-only
     boundary).
  2. Adapter cannot forge `model_id` — Operator must sign the
     `model_id` in a way the adapter cannot bypass.
  3. Adapter version and framework version are both part of the
     receipt's canonical serialisation (HASH_ALGORITHM_ANALYSIS §2.2
     Q2).
- **Migration urgency**: low. A single well-documented reference
  adapter is more valuable than five poorly-documented ones.

### 4.2 Model-serving stacks

- **Candidates (informational only)**: OpenAI-compatible endpoints,
  local model servers (llama.cpp, vLLM, TGI), managed inference
  platforms.
- **Status**: `SIMULATION` for any "the model actually ran" claim
  from stacks that don't provide their own attestation.
  Attestation-of-invocation without proof-of-execution is honest
  under `REAL` iff the label is spelled out.
- **Preconditions before `REAL (ZK-ML)`**: all four conditions from
  `docs/ZKML_HONEST_VERDICT.md`.

### 4.3 Decision-logging harnesses

- **Status**: reference example shipped as part of engine. `REAL`.
- **Roadmap**: framework-agnostic harness (§4.1) subsumes this.

---

## 5. Chain integrations

### 5.1 Casper Network (target)

- **Status**: testnet-anchor path is `REAL / ON-CHAIN` (testnet).
- **Preconditions for mainnet**: all G1–G8 gates from
  `docs/MAINNET_LAUNCH_PLAN.md`.
- **Metadata implication**: METADATA_PRIVACY §2.3.

### 5.2 Multi-chain anchor stubs (AA)

- **Status**: stubs only; label `SIMULATION` per AA.
- **Preconditions before any secondary chain moves to `REAL`**:
  1. The chain's finality model is documented in
     `docs/OPS_RUNBOOKS.md` (rollback and reorg handling differ per
     chain).
  2. The chain's fee model is documented in
     `docs/MAINNET_LAUNCH_PLAN.md` §8 with a real cost estimate.
  3. G2 has audited the anchor-write path *for that chain* (a
     Casper G2 audit does not transitively cover a second chain).
  4. LEGAL/DATA_PROTECTION.md is amended if the second chain has
     different data-retention semantics.
- **Migration urgency**: low; multi-chain is a scale concern, not a
  correctness concern.

### 5.3 Bridges (rejected as integration surface)

- **Status**: **explicitly rejected**. Bridges are the least-audited
  surface in the ecosystem; adopting one imports its risk profile
  wholesale. If a partner needs cross-chain anchoring, they use §5.2
  per-chain writes, not bridge messaging.

---

## 6. Verifier-side integrations

### 6.1 Block-explorer receipt viewer (planned, `SIMULATION`)

- **Status**: `SIMULATION`. Not shipped.
- **Preconditions**:
  1. Public-mirror mode (METADATA_PRIVACY §2.4) is implemented so
     the viewer can operate without fingerprinting Verifiers.
  2. Viewer implementation is open-source and independently
     runnable (no Service-side dependency).
- **Migration urgency**: medium; a public viewer meaningfully
  improves the reputation-economics dispute channel (AL §5) by
  making Challenger evidence discoverable.

### 6.2 Dispute UI (planned, `SIMULATION`)

- **Status**: `SIMULATION`. Bound to `docs/REPUTATION_ECONOMICS.md`
  §5.
- **Preconditions**: reputation contract exists on testnet and has
  been through G2 audit.

---

## 7. Standards conformance (integrations with the outside world)

### 7.1 FIPS 204 (ML-DSA) and FIPS 205 (SLH-DSA)

- **Status**: SLH-DSA integration is `REAL` per AD (FIPS-205
  parameter sets shipped). ML-DSA integration referenced in existing
  hybrid signing path.
- **Roadmap**: parameter-set upgrade tracked in
  HASH_ALGORITHM_ANALYSIS §4-Q4.

### 7.2 RFC 5869 (HKDF)

- **Status**: planned use in CSL (AL); `SIMULATION` until CSL ships.

### 7.3 RFC 8446 (TLS 1.3), RFC 8280 (Encrypted SNI)

- **Status**: TLS 1.3 required at Service edge; encrypted SNI where
  supported (METADATA_PRIVACY §2.7).

### 7.4 W3C Trace Context

- **Status**: `REAL` per AG (Trace Context IDs).

### 7.5 OpenTelemetry (metrics + traces)

- **Status**: local shape emitted per AG. `REAL / LOCAL-STACK`.
  Any hosted OTel backend is a §3.3 integration, `SIMULATION`
  until authorised.

### 7.6 NIST post-quantum guidance

- **Status**: alignment documented in AD. `REAL` for the aligned
  primitives; `SIMULATION` for anything experimental.

### 7.7 GDPR + hash-only boundary

- **Status**: alignment documented in AI's DATA_PROTECTION draft.
  Counsel-reviewable at G5.

---

## 8. Prioritisation (informational only)

Any prioritisation here is a **planning artefact**, not a schedule.
The real prioritisation happens at G2/G6.

| Track                                    | Blocking gate | Blocking dependency          |
|------------------------------------------|:-------------:|------------------------------|
| Python/TypeScript SDK parity             | G6 (ops)      | Contract-driven generation   |
| CSL (object storage sink)                | G3 + G5       | CONFIDENTIAL_STORAGE draft   |
| Framework adapters                       | none          | Reference adapter first      |
| Multi-chain anchor writes (Chain-2)      | G2 + G8       | Per-chain audit              |
| Block-explorer receipt viewer            | G6            | METADATA_PRIVACY §2.4 mirror |
| Dispute UI                               | G2 + G3       | Reputation contract audit    |
| SIEM / hosted analytics                  | G6            | Log schema freeze            |
| ZK-ML relabelling (any pack that uses it)| G2            | ZKML_HONEST_VERDICT §4 conditions |

Cell "blocking gate" is a *hard* prerequisite; cell "blocking
dependency" is what must ship in-tree first.

---

## 9. Open questions (routed to `docs/KNOWN_LIMITATIONS.md`)

**Q1** — Which SDK is the second one after Go, and on what test-vector
parity? Preliminary: Python, using the AE Merkle-provenance vectors.
Deferred to G6.

**Q2** — Is the framework-adapter surface a first-class integration or
a reference-only example? Preliminary: reference-only. First-class
requires named partner and G2 audit of the adapter's honesty
invariants.

**Q3** — Does the public-mirror mode (METADATA_PRIVACY §2.4) require a
separate service or piggyback on the anchor tx? Preliminary:
piggyback (Merkle root is already public via the anchor).

**Q4** — What is the honest label for a partner integration that
attests on behalf of downstream Operators? They become Attesters in
the reputation model and inherit its bond/challenge (§AL). Preliminary
answer: yes, they must post bond. Confirm at G7 (financial
resilience gate).

**Q5** — Is bridge-based cross-chain anchoring reconsidered later, or
permanently rejected? Preliminary: permanently rejected in the
current design. Any future re-evaluation must start from a security
posture at least as strong as the primary chain, not by relaxing.

---

## 10. What this document does not do

- It does not select any vendor.
- It does not announce any partnership.
- It does not authorise any dependency or paid service.
- It does not commit to a schedule.
- It does not relabel any existing `SIMULATION` claim.
- It does not treat integrations as a marketing surface.
- It does not treat SDK ergonomics as a substitute for honesty
  invariants.

The single deliverable is an integration inventory that survives
audit: every integration surface catalogued, gated on the correct
mainnet-plan gate, labelled honestly, and free of vendor
commitments.

---

*This is a design plan. It ships no code and commits to no partner.
Its only purpose is to make CasperProver's integration surface
auditable per category, per honesty label.*
