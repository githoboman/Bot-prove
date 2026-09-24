# Decision Attestation Layer

**Last verified:** 2026-07-25 (commits `3c5d84a` · `6db0849` · attestation slice).

The decision attestation layer is a **verifiable off-chain judgement flow with
on-chain commitments and gates**. Any principal holding a Casper wallet key
can submit a *decision commit* — a structured payload plus a nonce — to the
proof-registry. Independent facet evaluators judge the commit against a fixed
set of dimensions, an aggregation function collapses those judgements into a
single verdict, and the final commit digest is what downstream modules (e.g.
`defi-mock`) gate on.

Nothing in this layer runs autonomous execution loops, invokes large language
models, or takes non-deterministic actions at runtime. Providers are
pluggable; the default provider is a deterministic fixture so the demo and
the reproducer script give byte-identical output across machines.

The layer is delivered as pure Go under `engine/internal/decision/` plus a
CLI reproducer under `engine/cmd/decision-demo/`. Its ZK-friendly commit
digest (`CommitDigest()`) is the value bound as public input in the existing
off-chain Groth16 pipeline (`engine/internal/zkverifier`).

## Facets

The layer evaluates every decision along four dimensions. Two are **critical**
— a `REJECT` on either short-circuits the aggregation to `REJECT` regardless
of the numerical quorum.

| Kind              | Meaning                                                      | Critical |
|-------------------|--------------------------------------------------------------|:--------:|
| `safety`          | Hard-policy invariant: no prompt injection, no exfiltration, no unsafe payload. | ✅ |
| `equivocation`    | The same signer previously committed a *conflicting* decision in the current window. | ✅ |
| `correctness`     | The decision satisfies its own declared post-condition (numeric bounds, referenced hash exists, …). | — |
| `spec_compliance` | The payload matches the declared `spec_id` (schema, versioned rule set). | — |

A critical facet that **abstains** (not enough evidence to approve, but no
evidence to reject either) is not treated as a green light: the aggregate
becomes `ABSTAIN` rather than being carried by the non-critical quorum.

## Verdicts

Every facet and the aggregate resolve to exactly one of:

- **`APPROVE`** — accept the decision.
- **`ABSTAIN`** — refuse to take sides (used when confidence is below the
  policy threshold, or when a critical facet cannot produce a verdict).
- **`REJECT`** — reject the decision.

## Aggregation

`decision.Aggregate(policy, verdicts)` runs three passes:

1. **Critical-veto pass.** If any critical facet returns `REJECT` the
   aggregate is `REJECT` and the vetoing facet is recorded in
   `VetoedBy` for the receipt.
2. **Critical-abstain guard.** If any critical facet did not itself reject
   but also did not approve, the aggregate is `ABSTAIN`. A missing safety
   judgement is not a green light.
3. **Non-critical quorum.** Among the non-critical facets, `APPROVE`
   verdicts below the policy `MinConfidence` are treated as `ABSTAIN`
   for counting. The aggregate is `APPROVE` iff the count of remaining
   approves meets `ApproveThreshold`. A non-critical `REJECT` also
   rejects the aggregate.

`DefaultAggregationPolicy` requires 2/2 non-critical approves at confidence
≥ 0.6.

## Commit digest and proof binding

`DecisionCommit.CommitDigest()` returns `sha256` over:

```
LP(DecisionID) ‖ (LP(Kind) ‖ Verdict)⁺  ‖ AggregateByte ‖ LP(VetoedBy)
```

with facet entries sorted by `Kind` so evaluator scheduling never affects
the digest. `LP(x)` is 8-byte big-endian length + bytes. Two commits with
identical decisions and identical facet verdicts always produce the same
digest across languages and processes.

The digest is what the on-chain proof-registry stores and what
`engine/internal/zkverifier` binds as public input for the off-chain Groth16
proof (real gnark, BN254). The ZK circuit itself is unchanged — it now takes
`CommitDigest` where it previously took the raw proof-registry key.

## Downstream gate

`decision.GateEvaluator.Evaluate(commit, challenge)` maps a `DecisionCommit`
and an optional `ChallengeResult` to one of:

- **`PENDING`** — challenge window still open, no downstream action yet.
- **`ALLOWED`** — approved, window closed, no successful challenge.
- **`BLOCKED`** — rejected/vetoed or a successful challenge landed inside
  the window.
- **`ABSTAINED`** — the aggregate was `ABSTAIN`; downstream modules must
  treat this as "no answer" — they must not allow, and they must not
  silently block.

Challenges filed **after** the window closed are defensively ignored by the
local evaluator. The authoritative on-chain rule is expected to be the same.

## Reproducer command

The demo binary reproduces all five paths (APPROVE, ABSTAIN, REJECT via
safety veto, REJECT via equivocation veto, and a successful in-window
challenge blocking an APPROVE) with a fixed clock so output is diffable:

```
go run ./engine/cmd/decision-demo                  # all paths
go run ./engine/cmd/decision-demo -path=approve
go run ./engine/cmd/decision-demo -path=abstain
go run ./engine/cmd/decision-demo -path=inject
go run ./engine/cmd/decision-demo -path=equivocate
go run ./engine/cmd/decision-demo -path=challenge
```

Each receipt is JSON with the fields:

- `decision_id` — hex sha256 of the decision, exactly what the proof-registry
  stores.
- `facets` — verdict for every kind, in evaluator order.
- `aggregate` — one of `APPROVE`, `ABSTAIN`, `REJECT`.
- `vetoed_by` (optional) — critical facet that forced a REJECT.
- `abstain_reason` (optional) — quorum-gap or critical-abstain explanation.
- `commit_digest` — hex sha256 that is bound into the ZK proof.
- `gate` — downstream gate decision under the fixed clock.
- `challenge` (optional) — the challenge input, if any.

## What is NOT in this layer

The following are deliberately out of scope for the hackathon slice; each is
listed in `docs/KNOWN_LIMITATIONS.md` with a link back to the roadmap:

- **Threshold BLS quorum registry.** The current provider is a single
  deterministic fixture. Multi-evaluator threshold signatures are the
  30-day roadmap item.
- **On-chain Groth16 verifier.** Casper 2.0 lacks a pairing precompile.
  The Groth16 proof is verified off-chain; the on-chain commitment is
  the ZK-friendly `CommitDigest`.
- **Conformal / risk-controlled abstention.** The current confidence
  threshold is a fixed number in `AggregationPolicy`, not a calibrated
  prediction set. Roadmap.
- **Byzantine-robust facet aggregation.** The current aggregator trusts
  the single provider it was configured with. Roadmap.
- **Incremental Merkle batch receipt.** Each decision emits its own
  receipt; batching several receipts into one Merkle root with an
  inclusion proof is a roadmap item.

## Provenance lineage

The receipt-emission surface is `docs/PROVENANCE_LINEAGE.md` —
`receipts.Service` (in `engine/internal/receipts/`) signs each
`DecisionCommit` under the active PQ key and returns it in three
interoperable shapes: internal `DecisionReceipt`, W3C VC 2.0, and
Agent Receipt draft v0.3. Upstream providers are referenced by
canonical hash so receipts form a DAG walkable via
`GET /v1/receipts/{id}/lineage`. See the honest contract for the
OTel-span attribute mapping and the exact out-of-scope list.
