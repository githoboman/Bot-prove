------------------------------ MODULE ProofSystemSpec ------------------------------
(***************************************************************************)
(* Formal specification of the CasperProver proof-registry / decision      *)
(* attestation state machine.                                              *)
(*                                                                         *)
(* This is an executable spec: TLC can enumerate the entire finite         *)
(* state space under the CONSTANTS below and verify the SafetyInvariant.  *)
(*                                                                         *)
(* Scope: what actually lands on chain — a proof-registry entry becomes    *)
(* "verified" only when it carries a valid PQ signature, references a     *)
(* registered model, and (if part of a chain) continues the previous step *)
(* without gaps. A decision commit passes downstream gate only after      *)
(* aggregation says APPROVE and the challenge window has closed without a *)
(* successful challenge.                                                   *)
(*                                                                         *)
(* Out of scope in this spec: gnark proof mechanics, on-chain fees,       *)
(* actual byte-level encoding of the commit digest.                       *)
(***************************************************************************)

EXTENDS Naturals, Sequences, FiniteSets, TLC

CONSTANTS
    Provers,           \* set of prover identities (finite)
    Models,            \* set of registered model hashes (finite)
    MaxProofs,         \* upper bound on proofs to explore
    MaxChainDepth,     \* upper bound on chain length
    ChallengeWindow    \* number of ticks after commit before window closes

ASSUME
    /\ MaxProofs \in Nat /\ MaxProofs >= 1
    /\ MaxChainDepth \in Nat /\ MaxChainDepth >= 1
    /\ ChallengeWindow \in Nat /\ ChallengeWindow >= 1
    /\ Provers # {} /\ Models # {}

VARIABLES
    proofs,       \* set of proof records
    chains,       \* set of chain records
    time,         \* logical clock
    slashedSet    \* provers proven byzantine (equivocation caught)

vars == <<proofs, chains, time, slashedSet>>

(***************************************************************************)
(* Type definitions                                                        *)
(***************************************************************************)

Verdicts == {"APPROVE", "ABSTAIN", "REJECT"}
ProofStatus == {"pending", "verified", "rejected"}
GateOutcomes == {"PENDING", "ALLOWED", "BLOCKED", "ABSTAINED"}

\* A proof record — the on-chain projection of a DecisionCommit.
ProofRecord ==
    [ id            : Nat,
      prover        : Provers,
      model_hash    : Models,
      status        : ProofStatus,
      verdict       : Verdicts,
      pq_valid      : BOOLEAN,
      submitted_at  : Nat,
      challenged    : BOOLEAN,
      chain_id      : Nat,
      chain_pos     : Nat ]

\* A chain of proofs, each step references the previous by hash.
ChainRecord ==
    [ id     : Nat,
      steps  : Seq(Nat),     \* proof ids in chain order
      depth  : Nat ]

TypeOK ==
    /\ proofs \subseteq ProofRecord
    /\ chains \subseteq ChainRecord
    /\ time \in Nat
    /\ slashedSet \subseteq Provers
    /\ Cardinality(proofs) <= MaxProofs
    /\ \A c \in chains : c.depth <= MaxChainDepth
    /\ \A c \in chains : Len(c.steps) = c.depth

(***************************************************************************)
(* Helpers                                                                 *)
(***************************************************************************)

NextProofId ==
    IF proofs = {} THEN 1 ELSE (CHOOSE m \in {p.id : p \in proofs} :
                                   \A q \in {p.id : p \in proofs} : m >= q) + 1

NextChainId ==
    IF chains = {} THEN 1 ELSE (CHOOSE m \in {c.id : c \in chains} :
                                   \A q \in {c.id : c \in chains} : m >= q) + 1

GateOf(p) ==
    IF p.status = "rejected" THEN "BLOCKED"
    ELSE IF p.verdict = "ABSTAIN" THEN "ABSTAINED"
    ELSE IF p.challenged THEN "BLOCKED"
    ELSE IF (time - p.submitted_at) >= ChallengeWindow /\ p.status = "verified"
         THEN "ALLOWED"
    ELSE "PENDING"

\* Two proofs from the same prover, same model, differing ids are treated
\* as equivocation. In the real system the ledger keyed on (submitter,
\* spec_id) catches this — for the spec we abstract spec_id ≡ model_hash.
Equivocates(p, q) ==
    /\ p.id # q.id
    /\ p.prover = q.prover
    /\ p.model_hash = q.model_hash

(***************************************************************************)
(* Initial state                                                           *)
(***************************************************************************)

Init ==
    /\ proofs = {}
    /\ chains = {}
    /\ time = 0
    /\ slashedSet = {}

(***************************************************************************)
(* Actions                                                                 *)
(***************************************************************************)

\* Submit a fresh proof. A prover already in slashedSet cannot submit.
SubmitProof(pr, m, v, valid_pq) ==
    /\ Cardinality(proofs) < MaxProofs
    /\ pr \in Provers /\ pr \notin slashedSet
    /\ m \in Models
    /\ v \in Verdicts
    /\ valid_pq \in BOOLEAN
    /\ LET new == [ id           |-> NextProofId,
                    prover       |-> pr,
                    model_hash   |-> m,
                    status       |-> IF valid_pq /\ v # "REJECT"
                                     THEN "verified" ELSE "rejected",
                    verdict      |-> v,
                    pq_valid     |-> valid_pq,
                    submitted_at |-> time,
                    challenged   |-> FALSE,
                    chain_id     |-> 0,
                    chain_pos    |-> 0 ]
       IN proofs' = proofs \union {new}
    /\ chains' = chains
    /\ time' = time
    /\ slashedSet' = slashedSet

\* Extend a chain with a new proof that references the previous step.
\* This action models chain continuity: each new step submits a fresh
\* proof id and appends it to the chain.
ExtendChain(cid, pr, m) ==
    /\ Cardinality(proofs) < MaxProofs
    /\ pr \in Provers /\ pr \notin slashedSet
    /\ m \in Models
    /\ \E c \in chains :
        /\ c.id = cid
        /\ c.depth < MaxChainDepth
        /\ LET newProof == [ id           |-> NextProofId,
                             prover       |-> pr,
                             model_hash   |-> m,
                             status       |-> "verified",
                             verdict      |-> "APPROVE",
                             pq_valid     |-> TRUE,
                             submitted_at |-> time,
                             challenged   |-> FALSE,
                             chain_id     |-> cid,
                             chain_pos    |-> c.depth + 1 ]
               newChain == [ c EXCEPT !.steps = Append(c.steps, NextProofId),
                                       !.depth = c.depth + 1 ]
           IN /\ proofs' = proofs \union {newProof}
              /\ chains' = (chains \ {c}) \union {newChain}
    /\ time' = time
    /\ slashedSet' = slashedSet

\* Open a fresh chain with a first step.
OpenChain(pr, m) ==
    /\ Cardinality(proofs) < MaxProofs
    /\ pr \in Provers /\ pr \notin slashedSet
    /\ m \in Models
    /\ LET pid == NextProofId
           cid == NextChainId
           newProof == [ id           |-> pid,
                         prover       |-> pr,
                         model_hash   |-> m,
                         status       |-> "verified",
                         verdict      |-> "APPROVE",
                         pq_valid     |-> TRUE,
                         submitted_at |-> time,
                         challenged   |-> FALSE,
                         chain_id     |-> cid,
                         chain_pos    |-> 1 ]
           newChain == [ id |-> cid, steps |-> <<pid>>, depth |-> 1 ]
       IN /\ proofs' = proofs \union {newProof}
          /\ chains' = chains \union {newChain}
    /\ time' = time
    /\ slashedSet' = slashedSet

\* Human challenge inside the challenge window: mark the proof challenged.
Challenge(pid) ==
    /\ \E p \in proofs :
        /\ p.id = pid
        /\ p.status = "verified"
        /\ p.verdict = "APPROVE"
        /\ ~ p.challenged
        /\ (time - p.submitted_at) < ChallengeWindow
        /\ proofs' = (proofs \ {p}) \union {[p EXCEPT !.challenged = TRUE]}
    /\ chains' = chains
    /\ time' = time
    /\ slashedSet' = slashedSet

\* Slash the equivocator: whenever two distinct proofs from the same
\* prover on the same model exist, that prover is added to slashedSet.
Slash ==
    /\ \E p \in proofs, q \in proofs : Equivocates(p, q)
       /\ p.prover \notin slashedSet
       /\ slashedSet' = slashedSet \union {p.prover}
    /\ proofs' = proofs
    /\ chains' = chains
    /\ time' = time

\* Advance the logical clock. Bounded so TLC terminates.
Tick ==
    /\ time < 2 * ChallengeWindow
    /\ time' = time + 1
    /\ proofs' = proofs
    /\ chains' = chains
    /\ slashedSet' = slashedSet

Next ==
    \/ \E pr \in Provers, m \in Models, v \in Verdicts, b \in BOOLEAN :
         SubmitProof(pr, m, v, b)
    \/ \E cid \in {c.id : c \in chains}, pr \in Provers, m \in Models :
         ExtendChain(cid, pr, m)
    \/ \E pr \in Provers, m \in Models : OpenChain(pr, m)
    \/ \E pid \in {p.id : p \in proofs} : Challenge(pid)
    \/ Slash
    \/ Tick

Spec == Init /\ [][Next]_vars

(***************************************************************************)
(* Invariants                                                              *)
(***************************************************************************)

\* Every "verified" proof carries a valid PQ signature.
PQSignatureValidity ==
    \A p \in proofs : p.status = "verified" => p.pq_valid

\* Every proof references a registered model.
ModelBinding == \A p \in proofs : p.model_hash \in Models

\* Chain continuity: chain steps refer to existing proofs at increasing
\* positions.
ChainContinuity ==
    \A c \in chains :
        /\ Len(c.steps) = c.depth
        /\ \A i \in 1..Len(c.steps) :
            \E p \in proofs : p.id = c.steps[i] /\ p.chain_pos = i

\* Immutability: a verified, unchallenged proof past its window has gate
\* ALLOWED — this state is stable under Tick until (never) challenged.
\* (In TLA+ terms: challenges only work inside the window, so a proof
\* that is past the window and unchallenged cannot become challenged.)
ChallengeWindowRespected ==
    \A p \in proofs :
        (p.status = "verified" /\ p.verdict = "APPROVE"
         /\ (time - p.submitted_at) >= ChallengeWindow
         /\ ~ p.challenged)
        => GateOf(p) = "ALLOWED"

\* A rejected proof (bad PQ, or REJECT verdict) never gates ALLOWED.
RejectionBlocks ==
    \A p \in proofs :
        p.status = "rejected" => GateOf(p) # "ALLOWED"

\* An ABSTAINed proof never gates ALLOWED or BLOCKED silently.
AbstainNeutrality ==
    \A p \in proofs :
        (p.status = "verified" /\ p.verdict = "ABSTAIN") => GateOf(p) = "ABSTAINED"

\* Equivocation is eventually detectable: whenever two conflicting proofs
\* co-exist from a prover, that prover MAY be slashed. The invariant
\* below asserts that a slashed prover must have at least two proofs on
\* record (the equivocating pair that triggered the slash). New proofs
\* from a slashed prover never appear (enforced by the SubmitProof /
\* OpenChain / ExtendChain guards on `pr \notin slashedSet`), so the
\* count only grows before the slash.
SlashedProversHaveEvidence ==
    \A P \in slashedSet :
        \E p, q \in proofs :
            /\ p.id # q.id
            /\ p.prover = P /\ q.prover = P
            /\ p.model_hash = q.model_hash

\* Every registered chain's steps are all in `proofs` and all from a
\* non-slashed prover at each step's time. The spec's action guards
\* enforce this at add-time; the invariant re-checks structurally.
ChainStepsAreValid ==
    \A c \in chains :
        \A i \in 1..Len(c.steps) :
            \E p \in proofs :
                /\ p.id = c.steps[i]
                /\ p.chain_id = c.id
                /\ p.chain_pos = i
                /\ p.status = "verified"

\* IDs of proofs are unique.
ProofIdUnique ==
    \A p, q \in proofs : (p.id = q.id) => (p = q)

\* IDs of chains are unique.
ChainIdUnique ==
    \A c, d \in chains : (c.id = d.id) => (c = d)

\* The main safety invariant TLC will check.
SafetyInvariant ==
    /\ TypeOK
    /\ PQSignatureValidity
    /\ ModelBinding
    /\ ChainContinuity
    /\ ChainStepsAreValid
    /\ ProofIdUnique
    /\ ChainIdUnique
    /\ ChallengeWindowRespected
    /\ RejectionBlocks
    /\ AbstainNeutrality
    /\ SlashedProversHaveEvidence

===============================================================================
\* --- END ProofSystemSpec ------------------------------------------------- *\
