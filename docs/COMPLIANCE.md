# CasperProver — Compliance Baseline

**Status:** compliance-by-design baseline for the hackathon submission. This
document catalogues how the current architecture maps to major regulatory
regimes, what is designed-in, what is a documented gap, and what needs a
formal legal opinion before mainnet / commercial launch.

**This is not legal advice.** It is a good-faith engineering position paper
that a real counsel can pick up as a starting point.

---

## 1. Regulatory scope — what CasperProver actually is

CasperProver is a **cryptographic audit-trail engine for AI agent
decisions**. Its purpose is *evidentiary*: it commits an agent's inputs
and outputs to Casper so that a verifier can later prove, in
milliseconds, that a decision was made against a specific set of
inputs and produced a specific set of outputs.

The relevant surfaces:

| Surface | What it does | Custody? | Autonomy |
|---|---|---|---|
| Commit contract | Anchors Merkle roots of agent I/O on Casper | No fund custody; anchors hashes | Deterministic |
| ZK layer (gnark Groth16) | Off-chain zero-knowledge proofs of statements about committed data | N/A | Off-chain |
| PQ layer | Post-quantum signature/commitment scheme | N/A | Off-chain |
| Proof-chain DAG | Directed acyclic graph linking sequential decisions | N/A | Deterministic |
| Verifier registry | Contract-side registry of verifier types | N/A | Deterministic |
| API + CLI | Commit, prove, verify, query surfaces | N/A | Coordination-layer only |
| HITL sinks (Slack / Telegram) | Human-in-the-loop notifications for flagged decisions | N/A | Off-chain notify |
| Attack evidence UI | Interactive display of prompt-injection / equivocation fixtures | N/A | Read-only |

**Key architectural facts** that drive downstream classification:

1. **CasperProver moves no money.** It anchors commitments and proofs.
   The chain-side operation is `write hash` + `emit event`, not `transfer
   value`. No user funds are ever held or moved by the protocol.
2. **CasperProver stores no personal data on-chain by default.** The
   on-chain object is a Merkle root over an off-chain evidence set. What
   is on-chain is a fixed-size hash. Personal data, if any, lives off-chain
   under retention policies the deployer configures.
3. **CasperProver's ZK layer is data-minimising.** ZK proofs let a
   verifier confirm a statement about the committed data without
   revealing the underlying data. This is the GDPR data-minimisation
   principle operationalised in crypto.
4. **CasperProver is deterministic verification.** Verification uses
   pinned tooling (`nightly-2025-01-01` toolchain, contract-build parity
   in CI). A judge or auditor can rebuild the deployed artifacts from
   source and independently check every anchored proof.

---

## 2. EU — MiCA (Regulation (EU) 2023/1114)

**Deadline:** transitional regimes fully closed by 1 July 2026.

### Position

CasperProver is **outside the CASP (Crypto-Asset Service Provider)
perimeter** because it performs none of the regulated crypto-asset
services listed in Article 3 MiCA:

- Not custody / administration of crypto-assets on behalf of clients.
- Not operation of a trading platform.
- Not exchange of crypto-assets for funds or for other crypto-assets.
- Not execution of orders, placement, or reception / transmission of
  orders on behalf of clients.
- Not investment advice.

CasperProver writes commitment hashes to the chain and reads them back.
This is a **utility-layer use of blockchain**, not a crypto-asset service
under MiCA. Recital 22 makes clear that MiCA does not regulate every
activity that touches blockchain; only the listed CASP services.

### Gaps / open questions for counsel

1. **CSPR consumption for gas.** A deployer running CasperProver spends
   CSPR to pay for chain writes. This is *use* of a crypto-asset by the
   deployer, not provision of a crypto-asset service to a client.
   Nothing here creates a CASP obligation.
2. **Commercial API access.** If a deployer offers "verifiable audit
   trail as a service" and clients pay in fiat or in CSPR, the deployer
   is selling a SaaS. The payment mechanism does not import MiCA scope
   on its own. If clients pay in CSPR, standard e-money / VASP analysis
   for the deployer's payment processor applies, independently of CP.

### What we ship for EU deployers

- `docs/ARCHITECTURE.md` documenting the utility-layer character of
  every on-chain write.
- This document as the good-faith position statement.

---

## 3. EU — AI Act (Regulation (EU) 2024/1689)

**Deadline:** high-risk AI system obligations phase in through Aug 2026.

### Position

**CasperProver is not itself an AI system.** It is an audit-trail engine
that anchors evidence *about* AI systems. Its role is verification,
not decision-making. This puts CP in a fundamentally different position
from an LLM-driven arbitration platform: CP is precisely the kind of
**post-market monitoring infrastructure** the AI Act calls for.

CP's alignment with the AI Act is on the **compliance-enabler** side:

| AI Act obligation | How CP helps a deployer meet it |
|---|---|
| Art. 12 Record-keeping | CP commits decision inputs and outputs to a Merkle-anchored on-chain trail; the deployer can produce record integrity proofs on demand. |
| Art. 13 Transparency to deployers | CP's proof-chain DAG lets a deployer show every step in a chain of AI decisions, with cryptographic linkage. |
| Art. 14 Human oversight | HITL sinks (Slack, Telegram) surface flagged decisions to human operators before or after they take effect. |
| Art. 15 Accuracy, robustness, cybersecurity | Post-quantum layer + real gnark Groth16 as primary + proof-chain DAG validation give tamper-evidence that survives long retention windows. |
| Art. 72 Post-market monitoring | Every anchored commitment is a checkpoint. Regulators or internal auditors can audit-in-place without reconstructing history. |
| Art. 73 Serious incident reporting | The audit trail *is* the incident evidence: what the model saw, what it produced, when. |

### Gaps / open questions for counsel

1. **CP itself as an AI system.** CP does not classify, predict, or
   generate outputs; it commits and verifies. It does not fit the AI
   system definition in Art. 3(1) of the AI Act.
2. **Prompt-injection / equivocation fixtures.** These are *tests*, not
   inference. Shipping fixtures does not make CP an AI system.
3. **Downstream deployer classification.** A deployer using CP to log a
   high-risk AI system inherits that system's obligations. CP's job is to
   make those obligations *auditable*. This document does not classify
   the deployer's system.

### What we ship for AI Act-conscious deployers

- Merkle-anchored commit contract + on-chain verification.
- HITL sinks (Slack, Telegram) with env-driven `MultiSink` config.
- Attack evidence UI + prompt-injection / equivocation fixture batteries.
- Post-quantum-ready commitment layer.

---

## 4. EU — GDPR (Regulation (EU) 2016/679)

This is where CasperProver is at its strongest. **ZK proofs are the
canonical data-minimisation technique** under Art. 5(1)(c) GDPR.

### Position

CP is designed around three GDPR principles:

**Data minimisation (Art. 5(1)(c)).** The on-chain object is a Merkle
root — a fixed-size hash that reveals no personal data. Verification of
a statement about the committed data uses a ZK proof: the verifier
learns only whether the statement holds, not the underlying data. This
maps directly to the *adequacy, relevance, necessity* three-part test
in Art. 5(1)(c). Recent academic work (Internet Policy Review, 2024;
ScienceDirect on Verifiable Credentials, 2025) treats ZKPs as the
state-of-the-art for GDPR data minimisation.

**Storage limitation (Art. 5(1)(e)).** Off-chain evidence lives under a
deployer-configured retention window. On-chain commitments are
retention-permanent by chain design; because they contain only hashes,
they do not create GDPR retention problems (a hash of personal data is
not itself personal data if the pre-image cannot be recovered — see the
CJEU's stated posture on strong-hash pseudonymisation).

**Integrity and confidentiality (Art. 5(1)(f)).** Post-quantum layer
extends the integrity guarantee beyond the horizon of classical
cryptanalysis. Deterministic verification prevents silent tampering.

### How the design already meets GDPR obligations

- **Data minimisation.** Merkle-root-only anchoring + gnark Groth16 ZK
  proofs for statements about committed data.
- **Purpose limitation.** The purpose of every anchored object is
  audit-evidence; the technical shape (hash + optional proof) enforces
  that limit.
- **Integrity.** Post-quantum-ready commitments + proof-chain DAG.
- **Right to erasure caveat.** On-chain commitments are immutable. This
  is only a problem if personal data is written on-chain. **CP writes
  hashes, not personal data.** Off-chain personal data (if any) remains
  erasable under the deployer's retention policy.

### Gaps / open questions for counsel

1. **The pre-image argument.** For a hash of personal data to be treated
   as non-personal-data, the pre-image must be effectively unrecoverable
   under GDPR's "reasonably likely" standard (Recital 26). This holds
   for high-entropy inputs; it fails for low-entropy inputs (e.g., a
   hash of an email address without salt). **Guidance:** deployers must
   never commit low-entropy personal data unhashed or unsalted. Fixture
   design in CP is high-entropy by construction.
2. **Right of access (Art. 15).** A data subject who asks the deployer
   "what proofs did you anchor about me?" — the deployer must be able to
   answer from off-chain records. On-chain data alone is insufficient
   for this reply.
3. **DPIA (Art. 35).** For high-risk processing, a DPIA is required. CP
   is an *enabler* of DPIA-friendly architectures, not a replacement for
   the DPIA itself.
4. **Cross-border transfers.** CP does not itself transfer personal data
   cross-border; the on-chain layer is a public ledger of hashes.

### What we ship for GDPR-conscious deployers

- Merkle-root-only on-chain surface.
- Real gnark Groth16 ZK proofs as the primary verification path.
- Post-quantum-ready commitments for long-retention integrity.
- HITL sinks that enable timely notification to data subjects when
  automated processing flags them.

---

## 5. US — FinCEN money-transmitter analysis

Reference: FinCEN FIN-2019-G001 (May 9, 2019).

### Position

**CasperProver is not a money transmitter.** It does not accept value
and transmit it. It does not touch user funds at all. Even the CSPR
spent for gas is the deployer's own operational expense; it does not
transit through CP as a service.

Under the 2019 guidance:

- CP is not an administrator of a CVC payment system (Section 4.5.2).
- CP is not an exchanger (Section 4.5.3).
- CP is closest in character to a *supplier of tools* that may be used
  in money transmission (Section 4.5.1(b)) — even though CP is not used
  in money transmission at all; it's used in verifying AI decisions.
  The software-provider posture is the appropriate analog.

### Gaps / open questions for counsel

1. **State-by-state.** No US state should treat an audit-trail engine
   that never touches user funds as a money-transmitter. But the
   deployer's business model is what matters — if a deployer bundles CP
   with a value-moving service, the value-moving service is analysed
   separately.
2. **Wyoming.** § 40-22-104(a)(vi) virtual-currency exemption is
   irrelevant to CP directly (no value transfer), but confirms Wyoming
   is a friendly jurisdiction for deployers domiciled there.
3. **CFTC / SEC.** No token issuance, no securities-adjacent activity.

---

## 6. US — NIST AI RMF 1.0 + GenAI Profile

CasperProver is the closest thing in the ecosystem to an
**operationalised AI RMF Manage / Measure control**. Mapping:

- **Govern.** This document is a governance artefact; the deployer's
  organisational governance uses CP as record-keeping infrastructure.
- **Map.** Prompt-injection & equivocation fixture batteries enumerate
  known attack surfaces the model must resist.
- **Measure.** Every anchored decision is a measurable checkpoint.
  Proof-chain DAG lets an auditor sample and verify.
- **Manage.** HITL sinks (Slack, Telegram) close the loop between an
  auto-classified event and a human review.

The GenAI profile subcategories most relevant here: MS-2.5 (content
provenance and traceability), MS-2.11 (measurement of AI system
outputs), MG-4.1 (post-deployment monitoring), MG-4.2 (feedback
mechanisms). CP is a fit-for-purpose implementation surface for all
four.

---

## 7. Enterprise-sales positioning

Three lines for a compliance officer:

1. **Cryptographic evidence, not custody.** CP writes hashes, never
   money and never personal data. No CASP, no money-transmitter, no
   BSA obligations follow from using CP.
2. **GDPR-native.** ZK proofs are the canonical data-minimisation
   technique under Art. 5(1)(c). CP's Groth16 primary path and PQ
   layer make CP the technical substrate for GDPR-compliant audit
   trails of automated decisions.
3. **AI Act enabler.** CP is not itself a high-risk AI system. It is
   the record-keeping (Art. 12), transparency (Art. 13), human-oversight
   (Art. 14), and post-market-monitoring (Art. 72) surface that
   high-risk deployers need to actually meet those obligations.

---

## 8. Gap list (short)

- [ ] DPIA template for deployers using CP with high-risk AI.
- [ ] Data-subject access playbook: how a deployer answers
      Art. 15 GDPR requests with CP data.
- [ ] Salting guidance for low-entropy pre-images (so hash ≠ personal data).
- [ ] Per-jurisdiction deployer runbook.
- [ ] Formal legal opinion on the ZKP-as-data-minimisation position
      for the first paying deployer's jurisdiction.
- [ ] Article 30 GDPR records-of-processing template for CP-adjacent
      processing.

---

## 9. Change log

- 2026-07-21 — v0.1 initial baseline (hackathon submission).

---

*This document is written to hand to counsel, not to replace them. If you
are deploying CasperProver in production, retain local counsel.*
