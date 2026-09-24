# Interactive Verification — Bisection Dispute Game (AZ / 2.19)

> **Status: `[SPEC / DEFERRED / POST-AUDIT]`**
> This document defines the *design* for interactive fraud proofs over
> CasperProver decision traces. **No runtime code is shipped for this
> component in the hackathon build.** The seam is the audited proof-trace
> format already emitted by `receipts/` and `aggregator/`.
>
> **Honesty ladder:**
> - REAL — nothing runtime yet.
> - ON-CHAIN — no on-chain touchpoint added.
> - SIMULATION — no simulation shipped.
> - SPEC — this document + threat model + protocol sketch + honest-verifier assumption.

---

## 1 · Motivation

The default CasperProver anchoring is **non-interactive** — the prover signs
`(inputs, outputs, model_id, timestamp)`, hashes into a Merkle tree, anchors
the root on Casper. Verification is a single-shot Merkle-inclusion check.

That model is efficient but **fragile against a lying prover**: if the prover
publishes a root that is inconsistent with its declared inputs (e.g. a
different model was actually run), *no verifier can detect this*
non-interactively without re-executing the full trace themselves.

Interactive verification (a.k.a. *refutation game*, *bisection*, *fraud
proof*) is the escalation path: any observer can *challenge* the anchored
root and force the prover into a bounded interactive game that terminates in
one of two outcomes:

1. **Challenger wins** — the prover cannot produce a consistent step-level
   witness for some sub-interval; the anchored root is proven fraudulent;
   the prover is slashed (see `SLASH_EQUIVOCATION_SPEC.md`).
2. **Prover wins** — the challenger cannot exhibit a divergence within the
   depth budget; the challenger's bond is forfeit.

The construction is essentially the **Optimistic Rollup dispute game**
adapted from Arbitrum/Truebit, specialised to CasperProver decision
traces. The novelty for our stack is that the *underlying "computation"*
being bisected is not a VM but an **agent decision trace** — an ordered
sequence of `(inputs, model_id, prompt_hash, output_hash)` steps.

---

## 2 · Non-goals

- **Not a general-purpose VM fraud proof.** Bisection is over decision
  *steps* (agent invocations), not machine instructions.
- **Not a live-consensus mechanism.** The game runs *after* anchoring; live
  consensus over which decisions to sign is the multi-verifier gossip layer
  (`MULTI_VERIFIER_GOSSIP.md`).
- **Not zk-of-fraud.** Fraud proofs are transparent — everyone can replay
  the disputed step. Privacy-preserving dispute is out of scope.
- **Not a payment channel.** No off-chain balance is being tracked; only the
  authenticity of an anchored decision root.

---

## 3 · Assumptions

- **Honest-verifier assumption.** The game is safe as long as *at least
  one* honest observer is willing to challenge. This is standard for
  optimistic rollups.
- **Bounded trace length.** Each decision root corresponds to a trace of
  ≤ `2^d` steps where `d ≤ 20`. Practically: a decision trace is a batch
  of ≤ 1M agent invocations. Larger batches are split into sub-roots.
- **Deterministic re-execution.** For the *disputed step only*, both parties
  can re-execute the model given the same inputs and produce a canonical
  digest. This is honest re-labeling (see § 6.5).
- **Bonded participation.** Both prover and challenger post a bond in the
  slashing contract before the game starts. Loser forfeits.
- **Time-bounded rounds.** Each round has a wall-clock deadline (e.g.
  6 hours). A non-response is a loss.

---

## 4 · Data Model

### 4.1 · Trace commitment

Every anchored decision root `R` is the Merkle root of a sequence of
*state commitments* `s₀, s₁, …, s_n` where `n ≤ 2^d`:

```
s_i := H(state_i)                     -- 32-byte digest
state_i := (
    step_index,       -- u32
    ts_ns,            -- i64 nanos
    inputs_hash,      -- H(canonical inputs) — 32B
    model_id,         -- utf8, ≤ 64B
    prompt_hash,      -- H(prompt template) — 32B
    output_hash,      -- H(canonical output) — 32B
    prev_state,       -- s_{i-1} — 32B (s_0 uses zero)
)
R := merkle_root([s_0, …, s_n])
```

The **inclusion proof** for `s_i` is a standard Merkle path of length ≤ `d`.

### 4.2 · Log commitment (Merkle-of-Merkle)

For long traces, an intermediate *log commitment* is added: every 2^k steps
are aggregated into a **sub-root** first, and the anchored root is a Merkle
of these sub-roots. This turns bisection into a two-level game:

```
level 0 (state):  s_0 … s_n
level 1 (subroot): T_j := merkle_root(s_{j·2^k} … s_{(j+1)·2^k - 1})
level 2 (root):    R  := merkle_root([T_0, T_1, …])
```

The dispute game descends level-by-level: first bisect over `T_j`, then
inside the winning `T_j` bisect over `s_i`. The construction is
straightforward and matches Arbitrum's *fragment / segment* design.

---

## 5 · Roles

| Role         | Bond      | Wins if                                            |
|--------------|-----------|----------------------------------------------------|
| Prover `P`   | `B_P`     | challenger cannot exhibit a divergent step within `d` rounds |
| Challenger `C` | `B_C`   | at some round the prover cannot produce a consistent state commitment for the disputed interval |
| Referee (on-chain contract) | — | executes protocol rules; awards bond |

**Slashing hooks:** referee outcome feeds `SLASH_EQUIVOCATION_SPEC.md`
directly; the loser's stake is burned + partially awarded to winner (payout
split TBD in launch-plan doc).

---

## 6 · Protocol

### 6.1 · Kick-off — `challenge_root(R, subroot_index)`

The challenger publishes:

- The disputed anchored root `R` (already on-chain).
- The subroot index `j` inside `R` it believes is fraudulent (level 1).
- A bond `B_C` sent to the referee contract.

Referee enters `Level1Bisect(R, j, P, C)`.

### 6.2 · Level 1 bisection (log-of-subroots)

Standard binary bisection over `T_j` recovering the *state* interval
`[a, b]` where `P` and `C` disagree on `s_i`:

Loop invariant: `[a, b]` is the interval currently under dispute; `P`
claims `s_a`, `s_b`, and (implicitly, via `T_j`) all states between.

**Round k** (k = 1, 2, …, ≤ d):

1. Referee asks `P` for the midpoint state:
   ```
   m := (a + b) / 2
   P → s_m'  (P's claimed state at index m)
   ```
   with Merkle path proving `s_m'` is inside `T_j`.

2. Referee asks `C`:
   ```
   C picks half := "left" if C disputes s_m'
                else "right" if C accepts s_m' but disputes some s > m
   ```

3. Referee updates `[a, b] := [a, m]` or `[m, b]` accordingly.

4. If `b - a == 1`, exit loop → **execution round** (§ 6.3).

Any missed deadline in `d + O(log(response_time))` on-chain rounds ends the
game with the non-responder losing.

### 6.3 · Execution round

The disputed step is `i := a`. Referee now needs to determine which of
`P`'s claimed `s_i` and `C`'s claimed `s_i` (if any) is correct.

- `P` publishes the *witness*:
  ```
  witness_i := (inputs_i, model_id_i, prompt_i, output_i, prev_i)
  H(canonicalize(witness_i)) ?= s_i (from Merkle path)
  ```
- Referee verifies canonicalisation: recomputes `H(state_i)` from the
  witness fields and checks it matches the `s_i` `P` committed to in
  round `d` of § 6.2.

If the recomputation **matches**, `P` wins — `C` failed to exhibit a
divergence within the depth budget.

If the recomputation **does not match**, `P` committed to an internally-
inconsistent `s_i`; `P` loses; `C` wins.

### 6.4 · One-step model re-execution (optional escalation)

Even a canonicalisation-consistent `s_i` may still be a lie: `P` could have
recorded a *different* `output_hash` than the model actually produces on
`(inputs_i, prompt_i, model_id_i)`.

Detecting this requires model re-execution. Options:

- **A. Trusted re-execution provider** — referee dispatches
  `(inputs_i, prompt_i, model_id_i)` to a pre-registered *arbiter* model
  server (identity attested via AT-attestor). Compare canonical output hash
  to `output_i`. This introduces a trust root — the arbiter itself must
  be attested and periodically rotated.

- **B. zkML proof of one-step re-execution** — `P` supplies a
  succinct proof that a specified model on given inputs produces `output_i`.
  This is the *long-term* path; ties into `docs/ZK_ML_RESEARCH_SPIKE.md`
  (AM). **Not viable at hackathon scale.**

- **C. Statistical rejection** — sample multiple re-executions from an
  arbiter panel; reject if agreement is below quorum threshold. Ties into
  `MULTI_VERIFIER_GOSSIP.md`.

The hackathon-successor deployment starts with option **A** (single
attested arbiter), option **C** post-audit, option **B** as long-term.

### 6.5 · Determinism escape hatch

Real-world LLM outputs are non-deterministic. The trace commitment protocol
must canonicalise:

- **Temperature > 0 sampling.** `output_hash` binds the *sampled* token
  stream. Provider RNG seed is part of `state_i` (via `prompt_hash`
  extension). Re-execution uses the same seed.
- **Provider-side model updates.** `model_id` includes a version pin
  (`gpt-4o-2024-05-13`, not just `gpt-4o`).
- **Tool-call side effects.** For decisions involving tool calls, the trace
  records the *canonical hash of the tool response*, not the tool payload
  itself. Tool servers issuing signed responses (see
  `docs/AGENT_RECEIPT.md`) make this reproducible.

Steps that cannot be canonicalised are marked **`bisection_ineligible`** in
the receipt; they can be committed and anchored but cannot enter a
bisection game. Anchoring an ineligible step is not fraudulent — it is
transparent about its own non-verifiability.

---

## 7 · Round complexity

For a trace of `n ≤ 2^d` steps:

| Level | Interactions | On-chain size per round |
|-------|--------------|-------------------------|
| Level 1 (subroots) | ≤ `d_1 = ceil(log2(m))` | 1 hash + Merkle path (≤ `d_1 · 32B`) |
| Level 2 (states)   | ≤ `d_2 = ceil(log2(2^k))` = `k` | 1 hash + Merkle path (≤ `d_2 · 32B`) |
| Execution round    | 1 | canonical witness (bounded by inputs+prompt+output size) |

With `d ≤ 20` total, the on-chain cost is bounded by **~40 messages** and
**~2 KB** of Merkle path data per game. Well within Casper mainnet limits.

---

## 8 · Adversary Analysis

Attacker `A` wants to anchor a fraudulent root and *survive* interactive
challenge. Threats:

- **`A` forks the trace state.** Bisection converges to `i` where `A`
  cannot produce a consistent witness (§ 6.3). `A` loses. ✓
- **`A` delays by never responding.** Timeout → `A` loses. ✓
- **`A` DOSes the referee.** Referee is an on-chain contract; DOS = gas
  griefing; irrelevant for correctness. ✓
- **`A` publishes a challenge against an honest prover.** Bisection
  converges without exhibiting a witness mismatch; `A` (the challenger)
  loses their bond. ✓
- **`A` = both prover and challenger** (self-challenge to lose only bond,
  griefing the arbiter). Referee's bond schedule must ensure
  self-challenge is strictly negative-EV: `B_P + B_C > cost_of_arbiter_call`.
  See § 9. ✓
- **`A` bribes the arbiter (option A in § 6.4).** Requires attestor
  hardening (AT) plus arbiter rotation policy (§ 10). Not fully defeated
  by option A alone.

---

## 9 · Bond schedule (spec-only)

Let `C_arbiter` be the marginal cost of one arbiter re-execution.

```
B_P ≥ 10 · C_arbiter                  -- prover posts 10x re-execution cost
B_C ≥ 3 · C_arbiter                   -- challenger posts 3x
loser_pays := 100% of own bond
winner_gets := 90% of loser bond      -- 10% to referee (arbiter fee)
```

Both bond values are configurable per-tenant (per `TENANT_ISOLATION.md`,
BA) — a strategic account may want higher `B_P` to signal reliability.

---

## 10 · Arbiter Rotation (option A)

The arbiter model server is a trust root; if compromised, `A` can bribe
it to lie in § 6.4. Mitigations:

- **Attested execution.** Arbiter runs behind AT-attestor interfaces
  (TPM / SGX / SEV-SNP). Attestation quotes are anchored on Casper
  alongside the arbiter registration.
- **Rotation cadence.** New arbiter every 30 days. Old arbiter kept as
  cold reserve for 90 days for possible dispute over old traces.
- **Panel mode.** For high-value disputes, referee polls N arbiters and
  requires ≥ ⌈2N/3⌉ agreement (option C).

---

## 11 · Interface Sketch (post-audit)

**Not implemented.** For future audited PR:

```go
// engine/internal/dispute/bisection.go (POST-AUDIT, NOT SHIPPED)

package dispute

type Game struct {
    ID           string       // uuidv7
    Root         [32]byte     // disputed anchored root
    Level        int          // 1 (subroots) or 2 (states)
    IntervalLow  uint64
    IntervalHigh uint64
    Prover       PlayerID
    Challenger   PlayerID
    Round        int
    Deadline     time.Time
    State        GameState
    BondsPosted  map[PlayerID]Amount
}

type Referee interface {
    // Challenge opens a new game against an anchored root.
    Challenge(ctx context.Context, root [32]byte, subrootIndex uint64, bond Amount) (*Game, error)

    // Respond is called by the prover with a midpoint commitment.
    Respond(ctx context.Context, gameID string, midpointCommit [32]byte, proof MerkleProof) error

    // Pick is called by the challenger to pick left or right half.
    Pick(ctx context.Context, gameID string, half Half) error

    // Execute is called at the leaf level with the witness.
    Execute(ctx context.Context, gameID string, witness StateWitness) (Verdict, error)

    // Timeout is called by anyone once the current round deadline passed.
    Timeout(ctx context.Context, gameID string) (Verdict, error)
}

type Arbiter interface {
    // Reruns the one-step model call under attested conditions.
    Rerun(ctx context.Context, inputsHash [32]byte, promptHash [32]byte, modelID string) (outputHash [32]byte, quote AttestationQuote, err error)
}
```

Hooks:

- `receipts/canonical.go` — provides the canonical `state_i` shape.
- `attestor.Interface` (AT) — arbiter runs behind an attestor.
- `SLASH_EQUIVOCATION_SPEC.md` (BD) — verdict feeds slash burn/award.
- `MULTI_VERIFIER_GOSSIP.md` (BC) — panel mode uses gossip peer set.

---

## 12 · Migration Path

1. **Phase 0 (hackathon):** anchored roots + non-interactive Merkle proofs.
   No dispute game; disputes handled offline via human review.
2. **Phase 1 (post-audit):** deploy referee contract on testnet; opt-in for
   power users; option A arbiter (single attested).
3. **Phase 2:** enable panel mode (option C); default-on for enterprise
   tenants; challenger UX in frontend.
4. **Phase 3 (long-term):** experiment with option B (zkML proofs) for
   *specific* high-stakes model classes (e.g. small classifiers for lending
   decisions); leaves LLM decisions on option A/C.

---

## 13 · Open Questions

- **Bond currency.** CSPR vs stablecoin. Locking bonds in a volatile asset
  changes strategic dynamics; defer to launch-plan.
- **Retroactive challenge window.** How far back can a challenger open a
  game against an anchored root? Trade-off: longer window = higher safety
  but more prover capital locked. Proposal: 90 days.
- **Anonymised challengers.** Whistleblower mode (Semaphore-style) for
  compliance-sensitive callers. Design uncertain; deferred.
- **Interaction with MPC prover (AU).** When the prover is a threshold
  group, which member responds in bisection? Coordinator-of-record;
  session-signed responses. Defer to Phase 1 integration.

---

## 14 · References

- Kalodner et al. — *Arbitrum: Scalable, private smart contracts* (2018).
- Teutsch & Reitwießner — *A scalable verification solution for
  blockchains* (Truebit, 2017).
- Optimism — *Fault Proofs Alpha* (2024).
- CasperProver internal:
  - `docs/DISTRIBUTED_PROVER_MPC.md` (AU)
  - `docs/MULTI_VERIFIER_GOSSIP.md` (BC)
  - `docs/SLASH_EQUIVOCATION_SPEC.md` (BD)
  - `docs/ZK_ML_RESEARCH_SPIKE.md` (AM)
  - `docs/HARDWARE_ATTESTOR_INTERFACES.md` (AT)
  - `docs/AGENT_RECEIPT.md`

---

## 15 · Ladder statement

| Property                        | Ladder                     |
|---------------------------------|----------------------------|
| Bisection referee runtime       | **[SPEC / DEFERRED]** — no code shipped. |
| Trace commitment shape          | REAL — matches `receipts/canonical.go`. |
| On-chain referee contract       | **[POST-AUDIT]** — deferred. |
| Arbiter (option A)              | **[POST-AUDIT]** — attested. |
| zkML arbiter (option B)         | **[LONG-TERM RESEARCH]**. |
| Panel mode (option C)           | **[POST-AUDIT]** — depends on gossip. |
| Bonds & slashing wiring         | **[SPEC]** — feeds BD spec. |

_Deliberately code-free for the hackathon submission. Any future PR that
adds runtime dispute code is safety-critical and MUST land behind external
audit + on-chain testnet drills before mainnet exposure._
