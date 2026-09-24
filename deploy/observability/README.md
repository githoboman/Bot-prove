# CasperProver — Local observability stack

Opt-in Prometheus + Grafana for dev-only use. Zero paid services.

## Start

```bash
docker compose up -d
```

- Prometheus: <http://localhost:9090>
- Grafana:    <http://localhost:3001>   (anonymous viewer + admin/admin)

The Prometheus job scrapes the engine at `host.docker.internal:8081/metrics`.
Start the engine on port 8081 (default) with `PORT=8081 ./cp-engine`.

## Stop / reset

```bash
docker compose down -v && rm -rf _data
```

## What's here

- `docker-compose.yml` — Prometheus + Grafana OSS, pinned tags, local
  bind-mounts under `_data/`.
- `prometheus/prometheus.yml` — one static scrape target.
- `grafana/provisioning/` — auto-provisioned Prometheus datasource +
  dashboards folder.
- `grafana/dashboards/casperprover-engine.json` — starter dashboard: request
  rate, p50/p95 latency, 5xx rate, in-flight.

Full docs: [`docs/OBSERVABILITY.md`](../../docs/OBSERVABILITY.md).
