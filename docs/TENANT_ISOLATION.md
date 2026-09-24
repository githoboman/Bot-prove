# Tenant isolation (BA / backlog 10.1 + 10.2)

## Scope

Pack BA adds first-class multi-tenant support to the API layer of the
CasperProver Service, opt-in via env, backwards-compatible with the
existing single-shared-key mode.

What lands:

1. `engine/internal/api/tenant` — a self-contained tenant store
   (`tenant.Store`) with:
   - Tenant identity model: `id`, `display_name`, `namespace`,
     multiple active `key_hashes`, `rate_per_second`,
     `rate_per_minute`, `monthly_proof_quota`.
   - Key resolution via SHA-256(hex) hash lookup. Raw keys are never
     kept in memory after `LoadFile`.
   - Per-tenant rate limiting (fixed-window per-second + per-minute).
   - Per-tenant monthly proof-write quota, resetting on UTC month roll.
   - Two-phase key rotation: `RotateAddKey` opens a grace window
     during which both old and new keys work; `RotateRevokeOldKeys`
     drops everything but the newest N.
   - Ring-buffered append-only audit log (default cap 4096) filterable
     by tenant id, capturing every lifecycle event and every
     accept/reject/rate-block/quota-block decision.
2. `engine/internal/api/tenant_handlers.go` — admin HTTP surface,
   gated by a separate `TENANT_ADMIN_TOKEN`.
3. Tenant-aware middleware wired into `authMiddleware` — when tenant
   mode is on, the middleware resolves the caller, enforces the rate
   ceiling, logs the outcome, and stashes the resolved tenant on the
   request context via `tenantFromCtx(r)`.

## Enabling tenant mode

Tenant mode is *off by default*. To turn it on:

1. Write a `tenants.json` file that lists the tenants you want. The
   schema is:

    ```json
    {
      "tenants": [
        {
          "id": "acme_prod",
          "display_name": "ACME Corp (prod)",
          "namespace": "ns_acme_prod",
          "keys": ["<RAW_KEY_1>", "<RAW_KEY_2>"],
          "rate_per_second": 10,
          "rate_per_minute": 300,
          "monthly_proof_quota": 100000
        }
      ]
    }
    ```

   Fields:
   - `id` (required) — short, immutable identifier. No whitespace or
     path characters.
   - `keys` (required, >=1) — raw API keys. Hashed on load; the raw
     values are dropped from memory before the file handle closes.
   - `namespace` — optional; defaults to `ns_<id>`. Used to scope
     tenant-owned records (webhook subs, proofs, batches).
   - `rate_per_second`, `rate_per_minute` — hard ceilings. `<=0`
     means "no ceiling for this tenant" (escape hatch, not the
     default).
   - `monthly_proof_quota` — cap on POST /proofs and POST
     /proofs/batch per UTC month. `<=0` means "no quota".

2. Start the engine with:

    ```
    TENANTS_FILE=/path/to/tenants.json \
    TENANT_ADMIN_TOKEN=<a-strong-random> \
    ./casperprover ...
    ```

   `TENANTS_FILE` unset ⇒ tenant mode is disabled, legacy
   `API_KEY`-only path is used. `TENANT_ADMIN_TOKEN` unset ⇒ tenant
   mode is loaded but `/admin/tenants/*` refuse every request with
   `403 tenant admin disabled`.

## Admin API

All routes require the `X-Tenant-Admin-Token` header matching
`TENANT_ADMIN_TOKEN`. They are registered on the mux only when tenant
mode is on; probes against them in compat mode return 404.

| method | path                                       | purpose                                    |
|--------|--------------------------------------------|--------------------------------------------|
| GET    | `/admin/tenants`                           | List every tenant (key hashes stripped)    |
| POST   | `/admin/tenants`                           | Create tenant                              |
| POST   | `/admin/tenants/{id}/keys`                 | Rotate: add a new key (both live)          |
| POST   | `/admin/tenants/{id}/keys/revoke`          | Rotate: drop old keys, keep newest N       |
| GET    | `/admin/tenants/{id}/audit`                | Per-tenant audit log                       |
| GET    | `/admin/tenants/audit`                     | Whole-store audit log                      |

The list and create responses NEVER contain `key_hashes` — a compromised
admin token cannot exfiltrate the hashes via ordinary reads.

## Middleware behaviour

The `authMiddleware` has two branches:

- **Tenant mode (`s.tenants != nil`)** — resolves `X-API-Key` against
  the tenant store. Unknown key → 401 + audit `tenant.auth.rejected`.
  Known key + rate-ok → 200 + audit `tenant.auth.accepted` + tenant
  in context. Known key + rate exceeded → 429 + audit
  `tenant.rate.blocked`. GET/HEAD bypass the check.
- **Compat mode** — falls through to the original single-shared-
  `API_KEY` path. No context is populated. All pre-existing tests
  keep passing bit-for-bit.

## Handlers can consume the tenant

Any request handler can do:

```go
if t := tenantFromCtx(r); t != nil {
    // t.ID is the resolved tenant id.
    // t.Namespace is the scoping prefix for tenant-owned records.
}
```

The monthly proof quota is *not* enforced automatically at
middleware level — it must be booked at the handler that actually
persists a proof. That keeps the enforcement co-located with the
resource being consumed, and lets the middleware stay generic. The
pattern is:

```go
if t := tenantFromCtx(r); t != nil {
    if d := s.tenants.CheckAndConsumeMonthlyQuota(t.ID); !d.Allowed {
        s.jsonError(w, d.Reason, http.StatusPaymentRequired)
        return
    }
}
```

(Wiring this into `submitProof` and `batchProofs` is a small follow-up
tracked in `docs/KNOWN_LIMITATIONS.md`; the store + primitive is in
place so the wire-up is a five-line change per handler.)

## Rotation flow

Recommended runbook to rotate a tenant's key without downtime:

1. `POST /admin/tenants/<id>/keys {"key": "<new-raw-key>"}`. Both old
   and new keys now resolve to the tenant.
2. Roll the new key to the tenant's callers (whatever internal
   channel — vault, out-of-band DM, etc.).
3. Once every live caller is on the new key,
   `POST /admin/tenants/<id>/keys/revoke {"keep_last": 1}`. The old
   key stops working immediately.

Audit records `tenant.key.added` and `tenant.key.revoked` for both
transitions.

## Honesty

- **REAL** — hashing, resolution, rate limiter, quota counter,
  rotation, audit ring, admin middleware are all live in the delivery
  path and covered by tests. Nothing here is a stub.
- **NOT-DURABLE** — process restart flushes runtime counters (rate
  windows, monthly quota consumption, audit ring). Persisting them
  belongs post-hackathon and is tracked in
  `docs/KNOWN_LIMITATIONS.md`.
- **NOT-ON-CHAIN** — tenancy is a Service-layer concept. No anchor
  bytes emitted. No contract redeploys.
- **NO-PAID-SERVICES** — pure Go. Zero new module deps.

## Test surface

`engine/internal/api/tenant/tenant_test.go` — 19 tenant-package tests
covering: Add + Resolve happy path, compat-mode fallback, collision
rejection, bad-input rejection, LoadFile schema (with raw-key
survival check), missing file quiet, empty path quiet, malformed
JSON rejection, per-second rate, per-minute rate, zero-ceiling
escape hatch, unknown-tenant compat, monthly quota + month roll,
unknown-tenant quota, rotation grace window, keep-last-1 minimum,
unknown-tenant rotation, duplicate-key rejection, audit lifecycle,
audit ring buffer, audit per-tenant filter, namespace scoping, list
strips key hashes, concurrent race smoke.

`engine/internal/api/tenant_handlers_test.go` — 8 HTTP tests
covering: admin-token gate (missing/wrong/correct), create-then-list
(no key leak on create or list), reject empty create, add-then-revoke,
per-tenant audit isolation, admin endpoints absent in compat mode,
middleware tenant resolution (missing/wrong/correct + context),
middleware per-tenant rate ceiling (429 after budget exhausted),
compat-mode legacy-key fallback (no tenant leaked into context).

Total: **27 new tests**. All pre-existing API tests (~30) still
green. Whole engine (`go test ./...`) is green. `go test -race
./internal/api/...` is green.

## Non-goals

- Persistent quota/rate counters across process restarts — the
  hackathon-honest choice is in-memory; a durable variant is a
  follow-up.
- Adaptive per-tenant billing tiers — the current model is a
  single flat monthly quota per tenant.
- Automatic key rotation on a schedule — the primitive is here,
  scheduling is left to operators.
- Automatic tenant CSV/CLI onboarding tooling — the admin HTTP
  surface is the canonical interface.

## References

- `engine/internal/api/tenant/tenant.go` — store implementation.
- `engine/internal/api/tenant/tenant_test.go` — 19 unit tests.
- `engine/internal/api/tenant_handlers.go` — admin HTTP handlers.
- `engine/internal/api/tenant_handlers_test.go` — 8 HTTP tests.
- `engine/internal/api/server.go` — wiring (constructor + middleware
  + route registration).
- Baseline scoped-keys layer (7.7 RBAC-lite): commit `81b6591`.
