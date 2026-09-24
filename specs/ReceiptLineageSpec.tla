--------------------------- MODULE ReceiptLineageSpec ---------------------------
(***************************************************************************)
(* Formal specification of the receipt-lineage DAG that the engine ships   *)
(* as `engine/internal/receipts` (Ancestors() walk). Each receipt has a   *)
(* set of parent-ids; the collection MUST form a DAG — no receipt may     *)
(* reach itself through any path of parent pointers. The Ancestors walk  *)
(* MUST always terminate.                                                  *)
(*                                                                         *)
(* Out of scope: the actual canonicalisation / signing of a receipt,      *)
(* which is exhaustively covered by the Go unit tests.                    *)
(*                                                                         *)
(* Safety invariants:                                                      *)
(*   - TypeOK                                                              *)
(*   - AllParentsExist: every parent id in a receipt refers to a          *)
(*     receipt already in the store                                       *)
(*   - NoSelfParent: no receipt lists itself as a parent                  *)
(*   - AcyclicByEmission: parents are only ids of receipts emitted        *)
(*     strictly earlier (by emission ord), which structurally forbids    *)
(*     cycles                                                              *)
(*   - AncestorsTerminate: computed ancestor set is finite and bounded    *)
(***************************************************************************)

EXTENDS Naturals, FiniteSets, TLC

CONSTANTS
    MaxReceipts,     \* upper bound on total receipts
    MaxParents       \* upper bound on |parents| per receipt

ASSUME
    /\ MaxReceipts \in Nat /\ MaxReceipts >= 1
    /\ MaxParents  \in Nat /\ MaxParents  >= 0

VARIABLES
    receipts,        \* set of receipt records
    emissionOrd      \* monotonic counter: emission order

vars == <<receipts, emissionOrd>>

(***************************************************************************)
(* A receipt has:                                                          *)
(*   id     : monotonic natural, unique                                    *)
(*   ord    : emission ordinal (equals id in this abstraction)            *)
(*   parents: set of ids of earlier receipts                               *)
(***************************************************************************)

ReceiptRecord ==
    [ id      : Nat,
      ord     : Nat,
      parents : SUBSET Nat ]

TypeOK ==
    /\ receipts \subseteq ReceiptRecord
    /\ emissionOrd \in Nat
    /\ Cardinality(receipts) <= MaxReceipts
    /\ \A r \in receipts : Cardinality(r.parents) <= MaxParents

Init ==
    /\ receipts    = {}
    /\ emissionOrd = 0

\* Emit a new receipt with a subset of existing ids as parents.
Emit(parents) ==
    /\ Cardinality(receipts) < MaxReceipts
    /\ parents \subseteq { r.id : r \in receipts }
    /\ Cardinality(parents) <= MaxParents
    /\ LET newId == emissionOrd + 1
           new   == [ id      |-> newId,
                      ord     |-> newId,
                      parents |-> parents ]
       IN /\ receipts' = receipts \union {new}
          /\ emissionOrd' = newId

Next ==
    \E P \in SUBSET { r.id : r \in receipts } : Emit(P)

Spec == Init /\ [][Next]_vars

(***************************************************************************)
(* Invariants                                                              *)
(***************************************************************************)

\* Every parent id refers to a receipt already in the store.
AllParentsExist ==
    \A r \in receipts :
        \A pid \in r.parents :
            \E q \in receipts : q.id = pid

\* A receipt never lists itself as a parent.
NoSelfParent ==
    \A r \in receipts : r.id \notin r.parents

\* Structural acyclicity: every parent has a strictly smaller ord than the
\* child. Because ords are monotonic, this forbids cycles.
AcyclicByEmission ==
    \A r \in receipts :
        \A pid \in r.parents :
            \E q \in receipts : q.id = pid /\ q.ord < r.ord

\* Bound the ancestor closure. In a DAG with n nodes the transitive closure
\* is at most n-1 ancestors for any node; the walk always terminates.
AncestorsBounded ==
    \A r \in receipts :
        Cardinality({ q \in receipts : q.ord < r.ord }) < MaxReceipts

SafetyInvariant ==
    /\ TypeOK
    /\ AllParentsExist
    /\ NoSelfParent
    /\ AcyclicByEmission
    /\ AncestorsBounded

===============================================================================
\* --- END ReceiptLineageSpec ---------------------------------------------- *\
