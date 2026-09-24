# API Lifecycle — Versioning, Webhooks, RBAC-Lite

Ref: `handoff/CP_FINAL_TASKS_V2.md` §D.

## Problem

The current engine surfaces 32 endpoints under a single implicit "v0" and
no formal versioning, no webhook out-flow, and single-key API auth. To
onboard SDK consumers and a first paying design partner in the 30-day
window we need:

- Explicit version negotiation.
- Reliable webhooks for asynchronous events (proof registered, slashing
  applied, decision receipt emitted).
- RBAC-lite for tenants who need multiple scoped API keys.

## Versioning

- Path prefix: `/v1/...`. Existing endpoints alias into `/v1/...` for
  compatibility; the un-prefixed routes emit a deprecation header for one
  minor release then are removed.
- Header negotiation: clients may send `Accept: application/vnd.cp+json;
  version=1`; server returns the matching version or 406.
- Breaking changes bump the major version; additive changes bump the
  minor. Changelog in `docs/API_CHANGELOG.md` (created alongside the
  first `/v1/` release).

## Webhooks

- Subscription model: `POST /v1/webhooks` with `{event_type, url,
  secret}`. Server stores the secret encrypted.
- Delivery: signed POST with header
  `X-CP-Signature: t=<ts>, v1=<hmac-sha256>`, where `hmac-sha256` is
  computed over `t + '.' + body` using the shared secret.
- Retries: exponential backoff, cap at 8 attempts, dead-letter queue after
  cap. `GET /v1/webhooks/:id/deliveries` exposes the delivery log.
- Event catalogue (initial):
  - `proof.registered`, `proof.revoked`
  - `slashing.applied`, `slashing.challenged`
  - `decision.receipt.emitted`
  - `hitl.ticket.opened`, `hitl.ticket.resolved`
  - `governance.proposal.executed`, `governance.emergency_pause`

## RBAC-lite

- API keys become tenant-scoped: `sk_<tenantid>_<random>`.
- Each key carries a `role` in `{owner, admin, service, readonly}`.
- Per-role scope allow-list stored server-side, not encoded in the key
  material (so revocation is instant).
- Auditable: every mutating request logs `{tenant_id, key_prefix, role,
  request_id}`.

Not full RBAC yet — see `docs/roadmap/MULTITENANCY.md` for the 90-day
plan.

## Rate limiting

Existing per-IP limiter stays. Add per-tenant limiter driven by the
tenant plan; enforce at ingress; expose current counters via
`GET /v1/tenants/me/quota`.

## Milestones

1. **Versioning (3 days).** `/v1/` alias + deprecation header + doc.
2. **Webhooks (7 days).** Subscription CRUD, signed delivery, retry
   loop, delivery log.
3. **RBAC-lite (5 days).** Tenant model in Postgres, role check middleware,
   audit log.
4. **Docs + SDK integration (3 days).** All three SDKs learn how to
   negotiate the version, subscribe to a webhook, and check quota.

## Non-goals

- OAuth2 / OIDC identity federation. Roadmap (90-day).
- Fine-grained per-endpoint scopes beyond the role level. Roadmap.
- Billing integration. See `docs/roadmap/MULTITENANCY.md`.

## Acceptance criteria

- [ ] `/v1/` prefix live for every existing route with a deprecation
      header on the un-prefixed alias.
- [ ] Webhook delivery survives a target-side 5xx / network partition
      via retry loop; DLQ inspectable.
- [ ] Every mutating request logs tenant + role.
- [ ] `docs/API_CHANGELOG.md` exists and is updated per PR that touches
      the API surface.
- [ ] `docs/roadmap/API_LIFECYCLE.md` cross-linked from `30-DAY.md`.
