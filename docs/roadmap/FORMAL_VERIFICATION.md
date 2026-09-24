# Formal verification (Pack AV) — TLA+ specs + TLC in CI

**Slot:** Pack AV — item 4.1 of the post-hackathon backlog.
**Delivered as:** three TLA+ specs under `specs/`, a portable TLC runner
(`specs/run-tlc.sh`), and a GitHub Actions workflow
(`.github/workflows/formal-verification.yml`) that model-checks every spec on
every push and PR that touches `specs/`.

## What this slot is (and is not)

Full formal verification of a system this size is many months of work — it
is not what a single-commit slot is meant to deliver, and the backlog is
explicit about that ("Full formal verification — man-months, не одна ветка").

What is inside the AV slot is the **scaffolding that makes formal
verification a first-class, always-green CI signal**: three finite-state
specs that already model-check clean, a runner that any developer can drop
a new spec into and have it verified by CI on the same push, and an escape
hatch (`workflow_dispatch`) to re-run on demand.

Where "TLC pass" appears in the roadmap up to this slot, it was ad-hoc: one
spec (`ProofSystemSpec.tla`) checked once by hand, with the resulting log
saved as `specs/tlc_output.txt`. After this slot the check is:

- **Automatic** — every push and PR touching `specs/` runs TLC in Actions.
- **Comprehensive** — every `.tla` in `specs/` is checked (not one file
  cherry-picked).
- **Portable** — same runner runs locally (`bash specs/run-tlc.sh`) and in
  CI. Same jar, same JVM flags, same invariants.

## Specs shipped in this slot

### 1. `ProofSystemSpec.tla` — proof-registry / decision-attestation state machine

Extended in this slot (was already present as an ad-hoc spec).

**Added:**
- `SlashedProversHaveEvidence` — every slashed prover has ≥2 co-existing
  proofs on the same model (i.e. the equivocating pair that triggered the
  slash). This replaces the previous `SlashedProversStop`, which was
  malformed (a universal quantifier that never held).
- `ChainStepsAreValid` — every chain step references a `verified` proof
  bound to that chain and position.
- `ProofIdUnique` / `ChainIdUnique` — id-space integrity.

**Model:** 2 provers, 2 models, 3 proofs, chain depth 2, challenge window 2.
**State count:** 12,642,985 states generated, 6,153,405 distinct.
**Runtime:** ~2m40s on the pod's TLC. In CI budgeted at 1500s ceiling.

### 2. `QuorumSpec.tla` — BLS12-381 threshold-quorum registry

New in this slot. Models the state machine of
`engine/internal/quorum/registry.go`:

- Signer lifecycle `active → slashed / removed` (both terminal).
- `Threshold(n) = ⌊2n/3⌋ + 1` for `n > 0`.
- `AcceptWitness(S)` action only fires when `S ⊆ active` and
  `|S| ≥ Threshold(|active|)` at accept time.

**Invariants checked:**
- `StateInvariant` — active/slashed/removed are pairwise disjoint.
- `WitnessBudget` — every accepted witness structurally meets the threshold
  it recorded at accept time (`|signers_used| ≥ threshold`,
  `threshold = Threshold(active_at)`).
- `MonotonicSlashing` — slashed never returns to active or removed.
- `RetireIsClean` — retired never returns.
- `AcceptedWitnessImpliesByzantineMajority` —
  `3·|signers_used| > 2·active_at` (the property downstream code relies
  on).

**Threshold arithmetic** is asserted as a top-level `ASSUME
ThresholdCorrect` since it does not depend on the variables — TLC's warning
about constant-level formulas prompted this reshape.

**Model:** 3 signers, at most 2 accepted witnesses.
**State count:** 3,063 states generated, 1,576 distinct.
**Runtime:** ~1s.

**Injection test.** During development we intentionally weakened the
`AcceptWitness` guard from `Cardinality(S) >= Threshold(...)` to
`Cardinality(S) >= 1` and re-ran TLC. It immediately produced a
counter-example trace (2 active signers, threshold 2, single signer
witness accepted) — confirming that `WitnessBudget` really does catch a
broken quorum gate. The buggy spec was deleted after the check.

### 3. `ReceiptLineageSpec.tla` — receipt-lineage DAG

New in this slot. Models the receipt store's parent-pointer graph
(`engine/internal/receipts/service.go: Ancestors`).

**Invariants checked:**
- `AllParentsExist` — every parent id refers to an already-emitted receipt.
- `NoSelfParent` — no receipt lists itself.
- `AcyclicByEmission` — every parent has strictly smaller emission ord
  than the child, so `Ancestors()` cannot loop.
- `AncestorsBounded` — the ancestor closure is bounded above by receipt
  count, so the walk always terminates.

**Model:** up to 4 receipts, up to 2 parents each.
**State count:** 68 states, no error.
**Runtime:** <1s.

## The runner

`specs/run-tlc.sh` is a portable bash driver:

- Auto-discovers every `*.tla` in `specs/` and pairs it with its `*.cfg`.
- Uses `TLA_TOOLS_JAR` (defaults to `/data/opt/tla2tools.jar` in the pod,
  falls back to downloading tla2tools v1.8.0 to `/tmp` if missing —
  keeping the script usable from anywhere).
- Uses `java` on `PATH` when present, otherwise falls back to
  `/data/opt/jdk-21.0.4+7-jre/bin/java` in the pod.
- Runs each spec under `timeout` (default 900s, override with
  `TLC_TIMEOUT`) and `-workers auto`.
- Cleans up TLC trace / state directories on success.
- Exits non-zero if any spec fails; prints the list of failed specs at
  the end.

Local: `bash specs/run-tlc.sh` (or `bash specs/run-tlc.sh QuorumSpec` to
run just one).

## The workflow

`.github/workflows/formal-verification.yml`:

- Triggers on `push` / `pull_request` on `specs/**` and on manual
  `workflow_dispatch`.
- Cancels superseded runs on the same ref (`concurrency` block).
- Caches `tla2tools.jar` across runs (`actions/cache`, key
  `tla2tools-v1.8.0`).
- Runs `bash specs/run-tlc.sh` with `TLC_TIMEOUT=1500` (25 min per spec
  ceiling; job timeout 30 min).
- On failure uploads counter-example traces
  (`specs/*_TTrace_*.tla`, `specs/states/`) as an artifact for 7 days so
  the developer can rerun the offending trace locally.

## Extending

Adding a new spec is `write specs/NewSpec.tla + specs/NewSpec.cfg` and
push. The runner and the workflow pick it up automatically — no changes
required to CI plumbing. Keep the model small enough to fit under 25
minutes per spec; bump `TLC_TIMEOUT` if the state space needs to grow.

Good candidates for the next specs, all backed by shipped engine code:

- Provenance-receipt canonical hash order-invariance (`ToCanonicalBytes`).
- Nova / Merkle aggregation fold — the `pedersen-fold-v1` and
  `merkle-recursion-v1` invariants.
- Webhook delivery state machine — enqueue / attempt / dead-letter /
  replay round-trip (idempotency, no-drop).

## Reproducing today's numbers

```
# Inside the pod
bash specs/run-tlc.sh
```

Expected output tail: `>>> All 3 specs passed`. Combined wall clock on the
pod is ~3 minutes (`ProofSystemSpec` dominates).
