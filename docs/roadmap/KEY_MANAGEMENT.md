# Production Key Management, HSM, Evidence Retention

Ref: `handoff/CP_FINAL_TASKS_V2.md` §E.

## Key inventory

| Key                                | Purpose                                         | Storage                         |
|------------------------------------|-------------------------------------------------|---------------------------------|
| Engine signing key (Ed25519)       | Sign decision receipts                          | HSM (YubiHSM 2 or cloud KMS)    |
| Engine signing key (ML-DSA-65)     | PQ half of hybrid signature                     | HSM (with software fallback)    |
| Ceremony coordinator key           | Sign ceremony transcript entries                | HSM                             |
| Governance multi-sig keys          | Emergency pause, ownership recovery             | Distributed hardware (m-of-n)   |
| Casper deploy keys                 | Deploy / upgrade contracts                      | HSM + m-of-n approval           |
| DB DEKs (data-encryption keys)     | Encrypt PII columns at rest                     | Wrapped by KMS root; rotated qtr|
| Object storage encryption keys     | Encrypt receipt / evidence blobs at rest        | KMS-managed                     |
| Tenant API-key hashing salt        | Compute SHA-256 hash of raw keys                | KMS-derived, per-tenant         |
| Webhook shared secrets             | HMAC over delivery bodies                       | KMS-encrypted at rest           |
| TLS private keys                   | Terminate HTTPS                                 | ACME + KMS-managed              |

## HSM / KMS layering

- **Root of trust:** HSM (YubiHSM 2 for on-prem, cloud KMS root for
  cloud-hosted). Root keys never leave the HSM.
- **Data-encryption keys (DEKs):** short-lived, wrapped by the HSM root.
  Cached in-process with strict lifetime (≤ 1h).
- **Rotation cadence:**
  - Engine signing keys: annually, with grace overlap.
  - DEKs: quarterly.
  - Tenant API keys: on-demand (user-triggered rotation).
- **Ceremony key:** rotates per ceremony round; old key is verifiably
  destroyed (transcript records the destruction event with a witness
  signature).

## Access control

- HSM operations require m-of-n human approval for privileged actions
  (governance emergency pause, owner recovery, ceremony sealing).
- Break-glass procedure documented in `docs/runbooks/break-glass.md`;
  every use triggers a mandatory postmortem.

## Evidence retention

Evidence blobs (facet outputs, HITL resolutions, receipt Merkle roots)
are the crown jewels: they justify every on-chain commitment.

- **Storage:** content-addressed object storage; ciphertext at rest with
  a per-blob DEK.
- **Integrity:** every blob is signed by the issuing engine + hashed into
  the receipt Merkle root; the on-chain anchor closes the loop.
- **Retention:** see `docs/roadmap/LEGAL.md#retention`.
- **Deletion:** cryptographic shred — destroy the wrapping DEK, keep the
  ciphertext for regulatory-audit continuity.

## Audit logging

Every HSM operation logs:

- Requester identity (human or service account).
- Operation type.
- Input digest (never the plaintext).
- Timestamp with monotonic + wall clock.
- Approver signatures for m-of-n operations.

Log is append-only + KMS-signed + shipped to cold storage nightly.

## Milestones

1. **HSM provisioning (10 days).** YubiHSM 2 for the on-prem story;
   cloud KMS for the hosted story. Both live in staging first.
2. **DEK / wrapping / rotation pipeline (15 days).**
3. **m-of-n approval workflow (10 days).**
4. **Break-glass runbook + drill (5 days).**
5. **Evidence retention wired (10 days).**

## Non-goals

- Full customer-managed keys (BYOK). Roadmap.
- Threshold-signing at the tenant boundary (as opposed to at the
  governance boundary). Roadmap.

## Acceptance criteria

- [ ] Every key in the [Key inventory](#key-inventory) has a documented
      storage + rotation policy.
- [ ] HSM audit log queryable + shipped to cold storage.
- [ ] Break-glass drill passes.
- [ ] Evidence blobs cryptographically shreddable per the retention
      table in `docs/roadmap/LEGAL.md`.
