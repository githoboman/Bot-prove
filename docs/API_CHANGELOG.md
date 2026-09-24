# CasperProver API — versioning + changelog

Contract for external SDK authors and integrators.

## Version discovery

Every response carries `X-CP-API-Version: v1`. Clients MAY assert on this
header to detect a server upgrade. The version bumps only on breaking
changes to request/response schemas or authentication rules.

## Path scheme

Two prefixes are served during the migration window:

| Prefix        | Status      | Behavior                                                                 |
|---------------|-------------|--------------------------------------------------------------------------|
| `/v1/<path>`  | **stable**  | Preferred. Header `X-CP-API-Version: v1`. No deprecation header.         |
| `/<path>`     | deprecated  | Same handler, plus `X-CP-Deprecation`, `Sunset`, and `Link` headers.     |

`GET /health` is exempt from the deprecation header (used by dumb probes).

### Deprecation contract

Legacy unprefixed paths respond with (RFC 8594):

```
Sunset: 2027-01-01
X-CP-Deprecation: 2027-01-01
Link: </v1/<same path>>; rel="successor-version"
```

Sunset is a **signal**, not enforcement. The server will keep serving the
alias until the codebase drops it — tracked here in the "Deprecated" section
below.

## Idempotency

Mutating requests (`POST` / `PUT` / `PATCH` / `DELETE`) MAY carry an
`X-Idempotency-Key: <opaque>` header. Rules:

- Same key + same payload within **15 minutes** — the server replays the
  cached response with `X-Idempotency-Replay: true`. The inner handler
  is not re-executed.
- Same key + **different payload** — the server returns `409 Conflict`
  with body `{"error":"idempotency-key reused with different payload"}`.
  This is intentional: silently coalescing distinct requests hides bugs.
- No key — no dedup. Every retry runs the handler.
- Only `2xx` responses are cached. `5xx` and other errors are safe to
  retry with the same key.

Storage is in-process only; a server restart clears the state. For
cross-node deduplication, put the API behind a proxy that pins requests
by `X-Idempotency-Key` to the same instance, or wait for the Redis-backed
implementation (tracked in backlog 7.6+).

## Authentication

Mutating requests require `X-API-Key` when `API_KEY` is configured on the
server. Read-only endpoints (`GET`, `HEAD`, `OPTIONS`) stay public.

When `CP_STRICT=1` **and** `API_KEY` is unset, the server refuses to start
(fail-loud) rather than serve open.

## Rate limiting

Per-IP: 60 requests/minute. Exceeding returns `429 Too Many Requests`.
POST body cap: 1 MB. Enforced before the handler runs.

## Change log

### 2026-07-26 — v1 introduced

- **Added:** `/v1/*` alias for every endpoint.
- **Added:** `X-Idempotency-Key` support on mutating requests.
- **Added:** `X-CP-API-Version` response header.
- **Deprecated:** unprefixed paths. Sunset date `2027-01-01`.

### 2026-07-24 — pre-v1 baseline

- `CP_STRICT=1` requires `API_KEY`.
- Per-IP rate limit at 60 req/min; POST body cap at 1 MB.
- CORS opens `*`.

## Deprecated — not yet removed

| Item                         | Since       | Sunset      | Removed |
|------------------------------|-------------|-------------|---------|
| Unprefixed paths (`/proofs`) | 2026-07-26  | 2027-01-01  | —       |

## Removed

*(none yet)*
