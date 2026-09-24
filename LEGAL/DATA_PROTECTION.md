# CasperProver — Data Protection Notice

> **Status: DRAFT — self-authored, not reviewed by counsel or by a DPO.**
> This document is a good-faith draft of the intended data-protection
> posture. It has **not** been reviewed by qualified legal counsel or by
> a certified Data Protection Officer. It will be replaced by a
> counsel-reviewed version, and a formal Article 30 Record of Processing
> Activities (ROPA) will be produced, before any commercial launch.
> See `docs/MAINNET_LAUNCH_PLAN.md` (Pack AK) for the paid-legal-review
> milestone.

**Effective date (draft):** 2026-07-26
**Version:** 0.1-draft
**Data controller (draft):** CasperProver project maintainers
**Contact:** khrol.studio@gmail.com

---

## 1. Scope

This notice describes how the CasperProver Service ("the Service")
processes data provided by operators, and by data subjects whose data
operators submit for hashing and attestation. It is written to be
compatible with the General Data Protection Regulation (Regulation
(EU) 2016/679, "GDPR") and adjacent regimes (UK GDPR, Swiss FADP);
it is not tailored to US sectoral regimes (HIPAA, GLBA, etc.) and
must be supplemented by operators whose use cases fall under those
regimes.

## 2. Roles

- **Operator** — the legal or natural person integrating the Service.
  Under GDPR the operator is typically the **controller** with respect
  to any personal data it submits.
- **Maintainers / Service** — the project maintainers operate the
  hosted anchoring endpoints and the on-chain contracts. With respect
  to the personal-data payloads themselves the maintainers do **not**
  act as a controller, because the Service never sees them in the
  clear (see §3). With respect to operational telemetry (§4) the
  maintainers act as controller.
- **Data subject** — a natural person to whom the underlying decision
  relates (e.g. a loan applicant, a patient).

## 3. Hash-only boundary — architectural minimisation

The Service is designed around a **hash-only** boundary. The
maintainers never observe personal data in the clear:

- Decision **inputs** are hashed at the operator's edge, using
  SHA-256 (with domain separation, see `docs/MERKLE_SCHEME.md`).
  Only the resulting digest crosses the network.
- Decision **outputs** are hashed the same way.
- Only the digest, the model identifier, the timestamp, the operator
  identifier, and the resulting Merkle root are ever seen by the
  Service.
- The Merkle **root** is then anchored on the Casper Network testnet.
  On-chain state contains only the root and the anchoring transaction
  metadata — **never** raw personal data.

Operators are required by the AUP (§3.4) to preserve this boundary and
not to submit unhashed personal data.

**Exception — receipts.** A receipt may contain operator-supplied
labels or metadata fields. Operators must treat receipt payloads as if
they will become public: they are shared with any verifier the
operator hands the receipt to, and, once anchored, the root that binds
them is permanently on the testnet.

## 4. Operational telemetry the Service *does* collect

Even under the hash-only boundary, the Service collects a limited set
of operational telemetry directly identifiable to the operator (but
not directly to data subjects):

| Category | Data | Purpose | Retention | Lawful basis |
|---|---|---|---|---|
| API metadata | Operator API key hash, request path, HTTP status, latency, size | Rate-limiting, abuse prevention, capacity planning | 30 days | Legitimate interests (Art. 6(1)(f)) |
| Metric samples | Prometheus counters / histograms (Pack AG) — aggregated per route, no user ids | SLO monitoring | 15 days | Legitimate interests |
| Trace spans | OTel spans (Pack AG) — HTTP method, path, status, latency, span ids | Debugging failed requests | 7 days | Legitimate interests |
| Contact | E-mail supplied when reporting incidents / requesting keys | Coordination of onboarding, incident response, subject-rights requests | Duration of relationship + 2 years | Contract (Art. 6(1)(b)) / legal obligation |
| Security logs | Auth failures, rate-limit trips, gitleaks findings | Security monitoring | 90 days | Legitimate interests |

**IP addresses** appearing in request logs are truncated at ingest
(`/24` for IPv4, `/48` for IPv6) before the 30-day API-metadata
retention starts. Full IPs are only retained during an active
security investigation.

## 5. Special categories

The Service is designed not to receive special-category personal data
in the clear. If an operator submits hashed inputs that are
*derived from* special-category data (e.g. a hash of a health record),
the digest itself is not special-category data by nature, but the
operator remains the controller of the underlying record and its
obligations under Art. 9 GDPR are unaffected.

## 6. Data transfers

The current deployment runs entirely on infrastructure controlled by
the maintainers. No third-country transfers of personal data occur
under §3 (there is no personal data to transfer).

Post-hackathon hosting decisions (`docs/MAINNET_LAUNCH_PLAN.md`) will
be revisited with the counsel-reviewed version of this notice, at
which point Standard Contractual Clauses or equivalent will be applied
to any relevant sub-processor.

## 7. Retention schedule

Retention is minimised by design. Non-personal cryptographic material
is kept longer because it *cannot* identify a data subject:

| Artefact | Contains PD? | Retention |
|---|---|---|
| Merkle roots on testnet | No | Indefinite (chain-native; testnet may reset) |
| Receipts (JSON) | Operator-controlled | Operator's schedule; maintainers do not store copies |
| API metadata log | Indirectly (operator-level) | 30 days rolling |
| Prometheus metrics | No (aggregated) | 15 days rolling |
| OTel traces | Indirectly | 7 days rolling |
| Auth / security log | Indirectly | 90 days rolling |
| E-mail correspondence | Yes | Duration of relationship + 2 years |
| Ceremony transcripts | No (public artefact) | Indefinite |

Rolling retentions are enforced by the local observability stack
(Pack AG); no external log SaaS is used in the hackathon phase.

## 8. Data-subject rights

Because the Service does not process personal data of end-user data
subjects in the clear (§3), most data-subject requests must be
addressed to the **operator**, not to the maintainers. Where an
operator's request references only a decision digest, the maintainers
cannot re-identify the underlying data subject and cannot fulfil the
request directly.

For personal data the maintainers do process (§4 — operator contact
details, security logs pertaining to a natural-person operator), the
following rights apply, subject to law:

- Right of **access** (Art. 15) — request a copy of the personal data
  the maintainers hold about you.
- Right of **rectification** (Art. 16).
- Right of **erasure** (Art. 17), subject to retention obligations
  in §7.
- Right of **restriction** (Art. 18).
- Right of **data portability** (Art. 20) — where processing is based
  on contract or consent.
- Right to **object** (Art. 21) — including to processing based on
  legitimate interests.

### 8.1 Response template — for operator-side requests

Operators fulfilling their own data-subject requests may need to
demonstrate that a specific decision was recorded at a specific time.
The following template can be used to help an operator answer a
subject-access request without exposing the underlying data:

```
Dear [data subject],

In response to your access request dated [DATE], we can confirm that on
[TIMESTAMP UTC] our system recorded an automated decision concerning
you.

For integrity purposes, the decision has been anchored via
CasperProver, an independent verifiable-attestation service, under
receipt ID [RECEIPT_ID]. The receipt binds:

  - the SHA-256 hash of the inputs used for that decision,
  - the SHA-256 hash of the decision output,
  - the model identifier that produced the decision,
  - the timestamp,
  - the anchoring Merkle root [ROOT] recorded on
    testnet transaction [TX_HASH].

We can, on request, provide you with (a) the underlying inputs and
outputs in the clear, (b) a copy of the receipt so that you can
independently verify integrity via `verify.sh`, and (c) our decision
justification.

Note: CasperProver never sees the inputs or outputs in the clear; it
only sees their hashes. It therefore cannot itself answer this
request on our behalf.

Yours sincerely,
[Operator]
```

### 8.2 Complaints

Data subjects and operators dissatisfied with the maintainers'
handling of a rights request may complain to their national data
protection authority. For the EU / EEA, the list is maintained at
<https://www.edpb.europa.eu/about-edpb/about-edpb/members_en>.

## 9. Security of processing

Technical and organisational measures include:

- TLS in transit (documented in `docs/API_POLICY.md`).
- Rate limiting and preflight validation (Pack AB).
- Secret-scanning pre-push and in CI (`gitleaks`).
- SLO burn-rate alerting (Pack AH) as early warning for degraded
  security posture.
- Coordinated vulnerability disclosure via `SECURITY.md`.

Planned but not yet in place — see `docs/HSM_INTEGRATION_PLAN.md`
(Pack AJ): hardware-backed key custody and split-custody key
ceremony.

## 10. Incidents / breach notification

The maintainers will:

- investigate reports received at `khrol.studio@gmail.com` within
  72h of receipt during the hackathon and beta period;
- notify affected operators without undue delay if a personal-data
  breach concerning them is confirmed;
- publish post-incident reviews in `docs/OPS_RUNBOOKS.md` (Pack AH)
  with sensitive details redacted.

The Service is designed so that a compromise of the maintainers'
infrastructure does **not** compromise end-user personal data,
because such data is never processed in the clear (§3).

## 11. Data-flow map

```
+-----------------+        +--------------------+        +---------------------+
| Operator's edge |--(1)-->| CasperProver API   |--(4)-->| Casper testnet      |
|  (raw PD here)  |        |  (hash-only zone)  |        | (Merkle root only)  |
+-----------------+        +--------------------+        +---------------------+
        |                          |
        |                          |
   (2) hash + sign          (3) telemetry
        |                          |
        v                          v
+-----------------+        +--------------------+
| SDK client-side |        | Local observability|
|  Merkle tree    |        |  (Prom / traces)   |
+-----------------+        +--------------------+
```

1. Operator's edge sends only digests (never PD in the clear).
2. Operator SDK hashes and signs locally.
3. Service records aggregated telemetry (§4) with rolling retention.
4. Only the Merkle root reaches the Casper testnet.

## 12. Contact & changes

- **Data-protection contact:** khrol.studio@gmail.com.
- **DPO:** none appointed yet; a DPO will be evaluated at
  commercial launch (Pack AK).
- **Changes:** material updates will be called out in
  `CHANGELOG.md` and dated at the top of this file.

---

*End of DRAFT Data Protection Notice v0.1. This document is a
placeholder for counsel-reviewed terms. Do not rely on it as legal
advice.*
