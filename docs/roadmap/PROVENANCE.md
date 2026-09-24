# Provenance Lineage — W3C VC / Agent Receipt / OpenTelemetry

Ref: `handoff/CP_FINAL_TASKS_V2.md` §D.

**Status (2026-07-26): PARTIALLY SHIPPED.** The receipts package, W3C VC / Agent Receipt emitters, HTTP surface, and JSONL OTel sink are live in `engine/internal/receipts/` (Pack AR — backlog items 5.1 / 5.2 / 5.3). Honest contract in `docs/PROVENANCE_LINEAGE.md`. Remaining: Postgres-backed store driver, provider-side signatures, retention policy, non-W3C schema bridging — see the honest doc for the exact out-of-scope list.

## Problem

CP already emits a signed decision receipt (see `docs/DECISION_LAYER.md`).
The 30-day roadmap adds *lineage*: a graph of receipts that captures which
inputs, models, providers and reviewers led to a downstream decision,
compatible with three widely-deployed standards so an evaluator can reuse
existing tooling.

## Target compatibility

- **W3C Verifiable Credentials 2.0.** The receipt maps onto a VC with the
  subject = decision id, the issuer = the engine's DID, and a proof
  section carrying the ML-DSA/hybrid signature.
- **Agent Receipts (draft, agentreceipts.org spec).** A superset of the
  facet outputs mapping onto the ledger events; the AR spec's evidence
  section maps onto the Merkle batch receipt from
  `engine/internal/prover/`.
- **OpenTelemetry.** Decision spans are emitted as OTel spans with
  attributes `cp.evidence_root`, `cp.model_id`, `cp.verdict`,
  `cp.confidence`, `cp.abstain` and links to upstream/downstream spans.

## Lineage schema

```
DecisionReceipt {
  id                : uuid
  issued_at         : rfc3339
  issuer            : did
  subject           : did | uri
  evidence_root     : hex(32)
  model_id          : hex(32)
  verdict           : approve|abstain|deny
  confidence        : float
  facets            : [ FacetOutput ]
  provider_receipts : [ ProviderReceipt ]  // upstream lineage
  hitl_resolution   : HITLResolution?     // if escalated
  proof             : {
    scheme          : hybrid-ed25519+ml-dsa-65 | ml-dsa-65 | lamport-ots
    signature       : base64
    verification_method : did-url
  }
}
```

## On-chain anchoring

Each `DecisionReceipt` produces two on-chain events:

1. `evidence_root` and `receipt_hash` in `proof-registry`.
2. A tombstone entry in `verifier-gate` binding the receipt to the model
   version.

Downstream receipts reference upstream receipts by hash; the chain of
receipts forms a DAG whose validity is checked by
`engine/internal/dag/ValidateDAG()` (already PBT-fuzzed — see commit
`d59b4d4`).

## Milestones

1. **Schema definition (3 days).** JSON Schema + Go structs +
   `engine/internal/receipts/` package.
2. **VC/AR emitters (5 days).** Two output modes:
   `POST /receipts/emit?format=w3c-vc`, `?format=agentreceipt`; each with
   fixture round-trip tests.
3. **OTel bridge (3 days).** OTel spans emitted whenever a receipt is
   issued; export config in `deploy/otel-config.example.yaml`.
4. **Lineage viewer (5 days).** Simple React graph in the frontend
   showing receipt DAG traversal from a starting hash.

## Non-goals

- Full VC issuer registry / trust list infrastructure. Roadmap.
- Long-term retention of receipts (see `docs/roadmap/LEGAL.md` for
  retention policy).
- Bridging to non-W3C schemas (e.g. CACAO / EIP-4361). Roadmap.

## Acceptance criteria

- [ ] `engine/internal/receipts/` package with JSON-Schema-validated
      serialisers.
- [ ] `POST /receipts/emit` endpoint with both output modes.
- [ ] OTel spans present on every decision path (verified via a
      collector-side test).
- [ ] `docs/roadmap/PROVENANCE.md` cross-linked from `DECISION_LAYER.md`.
