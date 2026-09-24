# Gate 3 — Agentic Vertical Slice CLI

**One command reproduces every required verdict path.**

```
make gate3-demo
```

or, without make:

```
cd engine
go run ./cmd/casperprover gate3 > receipt.json 2> summary.txt
```

The command runs four scripted scenarios end-to-end through the exact same
FacetJudge / equivocation / HITL code that serves `POST /inference/judge` in
production. **No network I/O, no LLM API keys, no external state.** Every
provider is an in-process deterministic fixture, so:

- the same command produces byte-identical evidence hashes across machines,
- CI can assert the receipt digest without provider secrets,
- a hackathon judge can diff the receipt on their own laptop.

## What it proves

| Scenario | What happens | Expected verdict | Evidence emitted |
|---|---|---|---|
| **approve** | Three providers agree the benign input is safe | `AGREE` | verdict + facet agreement fraction |
| **malicious** | Prompt-injection attack. Providers split 1‑1‑1 on a safety-critical facet | `DISAGREE` | equivocation proof + SHA-256 of canonical bytes |
| **conflict** | Non-adversarial deadlock: providers split 2‑2 below the agreement threshold | `DISAGREE` | equivocation proof + SHA-256 of canonical bytes |
| **abstain** | All providers error out — no live votes | `ABSTAIN` | HITL escalation event + canonical digest |

The four paths are the DoD of Gate 3 in the deadline plan (`CP_FINAL_TASKS_V2_new.md`):

> **DoD:** one command reproduces approve + abstain + malicious/conflict paths;
> UI shows the steps, hashes, and real/sim badges.

## What a passing run looks like

```
=== CasperProver Gate 3 — Agentic Vertical Slice ===

[PASS] approve    overall=AGREE     OverallVerdict == AGREE as expected.
         facet=safety          verdict=AGREE    live=3 agreement=1.00

[PASS] malicious  overall=DISAGREE  OverallVerdict == DISAGREE — attack surfaced, not swallowed.
         facet=safety.slurs    verdict=DISAGREE live=3 agreement=0.33
         equivocation_proof_sha256=<64 hex>

[PASS] conflict   overall=DISAGREE  DISAGREE + equivocation proof digest emitted.
         facet=fraud.flag      verdict=DISAGREE live=4 agreement=0.50
         equivocation_proof_sha256=<64 hex>

[PASS] abstain    overall=ABSTAIN   ABSTAIN + HITL escalation event emitted with canonical digest.
         facet=kyc.ok          verdict=ABSTAIN  live=0 agreement=0.00
         hitl_canonical_digest=<64 hex>

Receipt digest (sha256 over scenarios): <64 hex>
Overall: PASS — all four paths produced their expected verdicts.
```

Followed by a JSON receipt on stdout containing every scenario's full facet
breakdown, per-provider votes, and the equivocation / HITL evidence blob.

Non-zero exit code if any scenario diverges from its expected outcome.

## Reproducibility

Every wall-clock timestamp is zeroed after the judge runs and before evidence
is built, so the `receipt_digest` at the bottom of the JSON is stable across
runs on the same commit. This is enforced by
[`gate3_test.go`](../engine/cmd/casperprover/gate3_test.go) — `go test ./cmd/casperprover -run TestGate3`.

If two runs on the same commit produce different digests, the demo is broken;
open an issue with both receipts attached.

## How to verify one scenario by hand

The **conflict** scenario produces an equivocation proof whose canonical bytes
a slashing contract could recompute. To verify the emitted proof:

```bash
# Extract the conflict proof
jq '.scenarios[] | select(.scenario=="conflict").equivocation_proof' receipt.json \
  > conflict_proof.json

# The proof carries digest_hex — re-derive it from the canonical bytes
# with digest_hex zeroed (spec: engine/internal/judge/equivocation/proof.go)
```

The unit tests in `engine/internal/judge/equivocation/proof_test.go` cover the
verify path; a downstream contract or SDK would re-implement `MarshalCanonical`
in whatever language it needs and compare digests.

## Related

- Judge engine implementation → `engine/internal/judge/`
- LLM runner + fixture provider → `engine/internal/llm/`
- Equivocation proof + verify → `engine/internal/judge/equivocation/`
- HITL escalation events + sinks → `engine/internal/judge/hitl/`
- Live API endpoint using the same code path → `POST /inference/judge`
- Judge baseline (broader Gate 5 verification) → [`JUDGE_GUIDE.md`](./JUDGE_GUIDE.md)
