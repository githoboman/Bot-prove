# CasperProver — Ops Runbooks

**Honesty label:** DRAFT · REAL for local + testnet · Playbook, not production
policy. Nothing here binds a hosting provider, an on-call rotation, or a paid
alerting system. Wiring to a real pager / real infra happens post-investment
(see [MAINNET_LAUNCH_PLAN.md](./MAINNET_LAUNCH_PLAN.md)).

This document is the single source of truth for **how we deploy** and **what an
operator does when an alert fires**. Every alert in
[`deploy/observability/alerts/slo.alerts.yml`](../deploy/observability/alerts/slo.alerts.yml)
links back to an anchor in this file.

---

## 1. Scope

- **In-scope:** the CasperProver engine HTTP API (`/anchor`, `/verify`,
  `/prove`, `/metrics`, `/version`) plus the local observability stack from
  Pack AG.
- **Out-of-scope for this doc:** contract deployments (see
  [`docs/TX_MANIFEST.md`](./TX_MANIFEST.md) and the deploy scripts under
  `scripts/`), the Phase-2 ceremony (see
  [`zk/ceremony/README.md`](../zk/ceremony/README.md)), and mainnet promotion
  (see [MAINNET_LAUNCH_PLAN.md](./MAINNET_LAUNCH_PLAN.md)).

---

## 2. Blue/green deploy playbook

**Goal:** deploy a new engine build with zero user-visible errors and a fast,
mechanical rollback. Every deploy is a **two-slot cutover**, never an in-place
restart of production traffic.

### 2.1 Slot model

Two engine replicas at all times, behind the same reverse proxy /
load balancer:

| Slot  | Role                | Traffic weight |
|-------|---------------------|----------------|
| blue  | current live build  | 100% (steady)  |
| green | candidate build     | 0% (steady)    |

At cutover we swap them: blue drains, green takes 100%. Blue then becomes the
next candidate slot.

For the local demo stack the two slots are `engine-blue` and `engine-green`
services in `deploy/observability/docker-compose.yml` (Pack AG) — one is
actively wired to `prometheus.yml`'s scrape target, the other is idle. On real
infra the same shape maps to Fly.io `[[services]]` groups, Kubernetes
Deployments behind a Service selector, or AWS ECS blue/green target groups.

### 2.2 Pre-cutover checklist (must all be TRUE)

1. `verify.sh` passes on the release branch (8/8).
2. Full engine test suite green (`go test ./engine/...`).
3. `promtool check rules deploy/observability/alerts/*.yml` clean.
4. `promtool test rules deploy/observability/alerts/slo.tests.yml` green.
5. `gitleaks detect --no-git` clean on the release tree.
6. Release SHA present in `deploy-out/onchain.json` build metadata (for the
   contract-adjacent builds; N/A for engine-only builds).
7. Grafana dashboard "CasperProver — Engine RED" open in a browser tab.
8. Last 30m error-ratio < 0.5% on blue (steady-state guard).

If **any** item fails, abort — the deploy is not authorised.

### 2.3 Cutover procedure

```bash
# 1. Bring green up on the new build and let it fully warm.
docker compose -f deploy/observability/docker-compose.yml up -d engine-green

# 2. Wait until green passes its self-checks.
until curl -sf http://engine-green:8080/health >/dev/null; do sleep 2; done

# 3. Shadow-traffic phase (optional): mirror 5% of read-only traffic to green.
#    On the local stack this is a nginx `mirror` directive; on prod it is
#    the LB's traffic-split feature.
#    Watch dashboards for 5 minutes. Any SLI regression -> abort (see 2.4).

# 4. Cut over. Swap the upstream target from blue to green in the LB.
./scripts/lb_flip.sh blue green

# 5. Drain blue: 60s connection-drain window, then stop it.
sleep 60 && docker compose stop engine-blue
```

**Cutover exit criteria** — all must be true 10 minutes after step 4:

- Availability SLI on all in-scope routes ≥ 99.9% for the 10-minute window
  (stricter than the SLO — this is a *deploy* gate, not a steady-state gate).
- p95 latency within 1.5× of the pre-cutover baseline on all routes.
- No `CPAvailabilityErrorBudgetBurnFast` firing.
- No `CPLatencyErrorBudgetBurnFast` firing.

### 2.4 Rollback

Rollback is **the same script, run backwards** — flip the LB back, and drain
green. No conditional logic, no partial rollbacks, no cherry-picking.

```bash
# Restore blue immediately.
./scripts/lb_flip.sh green blue
sleep 30 && docker compose stop engine-green
```

Rollback is **not a decision**, it is a **reflex**: whenever any of the exit
criteria above is violated inside the 10-minute observation window, the on-call
runs rollback first and investigates second.

Post-rollback:

- File an incident (§4).
- Do NOT redeploy green until root cause is known and a mitigation is merged.

### 2.5 What blue/green does NOT protect against

- Schema-breaking DB migrations — those require a separate expand/contract
  playbook. This repo has no server-side DB today; when it does, this section
  gets an `expand-migrate-contract` sub-runbook.
- Contract deploys — each Casper testnet deploy is one-shot; there is no LB
  in front of a contract. The contract runbook lives in
  [`docs/TX_MANIFEST.md`](./TX_MANIFEST.md).
- ZK ceremony transcript rotations — see
  [`zk/ceremony/README.md`](../zk/ceremony/README.md).

---

## 3. Alert response runbooks

Every alert in `slo.alerts.yml` has a matching subsection here. When a page
fires the on-call opens this file, jumps to the anchor, and follows the steps
top-to-bottom.

### 3.1 <a name="availability-burn-fast"></a>`CPAvailabilityErrorBudgetBurnFast` (page)

**Meaning.** A given route is 5xx-ing at >7.2% (14.4× the 0.5% budget) on both
the 1h and 5m windows. At this burn rate 2% of the entire 30d budget is being
consumed per hour.

**Response (target: mitigate within 15 minutes).**

1. **Confirm:** open Grafana → "CasperProver — Engine RED" → filter by the
   route from the alert label. Confirm the 5xx spike is real (not a scrape
   glitch: `up{job="cp-engine"} == 1`).
2. **Correlate:** check the last deploy timestamp. If a deploy happened
   in the last 30 minutes, treat this as a bad deploy and **execute
   rollback (§2.4) immediately** — even before deep RCA.
3. If NO recent deploy:
   - Check downstream — Casper RPC health, contract endpoints, KMS/keystore
     availability.
   - Check saturation — `http_requests_in_flight` and container CPU/memory.
   - If a single downstream is guilty, degrade gracefully:
     shift `/anchor` off the impaired downstream if we have a fallback
     configured; otherwise return 503 with `Retry-After` (`api_hardening_v2`
     preflight already returns 503 on preflight failure).
4. **Communicate:** post to the incident channel with
   `INCIDENT-{{yyyy-mm-dd-N}}: availability burn on <route>`. Set severity to
   SEV-2 unless the burn covers > 3 routes simultaneously (then SEV-1).
5. **After mitigation:** file post-incident review (§4.4).

### 3.2 <a name="availability-burn-slow"></a>`CPAvailabilityErrorBudgetBurnSlow` (ticket)

**Meaning.** A route is 5xx-ing at >3% (6× the 0.5% budget) sustained over
1h and 6h. Slow but real budget consumption — we are not on fire but the
trend is bad.

**Response (target: mitigate within 24 hours).**

1. File a ticket. Do NOT roll back reflexively — a slow burn is often the
   symptom of a downstream degradation, not a bad deploy.
2. Grafana → route-level error rate. Identify pattern: bursty? steady? tied to
   time of day?
3. Check dependency health dashboards.
4. If SLO is projected to be missed for the month, escalate to SEV-3 and
   open a change ticket to relax the SLO OR ship a fix.

### 3.3 <a name="latency-burn-fast"></a>`CPLatencyErrorBudgetBurnFast` (page)

**Meaning.** p95 latency on the route is >1500ms (2× the 750ms SLO) sustained
over 1h and 5m windows.

**Response (target: mitigate within 15 minutes).**

1. Confirm on Grafana. Rule out a Prometheus scrape stall.
2. Correlate to last deploy — same reflex as §3.1 step 2.
3. If no recent deploy: check `http_requests_in_flight` — if saturated, see
   §3.4.
4. Downstream latency: Casper RPC round-trip, contract call latency, ZK prover
   queue depth. If ZK prover is the culprit, back-pressure `/prove` with 429s
   until queue drains.
5. Communicate + post-incident review as in §3.1.

### 3.4 <a name="latency-burn-slow"></a>`CPLatencyErrorBudgetBurnSlow` (ticket)

**Meaning.** p95 above the 750ms SLO but < 1500ms, sustained over 1h and 6h.

**Response (target: mitigate within 24 hours).** Same pattern as §3.2:
investigate before intervening, prefer capacity + downstream fixes over
rollback.

### 3.5 <a name="saturation"></a>`CPHTTPInFlightSaturation` (ticket)

**Meaning.** In-flight HTTP requests on a route exceeded 128 for 5 minutes.
The engine is not necessarily failing yet, but latency and error budgets are
about to be spent.

**Response.**

1. Check autoscaling / replica count. If the green slot is idle, scale it up
   and add it to the LB pool (temporary widening — this is NOT the same as
   a blue/green cutover; both slots serve).
2. If the load pattern is abusive (single API key), enable/tighten the
   per-key rate limiter (Pack AB — `api_hardening_v2`).
3. If sustained, plan capacity: file a ticket to raise the default replica
   count.

---

## 4. Incident response skeleton

### 4.1 Severity ladder

| SEV | Definition                                                                 | Response  |
|-----|----------------------------------------------------------------------------|-----------|
| 1   | Multiple in-scope routes 5xx-ing OR core `/anchor` unavailable > 5m       | Page 24×7 |
| 2   | Single in-scope route SLO burning fast OR user-facing degradation         | Page work-hours |
| 3   | Slow burn / capacity trend / non-user-facing regression                    | Ticket    |
| 4   | Cosmetic / doc / dashboard issue                                            | Ticket    |

Until the on-call rotation is real (see MAINNET_LAUNCH_PLAN.md), SEV-1 and
SEV-2 both notify the single maintainer via the local Alertmanager receiver.

### 4.2 During an incident

- Declare the incident: post to the incident channel with the alert name,
  route, and current SEV.
- Assign roles (even with one person):
  - **Commander** (decides — one voice on comms).
  - **Comms** (updates channel every 15 min).
  - **Operator** (runs commands).
- Every mitigation attempt goes into the timeline as it happens. No
  retroactive reconstruction.
- If the fix is not mechanical (rollback), do NOT deploy again from `main`
  during the incident — freeze changes.

### 4.3 Governance owner-key loss during an incident

The `governance` contract's guardians (`anna-stolbovskaja` / `defi_mock_owner`
/ reserved mainnet-ceremony slot) **cannot directly unpause** a paused
contract or otherwise act as owner. If the owner key itself is the one lost
or compromised during an incident, guardians must first complete
`sign_recovery` → `execute_recovery` (2-of-3) to become the new owner, and
only then call `propose_unpause` → `execute_unpause` (or any other
owner-gated action) under the normal timelock. Budget for the 48h timelock
when the incident timeline depends on this path — there is no emergency
bypass. See `docs/SECURITY_AUDIT.md` §2.9 for the full guardian/pause model.

### 4.4 Post-incident review

Within 5 working days of resolution, file `docs/postmortems/YYYY-MM-DD-<slug>.md`
with:

- Impact (routes, duration, users affected, error budget consumed).
- Timeline (UTC, ≤ 15-min granularity during peak).
- Root cause (one line, blameless).
- What went well.
- What did not.
- **Actions** (owner + due date + linked issue). No hand-wavy "we will do
  better" — every action is a ticket.

---

## 5. Change management

- **Every change to production goes through blue/green (§2).** No exceptions.
- **Contract redeploys** update `deploy-out/onchain.json` in the same commit
  as the redeploy — never post-hoc.
- **Alert rule changes** must pass `promtool test rules
  deploy/observability/alerts/slo.tests.yml` before merge. Adding an alert
  without a test is not permitted.
- **SLO changes** require an explicit line in the change PR justifying the
  new target and an update to §2.2 + §3.

---

## 6. On-call handoff template

At end-of-shift the outgoing on-call fills:

```
## On-call handoff — <YYYY-MM-DD HH:MM UTC>
- Incidents this shift: <count + IDs>
- Open follow-ups: <bullet list>
- Elevated-risk deploys today: <yes/no + link>
- Ongoing burn: <yes/no + route + status>
- Notes for next shift: <freeform>
```

Post it into the on-call channel and paste the link into the ticket tracker.

---

## 7. See also

- Pack AG — [`docs/OBSERVABILITY.md`](./OBSERVABILITY.md) (metrics endpoint,
  local stack, dashboards).
- Pack AB — [`docs/API_CHANGELOG_POLICY.md`](./API_CHANGELOG_POLICY.md)
  (versioning + preflight semantics).
- [`docs/KNOWN_LIMITATIONS.md`](./KNOWN_LIMITATIONS.md) — what these runbooks
  intentionally do NOT cover yet.
- [`docs/MAINNET_LAUNCH_PLAN.md`](./MAINNET_LAUNCH_PLAN.md) — the promotion
  path from this local playbook to a real paid on-call setup.
