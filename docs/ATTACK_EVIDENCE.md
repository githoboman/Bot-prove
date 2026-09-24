# Attack Evidence Lab

**Path**: `/lab/attack-evidence` (new tab in the CasperProver Lab).
**Component**: `frontend/src/components/lab/AttackEvidence.tsx`.
**Backend contract test**: `engine/internal/verifier/attack_evidence_test.go`.

## What it demonstrates

Five real-world tampering attempts, each executed against the **live** CasperProver verifier (`POST /verify`) — nothing is mocked. Every scenario:

1. Mints a fresh proof via `POST /proofs` with a known `(input, output, model)` tuple.
2. Calls `POST /verify` with the *honest* tuple → **baseline must succeed**.
3. Calls `POST /verify` with a mutated tuple → **attack must be rejected**, with a specific detection field.

If the engine ever accepted a mutated tuple, the UI would show a red `Not detected` badge where a green `Detected` should be — the panel doubles as a live regression signal.

## Scenarios and detection fields

| Scenario | Attacker mutation | Detection field | Backend error substring |
|---|---|---|---|
| **Frame injection** (input tampering) | flips one byte in the input | `ih` (input hash) | `input hash mismatch` |
| **Verdict swap** (output tampering) | rewrites the reported verdict | `oh` (output hash) | `output hash mismatch` |
| **Model substitution** | claims audited model but ran shadow weights | `mh` (model hash) | `model hash mismatch` |
| **Proof swap across sessions** | mutates all three fields to describe a different session | any of `ih`/`oh`/`mh` — first mismatch wins | `hash mismatch` |
| **Replay after revocation** | replays the honest tuple against a revoked proof | `revoked=true` flag | `revoked` |

Every substring in the right-hand column is asserted by `TestAttackEvidenceScenarios` in `engine/internal/verifier/attack_evidence_test.go`. If backend error wording drifts, that test fails loudly before the UI can silently mis-classify a rejection as "not detected".

## Why these five?

They span the four cryptographic bindings CasperProver anchors on-chain plus the on-chain revocation flag:

- `IH = sha256(input)` — protects **against input tampering**.
- `OH = sha256(output)` — protects **against output/verdict tampering**.
- `MH = sha256(model)` — protects **against model substitution**.
- `PH = sha256(input || output || model)` — the combined commitment; protects **against proof swap** even when individual field hashes coincidentally match.
- `revoked` flag anchored via the `stake_slashing` contract — protects **against replay after compromise**.

## How the UI reports detection

For each scenario the panel shows:

- **Baseline card**: honest tuple → `Verified` (green) or `Unexpected baseline result` (yellow — treat as broken test).
- **Attack replay card**: attacker tuple → `Rejected — <error>` (green) or `Not detected — regression` (red).
- **Raw payload**: full JSON of both `/verify` responses, copyable to clipboard.
- **Aggregate counter**: `Detected N / M attacks` in the header.

Everything is per-run and idempotent — clicking `Attempt attack` again mints a new proof and re-runs the pair.

## HTTP contract this lab depends on

```http
POST /proofs
Content-Type: application/json

{
  "agent": "attack-evidence-<scenario>",
  "input": "<honest input>",
  "output": "<honest output>",
  "model": "<honest model>",
  "use_case": "attack-evidence"
}
```

Returns a `Proof` object with `id`, `ih`, `oh`, `mh`, `ph`.

```http
POST /verify
Content-Type: application/json

{ "proof_id": "P-42", "input": "...", "output": "...", "model": "..." }
```

Returns:

```json
{
  "proof_id": "P-42",
  "valid": true,
  "revoked": false,
  "verified": false,
  "error": "input hash mismatch: got <sha> want <sha>",
  "checks": {
    "input_hash_match": false,
    "output_hash_match": true,
    "model_hash_match": true,
    "commit_valid": false,
    "merkle_valid": true
  }
}
```

The UI classifies an attack as **detected** when any of:

- `error` is non-empty (typical case), or
- `revoked === true`, or
- `verified === false`.

## Adding a new scenario

1. Add a new entry to `SCENARIOS` in `AttackEvidence.tsx` (title, storyline, `mutate()`, expected detection field).
2. Add a matching case to `TestAttackEvidenceScenarios` — same `wantErrContains` string.
3. Update the table above.

The pattern deliberately couples the UI copy to a backend assertion so a future engine refactor cannot break the demo silently.
