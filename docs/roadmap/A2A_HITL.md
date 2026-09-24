# A2A Workflow & HITL Policy Service — Design

Ref: `handoff/CP_FINAL_TASKS_V2.md` §D.

> **Status update (Pack AQ, 2026-07-26):** the 30-day slice is wired.
> Implementation notes and the honest contract of what is real today
> live in [`../DECISION_A2A_HITL.md`](../DECISION_A2A_HITL.md). This file
> stays as the design record.

## Problem

The current `Judge` orchestrator supports a single `Provider` per decision
run. Real agent-to-agent (A2A) workflows involve multiple providers with
different capabilities and trust levels, and require a human-in-the-loop
(HITL) escape hatch when confidence is low or a critical facet abstains.

## Design overview

```
   ┌──────────┐   ┌────────────┐   ┌──────────────┐   ┌────────────┐
   │ Provider │──▶│ FacetJudge │──▶│ Aggregator   │──▶│ Gate       │
   │ pool (N) │   │            │   │ (Byz-robust) │   │ (on-chain) │
   └──────────┘   └────────────┘   └──────────────┘   └────────────┘
                        │                 │
                        ▼                 ▼
                  HITL policy       Equivocation
                  service           ledger
                  (approve/         (existing)
                   escalate/
                   veto)
```

## Components

### Provider pool

- N ≥ 1 providers, each implementing the existing `decision.Provider`
  interface.
- Each provider declares a `TrustLevel` in `{system, delegated,
  observational}` and a `Capabilities` set (LLM, retrieval, oracle,
  compliance-check, …).
- A `ProviderRouter` selects a subset of providers for a given decision
  based on facets requested by the policy.

### HITL policy service

Small stateless service (out-of-tree in production; in-tree as a stub
package `engine/internal/hitl/`) that answers the question:

    given a facet aggregation result, does this decision require a human?

- Inputs: aggregation result (verdict, confidence, per-facet outcomes,
  abstention flag), policy identifier.
- Output: `{action: pass|escalate|veto, reason, ticket_id?}`.
- Policies are declarative: `{if any critical facet ABSTAINed then escalate;
  if confidence < τ_hitl then escalate; if same-signer conflict detected
  then veto}`.
- Escalation creates a durable ticket (`hitl_tickets` table) and blocks the
  downstream gate until the ticket is resolved.

### Evidence bus

- Every provider emits a signed attestation.
- Every facet-judge emits a signed verdict.
- Every HITL decision emits a signed resolution.
- All are hashed into the existing Merkle receipt so the on-chain commitment
  binds the full workflow, not just the final verdict.

## Milestones

1. **Design partner interviews (7 days).** Confirm the HITL policy shape
   against 2–3 real risk desks.
2. **Provider pool prototype (7 days).** `ProviderRouter` + two fixture
   providers with different trust levels; unit tests + one integration
   test through the existing `Judge`.
3. **HITL stub (7 days).** `engine/internal/hitl/` with `Decide()` and a
   Postgres-backed ticket store; API endpoints
   `POST /hitl/decide`, `GET /hitl/tickets/:id`,
   `POST /hitl/tickets/:id/resolve`.
4. **End-to-end demo (9 days).** One reproducer path where a facet abstains,
   HITL escalates, a human resolves via API, and the gate finally releases.

## Non-goals

- Autonomous HITL replacement by another AI (defeats the point).
- Cryptoeconomic incentives for human reviewers (design only in the
  90–180 day window).
- UI for the ticket queue (roadmap; a simple JSON API is enough for the
  30-day window).

## Open questions

- **Ticket storage:** Postgres in the engine, or a separate service? Leaning
  Postgres in engine for the 30-day slice; extract later.
- **Timeouts:** what happens when a ticket sits unresolved for > SLA? The
  gate must have a `stale_escalation` state distinct from `pending`.
- **Auditability:** every HITL resolution needs a signed evidence blob; the
  reviewer's identity attaches to the Merkle root.

## Acceptance criteria

- [ ] `engine/internal/hitl/` package with ≥ 6 unit tests.
- [ ] `docs/roadmap/A2A_HITL.md` cross-linked from `30-DAY.md` and
      `DECISION_LAYER.md`.
- [ ] Design partner feedback captured under `docs/roadmap/FEEDBACK.md`.
