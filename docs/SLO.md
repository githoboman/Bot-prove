# CasperProver — Service Level Objectives (SLO catalogue)

*Backlog 10.3.* Public commitment for what CasperProver's hosted API
and on-chain surface promise, measured, and reports against.

The point isn't a marketing number — the point is that every metric
has (a) a definition a judge can reproduce with `verify.sh` or a
Grafana dashboard, and (b) a stated error budget so degrade signals
route to the on-call.

## Availability

| Surface                  | Target | Window       | How measured                                                                 |
|--------------------------|--------|--------------|------------------------------------------------------------------------------|
| `GET /health`            | 99.9%  | 30-day rolling | Uptime probe every 30 s; a `2xx` under 500 ms counts as success             |
| `POST /v1/proofs`        | 99.5%  | 30-day rolling | 202/201 within 30 s at 60 req/min per IP                                    |
| `POST /v1/verify`        | 99.5%  | 30-day rolling | 200 within 5 s at 60 req/min per IP                                         |
| Casper on-chain anchor   | 99.0%  | 30-day rolling | Submit-and-confirm within 5 minutes on Casper testnet 2.0                   |

Anything below target is reported in the `/health` payload as
`degraded: true`; sustained breach (>1 h continuous) opens a `SEV-2`
in `docs/SECURITY.md`.

## Latency (p95)

| Operation                                   | Target       | Rationale                                                                    |
|---------------------------------------------|--------------|------------------------------------------------------------------------------|
| Proof generation (hash-based)               | ≤ 400 ms    | Hackathon demo path; hash chain over MiMC preimage                            |
| Groth16-real prove (BN254, MiMC circuit)    | ≤ 3.5 s     | Real gnark pairing; benchmarks land in `docs/benchmarks/`                    |
| Groth16-real verify                         | ≤ 25 ms     | Pairing check only                                                          |
| PQ (SPHINCS+) sign                          | ≤ 700 ms    | Reference impl; not tuned                                                    |
| PQ verify                                   | ≤ 30 ms     |                                                                              |
| On-chain deploy submit → finalized         | ≤ 5 min      | Casper testnet block time is ~30 s; 10x buffer                              |

## Correctness

| Guarantee                                                    | Enforcement                                                            |
|--------------------------------------------------------------|------------------------------------------------------------------------|
| No proof marked `verified=true` when the pairing fails       | `internal/zkverifier` fail-loud; regression covered by TestGate4       |
| Bit-identical proof digests across SDK languages             | `sdk/*/examples/smoke_hash.md` cross-check                             |
| Chain-root of an audit record is tamper-evident              | `internal/decision.VerifyRecord`, exercised in TestChainRoot_DetectsTampering |
| Simulation endpoints are self-declared                        | Every `[sim]` endpoint returns `{"simulation":true}` + `Deprecation` header |

## Error budget & burn-down

- SLO target of `99.5%` ⇒ 216 min error budget per 30 days.
- Burn-rate alerts (Grafana): 14.4x for 5 min ⇒ SEV-1 page; 6x for 1 h
  ⇒ SEV-2 investigate.
- Post-mortem template: `docs/POSTMORTEM_TEMPLATE.md` (to be added).

## What is NOT covered

- Client-side network reliability (browser → API).
- Casper Network consensus delays beyond 5 min (an ecosystem-wide
  incident, not a CasperProver SLO breach).
- Third-party LLM inference latency — the audit log records the
  request/response hashes, not the upstream latency.

## Reporting

- `/health` returns a snapshot of the last 5 min of each SLO in JSON.
- Weekly SLO report auto-generated to `docs/slo/<YYYY-Www>.md`
  (planned once mainnet‑ready — tracked in backlog).
