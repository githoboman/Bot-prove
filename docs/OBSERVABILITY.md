# Observability

The engine ships a first-party observability layer: a Prometheus `/metrics`
endpoint and W3C Trace Context propagation. Both are always on — they
cost effectively nothing when no scraper or tracing peer is connected.

Package: [`engine/internal/observability`](../engine/internal/observability/).

## What is honestly shipped

- **Prometheus text exposition (v0.0.4).** Fully self-hosted — we do not
  depend on `prometheus/client_golang`. Counter, gauge, and histogram
  metric kinds are supported. Any Prometheus-compatible scraper (a real
  `prometheus-server`, Grafana Agent, VictoriaMetrics, etc.) can scrape
  `GET /metrics` and store the results.
- **W3C Trace Context, level 1.** We parse and generate `traceparent`
  headers per the [W3C REC](https://www.w3.org/TR/trace-context-1/).
  Version `00` only. `tracestate` is passed through verbatim (opaque).
  Every request that enters the API gets a `SpanContext` attached to
  its `context.Context` — inherited from the upstream `traceparent`
  header when present, or created as a root span otherwise. The
  outgoing response echoes a new `traceparent` with a fresh span-id.

## What is NOT shipped

- **No native OTLP export.** We do not push spans to an OpenTelemetry
  collector. Trace propagation is inbound-and-outbound *headers*, so a
  downstream OTel collector that scrapes the engine can still stitch
  our spans into a distributed trace — but the engine itself does not
  export spans over gRPC/OTLP. That is out of scope for this pack.
- **No exemplars, summaries, or native histograms.** These are
  Prometheus 2.x+ extensions; when we need them we will pull in
  `prometheus/client_golang`. For now the classic bucket histogram
  covers everything we sample.
- **No auto-instrumentation of upstream deps.** Only the HTTP surface
  is measured. Postgres, submitter, and provers can be added later
  with the same Registry.

## Metrics currently emitted

The HTTP layer registers three metrics under the `cp_http` prefix:

| Metric | Kind | Labels | Meaning |
|---|---|---|---|
| `cp_http_requests_total` | counter | `method`, `route`, `status` | Total requests processed. `status` is bucketed as `2xx`/`4xx`/`5xx` etc. so label cardinality stays bounded. |
| `cp_http_request_duration_seconds` | histogram | `method`, `route` | Request latency in seconds. Buckets: `0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10`. |
| `cp_http_requests_in_flight` | gauge | (none) | Requests currently being served. |

The root-chain wrapper labels every request as `route="all"` — this
keeps cardinality at O(number-of-methods) rather than
O(number-of-distinct-URLs) and avoids the classic /:id explosion.
When you need per-route detail, add a dedicated app-level counter
with a bounded label (e.g. `proofs_generated_total{mode="anchored"}`).

## Enabling scraping

```
$ curl http://localhost:8080/metrics
# HELP cp_http_requests_total total HTTP requests processed
# TYPE cp_http_requests_total counter
cp_http_requests_total{method="GET",route="all",status="2xx"} 42
...
```

The alias `GET /v1/metrics` returns the same content — versioned URL
for scrape configs that pin to a specific API version.

Sample Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: casperprover
    static_configs:
      - targets: ["cp-engine:8080"]
    metrics_path: /metrics
    scrape_interval: 15s
```

## Trace propagation contract

When a client sends a `traceparent` header, the engine:

1. Parses it against the W3C level 1 grammar. Malformed headers do
   NOT reject the request — the engine falls back to a fresh root
   span (the spec's forgiveness posture).
2. Reuses the caller's `trace-id`, generates a fresh `span-id` for
   this request, and records the caller's span-id as `parent-id`.
3. Attaches a `SpanContext` to `context.Context` for downstream code
   (`observability.SpanContextFromContext(ctx)`).
4. Echoes a `traceparent` header on the response so ANY downstream
   observer (client-side APM, sidecar, mesh) sees a linked span.
5. Passes through `tracestate` verbatim.

Example:

```
$ curl -H 'traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01' \
       http://localhost:8080/health -i
HTTP/1.1 200 OK
traceparent: 00-0af7651916cd43dd8448eb211c80319c-<new-span-id>-01
...
```

## Where the code lives

- `engine/internal/observability/metrics.go` — Registry, Counter,
  Gauge, Histogram, text exposition writer.
- `engine/internal/observability/trace.go` — `traceparent` parse +
  generate, context helpers.
- `engine/internal/observability/middleware.go` — `HTTPMetrics` +
  the `Instrument` / `InstrumentAll` wrappers, `MetricsHandler` for
  `/metrics`.

All three are covered by unit tests
(`engine/internal/observability/*_test.go`) plus an integration test
that runs the full HTTP chain and validates the exposition
(`engine/internal/api/metrics_endpoint_test.go`).

## Roadmap items closed

Backlog item **10.3 (remaining)** — Prometheus `/metrics`, OTel spans,
sampled tracing — is now closed for the HTTP surface. Follow-ups:

- **10.4** SLO error budgets + burn-rate alerts (needs the metrics
  above scraped for ≥1 week to derive baseline SLOs).
- **10.5** Blue/green deploy runbook + Grafana dashboards (once metrics
  are wired into a stack).
- Optional: swap our in-tree Prometheus writer for
  `prometheus/client_golang` if we later need exemplars, summaries,
  or native histograms. The `Registry` API surface is compatible
  enough to make the swap mechanical.

---

## Local Prometheus + Grafana stack (`internal/obs` package)

## Status: REAL / LOCAL-STACK

**HONESTY BADGE.** The metrics and traces this repo emits are real and
serve a real Prometheus + Grafana stack you can bring up locally. The
in-process client library is a purpose-built, zero-dependency implementation
(exposition-format compatible with Prometheus 0.0.4, semantically aligned with
OpenTelemetry spans). It is not `prometheus/client_golang` and it does not
ship OTLP/gRPC transport. This is a deliberate trade-off — see
"Upgrade path" below.

No SaaS is used. No paid services are used. Only free/open-source components
(Prometheus, Grafana OSS).

## What lands on `/metrics`

The API server exposes `GET /metrics` (registered before authMiddleware
strips headers; see server.go). The exposition format is
`text/plain; version=0.0.4; charset=utf-8`.

Standard HTTP RED metrics:

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `cp_http_requests_total` | counter | `route`, `method`, `status` | Total requests handled. `route` is the mux pattern (bounded cardinality), never the raw URL. |
| `cp_http_errors_total` | counter | `route`, `method`, `status` | 5xx responses. |
| `cp_http_inflight` | gauge | `route` | Requests currently in-flight. |
| `cp_http_request_duration_seconds` | histogram | `route`, `method` | Latency in seconds. Buckets: 1 ms → 30 s exponential. |

Additional metrics can be registered by any subsystem via
`internal/obs.Registry`. See `internal/obs/metrics.go`.

## Traces

Traces are **opt-in** — set `CP_TRACES_ENABLED=1` in the engine environment
to enable them. When enabled:

- Every HTTP request emits an outer `http.request` span with OTel-flavoured
  attributes: `http.method`, `http.route`, `http.target`, `http.status_code`.
- Spans are written as NDJSON to stderr — one JSON record per line.
- Trace / span IDs follow W3C Trace Context (16-byte trace id, 8-byte span id,
  hex-encoded). Nested spans inherit their parent's trace id and record
  `parent_span_id`.
- Handlers may extend the ambient span via
  `span := obs.FromContext(r.Context()); span.SetAttr(...)`.

An external NDJSON→OTLP sidecar is trivial to write if you need to ship spans
to Tempo/Jaeger; it is not shipped in this repo yet (see upgrade path).

## Bringing up the local stack

```bash
# From the repo root:
cd deploy/observability
docker compose up -d

# Then start the engine locally so /metrics is reachable from the host:
#   API_KEY=devkey PORT=8081 CP_TRACES_ENABLED=1 ./cp-engine

# Prometheus  → http://localhost:9090
# Grafana     → http://localhost:3001   (anonymous viewer + admin/admin)
```

Grafana comes up with:

- Datasource `Prometheus` (auto-provisioned) pointing at `prometheus:9090`.
- Dashboard **CasperProver engine** with request rate, p50/p95 latency, 5xx
  rate, and in-flight requests.

## Tear-down / reset

```bash
cd deploy/observability
docker compose down -v
rm -rf _data
```

## Design notes

- **Zero external Go dependencies.** The `internal/obs` package uses only the
  standard library. No `prometheus/client_golang`, no OTel SDKs, no OTLP
  transport. This keeps the engine free of large trees during the hackathon
  window and avoids version drift.
- **Cardinality-safe labels.** The middleware labels every request by the
  matched mux pattern (`GET /proofs/{id}`), never the raw URL. A rogue caller
  cannot inflate label cardinality.
- **Concurrency-safe.** Counters and gauges use atomics; histograms use
  CAS on the float64 sum bits. Deterministic exposition order is enforced by
  sorted names and label keys.

## Upgrade path

When production hardening is warranted:

1. Swap `internal/obs/metrics.go` for `github.com/prometheus/client_golang`
   (same conceptual API: counters, gauges, histograms). The
   `Middleware`/`MiddlewareRoute` shapes stay identical.
2. Swap `internal/obs/tracing.go` for `go.opentelemetry.io/otel` with an OTLP
   exporter (Tempo, Honeycomb, DataDog, etc). The `Tracer`/`Span` shape maps
   1:1 onto the OTel API.
3. Add scrape auth (bearer token or mTLS) at the reverse-proxy layer — the
   `/metrics` endpoint itself is intentionally unauthenticated so Prometheus
   can scrape without secrets baked into the config.

Neither step requires changing any call site.

## What is NOT included

- No OTLP/gRPC transport. Traces are NDJSON on stderr; ship them with any
  sidecar you prefer.
- No log aggregation. Structured logs already go via `slog`; wire them into
  Loki/Vector separately if needed.
- No paid vendors. Repository is hackathon-clean.
