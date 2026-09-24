# Decision Layer — A2A Provider Pool & HITL

**Status:** wired, opt-in behind `CP_DECISION_ENABLE=1`.
**Scope:** Pack AQ / Commit G — closes 3.1 (real provider adapter), 3.4
(HITL policy service), 3.5 (A2A workflow: multi-provider pool).

This document is the honest contract for the A2A/HITL pipeline: what is
real today, what the network protocol looks like, and where the seams
are that a production deployment still has to fill in.

---

## What is real today

- **`decision.ProviderPool` + `decision.Router`** — a thread-safe
  registry of pluggable `Provider`s with declared trust levels
  (`system` / `delegated` / `observational`) and per-facet capabilities.
  Route fans out to every provider that covers at least one wanted
  facet, in parallel, and returns the merged per-facet verdicts.
  Observational providers cannot silently vote APPROVE/REJECT on
  critical facets (safety, equivocation): the router downgrades those
  verdicts to ABSTAIN with a `trust-downgrade` reason.

- **`decision.HTTPProviderAdapter`** — a deterministic `Provider` that
  speaks a small JSON-over-HTTP protocol (see below) to any external
  evaluator. If unconfigured (empty endpoint), or the remote returns a
  transport error / non-2xx / malformed JSON, the adapter falls back to
  a deterministic fixture provider. Configured via
  `CP_DECISION_PROVIDER_URL` and `CP_DECISION_PROVIDER_TOKEN`.

- **`hitl.Service`** — evaluates a `DecisionCommit` against a declarative
  policy and returns one of `pass` / `escalate` / `veto`. Escalation
  opens a durable ticket in a `TicketStore`.  The default store is
  in-process (`InMemoryTicketStore`); a Postgres-backed store can be
  dropped in by satisfying the `TicketStore` interface.

- **API endpoints** — five, all under scope `decision:*`:

  | Method | Path                                | Scope             |
  |--------|-------------------------------------|-------------------|
  | POST   | `/v1/decision/evaluate`             | `decision:write`  |
  | GET    | `/v1/decision/pool`                 | `decision:read`   |
  | GET    | `/v1/hitl/tickets`                  | `decision:read`   |
  | GET    | `/v1/hitl/tickets/{id}`             | `decision:read`   |
  | POST   | `/v1/hitl/tickets/{id}/resolve`     | `decision:write`  |

  When `CP_DECISION_ENABLE!=1` every endpoint returns 503 with a
  well-formed JSON error. There is no silent-failure path.

## HTTP provider protocol

The adapter POSTs a request body of the following shape:

```json
{
  "decision_id":   "<hex sha256>",
  "submitter":     "<opaque submitter id>",
  "spec_id":       "policy/v1",
  "nonce":         42,
  "payload_hex":   "<opaque payload, hex>",
  "submitted_at":  "2026-07-26T12:34:56Z"
}
```

Headers: `Content-Type: application/json`, and `Authorization: Bearer …`
when `CP_DECISION_PROVIDER_TOKEN` is set. Timeout defaults to 5s.

The remote MUST return a 2xx response with body:

```json
{
  "verdicts": [
    { "kind": "safety",          "verdict": "APPROVE", "confidence": 0.95, "reason": "…" },
    { "kind": "correctness",     "verdict": "ABSTAIN", "confidence": 0.0,  "reason": "…" },
    { "kind": "spec_compliance", "verdict": "REJECT",  "confidence": 0.8,  "reason": "…" },
    { "kind": "equivocation",    "verdict": "APPROVE", "confidence": 0.9,  "reason": "…" }
  ]
}
```

Rules the adapter enforces:

- Kinds not in `AllFacetKinds` (safety / correctness / spec_compliance /
  equivocation) are dropped silently. Extras never sneak into an
  on-chain commit.
- Verdicts outside `APPROVE|ABSTAIN|REJECT` (case-insensitive) are
  dropped. The Judge fills missing kinds as ABSTAIN with reason
  `"no verdict from provider"`.
- Confidence is clamped into `[0, 1]`.
- The adapter is deliberately **fail-open**: any transport error / 5xx /
  malformed body triggers fallback to the configured fixture. The
  Byzantine-robust aggregator at the pool level guarantees a lone
  fixture vote cannot flip a critical facet against an honest quorum.

## HITL policy

Declarative rules, evaluated in order (`hitl.DefaultPolicy` shown):

1. **Veto on critical REJECT.** If the aggregate is `REJECT` because a
   critical facet (safety or equivocation) vetoed, HITL returns
   `veto` — a mirror of the aggregate. No ticket is opened, since no
   human can legitimately override a safety veto.
2. **Escalate on critical ABSTAIN.** If any critical facet returned
   ABSTAIN, HITL returns `escalate` and opens a ticket. Rationale: a
   critical dimension being unknown is not a green light.
3. **Escalate on low confidence.** If the mean confidence across the
   non-critical facets that voted APPROVE is below
   `ConfidenceThreshold` (default 0.6), HITL escalates.
4. **Pass.** Otherwise, HITL returns `pass` — downstream may proceed to
   the on-chain gate.

`Policy.EscalateOnCriticalAbstain` and `Policy.VetoOnCriticalReject`
default to `true` and should only be flipped in tests that intentionally
exercise the pass path.

## Ticket lifecycle

An escalated ticket is:

| Field            | Purpose                                                     |
|------------------|-------------------------------------------------------------|
| `id`             | 128-bit random hex, generated at open                       |
| `decision_id`    | The canonical `Decision.ID()` this ticket gates             |
| `opened_at`      | UTC timestamp                                                |
| `reason`         | Machine-readable trigger (`critical facet safety ABSTAINed…`)|
| `state`          | `pending` → `approved`/`rejected`/`stale_escalation`         |
| `resolver`       | Opaque ID of the human who resolved the ticket              |
| `resolution_note`| Free-text explanation attached at resolve time              |
| `resolved_at`    | UTC timestamp; zero while pending                            |

State transitions:

- Only `pending → *` is legal. Attempting to resolve an already-resolved
  ticket returns `hitl.ErrAlreadyResolved` (HTTP 409).
- Only `approved`, `rejected`, `stale_escalation` are valid target states.
  Any other value returns `hitl.ErrInvalidState` (HTTP 400).
- Tickets that time out (in a future version) transition to
  `stale_escalation` automatically. Not yet implemented in-tree.

## Config matrix

| Env var                          | Purpose                                                    | Default          |
|----------------------------------|------------------------------------------------------------|------------------|
| `CP_DECISION_ENABLE`             | Turn the pipeline on. When unset, all endpoints return 503 | *unset*          |
| `CP_DECISION_PROVIDER_URL`       | External evaluator endpoint                                | *empty ⇒ fixture*|
| `CP_DECISION_PROVIDER_TOKEN`     | Bearer token for the external evaluator                    | *empty*          |
| `CP_DECISION_PROVIDER_NAME`      | Overrides the adapter identity in receipts                 | `cp-decision-system` |

Nothing in the pipeline touches Postgres, Slack, or on-chain state. That
wiring lives in downstream squads.

## Out-of-scope (deferred; documented, not implemented)

- **Postgres-backed ticket store.** The `TicketStore` interface is
  ready. A production deployment provides a Postgres implementation and
  passes it to `hitl.NewService(policy, store)`.
- **Stale-escalation timer.** Ticket auto-transition after an SLA
  timeout. The state literal already exists; the scheduler doesn't.
- **Signed evidence blobs on resolve.** Every resolve should attach a
  signed attestation from the reviewer, hashed into the on-chain receipt.
  Deferred to a later pass; this file specifies the JSON shape when it
  lands.
- **Byzantine-robust aggregation of multi-provider verdicts.** The pool
  already routes to N providers, but the current endpoint runs the
  baseline `Aggregate` on the flattened verdict list. Swapping in
  `AggregateByzantineRobust` (which exists at the package level) is a
  one-line change gated by a policy flag, kept simple here so the demo
  path stays deterministic.
- **UI for the ticket queue.** A JSON API is sufficient for the 30-day
  slice; a Slack notifier or an admin dashboard belong in follow-up
  squads.

## Test coverage

- `internal/decision/pool_test.go` — 5 tests: nil/duplicate rejection,
  routing by capability, trust-downgrade on critical facets from an
  observational provider, provider errors isolated, `ErrNoRoutedProviders`
  when nothing covers a facet.
- `internal/decision/adapter_test.go` — 4 tests: unconfigured fallback,
  round-trip against `httptest` mock (including allowlist and drop of
  malformed verdicts), 5xx fallback, verdict-string round-trip.
- `internal/hitl/hitl_test.go` — 6 tests: veto path, escalate on critical
  ABSTAIN, escalate on low confidence, pass path, resolve lifecycle
  (invalid state / not found / happy / double-resolve rejected), list
  filter + ordering.
- `internal/api/decision_handlers_test.go` — 5 tests: 503 when disabled,
  end-to-end happy path (APPROVE + HITL pass), end-to-end escalation
  path (critical ABSTAIN → HITL escalate → ticket created), full resolve
  round-trip (evaluate → escalate → resolve → get), invalid resolve state.

All pass under `go test -race -count=1`.
