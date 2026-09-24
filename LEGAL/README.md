# LEGAL/ — draft legal-and-policy surface

This directory is a first-class part of CasperProver's honesty posture:
we do not want to ship a "verifiable AI attestation" product with no
articulated terms, no acceptable-use rules, and no data-protection
notice. So the maintainers wrote first drafts of each — clearly labelled
**DRAFT** and **not counsel-reviewed** — during the hackathon, and
committed them alongside the code.

Every file here is a placeholder for a counsel-reviewed version, which
is planned for the commercial-launch milestone in
`docs/MAINNET_LAUNCH_PLAN.md` (Pack AK). Until that milestone the
maintainers do not represent these drafts as legal advice.

## Contents

- **[TOS.md](TOS.md)** — Terms of Service draft. Grant of use,
  operator responsibilities, testnet-only status, warranty disclaimer,
  liability, indemnity, governing-law placeholder.
- **[AUP.md](AUP.md)** — Acceptable Use Policy draft. What the Service
  is for, what it is *not* for, prohibited uses, coordinated security
  research, enforcement, reporting.
- **[DATA_PROTECTION.md](DATA_PROTECTION.md)** — GDPR-adjacent data
  protection notice draft. Roles, hash-only architectural boundary,
  operational telemetry with retention schedule, data-subject rights,
  data-flow map, breach notification.

## Honesty labels used here

- **DRAFT** — self-authored by project maintainers, not reviewed by
  counsel; may contain gaps a lawyer would immediately flag.
- **TESTNET-ONLY** — every clause that mentions "on-chain" refers to
  the Casper testnet during the hackathon and beta period.

## Related non-legal surfaces referenced by these documents

| File | Purpose |
|---|---|
| `KNOWN_LIMITATIONS.md` | Honesty checklist of gaps in real / simulation / on-chain claims |
| `docs/API_POLICY.md` (Pack AB) | Rate limits, preflight, versioning |
| `docs/OBSERVABILITY.md` (Pack AG) | Metrics/traces retention, what is collected |
| `docs/OPS_RUNBOOKS.md` (Pack AH) | Incident response and SLO targets |
| `docs/HSM_INTEGRATION_PLAN.md` (Pack AJ) | Hardware key custody plan (draft) |
| `docs/KEY_CEREMONY_PLAN.md` (Pack AJ) | Split-custody key ceremony plan (draft) |
| `docs/MAINNET_LAUNCH_PLAN.md` (Pack AK) | Paid-services roadmap incl. counsel review |
| `SECURITY.md` | Coordinated vulnerability disclosure |
| `docs/MERKLE_SCHEME.md` | Domain separation & known deviations from RFC 6962 |

## Contact

Legal notices, data-subject requests, and security disclosures:
**khrol.studio@gmail.com**.
