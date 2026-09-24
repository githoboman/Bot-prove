# ZK-ML Research Spike — Landscape & Feasibility

**Status**: `DRAFT — research spike`. Non-binding survey. **No code is shipped as
part of this document. No third-party service is authorised. No architectural
commitment is made.** This file exists solely to justify — with an auditable
paper trail — why CasperProver's ML-inference claims are labelled
`SIMULATION` today and what would have to change (mathematically, operationally,
economically) before any of them could be re-labelled `REAL`.

Cross-refs:
- `docs/KNOWN_LIMITATIONS.md` — the honesty ladder (`REAL` / `ON-CHAIN` /
  `SIMULATION`) that this spike is bound to.
- `docs/HSM_PLAN.md` — key custody surface any real prover would eventually
  touch.
- `docs/MAINNET_LAUNCH_PLAN.md` — gates G2 (independent security audit) and
  G8 (launch review). No ZK-ML claim can move past `SIMULATION` without an
  explicit sign-off in G2 that names the exact circuit and its threat model.
- `docs/REPUTATION_ECONOMICS.md` — attestation cost enters payoff matrix; any
  real prover has to be cheap enough that a Challenger can afford to reproduce
  it.

---

## 1. Framing — what "ZK-ML" actually means in CasperProver's context

CasperProver anchors *attestations of agent decisions*: inputs, outputs, model
identifier, timestamp, hashed into a Merkle tree with a chain-anchor receipt.
"ZK-ML" is the (much stronger) claim that the receipt *also* carries a
succinct cryptographic proof that a **named model, on the named inputs,
actually produced those outputs**, verifiable without disclosing the inputs,
the weights, or both.

There are four honesty rungs. The spike matters because only the top rung
justifies re-labelling from `SIMULATION`.

1. **Attestation only** (today, labelled `REAL`) — the operator signs
   `H(inputs)`, `H(outputs)`, `model_id`, `timestamp`; nothing binds `model_id`
   to the actual computation. Cheap. Enough for many logging/audit
   use-cases. Not ZK-ML.
2. **Trusted-hardware attestation** (TEE / TDX / SEV-SNP) — the computation
   runs inside an enclave whose remote attestation is included in the
   receipt. Hardware-trust, not math-trust. Not ZK-ML in the cryptographic
   sense; downgrades honestly to `REAL (hardware-attested)` at best. Out of
   scope for the spike but noted so it isn't confused with ZK.
3. **Proof of correct execution of a fixed circuit that *represents* a
   specific model** — a SNARK/STARK over the arithmetic circuit of one
   specific inference. This is what "on-chain Groth16 for ML" usually
   means in the literature (`ezkl`, `Modulus`, `EZKL`, DeepProve). Labels
   would still be `REAL (fixed circuit, model-id X, weights-hash Y)`.
4. **Proof of correct execution of an *arbitrary* named model** (universal
   ML verifier) — an open research problem. No production system today
   satisfies this at model sizes CasperProver cares about (>10^7 parameters)
   in a way that is cheap enough for per-inference attestation. If it
   existed, it would be the honest `REAL` rung the marketing wants.

Rungs 3 and 4 are the ones this spike surveys. Rung 3 is potentially in
reach; rung 4 is not.

---

## 2. Landscape survey (informational, non-endorsement)

The following categories are catalogued to justify feasibility scoring in
§3. **No vendor selection, no procurement, no integration commitment.**
Each category is described by structural properties, not brand names.

### 2.1 Circuit-fixed SNARK provers (Groth16 / PLONK family)

**How they work.** The model is compiled to a fixed arithmetic circuit
(`R1CS` or `PLONKish` constraints). A trusted setup (Groth16: per-circuit;
PLONK/Marlin: universal) produces a proving key and a verifying key.
Every inference is a witness for that circuit; the prover emits a
constant-size proof (Groth16: ~200 bytes) verifiable in a few milliseconds
on-chain.

**What they buy.** Succinct proofs. Cheap on-chain verification. Well-studied
soundness proofs.

**What they cost.**
- Trusted setup per model (Groth16) or per-parameter-size (PLONK). Changing
  the model *at all* means a new circuit, a new setup, a new deploy.
- Prover memory scales super-linearly with circuit size. Circuits for
  moderate CNN/transformer inference have ranged from GB to hundreds of
  GB of RAM in reported benchmarks.
- Non-arithmetic operations (softmax, GELU, quantisation edge cases) are
  approximated with lookup tables and range proofs — every such
  approximation is a place where the "proof of the model" quietly becomes
  "proof of a *simplification* of the model", and that gap must be
  disclosed to remain honest.

**Fit for CasperProver.** Compatible with rung 3 for a **single, small,
well-defined model**. Structurally incompatible with rung 4 (universal
verifier).

### 2.2 STARK / FRI-based provers

**How they work.** Hash-based (no trusted setup), post-quantum plausible.
Prover cost polynomial in circuit size; verifier cost logarithmic; proofs
are larger (tens of KB up to MB).

**What they buy.** No trusted setup — matches the honesty posture better
than Groth16, and aligns with the SLH-DSA / post-quantum stance already
adopted in AD.

**What they cost.**
- Larger proofs → more chain storage or off-chain-with-hash-anchor patterns.
- Prover memory still large; still non-arithmetic op problem.
- Verifier is fast in absolute terms but expensive in on-chain gas relative
  to Groth16.

**Fit for CasperProver.** Structurally the *most honest* rung 3 option
because it composes with the AD/AJ posture (no trusted setup, PQ hedged).
Still not rung 4.

### 2.3 zkVM approaches (RISC-V zkVMs, WASM zkVMs)

**How they work.** Instead of compiling the model to a bespoke circuit, the
model is compiled to a general-purpose zkVM bytecode; the zkVM produces a
proof that the bytecode ran correctly on the inputs.

**What they buy.** Model-agnostic in principle; write once, prove any
inference. Ergonomically closest to rung 4.

**What they cost.**
- Per-cycle proving overhead is currently 10^4–10^6× native runtime in
  published benchmarks. A single inference that takes 200ms native can
  become minutes to hours in the zkVM.
- Memory footprint similarly large.
- Auditability of the zkVM itself is a research question; a bug in the VM
  is a soundness bug in every proof it emits. Two published zkVMs have
  had disclosed soundness bugs in the past 24 months (public record via
  their security advisories; details in §5).

**Fit for CasperProver.** Aspirational rung 4 candidate; not viable for
per-inference attestation at CasperProver's target throughputs today.
Revisit in G2 (mainnet audit gate) for models below a hard size ceiling.

### 2.4 Lookup-argument-heavy DSLs (halo2-lookup, Plonkup, cq)

**How they work.** Non-arithmetic operations (softmax, activation, integer
quantisation) are handled with lookup tables and range proofs — the same
primitive family CasperProver already ships as `SIMULATION` stubs in AC
(`docs`: 2.13, 2.14). Combined with a base PLONKish arithmetisation to
close the ML inference circuit.

**What they buy.** Handle the "non-arithmetic ops" problem more directly
than 2.1/2.2. Composable with universal-setup PLONK.

**What they cost.**
- Still per-model compilation. Still trusted-setup (for the PLONK base) or
  transparent-setup (with STARK base) depending on choice.
- Reference implementations are researcher-grade; production hardening
  would be a multi-month program with dedicated cryptographer review.
- Interaction between the lookup argument's soundness and the underlying
  IOP's soundness is subtle — a common source of published attacks in
  the last three years.

**Fit for CasperProver.** Rung 3 candidate with better honesty story for
non-arithmetic ops. Structural cost is a permanent per-model engineering
tax.

### 2.5 Recursion / aggregation

Independent of 2.1–2.4, recursive proof composition (Nova, HyperNova,
SuperNova, folding schemes; SNARKs-of-SNARKs) reduce per-inference cost
by batching. Composes with any of the above at the price of more moving
parts and larger dependency surface.

**Fit for CasperProver.** Not a rung; a cost-reduction technique layered
on top of a chosen rung. Reject until a rung-3 prover is actually shipping
and its unbatched economics are measured.

### 2.6 Trusted-hardware attestation (out of scope, noted for completeness)

TEE-based attestation (Intel TDX, SGX-DCAP, AMD SEV-SNP, Nitro Enclaves,
Confidential VMs). Structurally different from ZK-ML: the proof is a
signed statement from silicon, not a cryptographic argument about a
computation. Historical CVE record for these platforms is non-trivial
(side-channels, downgrade attacks, firmware-signed rollback). Downgrades
to `REAL (hardware-attested)` at best; **not** rung 3 or rung 4.

Called out here so a reader does not confuse a TEE deployment with ZK-ML.

### 2.7 Deprecated / discredited directions

- Weight-hiding proofs where the weights are the *witness* only (i.e. no
  binding to a public commitment). These prove that *some* model produced
  the output, not that *the named model* did. Marketing-grade, not
  cryptographic-grade. Explicitly rejected.
- Any scheme without an independent implementation or an independent
  soundness proof review. Explicitly rejected until both exist.

---

## 3. Feasibility matrix for CasperProver

Score is qualitative on the axes CasperProver actually cares about, not
abstract cryptographic aesthetics. Cells are `-2 blocker / -1 pain / 0
tolerable / +1 good / +2 excellent`. Aggregate is the sum, not a
weighted composite (weighting is intentionally not encoded so the reader
does not confuse the spike for a decision).

| Axis                                    | 2.1 Groth16-family | 2.2 STARK/FRI | 2.3 zkVM | 2.4 Lookup+PLONK | 2.5 Recursion |
|-----------------------------------------|:------------------:|:-------------:|:--------:|:----------------:|:-------------:|
| Trusted-setup posture (vs AD honesty)   | −1                 | +2            | +1       | 0                | 0             |
| Post-quantum hedged (vs AD SLH-DSA)     | −1                 | +2            | +1       | −1               | 0             |
| Prover cost per inference (RAM/CPU)     | −1                 | −1            | −2       | −1               | +1            |
| Verifier cost on-chain                  | +2                 | 0             | −1       | +1               | 0             |
| Handles non-arithmetic ops honestly     | −1                 | 0             | +1       | +1               | 0             |
| Per-model engineering tax               | −1                 | −1            | +2       | −1               | 0             |
| Model-agility (bench, swap, iterate)    | −2                 | −1            | +1       | −1               | 0             |
| Independent implementation availability | +1                 | +1            | 0        | 0                | 0             |
| Independent soundness review depth      | +2                 | +1            | −1       | 0                | −1            |
| Ecosystem maturity for auditors (G2)    | +1                 | 0             | −1       | 0                | −1            |
| **Aggregate**                           | **−1**             | **+3**        | **+1**   | **−2**           | **−1**        |

**Reading of the table.** Nothing in the surveyed landscape is a
clear +6 to +10 win. The category with the least-bad aggregate for
CasperProver's specific posture (transparent setup, PQ hedge, existing
provenance-vector primitives, upcoming G2 audit) is **STARK/FRI-family
rung-3 provers**, at aggregate +3. That is not "ready to ship"; that is
"least-bad candidate to prototype in a *research* branch behind a hard
`SIMULATION` label until G2 concludes".

**No candidate reaches rung 4.** The zkVM row exists in the table but
its aggregate is +1 despite matching rung 4 ergonomically, because the
per-inference cost and audit-surface penalties are structural, not
transient.

---

## 4. Honest verdict — why the label stays `SIMULATION` today

For a CasperProver claim to move from `SIMULATION` to `REAL (ZK-ML)`
the following four conditions all have to hold. None of them hold today.

1. **A single, specific, named model** is compiled to a specific circuit
   (or zkVM entry point), with a **weights hash** and a **circuit hash**
   both published and both anchored. Nothing in the current tree names
   such a model.

2. **An independent third-party audit** (G2 in MAINNET_LAUNCH_PLAN) has
   reviewed both the circuit and the underlying IOP/lookup argument and
   signed off on soundness *for the specific compilation*. A generic
   "we use Groth16" reference is not audit evidence.

3. **Per-inference proving cost** fits within a payoff model that a
   Challenger (`docs/REPUTATION_ECONOMICS.md` §5) can afford to
   reproduce — otherwise the reputation layer's dispute channel silently
   degrades to trust-in-the-operator, which is the failure mode the whole
   design is meant to avoid.

4. **The receipt format** carries the circuit hash, the verifying-key
   hash, the model-weights hash, and the compilation toolchain version;
   omitting any of these turns "REAL" into laundered "SIMULATION".

Until all four hold, the honest label is `SIMULATION`. This is not
timid; it is the whole reason the honesty ladder exists.

**Consequence for the current tree.** Every existing reference to
"zero-knowledge proof of model inference" outside this document must
carry a `SIMULATION` badge. The badges already shipped in AA–AL cover
the tree that exists today; this spike does not weaken them, and any
new component that would need one is a work item, not a claim.

---

## 5. Prior-art references (informational, non-endorsement)

Consulted structurally — no vendor is recommended and no dependency is
proposed. Each reference is included because it materially informed §2
or §3, not as a courtesy.

Category 2.1 (Groth16 / PLONK ML compilers): published circuits for
small CNNs (MNIST-scale) and quantised transformers, with reported
prover memory in the 10s of GB and prover time in the minutes-per-
inference range. Non-arithmetic ops universally handled by lookups or
range proofs; several published soundness fixes across 2022–2025.

Category 2.2 (STARK ML): Cairo-style provers and PlonKy2/PlonKy3-style
successors adapted to ML circuits. Transparent setup and PQ posture
match the AD/AJ line. Proof sizes are the main tax.

Category 2.3 (zkVM): RISC-V zkVMs (multiple independent implementations)
and WASM zkVMs. Two disclosed soundness bugs across 2024–2025 in the
zkVM category — cited in the aggregate cost of "independent soundness
review depth" scoring −1 in §3.

Category 2.4 (lookup arguments): Plonkup, cq, and successor systems.
Composable with either a PLONK base (universal setup) or a STARK base
(transparent). Sensitivity to soundness bugs in the interaction between
the lookup argument and the IOP is the main research risk.

Category 2.5 (recursion / folding): Nova and HyperNova family. Layered
on top of a rung-3 candidate, not an alternative.

No brand-name procurement is implied. Any transition from spike to
prototype would restart at G2 with named vendors, named audit firms,
and a named model — none of which this document proposes.

---

## 6. What this spike explicitly does not do

- It does not choose a prover family.
- It does not authorise a prototype branch that ships code.
- It does not commit any credential, dependency, or paid service.
- It does not weaken the `SIMULATION` label on any existing component.
- It does not commit to a schedule.
- It does not name a vendor.
- It does not promise that any candidate is production-ready.
- It does not treat "on-chain Groth16 for ML" as a solved problem.
- It does not treat trusted-hardware attestation as ZK-ML.

The single deliverable of this spike is a paper trail justifying **why
CasperProver's ML claims remain `SIMULATION` today** and **what would
have to be true before that label could honestly change**. Anything
beyond that scope is future work gated by G2.

---

## 7. Open questions (routed to `docs/KNOWN_LIMITATIONS.md`)

1. Is the target attestation surface a *single fixed model* per operator
   (rung 3 realistic) or an *arbitrary named model* (rung 4, not
   realistic today)?
2. What is the acceptable per-attestation cost ceiling before the
   reputation layer's Challenger dispute becomes unaffordable?
3. Does the receipt-format extension for ZK-ML claims (circuit hash,
   VK hash, weights hash, toolchain version) require a version bump on
   the attestation schema? If yes, that is a breaking change and must
   be scheduled as such.
4. Which G2 audit firms have surveyable prior work in the specific
   circuit family before we would even ask them to look at a compiled
   circuit? (Deliberately unresolved — vendor selection is out of scope.)
5. Does the SLH-DSA / PQ posture from AD tighten the acceptable candidate
   set to STARK/FRI or transparent-setup PLONK only? Preliminary answer:
   probably yes; formal ruling deferred to G2.

---

*This document is a research spike. It ships no code, purchases no
service, commits to no vendor, and does not move any label from
`SIMULATION` to `REAL`. Its purpose is to make the honesty of that
label auditable.*
