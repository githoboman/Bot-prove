# Multi-Tenancy, RBAC, API-Key Lifecycle, Quotas, Metering, Billing

Ref: `handoff/CP_FINAL_TASKS_V2.md` §E.

## Problem

The 30-day slice ships RBAC-lite with tenant-scoped API keys. The 90-day
slice hardens the tenant model into a real multi-tenant platform: full
role hierarchy, key lifecycle (rotate, revoke, expire), quotas + metering
+ billing, and audit logs.

## Tenant model

```
Tenant
  ├── Plan (design-partner | metered | enterprise)
  ├── Users
  │     └── Role (owner | admin | service | readonly)
  ├── API keys
  │     └── Role + scopes + expires_at + rotation_of
  ├── Quotas (per plan)
  │     ├── verifications/mo
  │     ├── receipts stored
  │     ├── webhook subscriptions
  │     └── HITL tickets/mo
  ├── Metering
  │     └── stream of billable events → billing ledger
  └── Billing
        └── monthly invoice; usage-based line items
```

## API-key lifecycle

- **Create:** `POST /v1/tenants/{tid}/api-keys` — returns the raw key
  once, stores a SHA-256 hash server-side.
- **Rotate:** creates a new key with `rotation_of = old_key_id`; the old
  key stays valid for `grace_hours` (default 24) then auto-expires.
- **Expire:** manual `DELETE /v1/tenants/{tid}/api-keys/{kid}` or auto
  after `expires_at`.
- **Reveal:** never. If a customer loses a key they rotate.

## Roles

| Role      | Scope                                                         |
|-----------|---------------------------------------------------------------|
| `owner`   | Full tenant admin, incl. billing & user management.           |
| `admin`   | Tenant admin except billing; can rotate keys, manage webhooks. |
| `service` | Machine role. Submit/verify decisions; no admin operations.   |
| `readonly`| GETs only. Auditors, dashboards.                              |

Roles are enforced server-side via middleware; the key does not encode
the role, so revocation and role changes take effect immediately.

## Quotas

- Hard caps per plan. When exceeded, mutating requests return 429 with
  `Retry-After` set to the next quota rollover.
- Soft caps at 80% send a `quota.warning` webhook.
- Quota state exposed via `GET /v1/tenants/me/quota`.

## Metering

- Every billable event writes `{tenant_id, event_type, amount,
  request_id, timestamp}` to a Postgres ledger.
- Nightly rollup produces monthly usage per tenant.
- Ledger is append-only; corrections happen as compensating entries with
  a signed reason.

## Billing

- Postgres ledger → Stripe (or the tenant's chosen billing rail).
- Invoice generation runs on the 1st of each month at 00:00 UTC.
- Failed payments trigger a grace period (7 days) then the tenant is
  downgraded to `readonly`.

## Audit log

- Every mutation logs `{tenant_id, user_id or key_id, role, request_id,
  ip, ua, resource, action, before_hash, after_hash}`.
- Retention: 400 days (see `docs/roadmap/LEGAL.md`).
- Immutable append-only Postgres table + monthly export to cold
  content-addressed storage.

## Milestones

1. **Data model (10 days).** Postgres schema, migrations, model tests.
2. **Middleware + role enforcement (5 days).**
3. **Quota engine (10 days).** In-request check, rollover cron, warning
   webhooks.
4. **Metering + rollup (10 days).**
5. **Billing wiring (10 days).** Stripe test-mode integration first,
   real integration behind a feature flag.
6. **Audit log (10 days).**

## Non-goals

- OAuth2 / OIDC identity federation (roadmap; 180-day window).
- Fine-grained per-endpoint scopes beyond role. Roadmap.
- Team-level billing (parent/child tenants). Roadmap.

## Acceptance criteria

- [ ] Tenant CRUD with role hierarchy live in prod.
- [ ] Quota rollover cron runs at 00:00 UTC and does not drift.
- [ ] Metering ledger reconciles against the API access log within
      0.5% variance for a random-sample month.
- [ ] Stripe test-mode invoice generated for a synthetic tenant.
- [ ] Audit log queryable and included in the `/v1/tenants/me/audit`
      endpoint.
