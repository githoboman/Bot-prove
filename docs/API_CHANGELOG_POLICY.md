# API Changelog & Versioning Policy

> Status: **REAL** (policy doc). Governs `/v1/*` HTTP surface and future majors.
> Author: CasperProver engine.
> Item 7.15 from `cp_BACKLOG_v3.md` (Pack AB).

---

## 1. Versioning axes

The API exposes **two** parallel versioning schemes. A client MAY use either;
both resolve to the same server-side handler.

### 1.1 Path-versioning (primary)

```
POST /v1/proofs
GET  /v1/proofs/{id}
```

The path prefix is the canonical identifier. Every response carries an
`X-CP-API-Version: 1` header.

### 1.2 Accept-header versioning (7.5)

```
Accept: application/vnd.cp+json; version=1
```

- `application/vnd.cp+json` without a `version=` parameter → latest served.
- `application/vnd.cp+json; version=N` where N is not served → HTTP `406
  Not Acceptable`; body enumerates supported versions.
- Any other `Accept` (empty, `*/*`, `application/json`, ...) → latest served,
  `X-CP-API-Version` still stamped.

Path and Accept-header can coexist on the same request; the path is
authoritative when the two disagree.

---

## 2. Startup preflight (7.1)

`CP_STRICT=1` forces the server to refuse to start until every critical
environment variable is defined:

- `API_KEY` — authentication for write endpoints.
- `CONTRACT_PROOF_REGISTRY`
- `CONTRACT_VERIFIER_GATE`
- `CONTRACT_DEFI_MOCK`
- `CONTRACT_STAKE_SLASHING`

Missing values are reported in a single log line and the process exits with
status code **2**. Whitespace-only values count as missing.

Dev/demo deployments (`CP_STRICT` unset or `0`) keep the low-friction default:
warnings only, no exit.

The 3 newly-deployed contracts (`proof_of_inference`, `model_registry`,
`proof_aggregation`) are sourced from `deploy-out/onchain.json`, not env,
so a strict-mode operator does not have to set 7 vars — 5 is enough.

---

## 3. Rate limiting

Two independent limiters run in series:

| Layer | Limit | Denominator | Failure mode |
|-------|-------|-------------|--------------|
| Per-IP (existing) | 60 req/min | `RemoteAddr` | `429 Too Many Requests` |
| Per-API-key (7.3) | 120 req/min | `X-API-Key` header value | `429` + `Retry-After: 30` |

Per-key runs AFTER auth, so unauthenticated calls only ever hit the per-IP
guard. The 2× ratio (120 vs 60) leaves room for one legitimate multi-agent
client per key while capping abuse from a leaked key used across many IPs.

Both buckets are in-memory. A future release MAY back them with Redis for
horizontal scale; that migration is a v1 → v1 compatible change (headers
and status codes unchanged).

---

## 4. Deprecation & sunset (RFC 8594 / 9745)

When a v1 endpoint is deprecated for its v2 successor, responses carry:

```
Deprecation: true
Sunset: Sat, 01 Feb 2027 00:00:00 GMT
Link: </v2/proofs>; rel="successor-version"
```

Timeline commitments:

- **≥ 90 days** between `Deprecation` header appearing and the `Sunset` date.
- **≥ 30 days** between `Sunset` date and hard removal (410 Gone).
- Once removed, the path stays reserved for 12 months — never rebound to a
  different resource, so cached clients get a clean 410.

---

## 5. Semver rules for the HTTP surface

Different rules than a library. The API version is the leftmost number in the
path prefix.

- **Major bump** (`/v1` → `/v2`) — required for any of:
  - a field removal or type change in a response,
  - an added *required* request field,
  - a status code change on a non-error path,
  - authentication semantics change,
  - error envelope shape change.

- **Additive change** (same major) — allowed inline:
  - new endpoints,
  - new optional request fields (server ignores extras today; unknown fields
    are logged at DEBUG),
  - new response fields — clients MUST tolerate them,
  - new *optional* response headers,
  - new enum values in a response — MUST be documented in the changelog
    before the first response carrying them.

- **Non-versioned changes** — always allowed:
  - performance,
  - internal contract addresses (they live in `deploy-out/onchain.json`),
  - error message text (`code` field is the stable contract, `message` is not),
  - middleware order.

---

## 6. Changelog

Every merged PR that touches the HTTP surface must add an entry to
`CHANGELOG.md` under `## [Unreleased] → ### API`.

Format:

```
- **[change]** endpoint: description. (item X.Y)
```

`change` is one of `added`, `changed`, `deprecated`, `removed`, `fixed`,
`security`.

---

## 7. Non-goals

- **No content-language negotiation.** All bodies are `en-US`.
- **No hypermedia links (HAL / JSON-LD).** Responses are flat JSON.
- **No SDK version pinning.** SDKs (`sdk/go`, `sdk/python`, `sdk/typescript`)
  release independently against the current major.
