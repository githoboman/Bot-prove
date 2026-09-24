# CasperProver — SLO alerts & error budgets

**Honesty label:** REAL / LOCAL-STACK / DRAFT-RULES.
The rule files here are Prometheus alert/recording rules — plain YAML you can
load into any Prometheus 2.x/3.x server. They **do not** ship with a paid
alerting relay (PagerDuty, OpsGenie, etc.); the receiver is Alertmanager local
webhook / null by default. Wiring to a real pager is a post-investment step
tracked in `docs/MAINNET_LAUNCH_PLAN.md`.

## What's here

- `slo.rules.yml` — recording rules that materialise the per-route SLI series
  (request rate, error rate, latency histograms) from the `/metrics` endpoint
  exposed by the CasperProver engine (see Pack AG — `docs/OBSERVABILITY.md`).
- `slo.alerts.yml` — burn-rate multi-window alerts on the recorded SLIs
  (fast-burn 1h & 6h, slow-burn 24h & 3d) following the Google SRE workbook
  ("Alerting on SLOs", ch. 5). Two SLOs are defined:
  - **Availability** — 99.5% of anchor + verify + prove HTTP requests return
    non-5xx over rolling 30d.
  - **Latency**    — 95th percentile of anchor + verify + prove HTTP requests
    below 750ms over rolling 30d.

## Prerequisites

- Pack AG (`feat/cp-observability-local`) merged/deployed: the engine exposes
  `/metrics` in Prometheus 0.0.4 text format with the series
  `http_requests_total{route,method,status}` and
  `http_request_duration_seconds_bucket{route,method,le}`.
- A local Prometheus scrape pointed at that endpoint (see
  `deploy/observability/prometheus.yml`).

## Wiring the alerts

```yaml
# In deploy/observability/prometheus.yml
rule_files:
  - "alerts/slo.rules.yml"
  - "alerts/slo.alerts.yml"

alerting:
  alertmanagers:
    - static_configs:
        - targets: ["alertmanager:9093"]
```

For a local demo without a pager, run Alertmanager with a
`null` receiver — see `alertmanager.yml.example`.

## SLO parameters (edit here first)

Both SLOs are declared in `slo.rules.yml` as recording expressions with
explicit constants at the top of the file. Change the constants only —
never patch the burn-rate multipliers in `slo.alerts.yml` blindly; the
1h/6h/24h/3d windows are derived from the standard multi-window burn-rate
table (2%, 5%, 10% budget-burn thresholds).

| SLO           | Target  | Window | Fast page | Slow ticket |
|---------------|---------|--------|-----------|-------------|
| Availability  | 99.5%   | 30d    | 1h & 5m   | 6h & 30m    |
| Latency (p95) | 750ms   | 30d    | 1h & 5m   | 6h & 30m    |

## Upgrade path

- Wire Alertmanager `webhook_configs.url` to a real on-call system
  (PagerDuty Events API v2, OpsGenie API) — post-investment, tracked in
  `docs/MAINNET_LAUNCH_PLAN.md`.
- Increase SLO tightness once traffic is representative (post-launch: 99.9%
  availability, 300ms p95).
