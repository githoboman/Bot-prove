# Adversarial input fixtures

Runnable adversarial batteries for the CasperProver proof pipeline.

## Files

- `tests/fixtures/prompt_injection_battery.json` — 8 input cases (control chars, unicode homoglyphs, jailbreak markers, RTL override, malformed UTF-8, oversized payloads, format-confusion). Declared as data so the same cases can be reused by frontend / SDK e2e tests.
- `tests/fixtures/equivocation_battery.json` — 6 double-signing / same-signer-flip scenarios, plus the on-chain enforcement flow via `stake-slashing::slash_equivocation`.

## Runnable tests

- `engine/internal/hasher/adversarial_test.go` — Go tests that enforce the invariants declared in the JSON fixtures at the hasher/commit layer. **11 passing**.

Run:

```bash
cd engine && go test ./internal/hasher/... -run "PromptInjection|Equivocation" -v
```

## Invariants enforced

**Prompt-injection battery** — every input is treated as opaque bytes:

- Control chars (NULL, RTL override) preserved in Merkle preimage
- Jailbreak markers (`IGNORE ALL PREVIOUS INSTRUCTIONS`, `<|im_start|>system`) are opaque, never interpreted
- Unicode homoglyphs (Cyrillic vs Latin lookalikes) commit to different hashes — verifier does not canonicalize
- SQL-injection-shaped payloads travel as literal strings — no SQL is ever built from user input in the proof path
- Malformed UTF-8 hashes deterministically — the pipeline does not depend on encoding validity
- Oversized inputs (>10 MiB) rejected by the API layer with HTTP 413 (see `server_test.go`)

**Equivocation battery** — the ZK / multi-agent analog of double-spend:

- Same `(agent, model_id, input)` with different `output` = equivocation. Detectable, slashable.
- **Byte-level equality is the criterion.** Whitespace-only diff, case diff, both count. Canonicalization is the agent's responsibility, never the verifier's.
- Different agents disagreeing = legitimate multi-agent behavior, NOT equivocation.
- Different `model_id` = allowed to differ, NOT equivocation.
- Identical re-submission = idempotent, NOT equivocation.

## On-chain enforcement

Equivocation is enforced by the `stake-slashing` contract:

- Entry: `slash_equivocation(proof_a_id, proof_b_id)`
- Anyone can call — no privileged observer required
- Contract validates both proofs are anchored, both belong to the same agent, both share `(model, input)`, both differ in `output`
- Deducts the offending agent's stake, emits `SlashedEvent`, marks the tuple in `SLASHED_DICT` — no re-slash of the same equivocation possible
- See `contracts/stake-slashing/src/lib.rs` for the reference implementation, `docs/RED_TEAM.md` #1 for the double-slash defense.

## Extending the battery

Add a case:

1. Append an entry to `tests/fixtures/*.json` with a stable `id` (e.g. `PIB-09`).
2. Add a matching Go test in `engine/internal/hasher/adversarial_test.go` that enforces the invariant.
3. If it's an API-surface invariant (rate limit, HTTP status, payload size), add it to `engine/internal/api/server_test.go` instead.
4. Cross-link from `docs/RED_TEAM.md` if it's a new attack vector.
