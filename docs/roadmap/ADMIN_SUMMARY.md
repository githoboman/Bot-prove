# Admin dashboard rollup endpoint

Slot: 9.6 engine-side. One read-only endpoint the FE admin dashboard
polls to answer "what is this engine doing right now?" — a single
network hop instead of six.

## Endpoint

```
GET /v1/admin/summary
Scope: admin:read
Cache-Control: no-store
```

Requires `admin:read` when the scoped-keys file is loaded; falls back
to blanket auth otherwise (matches the wider engine pattern).

## Response shape

```jsonc
{
  "version":     "post-hackathon/roadmap",
  "server_time": "2026-07-26T16:45:12Z",
  "uptime":      "3h14m",

  "subsystems": {
    "keyring":  true,
    "receipts": true,
    "decision": false,
    "quorum":   false,
    "scopes":   true,
    "postgres": true,
    "metrics":  true,
    "webhooks": true
  },

  "keystore": {
    "kind":    "memory",
    "backing": "in-memory",
    "keys":    3
  },

  "keys": [
    {
      "id":         "pq-abc123",
      "algo":       "ml-dsa-65",
      "created_at": "2026-07-25T18:00:00Z",
      "status":     "active"
    }
    /* ... one entry per key, metadata only ... */
  ],

  "webhooks": {
    "subscriptions": 4,
    "queue_depth":   0,
    "dead_letters":  2,
    "known_events":  ["proof.verified", "proof.anchored", ...]
  },

  "scopes": {
    "loaded":      true,
    "key_count":   6,
    "source_path": "/etc/casperprover/scoped-keys.json"
  },

  "contracts": {
    "proof_registry": "96e97c4d..."
  }
}
```

Optional sections (`keystore`, `keys`, `webhooks`, `scopes`) drop out
of the payload when the corresponding subsystem is disabled, rather
than appearing zero-valued. This gives the FE dashboard a clean
`if payload.webhooks:` render pattern.

## What is NOT in the payload

By design — every one of these is a leak vector, and there is a
regression test (`TestAdminSummary_NoSecretsInPayload`) that greps
for them:

- Private key bytes (only metadata, algo, key id and status)
- HMAC / webhook shared secrets
- Raw `X-API-Key` values from the scoped-keys file
- Per-caller identity headers

## Per-caller vs. pod-level fields

Everything in this endpoint is pod-level aggregate — this is not the
per-caller "my webhooks" view. Callers still hit `/v1/webhooks` for
their own subscription list. The rollup is an operator surface.

## Test coverage

- `TestAdminSummary_Shape` — bare Server, response shape, optional
  sections correctly omitted.
- `TestAdminSummary_WithWebhooks` — real `webhookStore` wired in;
  `Subscriptions`/`QueueDepth`/`DeadLetters` reflect live state.
- `TestAdminSummary_NoSecretsInPayload` — subscription secret is not
  present in the response body; guards against future extenders
  accidentally leaking key material.

## Follow-ups

- Prometheus scrape endpoint alongside this JSON payload — the two
  answer different questions (`/metrics` = numeric time-series;
  `/v1/admin/summary` = current-state snapshot). Neither replaces
  the other.
- Per-tenant `/v1/admin/summary?tenant=…` filter — deferred until
  the scoped-keys model supports multi-tenant enumeration.
