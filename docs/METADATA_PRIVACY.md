# Metadata Privacy

**Status**: `DRAFT — design plan`. This document catalogues the
*metadata* CasperProver's Service processes (traffic patterns, timing,
counterparty identifiers, external observers) and specifies the
minimisation, unlinkability, and disclosure posture per class. **No
code is shipped. No dependency is added. No paid service is
authorised. No vendor is selected.**

Cross-refs:
- `LEGAL/DATA_PROTECTION.md` (AI) — hash-only architectural boundary
  for *payload* privacy. This document extends that posture to
  *metadata* privacy, which is a strictly harder problem.
- `docs/OBSERVABILITY.md` (AG) + `docs/OPS_RUNBOOKS.md` (AH) — trace,
  metric, and log surfaces that are the biggest metadata sinks.
- `docs/CONFIDENTIAL_STORAGE.md` (AL) — object-level payload custody;
  its access logs are *themselves* metadata and are governed here.
- `docs/HSM_PLAN.md` (AJ) — signing latency and audit log surface are
  metadata sinks.
- `docs/MAINNET_LAUNCH_PLAN.md` (AK) — G2 audit gate under which any
  metadata-privacy commitment becomes contractual.

---

## 1. Framing — the metadata problem

CasperProver's hash-only payload boundary (`LEGAL/DATA_PROTECTION.md`)
solves *what* the Service sees: nothing but hashes. It does not solve
*how much can be inferred from the fact that the Service saw
anything*. The following are all metadata leaks even when payloads are
hashed:

- **Traffic-pattern inference**: request rate, batch size, and
  timing gap between attestation and anchoring reveal usage patterns
  (e.g. a lending Operator's daily decision cadence).
- **Counterparty identification**: source IP, TLS SNI, mutual-TLS
  client cert, API-key fingerprint, HTTP `Referer` — each identifies
  the Operator to the Service and to any on-path observer that
  breaks TLS.
- **Chain-side linkability**: an anchor transaction on Casper Network
  is public; the transaction's from-address, fee, timing, and value
  all leak information about the anchoring Operator, even if the
  Merkle root itself is opaque.
- **Model-identifier disclosure**: `model_id` is part of the receipt;
  it identifies which model produced the attested output. That is
  usually the intended signal, but for some Operators (regulated
  research settings) even *which* model was invoked is sensitive.
- **Verifier-side observation**: anyone who calls `verify.sh` on a
  public receipt learns that they cared. Verifier privacy is a
  separate axis from Attester privacy.

**A single metadata mitigation does not exist.** Different axes need
different tools, and some tools cost latency or throughput — those
costs must be paid explicitly, not smuggled in behind a "we protect
your privacy" marketing claim.

---

## 2. Metadata classes (per-usage)

### 2.1 Attester → Service ingress metadata

- **What is exposed**: source IP, TLS SNI, TLS ciphersuite, HTTP
  headers (User-Agent, API-key fingerprint, Accept-Encoding).
- **Minimum honest baseline**: truncate ingress IP at edge (AI:
  `LEGAL/DATA_PROTECTION.md` already requires IP truncation at
  ingest). No raw IP in any log downstream of edge.
- **Additional mitigations (optional, cost-bearing)**:
  1. **API-key hashing at edge**: log a keyed HMAC of the key, not the
     key itself. Cost: none. Recommendation: **default on**.
  2. **Rate-limiter obfuscation**: return the same latency profile for
     accepted, rate-limited, and rejected requests. Cost: small
     latency budget. Recommendation: default on for production.
  3. **Onion-routing / mixnet ingress**: Operator submits via a
     network privacy layer. Cost: latency, throughput. Recommendation:
     *documented as available*, not mandated. Never claim mixnet
     ingress unless the Operator actually uses one — that would be a
     `SIMULATION` claim.

### 2.2 Timing / traffic-pattern metadata

- **What is exposed**: request cadence, batch boundaries, anchoring
  cadence. Aggregate traffic pattern of a busy Operator reveals
  business rhythm.
- **Minimum honest baseline**: acknowledge in `LEGAL/TOS.md` (AI) that
  traffic-pattern privacy is *not* provided by default.
- **Mitigations (optional, cost-bearing)**:
  1. **Random jitter on anchoring**: delay anchor transactions by a
     bounded uniform random `[0, jitter_max]`. Cost: attestation
     finality bound loosened by `jitter_max`. Recommendation:
     `jitter_max ≤ 1 block-time` is honest and cheap.
  2. **Batch-size padding**: pad every attestation batch to the
     nearest power-of-two size with dummy leaves. Cost: on-chain
     data cost. Recommendation: available; not mandated.
  3. **Cover traffic**: Service emits dummy anchor transactions. Cost:
     significant on-chain cost; increases *observer* uncertainty about
     which anchors are real. Recommendation: **rejected** unless a
     specific Operator requires it and pays for it, because it also
     pollutes the public ledger.

### 2.3 Chain-side metadata (anchor transactions)

- **What is exposed**: `from` address of the anchor tx, tx fee,
  timestamp, block position. The anchor address is a persistent
  identifier of the Operator (or the Service, if the Service anchors
  on behalf).
- **Minimum honest baseline**: document in `LEGAL/DATA_PROTECTION.md`
  and `LEGAL/TOS.md` that the anchor address is a *pseudonym*, not an
  anonymity primitive. Anyone with chain-analysis capability can
  cluster anchors by address.
- **Mitigations (optional, cost-bearing)**:
  1. **Per-batch fresh anchor address**: rotate anchor keys every N
     batches. Cost: HSM key ceremony overhead (bound to
     `docs/HSM_PLAN.md`); more complex fee funding. Recommendation:
     available; default off until G2 review.
  2. **Anchor via aggregator**: Service anchors many Operators'
     roots under one aggregator address, disaggregation via the
     Merkle root. Cost: liability aggregation (see legal DRAFT); one
     Operator's root is on-chain-linkable to the aggregator's fee
     wallet. Recommendation: available; NEVER promise it as
     "unlinkable" — it is not.
  3. **ZK-KYC of Operator without disclosure of Operator identity**:
     research-grade. `SIMULATION`. Not shipped.

### 2.4 Verifier-side metadata

- **What is exposed**: when someone calls `verify.sh` or a
  `verify_receipt` API, the Verifier is fingerprinted (IP, TLS, API
  key). This is the *reader* side, not the *writer* side.
- **Minimum honest baseline**: document that verification is not
  anonymous by default. A Verifier who requires unlinkability must
  fetch receipts via public-mirror or via their own re-implementation
  of the verification path (which is possible because the receipt
  and root are public).
- **Mitigations (optional, cost-bearing)**:
  1. **Public-mirror publication**: Service publishes all anchored
     roots on an immutable public mirror; any Verifier can pull the
     mirror and verify locally without leaving a Service-side log.
     Cost: negligible. Recommendation: **default on** for anchored
     roots that the Operator has flagged as publicly-verifiable.
  2. **Private-verifier API**: authenticated Verifier calls the
     Service directly. Cost: metadata log entry per verification.
     Recommendation: available; honestly labelled as not-anonymous.

### 2.5 Observability metadata (traces, logs, metrics)

- **What is exposed**: request paths (route labels), status codes,
  latencies, error stack traces. Any of these can leak Operator
  identity if labels are chosen carelessly.
- **Minimum honest baseline** (already in AG design):
  1. Route labels are *bounded* (mux-resolver, no path-parameter
     high-cardinality labels).
  2. `model_id` and `api_key_id` are **never** metric labels; only
     coarse aggregate counters.
  3. Traces carry no payload bytes; only hashes when hashes are
     already in the receipt.
  4. Log lines carry no PD; already required by AI.
- **Mitigations (planned)**:
  1. Sample traces at a low rate for production; keep 100% only in
     staging.
  2. Delete trace/log data on the retention schedule from AI
     (traces 7d, metrics 15d, logs on category-dependent schedule).

### 2.6 Confidential-storage access metadata (planned)

- **What is exposed**: whenever the CSL disclosure workflow (AL §7)
  unwraps an object, the access log itself is metadata. It records
  *which reviewer accessed which object*, which discloses the
  existence of the object even if its contents remain private.
- **Minimum honest baseline** (bound to AL): access log entries are
  hashed by object-id (not object payload), signed by the Reviewer's
  session key, retained per LEGAL/DATA_PROTECTION.md. **Access log
  is durable evidence and cannot be silently deleted**, even for the
  Operator whose object was accessed.
- **Mitigations**:
  1. Access log entries themselves are anchored via the ordinary
     Merkle path. Cost: extra anchor cost. Recommendation: **default
     on** so that a Reviewer cannot silently omit an access.
  2. Access log queries are themselves logged (log-of-log-queries).
     Cost: negligible. Recommendation: default on.

### 2.7 External-observer metadata

- **What is exposed**: an observer on the same network segment as
  the Service can observe TLS SNI, JA3 fingerprints, TCP-timing.
- **Minimum honest baseline**: TLS ≥1.3 with encrypted SNI where
  supported; a matter of TLS termination hygiene, not application
  code.
- **Mitigations**: outside the application's control past TLS 1.3
  hygiene. Documented so it is not overclaimed.

---

## 3. Metadata retention schedule (aligned with AI)

| Metadata class                          | Retention  | Location                       | Source-of-truth doc |
|-----------------------------------------|-----------:|--------------------------------|---------------------|
| Ingress access logs (IP truncated)      | 30d        | edge log store                 | AI                  |
| API HMAC audit                          | 90d        | security log store             | AI                  |
| Trace spans                             | 7d         | observability backend          | AI, AG              |
| Metrics                                 | 15d        | Prometheus / metrics store     | AI, AG              |
| Anchor tx metadata (on-chain)           | perpetual  | Casper Network (public)        | AI                  |
| Confidential-storage access log         | 7y (draft) | separate CSL audit store       | AL                  |
| Anchored root public mirror             | perpetual  | public mirror                  | this doc, §2.4      |
| Attestation receipt (hash-only)         | perpetual  | Operator-controlled store      | AI                  |

**Retention diversity.** No single retention window covers all
metadata; there is no single "delete-my-metadata" button. This is a
consequence of the anchor being public and immutable — anyone
promising universal deletion would be lying. Documented plainly.

---

## 4. Disclosure model (per class)

- **Ingress metadata** → Operator can request its own via signed
  data-subject request (AI); Service verifies the requester is the
  data controller for the referenced Operator.
- **Chain-side metadata** → public; nothing to disclose because it is
  already disclosed.
- **Observability metadata** → not disclosed at Operator level (it is
  Service-operator's own operational data). Aggregate-only.
- **Confidential-storage access metadata** → disclosed to the
  Operator whose object was accessed *after* the Reviewer session
  closes; anchored so the disclosure is verifiable.
- **Verifier-side metadata** → not disclosed (the Verifier is a
  counterparty, not a data subject relative to this system).

---

## 5. Threat model

| Adversary                                 | Metadata they can extract by default                  | Mitigation possible? |
|-------------------------------------------|-------------------------------------------------------|:--------------------:|
| Passive on-path observer (post-TLS 1.3)   | SNI (unless ESNI), JA3, TCP timing                    | Partial              |
| Chain analyst                             | Anchor address clustering, fee patterns, timing       | Partial (per-batch rotation) |
| Aggregation counterparty                  | Every Operator that shares the aggregator             | Not honest to claim   |
| Compromised Verifier                      | Which receipts were verified from Verifier's IP       | Public mirror        |
| Malicious insider (Service operator)      | Everything the Service logs                           | Retention + minimisation |
| Legal-process compulsion                  | Everything covered by retention windows               | Retention (deletion is real if executed) |
| Model-identifier disclosure               | Which model was invoked per attestation               | Model-id can be hashed at Operator's choice |

**Hash-only *payload* boundary does not neutralise any of the above.**
Payload privacy and metadata privacy are separate problems and are
addressed by separate documents.

---

## 6. Honesty ladder for metadata claims

- **REAL (documented)**: baseline retention, minimum-honest-baseline
  mitigations in §2 (IP truncation, HMAC-of-key, bounded route
  labels, receipt hash only).
- **REAL (opt-in, per Operator)**: jitter, batch padding, per-batch
  anchor rotation, public-mirror mode, model-id hashing. Enabled
  explicitly by the Operator in their configuration; documented as
  such.
- **SIMULATION**: mixnet ingress, ZK-KYC of Operator, universal
  metadata deletion, unlinkable aggregator anchoring. Not shipped;
  labelled `SIMULATION` if referenced anywhere.
- **Explicitly out of scope**: anonymity guarantees against a
  well-resourced global observer. CasperProver does not attempt this
  and does not claim to.

Any commit anywhere in the tree that promises "anonymous attestation"
or "unlinkable proofs" without qualifying against this document is a
defect and should be rewritten with the specific `REAL / SIMULATION`
qualifier from §6 above.

---

## 7. Open questions (routed to `docs/KNOWN_LIMITATIONS.md`)

**Q1** — Which of the §2 mitigations are default-on, default-off, or
opt-in? Preliminary answer in §6; final ruling deferred to G2.

**Q2** — What is the intended retention window for confidential-storage
access logs? Draft: 7 years to survive a plausible legal-process
window. Confirm with counsel at G2.

**Q3** — Does the public-mirror mode (§2.4) require Operator opt-in per
batch, or is it default-on for publicly-flagged roots? Preliminary
answer: opt-in per batch, because default-on would leak Operator
existence.

**Q4** — Is jitter on anchoring compatible with the SLO alerts (AH)?
Preliminary answer: yes if `jitter_max ≤ 1 block-time` (SLO budget
already includes chain finality noise on that scale).

**Q5** — Aggregator anchoring is honest only when *not* labelled
unlinkable. Confirm no marketing surface claims otherwise.

---

## 8. What this document does not do

- It does not add code.
- It does not add a dependency.
- It does not authorise a paid service.
- It does not commit to a mixnet, an anonymity network, or a research
  system.
- It does not promise unlinkability without a hard qualifier.
- It does not treat payload privacy and metadata privacy as the same
  problem.
- It does not commit to a schedule.

The single deliverable is a per-class metadata inventory with
mitigation options priced honestly, so the honesty ladder covers
metadata as well as payload.

---

*This is a design plan. It ships no code. Its only purpose is to
make CasperProver's metadata-privacy posture auditable per class.*
