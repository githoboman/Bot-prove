# SLO / SLA, Observability, Incident Response

Ref: `handoff/CP_FINAL_TASKS_V2.md` §E.

## SLOs (initial targets)

| SLI                                     | Target       | Window |
|-----------------------------------------|--------------|--------|
| `POST /v1/proofs` p95 latency           | ≤ 500 ms     | 30d    |
| `POST /v1/proofs` availability          | ≥ 99.9%      | 30d    |
| `POST /v1/decisions` p95 latency        | ≤ 1500 ms    | 30d    |
| `POST /v1/decisions` availability       | ≥ 99.5%      | 30d    |
| On-chain anchor success (per plan)      | ≥ 99.0%      | 30d    |
| Webhook delivery success (with retries) | ≥ 99.5%      | 30d    |
| HITL ticket first-response time         | ≤ 60 min     | 30d    |

SLA to customers is set at 1 nine below the SLO (e.g. 99% availability
where the SLO is 99.9%) so we have headroom to fix issues without
breaching contract.

## Observability stack

- **Metrics:** Prometheus, exposed via `/metrics` on a private port.
- **Traces:** OpenTelemetry over OTLP to a self-hosted collector; sampled
  at 10% by default, 100% for `error` and `abstain` decisions.
- **Logs:** structured JSON via `slog`; shipped to Loki (or the tenant's
  chosen sink). Every log line carries `tenant_id`, `request_id`, `role`.
- **Dashboards:** Grafana. Panels for each SLI above; ready-to-import
  JSON committed under `deploy/grafana/`.
- **Alerts:** Prometheus Alertmanager rules committed under
  `deploy/alerts/`; routing to on-call PagerDuty (or the equivalent).

## Status page

- Public status page (Statuspage.io or a self-hosted alternative).
- Automatic status update on alert fire; manual update requires human
  confirmation to prevent flapping alerts from spamming customers.
- Rolling 90-day uptime published per component (API, ingestion,
  anchoring, webhook delivery).

## Incident response

- On-call rotation ≥ 2 people. No single-point-of-failure human.
- Runbook per SLI in `docs/runbooks/`. Every alert links to its runbook.
- Postmortem template in `docs/postmortems/TEMPLATE.md`.
- Every P1/P2 incident produces a blameless postmortem within 5 business
  days; postmortems are internally-linked but sanitised for external
  publication when the customer requests it.

## Staging / rollback / backups

- Staging environment mirrors prod with synthetic tenants; every deploy
  goes staging → prod with a 10-min soak.
- Rollback: last-known-good image tag pinned in `deploy/rollback.yaml`;
  automatic rollback on health-check failure.
- Postgres backups: continuous WAL + hourly base + daily off-region.
- Object storage backups: content-addressed, cross-region replication.
- Backup restore drill: monthly, tracked in `docs/drills/`.

## Milestones

1. **Metrics + traces + logs baseline (10 days).**
2. **Dashboards + alerts (10 days).**
3. **Runbooks per SLI (5 days).**
4. **Status page + on-call rotation (10 days).**
5. **Backup restore drill (2 days).**

## Non-goals

- Multi-region active-active. Roadmap.
- Chaos engineering. Roadmap.
- Real-time customer analytics. See `docs/roadmap/MULTITENANCY.md#audit-log`.

## Acceptance criteria

- [ ] All SLIs measured and published on the status page.
- [ ] Alert rules present under `deploy/alerts/`; ≥ 1 firing exercise
      per month per SLI (synthetic).
- [ ] Postmortem template used for every P1/P2.
- [ ] Backup restore drill passes.
