# Confidential Storage Layer — DRAFT plan

> **Status.** DRAFT. Off-repo design document. Not shipped as code. Not
> counsel-reviewed. No paid services referenced. Testnet posture unchanged.

## 0. Why this plan exists

CasperProver's core architectural boundary is **hash-only**: the Service
observes SHA-256 (and, in AC, SLH-DSA / VRF-tagged) commitments of agent
inputs, outputs, model ids, and timestamps. It does **not** observe the
plaintext. That property is the reason `LEGAL/DATA_PROTECTION.md` can
credibly claim data-minimisation — the controller (Operator) never hands
raw PD to the Service in the first place.

Some Operators cannot stop there:

- A **regulator** (or an internal auditor) may need to reproduce the
  attested decision from primary inputs, not just verify a commitment.
- A **counterparty** in a bilateral A2A workflow may need selective
  disclosure of specific fields (e.g. "the credit score used, but not the
  applicant's SSN").
- An **incident-response team** replaying a suspicious decision needs the
  actual payload, but only for authorised roles, only for a bounded time,
  and only with a full audit trail of who saw what.

This document is the roadmap for a **Confidential Storage Layer** (CSL)
that sits **beside** the current Service — never inside its hot path —
so that Operators who need reproducibility can opt in without eroding
the hash-only property for Operators who don't.

## 1. Scope (what this plan is)

- Architecture, threat model, key-management ties, storage-provider
  abstraction, retention model, disclosure workflow, DPIA delta.
- Explicit forward-refs to code contracts that make the future swap
  mechanical (interface first, provider later).
- A **honesty ladder**: what stays `SIMULATION`, what would become
  `REAL / OFF-CHAIN` once implemented, and what would remain out of
  scope indefinitely.

## 2. Non-scope (what this plan is NOT)

- Not a vendor selection. No provider procurement, no spend authorised.
- Not a mainnet dependency. AK gate ledger does **not** require CSL —
  hash-only Service can go GA without it.
- Not a general-purpose object store. CSL exists to serve reproducibility
  of already-attested decisions; it is not a data lake.
- Not a change to the on-chain surface. Nothing new is anchored.

## 3. Architectural boundary

```
+---------------------------+           +---------------------------+
|    Operator app / agent   |   TLS     |   CasperProver Service    |
| (data controller)         +---------->+   (hash-only path)        |
|                           |           |   /attest, /verify        |
+------------+--------------+           +---------------------------+
             |                                        |
             | (parallel, out-of-band)                | commitments only
             v                                        v
+---------------------------+           +---------------------------+
|   Confidential Storage    |           |   Casper testnet /        |
|   Layer (CSL) — OPTIONAL  |           |   anchor stubs (AA)       |
|   Encrypted payload blobs |           +---------------------------+
|   keyed by receipt digest |
+---------------------------+
```

Key property: the Service **never reads** from CSL on its hot path. A
verifier reproducing a decision fetches the payload from CSL, computes
the commitment locally, and only then calls `/verify` — CSL is not a
trusted oracle for the Service.

## 4. Data model

Each CSL object is:

- `receipt_digest` — the same commitment the Service already stamps on
  its Receipt. Used as the object key.
- `envelope` — authenticated encryption of the payload. Recommended:
  AES-256-GCM with 96-bit random nonce, or XChaCha20-Poly1305; both are
  in FIPS-relevant profiles.
- `wrapped_dek` — the per-object data-encryption key wrapped by one or
  more Operator KEKs (see §5).
- `manifest` — non-secret metadata: created_at (UTC), field schema
  version, retention class, disclosure policy id, integrity chain to the
  previous manifest (Merkle-chain over manifests, same primitive AE
  already uses for provenance).
- `access_log` — append-only record of every successful decryption
  attempt: subject id (never a raw name — always a role or hashed id),
  authorisation reference, timestamp, purpose code (see §7).

The manifest and access log are the only fields the Service is allowed
to *read* during an audit assist; it never sees `envelope` or the
unwrapped DEK.

## 5. Key management ties

CSL keys are governed by the same rules as anchor/attestation keys:

- **KEK lives in HSM** on the path defined in `docs/HSM_PLAN.md` (Gate
  G3 of `docs/MAINNET_LAUNCH_PLAN.md`). Until G3, KEK sits on the
  Operator machine (`SIMULATION` label on CSL). The `Signer` interface
  in AJ generalises to a `Wrapper` interface with the same swap
  discipline — no call-site changes when the KEK moves into HSM.
- **DEK is per-object**, generated via CSPRNG, wrapped by KEK, and
  discarded from memory after wrap. Rotating the KEK re-wraps DEKs in
  place; envelopes stay untouched.
- **M-of-N** disclosure quorum (see §7) mirrors the ceremony pattern
  from `docs/KEY_CEREMONY_PLAN.md`: no single actor can unlock a payload
  post-launch. Pre-launch (testnet) the quorum is 1.

## 6. Storage-provider abstraction

To keep the roadmap vendor-neutral:

```
// engine/internal/csl/store.go   (target — not yet implemented)
type Store interface {
    Put(ctx, key ReceiptDigest, envelope, wrappedDEK, manifest) error
    Get(ctx, key ReceiptDigest, auth Grant) (envelope, wrappedDEK, manifest, error)
    ListManifests(ctx, filter) iter.Seq[Manifest]  // no envelope access
    Delete(ctx, key ReceiptDigest, warrant Warrant) error
}
```

Candidate providers (informational only, no selection):

- **Local encrypted filesystem** — bootstrap posture, single-Operator,
  useful for development and for pilot regulator sandboxes.
- **Object storage with server-side encryption** — providers that
  support customer-managed keys and immutability locks; CSL still holds
  the KEK, so provider compromise ≠ payload compromise.
- **Dedicated append-only vault appliance** — for Operators who already
  run one for other regulated workloads.

Every candidate is subject to the same six selection gates as HSM
(FIPS validation, isolation, quorum, audit sink, latency, DPIA fit) —
promoted verbatim from `docs/HSM_PLAN.md` §6 into the CSL selection
checklist when a candidate is proposed.

## 7. Disclosure workflow

Retrieval is **never** a raw `Get` — it is a workflow:

1. **Warrant.** A disclosure request cites a receipt_digest, a purpose
   code (`audit`, `dispute`, `incident`, `subject_access`), and a
   references field pointing at the legal or contractual basis.
2. **Authorisation.** M-of-N holders of KEK shares co-sign the warrant.
   Testnet default M-of-N = 1-of-1; pilot = 2-of-3; mainnet target =
   3-of-5 minimum.
3. **Access log entry.** Warrant + co-signers + purpose + time land in
   the manifest's access log **before** the DEK is unwrapped.
4. **Bounded exposure.** The unwrapped payload is delivered to a
   time-boxed reviewer session (default 15 minutes, extendable once);
   session end zeroises memory and logs closure.
5. **Post-review artifact.** The reviewer files an outcome record
   (found / not found / escalated). The outcome ties back to the
   originating warrant id in an append-only journal.

Purpose codes are declared in advance (`schema v0.1`) and enforced by
the CSL server, not by the client. Adding a purpose code is a policy
change, not a runtime toggle.

## 8. Retention

CSL retention **piggybacks on `LEGAL/DATA_PROTECTION.md`** to avoid
drift:

| Class          | Default retention | Notes                                   |
|----------------|-------------------|-----------------------------------------|
| audit-primary  | 5 years           | Regulator-facing reproducibility        |
| dispute        | 2 years           | Counterparty A2A disputes               |
| incident       | 90 days           | Aligns with §Security in DATA_PROTECTION |
| ephemeral      | 24 hours          | Debug / QA payloads, never sent to prod |
| legal-hold     | until released    | Overrides all above; requires warrant   |

Retention deletion is **not a `DELETE`**; it is a *destructive re-wrap*
where the DEK is discarded and the envelope is left as an integrity
witness (the manifest still proves the object existed, but the payload
is provably unrecoverable). This preserves the audit chain while
honouring right-to-erasure.

## 9. Threat model

| Threat                                     | Property that defeats it                          |
|--------------------------------------------|---------------------------------------------------|
| Storage-provider compromise                | Envelopes are AE-encrypted; KEK sits outside      |
| Operator laptop exfil (KEK, testnet)       | Testnet-only; label `SIMULATION`; §11 disclosure  |
| Single insider unlocks payload             | M-of-N quorum on warrants (mainnet target)        |
| Retention bypass                           | Destructive re-wrap; envelope no longer decryptable |
| Silent payload substitution                | Manifest Merkle-chain; anchor-witnessed hash roots |
| Coerced disclosure without warrant         | KEK physically absent from CSL server; refusal    |
| Backup exfil                               | Backups store envelopes only; KEK never leaves HSM |
| Log erasure to hide access                 | Access-log entries anchored via same Merkle chain |

## 10. Interaction with existing packs

- **AA** anchor stubs: CSL does not add a new anchor. Its manifest
  Merkle-chain roots MAY be anchored under the existing `provenance`
  slot; no new contract.
- **AB** API hardening: CSL has its own admin surface (`/csl/*`) that
  inherits the same middleware chain (auth, rate-limit, quotas). No
  paths added to the Service hot path.
- **AC** VRF / range proofs: unrelated primitive; CSL orthogonal.
- **AD** SLH-DSA: warrant co-signatures MAY use SLH-DSA once §11.3 in
  HSM_PLAN is landed. Ed25519 acceptable pre-G3.
- **AE** Merkle provenance vectors: CSL manifests SHOULD emit the same
  provenance record shape so a single `verify.sh` covers both surfaces.
- **AF** Phase-2 ceremony: KEK generation MAY use the ceremony
  primitives; separate ceremony instance, not the anchor ceremony.
- **AG** observability: CSL exposes `csl_disclosures_total`,
  `csl_disclosure_latency_seconds`, `csl_warrant_denials_total` on
  `/metrics`. Registered with the same registry, cardinality-bounded.
- **AH** SLO / runbooks: warrant denials and manifest chain breaks are
  SEV-1 candidates; runbook stub prepared as forward-ref.
- **AI** LEGAL: DATA_PROTECTION.md is authoritative on retention;
  TOS.md and AUP.md constrain who may hold KEK shares.
- **AJ** HSM / ceremony: Wrapper (KEK) sits on same G3 HSM plan.
- **AK** mainnet launch: CSL is **not** on the G1–G8 gate ledger —
  Operators may launch without it. If CSL is offered post-launch, it
  gets its own gated rollout mirroring AK §4.

## 11. Honesty ladder

- Today: `SIMULATION`. No CSL code shipped.
- After code lands (post-hackathon): `REAL / OFF-CHAIN` for envelopes;
  `SIMULATION` for M-of-N quorum until G3 HSM.
- After G3 HSM: `REAL / OFF-CHAIN` end-to-end for KEK custody.
- After Operator-side counsel sign-off on their own controller
  obligations: still not `REGULATED`. That label is Operator-scoped and
  the Service will never claim it on their behalf.

## 12. What this plan does NOT do

- Does not authorise procurement.
- Does not commit to a schedule.
- Does not extend the Service hot path.
- Does not change the on-chain surface.
- Does not add a new trust boundary the Service depends on.
- Does not exempt any Operator from their controller obligations under
  `LEGAL/DATA_PROTECTION.md`.

## 13. Open questions

1. Should CSL manifests be optionally anchored, or always? (Bias:
   optional, Operator choice — some Operators cannot leak per-decision
   metadata via anchor cadence.)
2. What is the minimum M-of-N for pilot vs mainnet? (Bias: pilot 2-of-3,
   mainnet 3-of-5, both re-reviewed after first ceremony.)
3. Do we support cross-Operator disclosure (federated warrants)?
   (Bias: no in v1 — bilateral only.)
4. What is the escrow story for legal-hold KEK shares? (Bias: hardware
   escrow via the same HSM candidate; no cloud escrow.)
5. Do we ever offer a "CSL-managed" service where the Service holds a
   KEK share? (Bias: strong no — breaks the hash-only guarantee.)

Answers land here (in successive DRAFT revisions) or in a follow-up
pack, before any code lands.

## References

- `docs/HSM_PLAN.md` (AJ) — key custody & Signer/Wrapper interface
- `docs/KEY_CEREMONY_PLAN.md` (AJ) — quorum ceremony
- `docs/OPS_RUNBOOKS.md` (AH) — incident response envelope
- `docs/MAINNET_LAUNCH_PLAN.md` (AK) — gate ledger, phased rollout
- `LEGAL/DATA_PROTECTION.md` (AI) — retention authority, DPIA
- `LEGAL/TOS.md` (AI) — permitted-use clauses that reference disclosure
- `LEGAL/AUP.md` (AI) — prohibitions on misuse of disclosure workflow
- `docs/KNOWN_LIMITATIONS.md` — honesty labels
