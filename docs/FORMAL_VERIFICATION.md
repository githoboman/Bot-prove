# Formal Verification

Scope statement. **What has actually been done** in this repository and
**what a full formal verification effort would require**. Everything in the
second half is out of scope for the hackathon submission and is documented
here so a reviewer never has to guess whether "formal verification" is a
shipped feature or a roadmap item.

## TL;DR

- **Shipped:** none. No proof, no model-checker output, no machine-checked
  invariant lives in this repo.
- **Not shipped:** a TLA+ / Apalache / Coq / Isabelle artefact of any kind.
- **What holds instead:** informal invariants documented in
  `docs/CONTRACT_INVARIANTS.md` (I-*, X-*, F-*), Odra unit tests, and a
  `verify.sh` smoke pipeline. That is the current level of assurance —
  nothing more, nothing less.

If a reviewer sees the phrase "formal verification" anywhere else in the
repo (README, marketing site, pitch), and the phrasing implies more than
"we wrote down the invariants and unit-tested them", that phrasing is a
bug — please file it as an INVARIANT BREAK issue.

## What "shipped" would mean

To claim a small-model TLC / Apalache pass we would need, in this repo:

1. `models/` directory containing a `CasperProverCore.tla` module (or
   Quint / Apalache equivalent).
2. A `MC.tla` model config bounding state (typically at most 3 accounts,
   4 stake amounts, 2 slash amounts).
3. A CI job (`.github/workflows/tla.yml`) that runs `tlc` (or `apalache-mc
   check`) against `MC.tla` and blocks merge on failure.
4. A short write-up in this file mapping each modelled invariant to its
   source-code enforcement point (`contracts/*/src/main.rs`, line
   numbers).

None of the four exist today. This section is the checklist for the
change that would flip "not shipped" to "shipped: small-model TLC pass".

## What would ever be in scope

The invariants worth modelling first (in this order):

- **I-1 · Owner isolation.** Small state space (one owner slot, three
  candidate callers). Model checks that no reachable state has a non-owner
  successfully mutating configuration.
- **I-3 · Monotonic reputation.** Reputation floor never decreases across
  `revoke_proof` / `record_stake` / `report_and_slash` sequences.
- **I-4 · Checked arithmetic.** Small U64 domain, all pairs of
  `checked_add` / `checked_sub` cover boundary cases. Doable in Kani or a
  Rust proof harness rather than TLA+.
- **X-3 · Cross-contract writeback on slash.** Requires modelling two
  contracts as separate processes with a shared oracle. Non-trivial.

## What would never be in scope (for this project)

- End-to-end proof of the Groth16 verifier. The `arkworks` / `gnark`
  crates it depends on are large enough that "proving them correct"
  would be a multi-year research project. We rely on the audits and
  test suites those crates ship.
- Proof of the Casper Condor 1.5 host functions. Not our surface.
- Proof of the frontend or the SDK code paths. Not the kind of code
  that benefits from formal methods.

## Related

- `docs/CONTRACT_INVARIANTS.md` — the informal statement of the
  invariants that would eventually be modelled.
- `docs/JUDGE_GUIDE.md` — the assurance level actually shipped for
  the hackathon submission.
- `contracts/stake-slashing/src/main.rs` — the hardened redeploy
  that hand-audited checked-arithmetic invariants (I-4). Precedent
  for the kind of hand review a formal proof would replace.
