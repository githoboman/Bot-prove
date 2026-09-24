# Webhook Replay & Dead Letters

Closes backlog item **7.7-r** ("Webhooks real delivery worker — retries
+ backoff + DLQ + replay") for the parts that were still open:
**stable dead-letter delivery ids and an operator replay endpoint.**

## What was already in place

The webhook subsystem (`engine/internal/api/webhooks.go`) already
handled:

- HMAC-SHA256 signature envelope (`X-CP-Signature`, `X-CP-Timestamp`,
  `X-CP-Event`, `X-CP-Delivery`) verified via
  `VerifyWebhookSignature` for SDK consumers.
- Retry with exponential backoff (1s → 2s → 4s … capped at 5 min),
  up to 8 attempts.
- Dead-letter list (in-memory, capped at 1000 entries).
- Owner-scoped listing via `GET /v1/webhooks/dead-letters` — a
  caller only sees dead letters for its own subscriptions.

## What this change adds

- **Stable delivery id on every dead letter.** Each `deadLetter` now
  carries a `DeliveryID` of the form `dl_` + 12 hex chars derived
  from `sha256(subID | createdAt)`. It is stable for the lifetime
  of the process and returned in the `GET /v1/webhooks/dead-letters`
  response.
- **Owner key stored on the dead letter.** Previously ownership was
  reconstructed by cross-referencing the subscription. Now the
  dead-letter record captures the owning key directly, so an
  ownership check does not require an active subscription — which
  also means a caller can see (but not replay) dead letters for
  subscriptions they have already unregistered.
- **Replay endpoint.**
  `POST /v1/webhooks/dead-letters/{delivery_id}/replay` — pushes the
  dead-lettered event back onto the delivery queue with
  `attempts=0` and `NextTryAt=now`, and removes it from the
  dead-letter list. The event body is preserved *byte-for-byte* so
  the HMAC signature verifies identically on the receiver side —
  the replay is indistinguishable from a first attempt.
- **Ownership enforcement.** Only the caller whose API-key hash
  matches the dead letter's owner may replay. Any other caller
  receives an opaque `404 not found` — never leaks that the id
  exists.
- **Unregistered subscription = 409.** If the subscription targeted
  by a dead letter was already deleted, the replay refuses with
  `409 Conflict` and a `subscription unregistered — cannot replay`
  message.

## Wire contract

Request:

```
POST /v1/webhooks/dead-letters/dl_abcdef012345/replay
X-API-Key: sk_live_…
```

Response (success):

```
HTTP/1.1 202 Accepted
Content-Type: application/json

{"delivery_id":"dl_abcdef012345","status":"enqueued"}
```

Failure modes:

- `404 not found` — id does not exist or belongs to another caller.
- `409 subscription unregistered — cannot replay` — target sub
  gone; the event body is still visible via the dead-letters list
  but the destination URL cannot be re-attempted.
- `503 webhook subsystem disabled` — should never fire in production;
  present so a defensive nil-check does not panic in a stripped
  build.

## Tests

`engine/internal/api/webhooks_test.go` grows three new tests:

- `TestWebhookReplayDeadLetter` — 8 failing attempts → dead letter →
  replay → the receiver observes one more attempt with a
  byte-identical body and a valid HMAC signature. Wrong-owner replay
  is rejected; second replay of the same id is rejected.
- `TestWebhookReplayUnregisteredSubscription` — dead letter whose
  subscription was deleted cannot be replayed.
- `TestWebhookReplayHTTPHandler` — end-to-end through the HTTP
  handler with owner isolation.

Everything in `engine/…` still green
(`go test ./...` → 22 packages, all `ok`).

## Known limits

- **In-memory only.** Dead letters and the replay queue live in a
  single process; a restart loses both. A durable Postgres-backed
  variant is called out in
  `docs/roadmap/KNOWN_LIMITATIONS.md` (webhook subsystem section).
- **No admin-wide replay.** Replay is per-caller — an operator with
  a super-user key cannot yet replay every caller's dead letters
  from a single endpoint. Would need an explicit admin scope; not
  in scope for this change.
- **Single-node dedup.** If the receiver was down long enough that
  the event dead-lettered *and* the receiver had already accepted a
  duplicate through some other retry path, a replay will double-fire.
  Receivers should key idempotency on `X-CP-Delivery` (which stays
  the *dispatch attempt* id, not the delivery id — see
  `webhooks.go` `deliverEvent`).
