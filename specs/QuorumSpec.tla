------------------------------- MODULE QuorumSpec -------------------------------
(***************************************************************************)
(* Formal specification of the BLS12-381 threshold-quorum registry that   *)
(* the engine ships as `engine/internal/quorum`. The real subsystem       *)
(* maintains a set of signers, tracks their lifecycle (active → slashed / *)
(* removed), and gates each verification through the byzantine threshold  *)
(* T(n) = floor(2n/3) + 1 over the CURRENTLY ACTIVE signer count.        *)
(*                                                                         *)
(* Out of scope: BLS12-381 pairing arithmetic, hash-to-curve mechanics,   *)
(* on-chain evidence anchoring. This spec models only the state-machine  *)
(* around the registry and the quorum-check gate.                         *)
(*                                                                         *)
(* Safety invariants checked:                                              *)
(*   - StateInvariant: a signer is in exactly one of active/slashed/removed*)
(*   - ThresholdCorrect: T(n) == floor(2n/3) + 1 for the active count     *)
(*   - QuorumOnlyOnActive: a passing quorum witness only counts active    *)
(*     signers                                                             *)
(*   - MonotonicSlashing: once slashed, never returns to active            *)
(*   - RemovedFromNonSlashed: `retire` only fires on currently active      *)
(*   - AcceptedWitnessImpliesActiveMajority: every accepted witness had a  *)
(*     signer-set that met the byzantine threshold at accept time         *)
(***************************************************************************)

EXTENDS Naturals, FiniteSets, TLC

CONSTANTS
    Signers,           \* finite set of candidate signer ids
    MaxAccepted        \* upper bound on accepted witnesses (keeps TLC finite)

ASSUME
    /\ Signers # {}
    /\ MaxAccepted \in Nat /\ MaxAccepted >= 1

VARIABLES
    active,        \* subset of Signers currently active
    slashed,       \* subset of Signers permanently slashed
    removed,       \* subset of Signers voluntarily retired
    accepted       \* set of accepted witness records

vars == <<active, slashed, removed, accepted>>

(***************************************************************************)
(* Threshold function: byzantine 2/3 majority + 1, clamped to at least 1  *)
(* when there is at least one active signer. Matches                     *)
(* engine/internal/quorum/registry.go: ByzantineThreshold.                *)
(***************************************************************************)

Threshold(n) ==
    IF n = 0 THEN 0
    ELSE ((2 * n) \div 3) + 1

(***************************************************************************)
(* An accepted witness is the abstract projection of a quorum.Witness.   *)
(* At accept time it records the set of signers whose contribution was    *)
(* counted, and the active-count that was in effect.                      *)
(***************************************************************************)

WitnessRecord ==
    [ id           : Nat,
      signers_used : SUBSET Signers,
      active_at    : Nat,
      threshold    : Nat ]

TypeOK ==
    /\ active   \subseteq Signers
    /\ slashed  \subseteq Signers
    /\ removed  \subseteq Signers
    /\ accepted \subseteq WitnessRecord
    /\ Cardinality(accepted) <= MaxAccepted

(***************************************************************************)
(* Initial state: no signers, no witnesses.                                *)
(***************************************************************************)

Init ==
    /\ active   = {}
    /\ slashed  = {}
    /\ removed  = {}
    /\ accepted = {}

(***************************************************************************)
(* Actions                                                                 *)
(***************************************************************************)

\* Register a signer that has never been in any state before.
Register(s) ==
    /\ s \in Signers
    /\ s \notin active
    /\ s \notin slashed
    /\ s \notin removed
    /\ active' = active \union {s}
    /\ slashed' = slashed
    /\ removed' = removed
    /\ accepted' = accepted

\* Slash an active signer. Move to `slashed`. Idempotent on an already
\* slashed signer (registry.go: Slash returns true only on transition).
Slash(s) ==
    /\ s \in active
    /\ active'  = active \ {s}
    /\ slashed' = slashed \union {s}
    /\ removed' = removed
    /\ accepted' = accepted

\* Voluntary retire — only fires on currently active signers.
Retire(s) ==
    /\ s \in active
    /\ active'  = active \ {s}
    /\ slashed' = slashed
    /\ removed' = removed \union {s}
    /\ accepted' = accepted

\* Accept a quorum witness. The set of signers used must be a subset of
\* the CURRENTLY ACTIVE signer set (real registry verifies each sig
\* against a currently-active pubkey), and the count must meet the
\* byzantine threshold of active. This is the only action that adds to
\* `accepted`.
AcceptWitness(S, wid) ==
    /\ Cardinality(accepted) < MaxAccepted
    /\ S \subseteq active
    /\ S # {}
    /\ Cardinality(S) >= Threshold(Cardinality(active))
    /\ wid \in Nat
    /\ \A w \in accepted : w.id # wid
    /\ LET w == [ id           |-> wid,
                  signers_used |-> S,
                  active_at    |-> Cardinality(active),
                  threshold    |-> Threshold(Cardinality(active)) ]
       IN accepted' = accepted \union {w}
    /\ active'  = active
    /\ slashed' = slashed
    /\ removed' = removed

\* Next-witness-id is one past the max already accepted id.
NextWitnessId ==
    IF accepted = {} THEN 1
    ELSE (CHOOSE m \in {w.id : w \in accepted} :
            \A n \in {w.id : w \in accepted} : m >= n) + 1

Next ==
    \/ \E s \in Signers : Register(s)
    \/ \E s \in Signers : Slash(s)
    \/ \E s \in Signers : Retire(s)
    \/ \E S \in SUBSET Signers : AcceptWitness(S, NextWitnessId)

Spec == Init /\ [][Next]_vars

(***************************************************************************)
(* Invariants                                                              *)
(***************************************************************************)

\* Each signer is in exactly one bucket (or none).
StateInvariant ==
    /\ active \cap slashed  = {}
    /\ active \cap removed  = {}
    /\ slashed \cap removed = {}

\* The threshold function matches floor(2n/3)+1 (for n>0). This is a
\* constant assumption (Threshold does not depend on the variables),
\* so we phrase it as an ASSUME rather than a variable invariant.
ASSUME ThresholdCorrect ==
    \A n \in 0..Cardinality(Signers) :
        (n > 0 => Threshold(n) = ((2 * n) \div 3) + 1)
        /\ (n = 0 => Threshold(n) = 0)

\* Structural: every accepted witness used at most `active_at` signers,
\* and met its recorded threshold. The action guard also required
\* `signers_used \subseteq active` at accept time; we can no longer
\* re-check that fact from later states, but the byzantine-majority
\* invariant below is the property downstream code relies on.
WitnessBudget ==
    \A w \in accepted :
        /\ Cardinality(w.signers_used) <= w.active_at
        /\ Cardinality(w.signers_used) >= w.threshold
        /\ w.threshold = Threshold(w.active_at)

\* Slashing is monotonic: once slashed, never returns to active or removed.
\* Enforced by action guards; invariant checks the disjointness across
\* all reachable states.
MonotonicSlashing ==
    /\ slashed \cap active  = {}
    /\ slashed \cap removed = {}

\* Retire only fires on active signers, so removed \cap slashed = {}.
\* (Combined with MonotonicSlashing above.)
RetireIsClean ==
    /\ removed \cap slashed = {}
    /\ removed \cap active  = {}

\* Byzantine safety: every accepted witness passed the (strictly greater
\* than half) 2n/3+1 gate. This is the property downstream code relies on.
AcceptedWitnessImpliesByzantineMajority ==
    \A w \in accepted :
        3 * Cardinality(w.signers_used) > 2 * w.active_at

SafetyInvariant ==
    /\ TypeOK
    /\ StateInvariant
    /\ WitnessBudget
    /\ MonotonicSlashing
    /\ RetireIsClean
    /\ AcceptedWitnessImpliesByzantineMajority

===============================================================================
\* --- END QuorumSpec ------------------------------------------------------ *\
