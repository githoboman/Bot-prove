# MAINNET_LAUNCH_PLAN.md

> **STATUS: DRAFT — PLANNING ONLY.**
> This document is a forward-looking roadmap for a possible mainnet
> deployment. Nothing in this plan is scheduled, funded, or approved.
> Today the project is **testnet-only** and there is **no live mainnet
> surface**. This plan exists so that (a) the honesty ladder is
> discoverable from the front door, (b) later gate work does not have to
> reinvent scope, and (c) prospective partners and counsel have a single
> artifact to point at instead of scattered promises.
>
> Cross-refs: `docs/KNOWN_LIMITATIONS.md` (what is live vs planned),
> `LEGAL/TOS.md` and `LEGAL/DATA_PROTECTION.md` (legal posture),
> `docs/HSM_PLAN.md` (key custody), `docs/KEY_CEREMONY_PLAN.md`
> (multi-party ceremony), `docs/OPS_RUNBOOKS.md` (blue/green + incident
> response), `deploy/observability/alerts/` (SLO alert rules).

---

## 0. TL;DR

Mainnet is **not a switch**. It is the cumulative outcome of eight
independent gates. Each gate has an explicit owner, an explicit exit
criterion that can be evaluated by an outside reader, and an artifact
that has to exist and be signed off before the gate is called closed.

Until every gate is closed, the honesty labels in the repo
(`REAL / ON-CHAIN / SIMULATION`) must not be changed to imply a live
mainnet surface. In particular, no marketing, no docs, and no code
comment may state or imply that CasperProver runs on mainnet before
Gate 8 is closed.

This plan itself is **not sufficient** to launch. The gate criteria
below are the launch contract; this document is the map.

---

## 1. Scope of "mainnet"

For the purpose of this plan, **mainnet** means all of the following
simultaneously:

- Anchor commitments (Merkle roots + minimal metadata) are written to
  the **Casper Network mainnet** — not testnet, not a devnet, not a
  local single-node fork.
- The public API surface (`/attest`, `/verify`, `/audit/*`) is
  reachable at a **stable public hostname** with valid CA-issued TLS.
- The service is **operated under a legal entity** with an executed
  ToS, AUP, DPA template, and jurisdictional filings appropriate to
  the operator's country of incorporation.
- The signing keys used to anchor to mainnet live in an **HSM-backed
  custody model** (see `docs/HSM_PLAN.md`), never on operator laptops
  or in-repo secrets.
- At least one **independent third party** — security auditor and
  counsel — has signed off on the launch package.

If any one of the above is not true, the deployment is **not** mainnet
under this plan, regardless of what a URL or a marketing page says.

---

## 2. Non-goals of this plan

Called out explicitly so this document is not misread as an execution
plan or a commitment:

- **This plan does not schedule a launch date.** No calendar dates are
  fixed. Gate closures unlock the next gate; the last gate unlocks the
  launch decision, which is a separate go/no-go review.
- **This plan does not select vendors.** No cloud provider, no HSM
  vendor, no auditor, no law firm, no insurance broker is named.
  Vendor selection is a separate follow-up covered by Gate 3 and
  Gate 5.
- **This plan does not commit spend.** No procurement, no contracts,
  no paid subscriptions are authorized by this document. Everything
  in this plan is achievable today with **zero paid services**; paid
  work only begins when the operator explicitly signs a procurement
  approval outside this repo.
- **This plan does not alter the current live surface.** Everything
  currently live remains testnet-only until Gate 8.
- **This plan does not bind counsel.** All legal statements in this
  document are drafting for counsel to review; nothing here is legal
  advice.

---

## 3. Gate ledger

Eight gates, ordered by rough dependency, not by calendar. A gate is
**open** until an artifact exists that satisfies its exit criterion and
is signed off by the named owner role. Once all eight are closed, the
operator holds a **launch review** which is a separate go/no-go
meeting.

### Gate 1 — Testnet stability

- **Owner:** Engineering.
- **Purpose:** Prove the system holds up under sustained real-world
  load on testnet before considering a mainnet surface.
- **Exit criterion:**
  - `verify.sh` pass rate ≥ 99% over a rolling 30-day window on
    testnet.
  - No SEV-1 or SEV-2 incident open (see `docs/OPS_RUNBOOKS.md`
    incident ladder) at the moment of gate review.
  - SLO alert rules from `deploy/observability/alerts/slo.alerts.yml`
    have fired at least once and been resolved, so the alert path is
    proven end-to-end (not just syntactically valid).
- **Artifact:** A signed testnet stability report referencing the
  Prometheus recording rule outputs, the incident log, and the
  operator's sign-off.

### Gate 2 — Independent security audit

- **Owner:** Security lead.
- **Purpose:** External review of the cryptographic and web surfaces
  by a party with no equity in the project.
- **Scope of audit:**
  - Anchor payload construction and Merkle root binding.
  - Key custody model as defined in `docs/HSM_PLAN.md`.
  - Signature schemes actually used at anchor time (Ed25519 baseline;
    SLH-DSA / FIPS 205 hybrid if enabled).
  - API auth, rate limiting, and input validation on all mutating
    endpoints.
  - Threat model deltas in `docs/HSM_PLAN.md` and this document.
- **Explicitly out of scope for Gate 2:** ZK circuit soundness beyond
  the toy circuits currently shipped. Full-circuit ZK ML soundness is
  its own multi-party review and is **not** gated by mainnet launch
  (see §7 "What is explicitly not launched").
- **Exit criterion:**
  - All findings at High or Critical severity have been remediated,
    retested by the auditor, and closed in writing.
  - Medium findings have a written accept/mitigate decision from the
    security lead.
- **Artifact:** Auditor's final report, remediation log, sign-off
  letter. Retained per `LEGAL/DATA_PROTECTION.md` retention schedule.

### Gate 3 — HSM procurement and pilot

- **Owner:** Security lead + Operations.
- **Purpose:** Move every signing key that will touch mainnet off
  filesystem-resident storage and into HSM-backed custody, per
  `docs/HSM_PLAN.md`.
- **Scope:**
  - Vendor category selection (managed KMS vs on-prem appliance vs
    hybrid) — driven by the 6 selection gates in `docs/HSM_PLAN.md`
    §"Selection gates".
  - Procurement, install, and pilot on **testnet only** for a minimum
    of one full rotation cycle per key class.
  - `engine/internal/keys/signer.go` `Signer` interface implemented
    against the chosen HSM without changing any call site. No
    behavioural regression allowed at `/attest` p95 latency.
- **Exit criterion:**
  - Zero signing operations on mainnet keys happen outside the HSM
    boundary, verified by audit log inspection over a rolling 7-day
    window during pilot.
  - HSM audit sink integrates with `engine/internal/obs` so key
    operations show up as observable events, not opaque calls.
  - DPIA delta versus `LEGAL/DATA_PROTECTION.md` has been reviewed by
    counsel and either shows no change or has been amended.
- **Artifact:** HSM operations runbook, key inventory, pilot report.

### Gate 4 — Multi-party key ceremony

- **Owner:** Ceremony coordinator (see `docs/KEY_CEREMONY_PLAN.md`).
- **Purpose:** Upgrade the current single-coordinator ceremony (Pack
  AF) to a real multi-party ceremony for every key that will sign
  mainnet payloads.
- **Exit criterion:**
  - Ceremony has been run per `docs/KEY_CEREMONY_PLAN.md` with
    ≥ 11 independent contributors and ≥ 2 auditors.
  - Beacon binding to an announced external randomness source is
    verifiable by any outside observer against the ceremony
    transcript.
  - Ceremony transcript, contributor attestations, and auditor reports
    are archived per `LEGAL/DATA_PROTECTION.md`.
- **Artifact:** Ceremony transcript bundle, auditor sign-off, public
  ceremony announcement page.

### Gate 5 — Legal readiness

- **Owner:** Operator + external counsel.
- **Purpose:** Move the LEGAL drafts from DRAFT to counsel-reviewed
  and executable.
- **Exit criterion:**
  - `LEGAL/TOS.md`, `LEGAL/AUP.md`, and `LEGAL/DATA_PROTECTION.md`
    have been reviewed by counsel qualified in the operator's
    jurisdiction, and the DRAFT marker has been replaced with a
    counsel-review annotation showing reviewer, date, and version.
  - Governing-law and dispute-resolution placeholders are filled in.
  - A separate `LEGAL/DPA.md` template has been produced (this is
    beyond the AI pack scope) and reviewed.
  - Data-subject request response template has been dry-run by
    Operations.
  - Any regulatory filings required in the operator's jurisdiction
    have been made.
- **Artifact:** Counsel sign-off letter, executed corporate resolutions
  authorizing the launch.

### Gate 6 — Operations readiness

- **Owner:** Operations.
- **Purpose:** Prove the ops muscle is real, not just documented.
- **Exit criterion:**
  - Blue/green deploy playbook in `docs/OPS_RUNBOOKS.md` has been
    exercised on testnet **twice** with `scripts/lb_flip.sh` in both
    forward and rollback directions, and the drill log is archived.
  - On-call rota is staffed with at least two independent operators
    trained on the runbooks. A single-operator on-call rota does not
    satisfy this gate.
  - Disaster-recovery drill has been run at least once: full restore
    of anchor state from backups on a clean host, timed against a
    documented RTO/RPO that Operations commits to.
  - Alertmanager config has a real receiver (not the local null
    receiver from `deploy/observability/alerts/alertmanager.yml.example`)
    routed to a paging surface Operations has practised responding to.
- **Artifact:** Drill log, on-call rota, alerting proof.

### Gate 7 — Financial resilience

- **Owner:** Operator.
- **Purpose:** Make sure a single incident cannot end the project.
- **Exit criterion:**
  - Cyber-liability and E&O insurance quotes have been obtained from
    at least two brokers, reviewed against the risk profile, and one
    policy is bound before launch. Coverage must specifically include
    incident response cost, data-subject notification cost, and third-
    party liability, at limits reviewed by counsel.
  - A runway calculation showing the operator can absorb the annual
    cost of HSM, hosting, monitoring paging, and insurance for at
    least twelve months without new external funding.
  - A written kill-switch policy: under what conditions the operator
    will wind down mainnet operations, on what timeline, with what
    obligations to users under `LEGAL/TOS.md`.
- **Artifact:** Bound insurance policy, runway sheet, kill-switch
  policy.

### Gate 8 — Launch review

- **Owner:** Operator.
- **Purpose:** A single go/no-go review that reads the eight gate
  artifacts side by side and either closes or defers the launch.
- **Exit criterion:**
  - All prior gates closed with artifacts on file.
  - No open High or Critical security finding.
  - No open SEV-1 or SEV-2 incident on the current testnet surface.
  - Operator, security lead, and Operations lead each individually
    sign the launch decision. Any one veto blocks launch.
- **Artifact:** Launch decision record, signed. If go, this record is
  the authoritative moment at which the honesty ladder may be updated
  to describe a live mainnet surface — and only then.

---

## 4. Phased rollout

Even after Gate 8, mainnet is not opened at full traffic on day zero.
The rollout has three phases; each phase has an exit criterion before
progressing to the next. **All phases run against Casper mainnet;** the
"canary" name refers to *traffic exposure*, not to a separate network.

### Phase 4.1 — Canary (invitation-only)

- Traffic gate: a hard-coded allow-list of operator-controlled and
  design-partner API keys. Every other caller receives a 403 that says
  the surface is invitation-only.
- Rate ceiling: a global cap well below the tested ceiling. Cap value
  chosen by Operations before entering this phase.
- Duration: at least two weeks of continuous operation, with no SEV-1
  or SEV-2 incident, before considering Phase 4.2.
- Rollback path: `scripts/lb_flip.sh` back to testnet-only; API keys
  disabled by allow-list flip; anchor writes suspended by feature
  flag. All three paths must be exercised in dry-run once during
  canary.

### Phase 4.2 — Limited GA

- Traffic gate: public sign-up, but rate limits per account remain
  well below tested ceiling.
- New account creation requires positive email verification and
  agreement to `LEGAL/TOS.md` and `LEGAL/AUP.md` as of their
  counsel-reviewed version.
- Duration: at least four weeks. Move to Phase 4.3 only when Operations
  reports zero SEV-1 or SEV-2, and the SLO burn-rate alerts from
  `deploy/observability/alerts/slo.alerts.yml` show clean traffic per
  the promtool test rule scenarios.
- Rollback path: same three levers as canary. In addition, Operations
  may reduce back to invitation-only without a full launch reversal by
  flipping the allow-list flag.

### Phase 4.3 — General availability

- Traffic gate: standard sign-up, rate limits set to the tested
  ceiling with headroom.
- SLA is documented in `LEGAL/TOS.md` per counsel review and is
  measurable against the SLO recording rules already shipped.
- Rollback path: incident response ladder in `docs/OPS_RUNBOOKS.md` is
  authoritative from GA onward.

---

## 5. Rollback

If at any point after Gate 8 an incident triggers a rollback, the
sequence is:

1. **Freeze writes.** Feature flag disables `/attest` mutations. Reads
   and verification continue.
2. **Cut traffic.** `scripts/lb_flip.sh` diverts traffic away from the
   affected slot. Idempotent, per `scripts/lb_flip_test.sh`.
3. **Preserve state.** Snapshot anchor state and audit logs before any
   remediation begins. Retained per `LEGAL/DATA_PROTECTION.md`.
4. **Notify.** Users notified per `LEGAL/TOS.md` obligations. If the
   incident touches personal data, notification path per
   `LEGAL/DATA_PROTECTION.md` §breach notification.
5. **Post-incident review.** Follow the PIR template in
   `docs/OPS_RUNBOOKS.md`. Reopen the relevant gate if the root cause
   invalidates a gate artifact.

Rollback is not a failure of the plan; it is the plan working. The
kill-switch policy from Gate 7 governs a permanent wind-down; steps 1–5
above govern a temporary rollback.

---

## 6. Change control after launch

Once GA, changes to any of the following require a full gate re-review
before rolling out:

- Signing algorithm or key custody model.
- Anchor payload format on mainnet.
- Data retention schedule in `LEGAL/DATA_PROTECTION.md`.
- Governing law or dispute-resolution clauses in `LEGAL/TOS.md`.

Changes to observability, non-anchor endpoints, and non-legal docs
follow the standard `docs/OPS_RUNBOOKS.md` change management flow.

---

## 7. What is explicitly not launched

To keep the honesty ladder intact, the following are **not** part of
the mainnet launch under this plan, and any language that implies
otherwise must be corrected before Gate 8:

- **Full-circuit ZK proofs of ML inference.** The current circuits are
  toy circuits (MiMC preimage class), disclosed in
  `docs/KNOWN_LIMITATIONS.md`. Full-circuit ZK ML is a separate
  research programme and its readiness is not part of this launch
  contract.
- **STARK recursive aggregation.** Blocked on a mature Go
  implementation; documented in `docs/KNOWN_LIMITATIONS.md`.
- **Distributed prover network with MPC threshold.** Documented as
  medium-term and out of scope for the first mainnet launch.
- **Hardware attestation (TPM / SGX / SEV) for the request path.**
  Interfaces exist but no live hardware attestation is claimed at
  launch.
- **Fiat rails, custody of user funds, or any regulated financial
  activity.** CasperProver anchors proofs; it does not custody assets.
  Any change to this posture is a separate legal review outside this
  plan.

Anything not on this exclusion list is either in-scope for a specific
gate above or explicitly deferred by that gate's exit criterion.

---

## 8. Cost shape (planning only, not a budget)

Recorded here so gate owners have a shared mental model. **No costs
are authorized by this document.** Actual budgets are set outside the
repo when procurement approvals are signed.

Recurring cost categories the operator will need to fund for a real
mainnet operation:

- HSM custody (Gate 3).
- Independent security audit, initial and follow-up (Gate 2).
- Counsel review, initial and per material change (Gate 5).
- Hosting for the API surface plus observability stack (Gate 1, Gate 6).
- Paging and on-call surface for Alertmanager (Gate 6).
- Cyber-liability and E&O insurance (Gate 7).
- Ceremony logistics if run in person (Gate 4).

Non-recurring cost categories:

- Ceremony hardware, if the coordinator chooses air-gapped devices per
  `docs/KEY_CEREMONY_PLAN.md`.
- One-time legal filings in the operator's jurisdiction.

Every one of the above is deferred until the operator explicitly
signs procurement outside this repo. Nothing in this section
authorizes spend.

---

## 9. What this plan does not do

Repeating for clarity, because roadmap docs get misread:

- It does not schedule dates.
- It does not select vendors.
- It does not commit money.
- It does not authorize any change to the current testnet-only surface.
- It does not overrule `LEGAL/TOS.md`, `docs/HSM_PLAN.md`,
  `docs/KEY_CEREMONY_PLAN.md`, or `docs/OPS_RUNBOOKS.md` — those
  documents remain authoritative in their domains.

It is a **map**. Execution is a separate act, gated by the ledger in
§3 and the launch review in Gate 8.

---

## 10. Revision history

- **DRAFT v0.1** — initial draft. Roadmap only; no gate closed; no
  spend authorized; testnet-only surface unchanged.
