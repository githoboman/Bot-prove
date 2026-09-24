# HSM Plan — Key Custody Roadmap (DRAFT)

> **Status:** DRAFT — not implemented. This document defines the target key-custody
> architecture for CasperProver as it moves from hackathon testnet posture to a
> production-grade signing surface. No HSM is provisioned today.
>
> **Honesty label:** `SIMULATION` — the current codebase signs with software keys
> loaded from environment or filesystem paths. Every claim below is a **plan**, not
> a live capability. Cross-refs: `docs/KEY_CEREMONY_PLAN.md`, `docs/MAINNET_LAUNCH_PLAN.md`
> (forward reference — Pack AK), `LEGAL/DATA_PROTECTION.md`, `LEGAL/TOS.md`.

## 1. Scope

Keys within scope of this plan (every key that ever signs a byte the Service
publishes or the operator relies on):

| Key class | Purpose | Rotation target | Current custody | Target custody |
|---|---|---|---|---|
| `anchor-operator` | Signs Casper anchor deploys (attest Merkle root to chain) | 90 days | ENV / PEM file | HSM PKCS#11 slot, quorum 2-of-3 unwrap |
| `attestation-signer` | Signs receipt bundles handed to callers | 30 days | ENV / PEM file | HSM PKCS#11 slot |
| `slh-dsa-fips205` (post-quantum) | FIPS 205 SLH-DSA-SHA2-128s signing for PQ receipt lane | 180 days | in-memory seed | HSM with PQ-capable slot OR software vault with sealed backup |
| `groth16-vk-sealed` | Verifier key committed alongside ceremony transcript | tied to ceremony epoch | filesystem artifact | filesystem + hash committed on-chain; **not HSM-resident** |
| `groth16-pk-sealed` | Prover key (large, offline) | tied to ceremony epoch | filesystem artifact | offline media + tamper-evident storage; **not HSM-resident** |
| `platform-tls` | Public API TLS termination | 90 days (ACME) or 365 days | ENV / PEM file | HSM PKCS#11 slot OR ACME-managed private key on ephemeral compute |
| `admin-mfa-recovery` | Root break-glass credential | never rotated on-schedule; regenerated on incident | offline paper | offline vault + Shamir 3-of-5 |

Out of scope for HSM: Groth16 `pk`/`vk` artifacts (too large for HSM slot; integrity
is enforced by **commit-and-verify** — see `docs/KEY_CEREMONY_PLAN.md` §5).

## 2. Threat model deltas the HSM must close

Moving from "software key on filesystem" to "HSM-resident key" is meaningless
unless it closes concrete threats. This plan targets:

1. **Filesystem exfiltration** — an attacker with read access to the deploy host
   MUST NOT be able to obtain private key material.
2. **Insider single-actor abuse** — a single operator MUST NOT be able to sign
   an anchor deploy that changes on-chain state without a second quorum member.
3. **Backup exfiltration** — a stolen encrypted backup MUST NOT allow offline
   key extraction below the HSM's rated attack cost.
4. **Silent key substitution** — an attacker who compromises a signing service
   MUST NOT be able to swap in an attacker-controlled key without external
   audit trail correlation.
5. **Ceremony key compromise** — a compromised Phase-2 contribution MUST be
   detectable via the beacon-sealed transcript (see key-ceremony plan §5).

## 3. Candidate options (informational; no commitment)

All candidate evaluations are **research notes** and not endorsements. Any
selection requires:

- explicit approval from the operator (see `LEGAL/TOS.md` §Operator Responsibilities);
- a paid procurement pass reviewed against `docs/MAINNET_LAUNCH_PLAN.md`;
- an incident-response tabletop against `docs/OPS_RUNBOOKS.md` §Incident Response.

### 3.1 Cloud KMS category

Managed KMS services (generic — no vendor named until selection) with PKCS#11 or
gRPC-tunnelled signing endpoints. Trade-offs to score during procurement:

- FIPS 140-2 Level 3 or Level 4 rated backing HSM required.
- Per-key IAM boundary must express "sign only, no export."
- Audit log export must be pull-based (S3-like sink OR immutable append log)
  and stored under `LEGAL/DATA_PROTECTION.md` §Retention.
- Cost floor and latency floor MUST be measured against `deploy/observability/`
  p95 latency SLO before commitment.

### 3.2 Dedicated network HSM category

Standalone appliance (SafeNet, Utimaco, Thales, YubiHSM 2 for the very small
end). Trade-offs:

- Physical custody plan required (data centre or safe-deposit locker).
- Firmware update discipline — no auto-update on production HSMs.
- HA pair required for anchor-operator; single-unit acceptable for
  attestation-signer during pilot.

### 3.3 USB security-key category (bootstrap only)

YubiKey / Nitrokey / SoloKeys with PIV or OpenPGP applet. Explicitly **bootstrap
only** — usable for the first mainnet anchor if the operator judges the risk
proportional, but **not** the steady-state target.

## 4. Selection gates

A candidate MUST clear all gates before it becomes the plan of record:

- **GATE-HSM-1 — Compliance:** written attestation the module meets FIPS 140-2
  Level 3 or higher.
- **GATE-HSM-2 — Isolation:** demonstrated that private key material cannot
  be exported even by an authenticated administrator; only wrapped
  key-encryption-keys leave the module.
- **GATE-HSM-3 — Quorum:** anchor-operator slot supports M-of-N unwrap where
  M≥2, N≥3, and the M actors can be geographically separated.
- **GATE-HSM-4 — Audit:** signature events emit a structured audit record
  compatible with `engine/internal/obs` JSON span format OR the operator's
  chosen SIEM sink.
- **GATE-HSM-5 — Latency:** p95 signing latency measured on-site ≤200ms so the
  API `/attest` p95 SLO (≤750ms) is not consumed.
- **GATE-HSM-6 — Legal:** DPIA delta reviewed against `LEGAL/DATA_PROTECTION.md`;
  DPA in place if the HSM vendor is a processor.

## 5. Integration boundary in the codebase

To keep the code today compatible with any future selection, the plan freezes a
narrow interface. Implementation of the HSM adapter itself is out of scope for
this document — this section is a **contract**, not code.

```go
// engine/internal/keys/signer.go (target — NOT YET IMPLEMENTED)
type Signer interface {
    Sign(ctx context.Context, digest []byte, keyID string) (Signature, error)
    PublicKey(ctx context.Context, keyID string) (PublicKey, error)
    Health(ctx context.Context) error
    // Rotate is intentionally advisory — actual rotation is orchestrated by
    // the key-ceremony workflow, not the Signer.
    Rotate(ctx context.Context, keyID string, reason RotationReason) error
}
```

The current software signer becomes a `SoftSigner` implementing this interface
and clearly marked `SIMULATION` in the honesty ladder; a future `PKCS11Signer`
implements the same interface against the selected HSM. This lets the
`/attest` path be swapped without call-site change.

## 6. Migration steps (owned by operator + maintainers)

1. Complete `docs/MAINNET_LAUNCH_PLAN.md` §Procurement — sign vendor & appliance selection.
2. Provision two units (production + hot spare) or two-region KMS keys.
3. Land the `Signer` interface in `engine/internal/keys/` (currently a stub).
4. Implement `PKCS11Signer` against the selected HSM behind a build tag so
   test builds keep using `SoftSigner`.
5. Rehearse the **first mainnet anchor** using the ceremony playbook in
   `docs/KEY_CEREMONY_PLAN.md` §7, quorum 2-of-3.
6. Rotate the current software `anchor-operator` and `attestation-signer` out
   of service; publish the retirement fingerprint to `deploy-out/onchain.json`.
7. Update `docs/KNOWN_LIMITATIONS.md` to demote the current software-key
   posture from `LIVE` to `HISTORICAL` and promote the HSM lane to `LIVE`.
8. First quarterly audit: reconcile HSM audit log against on-chain deploy
   history; any unexplained sign event is an incident (see
   `docs/OPS_RUNBOOKS.md` §Incident Response).

## 7. What this plan explicitly does NOT do (yet)

- Does not select a vendor.
- Does not procure hardware.
- Does not obligate any spend before the operator green-lights
  `docs/MAINNET_LAUNCH_PLAN.md`.
- Does not change any live signing surface. Every current signer remains
  `SIMULATION`-labelled.

## 8. Cross-references

- `docs/KEY_CEREMONY_PLAN.md` — Groth16 Phase-1/Phase-2 ceremony playbook, which
  produces the ceremony-sealed artifacts referenced here.
- `docs/OPS_RUNBOOKS.md` — incident response for a suspected key compromise.
- `LEGAL/DATA_PROTECTION.md` — HSM audit logs as personal data category boundary.
- `LEGAL/TOS.md` §Operator Responsibilities — who owns key custody in production.
- `docs/KNOWN_LIMITATIONS.md` §Cryptography — current software-key posture.

_Owner: maintainers. Reviewer: operator + external security advisor. Cadence:
re-review at every mainnet epoch boundary or on any incident of SEV-2 or higher._
