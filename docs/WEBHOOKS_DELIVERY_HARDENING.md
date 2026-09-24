# Webhook delivery hardening (BF / backlog 7.8 extension)

## Scope

Pack BF extends the webhook subsystem shipped in `81b6591` (backlog 7.7 —
outbound webhooks with HMAC-SHA256 signing, retries, backoff, and an
in-memory dead-letter list). The 7.7 baseline already has: subscription
registry with owner-scoping, background delivery worker, deterministic
exponential backoff (1 s → 5 min, 8 attempts), a dead-letter list with a
1000-entry cap, and Prometheus counters/histograms/gauges (added by P in
`503634f`).

What BF adds — all additive, no wire-breaking changes:

1. **Full-jitter backoff** — each retry pulls a uniform random draw
   from `[0, cap)` instead of the deterministic `initial * 2^(n-1)`.
   Prevents a synchronised retry herd when many subscriptions fail
   against the same overloaded receiver.
2. **Retry-After respected** — a `Retry-After` header on a failed
   response (both `<seconds>` and HTTP-date forms) is honoured up to a
   hard ceiling of 15 minutes. Applies to any non-2xx, but is most
   useful on 429 / 503.
3. **Per-subscription circuit breaker** — after 20 consecutive failed
   attempts across events, a subscription is paused for 2 minutes. A
   single 2xx delivery resets the counter and clears the pause.
4. **Dead-letter TTL** — dead-letter entries older than 24 h are
   evicted on the same tick that runs delivery, in addition to the
   pre-existing 1000-entry cap.
5. **Idempotency key header** — every attempt emits
   `X-CP-Idempotency-Key: <sub_id>-<attempt>-<sha256(body)[:8]>`. A
   well-behaved receiver can dedupe a retried delivery whose 2xx
   crossed the wire but never reached us.

## Wire contract additions

All existing headers keep their meaning. One new header:

```
X-CP-Idempotency-Key   <sub_id>-<attempt>-<8-hex prefix of sha256(body)>
```

Receivers are still required to verify `X-CP-Signature`. The
idempotency key is advisory and safe to ignore.

## New subscription fields

Exposed via `GET /v1/webhooks/{id}`:

```json
{
  ...,
  "consecutive_failures": 0,
  "circuit_open_until": "0001-01-01T00:00:00Z",
  "circuit_tripped_total": 0
}
```

`consecutive_failures` is the current streak. It goes to zero on any
2xx delivery. `circuit_open_until` is only non-zero while the sub is
paused. `circuit_tripped_total` is monotonically increasing over the
process lifetime — useful in operations to spot pathological
receivers even after the current pause has expired.

## Configuration

Constants live in `engine/internal/api/webhooks.go`:

| constant                       | value      | role                                              |
|--------------------------------|------------|---------------------------------------------------|
| `webhookMaxAttempts`           | 8          | retries before dead-lettering (unchanged from 7.7) |
| `webhookInitialBackoff`        | 1 s        | jitter cap for attempt 1                          |
| `webhookMaxBackoff`            | 5 min      | jitter cap ceiling                                |
| `webhookRetryAfterCeiling`     | 15 min     | hard cap on receiver-requested delay              |
| `circuitBreakerThreshold`      | 20         | consecutive failures before pausing a sub         |
| `circuitBreakerCooldown`       | 2 min      | pause window before deliveries resume             |
| `webhookDeadLetterLimit`       | 1000       | ring-buffer cap (unchanged from 7.7)              |
| `webhookDeadLetterTTL`         | 24 h       | age eviction bound                                |

The values are chosen to be safe defaults for a single-node engine.
Making them configurable via `engine.toml` is tracked in
`docs/KNOWN_LIMITATIONS.md` (Webhooks section) but is not required for
the hackathon submission.

## Interaction with existing behaviour

* `deliverOnce` gains two guards executed under `s.mu`: evict expired
  dead-letters, and defer events whose subscription has an open
  circuit. Neither guard changes the observable outcome for a
  subscription that never trips — the jitter change is the only
  path-difference on the happy retry path.
* `recordSuccess` clears both the streak counter and any active pause,
  so a subscription "wakes up" as soon as it recovers.
* `recordFailure` still moves an event to the DLQ once it hits
  `webhookMaxAttempts`; the jitter change only affects the retry
  delay for events that have not yet hit that ceiling.

## Test surface

`engine/internal/api/webhooks_hardening_test.go` (15 test cases):

- `TestJitteredBackoffStaysInsideCap` — every draw for attempts
  1..20 lies in `[0, cap)`.
- `TestJitteredBackoffAttemptZeroTreatedAsOne` — defensive.
- `TestJitteredBackoffProducesSpread` — a naive constant-return
  implementation would fail this.
- `TestParseRetryAfter` — 8 sub-cases covering integer seconds,
  HTTP-date, past date, malformed input, whitespace.
- `TestRetryAfterHonouredOn429` — real HTTP receiver returns 429 with
  a 90 s delay; the event's `NextTryAt` is pushed at least 90 s out.
- `TestRetryAfterCappedByCeiling` — a receiver asking for 24 h is
  clamped to 15 min.
- `TestCircuitBreakerTripsAndSkips` — 20 consecutive 500s trip the
  circuit; a subsequent enqueue during cool-down does **not** fire an
  HTTP call; a 2xx after cool-down clears both counter and pause.
- `TestDeadLetterTTLEviction` — one 25 h-old and one 1 min-old
  dead-letter; after a tick only the fresh one survives.
- `TestIdempotencyKeyDeterministic` — same inputs → same key; bumping
  attempt or body changes the key.
- `TestDeliveryEmitsIdempotencyHeader` — end-to-end: the receiver
  sees the header the store computed.

Full suite: `go test ./...` in `engine/` — 18 packages green.

## Non-goals (out of scope for BF)

- Durable queue / DB-backed dead-letter store — tracked in
  `KNOWN_LIMITATIONS.md`; the in-memory design is a hackathon-honest
  choice, not a claim of production readiness.
- Adaptive rate-limiting per-receiver based on error signals — the
  circuit breaker gives us the safety-net portion; adaptive dosing is
  a follow-up.
- Retry-after semantics for successful (2xx) responses — the RFC
  allows it, but no engine caller emits 2xx+Retry-After today.
- Making thresholds configurable — deliberately deferred; the current
  values are defensible defaults.

## Honesty

- **REAL**: jittered backoff, Retry-After parsing, circuit breaker,
  DLQ TTL eviction, idempotency header are all live in the delivery
  path and covered by tests. Not simulated.
- **NOT ON-CHAIN**: this is API/engine-layer plumbing. No contract
  redeploys are required. No new anchor bytes are emitted.
- No paid services. No mainnet interaction. No new module dependencies.

## References

- `engine/internal/api/webhooks.go` — hardened delivery worker.
- `engine/internal/api/webhooks_hardening_test.go` — 15 new tests.
- Base pack (7.7): commit `81b6591`.
- P's DLQ replay endpoint: commit `62ef1a5`.
- P's Prometheus instrumentation: commit `503634f`.
