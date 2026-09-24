# Webhook subsystem — Prometheus instrumentation

Slot: 7.7-r follow-up. Ties the webhook worker's queue, delivery and
replay paths into the existing `/metrics` endpoint.

## Metrics

All metrics register under prefix `cp_webhook` on the same
`observability.Registry` that serves `cp_http_*` (single `/metrics`
scrape). Labels are bounded — see below.

| Metric | Type | Labels | What it counts |
|---|---|---|---|
| `cp_webhook_enqueued_total`           | counter   | `event` | events fanned out onto the queue |
| `cp_webhook_attempts_total`           | counter   | `event`, `status_class` | HTTP dispatch attempts by outcome |
| `cp_webhook_delivered_total`          | counter   | `event` | events that reached a 2xx before max attempts |
| `cp_webhook_dead_lettered_total`      | counter   | `event` | events that exhausted retries |
| `cp_webhook_replayed_total`           | counter   | `event` | operator-initiated dead-letter replays |
| `cp_webhook_attempt_duration_seconds` | histogram | `event`, `status_class` | per-attempt latency (default HTTP buckets) |
| `cp_webhook_queue_depth`              | gauge     | — | current pending-queue length |
| `cp_webhook_dead_letter_depth`        | gauge     | — | current dead-letter list length |

### Label vocabularies

- `event` — one of the `KnownWebhookEvents` constants declared in
  `internal/api/webhooks.go` (`proof.verified`, `proof.anchored`,
  `slash.executed`, `governance.executed`, `kyc.granted`,
  `kyc.revoked`). Closed set; adding a new event is a code change,
  not runtime cardinality.
- `status_class` — `"2xx"` / `"3xx"` / `"4xx"` / `"5xx"` / `"network"`
  (network = the HTTP client returned an error before any response
  arrived) / `"other"` (defensive). Bounded 6 values.

Total series ceiling per pod ≈ `6 events × 6 status classes` = 36
histograms + a handful of counters/gauges. Well within Prometheus
scrape budgets.

## Wiring

- `observability.NewWebhookMetrics(reg, prefix)` registers the whole
  set on a `*Registry`.
- `webhookStore.SetMetrics(*WebhookMetrics)` installs it (safe to
  pass `nil` — that keeps the store metric-free, which is what unit
  tests want).
- Server startup wires it once in `NewServer`, right after the
  webhook store is constructed, so `/metrics` reflects live traffic
  from the first request.

## Test coverage

- `TestWebhookMetrics_RegisterAndScrape` — registers, drives one of
  each metric, asserts the expected series lines land in a Prometheus
  scrape.
- `TestStatusClass` — the code→label mapping is exhaustive
  (`0/200/301/404/500/-1/999`).
- `TestWebhookMetrics_NilSafeUsage` — the "metrics disabled"
  code-path is safe under the same no-op guard the store uses.
- `TestWebhookMetrics_HistogramBucketsMonotonic` — bucket counts are
  cumulative (monotonic non-decreasing) after several observations.

Existing webhook tests in `internal/api/webhooks_test.go` continue to
pass unchanged — instrumentation is metric-only, no behavioural
change.

## Known limitations

- Delivered/DeadLettered counts are not backfilled on a process
  restart (the queue itself is in-memory — durable webhook state is
  tracked in `KNOWN_LIMITATIONS.md`).
- The gauges are sampled on every enqueue / success / dead-letter /
  replay, but not on a timer. If the worker sits idle for a while
  between deliveries, `queue_depth` may lag by exactly the backoff
  window until the next tick refreshes it. Acceptable for alerting;
  dashboards should not treat the gauge as sub-second precise.
