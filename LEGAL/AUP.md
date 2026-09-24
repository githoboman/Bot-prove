# CasperProver — Acceptable Use Policy

> **Status: DRAFT — self-authored, not reviewed by counsel.**
> This AUP is a good-faith draft. It has **not** been reviewed by
> qualified legal counsel. It will be replaced with a counsel-reviewed
> version before any commercial launch (`docs/MAINNET_LAUNCH_PLAN.md`,
> Pack AK).

**Effective date (draft):** 2026-07-26
**Version:** 0.1-draft
**Contact:** khrol.studio@gmail.com

---

## 1. Purpose

CasperProver ("the Service") exists so that AI agents can produce
verifiable, timestamped, cryptographically anchored evidence of their
decisions. This AUP describes the categories of use that are compatible
with that mission — and the ones that are not.

Users must comply with this AUP in addition to the Terms of Service
(`LEGAL/TOS.md`) and any applicable law. Violations may result in
immediate suspension of API access, revocation of any tenant credentials,
and referral to law enforcement where required.

## 2. Encouraged uses

The Service is designed for, and its honesty labels are calibrated for,
uses such as:

- Regulated-industry AI decision logging (lending, insurance, healthcare
  triage, KYC/AML flagging) where the *fact* and *inputs* of the decision
  must be provable after the fact.
- Multi-agent (A2A) coordination where each agent's contribution to a
  chain of reasoning must be attributable and non-repudiable.
- Human-in-the-loop (HITL) audit trails: proof that an operator
  reviewed and approved (or overrode) a model output at a specific time.
- Model-version accountability: proof that the model identifier used at
  decision time was `X`, so later retraining or drift cannot be silently
  swapped in.
- Research and reproducibility: pinning decision receipts so that
  published results can be independently re-verified.

## 3. Prohibited uses

You must not use the Service, or permit or encourage anyone else to use
the Service, for any of the following:

### 3.1 Illegal or harm-causing activity

- Any activity that is illegal in the operator's or the affected data
  subject's jurisdiction.
- Autonomous or semi-autonomous decisions on **lethal weapon targeting**,
  or any use case where a failed attestation could contribute to
  physical harm to a person, without an independent human safety
  interlock outside the Service.
- Decisions that materially deprive a natural person of a fundamental
  right (liberty, healthcare access, legal representation) without the
  additional human-review and appeal mechanisms required by applicable
  law.

### 3.2 Deceptive attestations

- Submitting decisions with model identifiers, timestamps, or input
  hashes that you know or should know are false, altered, or
  back-dated.
- Stripping, altering, or obscuring the honesty labels
  (`REAL` / `ON-CHAIN` / `SIMULATION`) in outputs presented to third
  parties.
- Representing a testnet-anchored receipt as if it were mainnet-anchored,
  or as if it carried the guarantees of a mainnet chain.
- Selling receipts, or Service access, on the false basis that the
  Service certifies AI model correctness (it does not — see TOS §3).

### 3.3 Abuse of the Service's technical surface

- Exceeding documented rate limits (`docs/API_POLICY.md`, Pack AB), or
  attempting to bypass them via multiple keys / tenants.
- Attempting to induce collisions, replay, or dup-padding on the Merkle
  scheme outside a coordinated security-research context (see §5).
- Attempting to exhaust testnet CSPR or otherwise degrade the anchoring
  contract's availability for other users.
- Reverse-engineering, probing, or attacking the trusted-setup ceremony
  artefacts outside a coordinated security-research context.

### 3.4 Data misuse

- Submitting decisions that contain unhashed personal data in fields
  that will be stored in the clear (see `LEGAL/DATA_PROTECTION.md`,
  §3 on the "hash-only" boundary).
- Submitting special-category personal data (health, biometric, sexual
  orientation, political opinions, etc.) without a lawful basis under
  applicable law and without appropriate additional safeguards.
- Submitting data that you do not have the right to hash and anchor
  (third-party trade secrets, classified information, export-controlled
  material).

### 3.5 Reputation and market manipulation

- Using the reputation-and-slashing sub-system (Pack AL,
  `docs/REPUTATION_DESIGN.md`) to falsely damage the standing of another
  operator you have a commercial dispute with.
- Coordinating with other operators to produce false quorum on a fact
  known to be untrue.

### 3.6 Illegal transfer

- Reselling or sublicensing the Service or receipts in a manner that
  bypasses the Terms of Service.
- Using the Service in a jurisdiction subject to comprehensive sanctions,
  or by any person or entity on a comprehensive sanctions list.

## 4. Content that is neither encouraged nor prohibited

The Service is content-neutral about the *substance* of a decision as
long as (a) the decision is lawful in the jurisdiction of its author,
(b) the honesty labels are preserved, and (c) the data-protection
obligations in §3.4 are met. In particular, controversial-but-legal AI
use cases (advertising personalisation, content moderation decisions,
etc.) are within scope.

## 5. Coordinated security research

Security research on the Service — including attempts to induce Merkle
collisions, to break the anchoring integrity, or to circumvent
rate-limiting — is welcome *if coordinated* through the process described
in `SECURITY.md` (coordinated disclosure, no data exfiltration, no
degradation of service for other users).

Uncoordinated attacks are treated as violations of §3.3.

## 6. Enforcement

The maintainers may, at their discretion, and with or without notice:

- suspend or terminate API access, tenant credentials, or any hosted
  endpoint used to violate this AUP;
- refuse to accept new attestations from the offending party;
- revoke sharing/ACL grants to any wiki or dashboard the offending party
  had access to;
- refer the matter to law enforcement or to a regulator where required
  by applicable law;
- pursue any other remedy available under the Terms of Service or
  applicable law.

For serious violations affecting third parties, the maintainers will
prefer prompt suspension over a warning process.

## 7. Reporting violations

Suspected AUP violations, security issues, or third-party harm caused by
a Service user should be reported to **khrol.studio@gmail.com**. Include
enough detail (receipt IDs, timestamps, operator identifiers) for the
maintainers to reproduce the issue.

## 8. Changes to this AUP

This AUP may be revised at any time by updating this file in the
repository. Material changes will be called out in `CHANGELOG.md`.
Continued use of the Service after a revision constitutes acceptance of
the revised AUP.

---

*End of DRAFT AUP v0.1. This document is a placeholder for
counsel-reviewed terms. Do not rely on it as legal advice.*
