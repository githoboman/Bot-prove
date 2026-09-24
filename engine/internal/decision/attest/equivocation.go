package attest

import (
	"sync"
)

// EquivocationLedger tracks conflicting decision commitments made by the
// same signer within the active window. Two commitments conflict when they
// share the same (Submitter, SpecID) but differ in payload or nonce — that
// is, the signer is trying to bind two incompatible statements to the same
// slot.
//
// The ledger is deliberately in-memory: it is a fast pre-check used by the
// equivocation facet to short-circuit obvious conflicts. The authoritative
// record of equivocation is the on-chain proof-registry, which persists
// commitments across restarts.
type EquivocationLedger struct {
	mu sync.RWMutex
	// key = Submitter||"|"||SpecID
	seen map[string]ledgerEntry
}

type ledgerEntry struct {
	DecisionID string
	Payload    []byte
	Nonce      uint64
}

// NewEquivocationLedger returns an empty ledger.
func NewEquivocationLedger() *EquivocationLedger {
	return &EquivocationLedger{seen: make(map[string]ledgerEntry)}
}

// Record adds a decision to the ledger. It returns true and the ID of the
// previously-seen conflicting decision when a conflict is detected; false
// and an empty string otherwise. Re-recording an identical decision (same
// ID) is a no-op and does not count as a conflict.
func (l *EquivocationLedger) Record(d Decision) (conflict bool, previous string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := d.Submitter + "|" + d.SpecID
	entry, ok := l.seen[key]
	if !ok {
		l.seen[key] = ledgerEntry{
			DecisionID: d.ID(),
			Payload:    append([]byte(nil), d.Payload...),
			Nonce:      d.Nonce,
		}
		return false, ""
	}
	// Same decision ID: idempotent, not a conflict.
	if entry.DecisionID == d.ID() {
		return false, ""
	}
	// Different ID for the same (submitter, spec): conflict.
	return true, entry.DecisionID
}

// Check reports whether a decision would conflict without recording it.
// Useful for a read-only equivocation facet that runs before the commit
// is finalised.
func (l *EquivocationLedger) Check(d Decision) (conflict bool, previous string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	key := d.Submitter + "|" + d.SpecID
	entry, ok := l.seen[key]
	if !ok {
		return false, ""
	}
	if entry.DecisionID == d.ID() {
		return false, ""
	}
	return true, entry.DecisionID
}

// EquivocationFacet builds a FacetVerdict for FacetEquivocation using the
// ledger. It never mutates the ledger — Record is the caller's job at the
// point the commit is finalised.
func (l *EquivocationLedger) EquivocationFacet(d Decision) FacetVerdict {
	conflict, prev := l.Check(d)
	if conflict {
		return FacetVerdict{
			Kind:       FacetEquivocation,
			Verdict:    VerdictReject,
			Confidence: 1.0,
			Reason:     "same signer previously committed conflicting decision " + prev,
		}
	}
	return FacetVerdict{
		Kind:       FacetEquivocation,
		Verdict:    VerdictApprove,
		Confidence: 1.0,
		Reason:     "no prior conflicting commitment",
	}
}
