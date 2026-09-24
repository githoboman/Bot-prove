# CasperProver — Terms of Service

> **Status: DRAFT — self-authored, not reviewed by counsel.**
> This document is a good-faith draft prepared by the project maintainers to
> describe the intended terms under which CasperProver software and any
> hosted endpoints are made available during the hackathon and early beta
> period. It has **not** been reviewed by qualified legal counsel and must
> not be relied upon as legal advice. Before any commercial launch, this
> document will be replaced with counsel-reviewed terms. See
> `docs/MAINNET_LAUNCH_PLAN.md` (Pack AK) for the paid-legal-review
> milestone.

**Effective date (draft):** 2026-07-26
**Version:** 0.1-draft
**Contact:** khrol.studio@gmail.com

---

## 1. Definitions

- **"Service"** — the CasperProver software, including the Go SDK, the MCP
  server, the on-chain smart contracts deployed on the Casper Network
  testnet, and any HTTP APIs or dashboards operated by the project
  maintainers.
- **"You" / "Operator"** — the natural person or legal entity that installs,
  runs, integrates, or otherwise makes use of the Service.
- **"Attestation"** — a cryptographic proof (Merkle inclusion proof, ZK
  proof, or signature) that a given AI-agent decision was recorded at a
  specific time with specific inputs, outputs and model identifier.
- **"Receipt"** — the JSON artefact returned by the Service that binds an
  Attestation to on-chain evidence (currently: a Casper testnet Merkle-root
  anchoring transaction).
- **"Testnet"** — the Casper Network testnet used exclusively during the
  hackathon and beta period. **The Service does not currently touch
  mainnet.** Any language in this document that references "on-chain"
  refers to the testnet unless explicitly stated.

## 2. Grant of use

Subject to your compliance with these Terms and the Acceptable Use Policy
(`LEGAL/AUP.md`), the maintainers grant you a non-exclusive, non-transferable,
revocable licence to install and run the Service under the source-repository
licence (see the root `LICENSE` file) and to submit hashed decision records
to the testnet contracts operated by the project.

## 3. What the Service is — and is not

The Service is a **verifiable-attestation layer**. It produces cryptographic
evidence about *what an AI agent decided, given what inputs, at what time,
using what model*. It is **not**:

- an oracle for the correctness of any AI decision;
- an audit or certification of any AI model, dataset, or process;
- a substitute for regulatory review (medical, financial, legal, etc.);
- an insurance product or a guarantee of any economic outcome;
- a mainnet-anchored production system (see §1: testnet-only).

Honesty labels used in the codebase (`REAL`, `ON-CHAIN`, `SIMULATION`) apply
to the outputs of the Service and must not be stripped or altered when the
Service is redistributed or embedded.

## 4. Testnet-only status

You acknowledge that:

- All on-chain anchoring happens on the Casper Network **testnet**, using
  test-CSPR tokens with no monetary value.
- Testnet contracts may be re-deployed, wiped, or reset at short notice by
  the network operator or the project maintainers.
- Historical receipts remain cryptographically verifiable off-chain via the
  local Merkle scheme (`docs/MERKLE_SCHEME.md`) even if the testnet chain
  state is reset, but the on-chain half of the receipt may become
  unresolvable.

Any promotional or commercial use that implies mainnet-grade guarantees is a
violation of these Terms.

## 5. Operator responsibilities

You are responsible for:

1. **Lawful inputs.** Not submitting data you do not have the right to
   process (personal data without a lawful basis, protected health
   information without appropriate safeguards, export-controlled data,
   etc.).
2. **Consent.** Obtaining any consent from data subjects that applicable
   law (GDPR, HIPAA, etc.) requires before hashing decisions that concern
   them. See `LEGAL/DATA_PROTECTION.md`.
3. **Key custody.** Safeguarding your own signing and wallet keys. The
   maintainers never receive them (see the key-management plans in
   `docs/HSM_INTEGRATION_PLAN.md` and `docs/KEY_CEREMONY_PLAN.md`, both
   drafts).
4. **Regulatory fit.** Assessing whether attestations produced by the
   Service meet the evidentiary or regulatory bar of your use case. The
   Service is a *building block*, not a certification.
5. **Rate and cost limits.** Respecting the API rate limits documented in
   `docs/API_POLICY.md` (Pack AB) and not attempting to bypass them.

## 6. Prohibited uses

See `LEGAL/AUP.md` for the full list. In summary: no illegal use, no abuse
of the anchoring surface for spam/DoS, no attempts to compromise the
integrity of the receipt chain, no attempts to launder false attestations
through the honesty-label system.

## 7. Availability, changes, discontinuation

The Service is provided on a best-effort basis with **no SLA** during the
hackathon and beta period. Documented SLO targets (`docs/OPS_RUNBOOKS.md`,
Pack AH) describe *aspiration*, not contractual commitment. The maintainers
may modify, suspend, or discontinue any part of the Service at any time,
including deprecating specific attestation schemes when superseded by
better cryptography.

Deprecation of an on-chain contract will be announced in the repository
`CHANGELOG.md`, with a documented migration path. Receipts produced before
deprecation remain verifiable off-chain.

## 8. Warranty disclaimer

TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE LAW, THE SERVICE IS PROVIDED
"AS IS" AND "AS AVAILABLE", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO IMPLIED WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE, NON-INFRINGEMENT, AND ACCURACY OR
CORRECTNESS OF CRYPTOGRAPHIC PROOFS UNDER ADVERSARIAL CONDITIONS THAT HAVE
NOT YET BEEN INDEPENDENTLY AUDITED.

Known limitations that qualify this disclaimer are enumerated in
`KNOWN_LIMITATIONS.md`. That file is a first-class part of the honesty
posture; you agree to read it before deploying the Service into anything
you would not describe as "an experiment".

## 9. Limitation of liability

TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE LAW, IN NO EVENT SHALL THE
MAINTAINERS BE LIABLE FOR ANY INDIRECT, INCIDENTAL, SPECIAL, CONSEQUENTIAL
OR PUNITIVE DAMAGES, OR ANY LOSS OF PROFITS OR REVENUES, WHETHER INCURRED
DIRECTLY OR INDIRECTLY, OR ANY LOSS OF DATA, USE, GOODWILL, OR OTHER
INTANGIBLE LOSSES, RESULTING FROM (a) YOUR USE OR INABILITY TO USE THE
SERVICE; (b) ANY UNAUTHORISED ACCESS TO OR USE OF THE SERVICE; (c) ANY
CONDUCT OR CONTENT OF ANY THIRD PARTY ON THE SERVICE; OR (d) TESTNET STATE
LOSS, RESET, OR REORGANISATION.

## 10. Indemnity

You agree to defend, indemnify and hold harmless the maintainers from and
against any and all claims, liabilities, damages, losses, and expenses
arising out of or in any way connected with (a) your violation of these
Terms or the AUP; (b) your use of the Service in a regulated context
without appropriate additional safeguards; or (c) your representation to
third parties of Service outputs beyond what the honesty labels support.

## 11. Governing law and dispute resolution

**Draft placeholder.** The final governing-law clause will be determined
during counsel review. Until then, the maintainers do not consent to any
particular jurisdiction, and this section is expressly non-binding.

## 12. Changes to these Terms

The maintainers may revise these Terms at any time by updating this file
in the repository. Continued use of the Service after a revision
constitutes acceptance of the revised Terms. Material changes will be
called out in `CHANGELOG.md`.

## 13. Contact

For legal notices, data-subject requests, or security disclosures:
**khrol.studio@gmail.com** (see also `SECURITY.md` for coordinated
vulnerability disclosure).

---

*End of DRAFT TOS v0.1. This document is a placeholder for
counsel-reviewed terms. Do not rely on it as legal advice.*
