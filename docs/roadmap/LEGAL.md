# Legal, Compliance, Retention, Third-Party Audit

Ref: `handoff/CP_FINAL_TASKS_V2.md` §E.

## Documents to publish

- **Terms of Service.** Draft under legal counsel review, targeting the
  regulated AI/DeFi buyer profile. Standard SaaS carve-outs (no
  reverse-engineering, no illegal use, force majeure, indemnification).
- **Privacy Policy.** Data collected, purpose, retention, sub-processors,
  data subject rights (GDPR / UK GDPR / CCPA compatible).
- **Data Processing Agreement (DPA).** Standard SCCs, sub-processor list.
- **Retention Policy.** See below.
- **Acceptable Use Policy.**

All documents live under `docs/legal/` and are versioned per publication.

## Retention

| Data class                          | Retention | Notes                                                          |
|-------------------------------------|-----------|----------------------------------------------------------------|
| Decision receipts (Postgres)        | 400 days  | Longer than typical audit-review windows; configurable per plan|
| Decision receipts (object storage)  | 7 years   | Cold, immutable; per financial-audit norms                     |
| Audit log                           | 400 days  | See `docs/roadmap/MULTITENANCY.md#audit-log`                   |
| API access log                      | 90 days   | Rolls off after quota rollup completes                         |
| Webhook delivery log                | 90 days   | Enough for retry cap + investigation                           |
| HITL evidence blobs                 | 7 years   | Regulatory audit                                               |
| Tenant PII                          | Lifetime of contract + 30 days | Then cryptographic shred                            |
| Backups                             | 30 days   | Cross-region                                                   |

Cryptographic shred = destroy the wrapping DEK; ciphertext becomes
unrecoverable. Documented per NIST SP 800-88 rev 1.

## Third-party cryptography / security audit

- **Scope:** engine crypto surface (`engine/internal/crypto/`,
  `engine/internal/prover/`, gnark integration, ML-DSA integration,
  Lamport OTS implementation), contract surface (`contracts/*/`),
  API auth + rate-limit + tenancy middleware.
- **Vendor selection:** issue RFP to ≥ 3 recognised firms (e.g. Trail of
  Bits, Kudelski, Zellic, OtterSec). Do NOT commit vendor names in copy
  until a contract is signed and a report is linked.
- **Timeline:** RFP by day 60 post-hackathon-day-30; report by day 150.
- **Publication:** full report under `docs/audits/`. Redactions only for
  actively-exploited findings still under remediation.

## Regulatory posture

- Not offering financial services directly. Buyers integrate CP into
  their own regulated flow.
- No token issuance.
- No custody. CP does not hold customer keys other than the tenant API
  keys (server-side hashed).
- Export controls: the crypto surface uses only NIST-standardised or
  peer-reviewed primitives; no ITAR / EAR concerns beyond ordinary SaaS.

## Milestones

1. **Legal counsel engaged (5 days).**
2. **First-draft ToS / Privacy / DPA (30 days).**
3. **Retention policy implemented in the engine (10 days).**
4. **Audit RFP out (10 days).**
5. **Audit contract signed (30 days).**
6. **Audit report + remediation (60–90 days).**

## Non-goals

- Formal SOC 2 Type II. Roadmap (post-180-day). Type I as a stretch goal.
- ISO 27001. Roadmap.
- HIPAA. Not the buyer profile.

## Acceptance criteria

- [ ] `docs/legal/{TERMS,PRIVACY,DPA,RETENTION,AUP}.md` present and
      versioned.
- [ ] Retention enforced automatically by the engine's data-lifecycle
      cron.
- [ ] `docs/audits/<vendor>-<date>.pdf` published.
- [ ] Sub-processor list current in the Privacy Policy.
