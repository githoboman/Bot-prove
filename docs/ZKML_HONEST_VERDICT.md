# ZK-ML — Honest Verdict

**Status**: `DRAFT — decision record`. Companion to
`docs/ZKML_RESEARCH_SPIKE.md`. This file is the single-page answer that
maintainers, auditors, and reviewers should be able to read in 60
seconds to understand *why every ML-inference claim in CasperProver is
currently `SIMULATION`* and *what would have to change*.

## The verdict

Every claim in the CasperProver tree that implies a cryptographic proof
of a model's inference — as opposed to an attestation of inputs, outputs,
and a model identifier — is labelled `SIMULATION`. This label is not
provisional. It reflects the state of publicly reviewable ZK-ML
provers as of the current research spike and the honesty posture
established across AA–AL.

## Why it stays `SIMULATION`

Four conditions must all hold before any ML claim can be relabelled to
`REAL (ZK-ML, model-id X, circuit-id Y)`. None hold today.

1. **Named model, named circuit, published hashes.** No such artefact is
   in the tree. The current attestation surface names a model
   identifier, not a compiled circuit.
2. **Independent third-party audit sign-off** on both the circuit and
   the underlying IOP/lookup argument. Reserved for G2 in
   `docs/MAINNET_LAUNCH_PLAN.md`.
3. **Per-inference proving cost** low enough that a Challenger under
   `docs/REPUTATION_ECONOMICS.md` §5 can afford to reproduce it.
   Public benchmarks for the surveyed prover categories exceed the
   viable cost ceiling by orders of magnitude for the model sizes
   CasperProver targets.
4. **Receipt format extension** to carry circuit hash, verifying-key
   hash, weights hash, and toolchain version. This is a breaking
   schema change and must be scheduled as such.

Skipping any one of these turns `REAL` into laundered `SIMULATION`. That
outcome is explicitly rejected.

## What was surveyed

`docs/ZKML_RESEARCH_SPIKE.md` §2 catalogues five structural approaches
(Groth16/PLONK-family, STARK/FRI, zkVM, lookup+PLONK, recursion) and
one out-of-scope comparator (trusted-hardware attestation). The
feasibility matrix in §3 scores each approach against ten axes that
matter to CasperProver's specific posture (transparent setup, PQ hedge,
existing provenance-vector primitives, upcoming audit gate). No
approach reaches a clear win.

- The **least-bad** rung-3 candidate is **STARK/FRI-family** (aggregate
  +3), because it matches the AD/AJ transparent-setup and PQ posture.
- Every candidate that ergonomically matches **rung 4** (universal ML
  verifier, no per-model recompilation) is disqualified by per-inference
  cost, not by ergonomics.

"Least-bad candidate for a research prototype" is not "ready to ship".
No prototype branch is authorised by this verdict.

## What is not decided

- No prover family is selected.
- No vendor, audit firm, or toolchain is named.
- No schedule is committed.
- No code is shipped.
- No paid dependency is introduced.
- No mainnet activation is implied.

These are all deliberately out of scope. They live in G2 of the mainnet
launch plan, and G2 exists precisely so that the decision can be made
under third-party review and not as a marketing calendar item.

## What this document does

It provides a durable, in-tree, auditable answer to the question
"*why does CasperProver label ML inference claims as `SIMULATION`?*" so
that:

- Reviewers of AA–AL can trust that no downstream pack has quietly
  upgraded the label.
- Anyone reading `docs/KNOWN_LIMITATIONS.md` sees a bounded reason,
  not an apology.
- Any future proposal to relabel a claim must cite this file and
  demonstrate all four conditions above are met.

That is the entire deliverable.

## Cross-refs

- `docs/ZKML_RESEARCH_SPIKE.md` — full landscape and feasibility matrix.
- `docs/KNOWN_LIMITATIONS.md` — honesty ladder that binds this verdict.
- `docs/MAINNET_LAUNCH_PLAN.md` §3 G2 — audit gate that any relabel
  proposal must pass.
- `docs/REPUTATION_ECONOMICS.md` §5 — Challenger cost model that any
  prover has to fit under.
- `docs/HSM_PLAN.md` — key custody surface any real prover would
  eventually touch.

---

*This is a decision record, not a plan. It ships no code and commits to
no schedule. Its only purpose is to make the current `SIMULATION` label
auditable and non-negotiable until the four conditions above are met.*
