package attest

import (
	"errors"
	"time"
)

// GateDecision is what a downstream module (e.g. defi-mock) sees when it
// asks whether a committed decision is currently authoritative.
type GateDecision uint8

const (
	// GatePending: challenge window still open, no downstream action.
	GatePending GateDecision = iota
	// GateAllowed: the decision APPROVEd and the challenge window closed
	// without a successful challenge.
	GateAllowed
	// GateBlocked: the decision REJECTed (or was vetoed) or a challenge
	// succeeded before the window closed.
	GateBlocked
	// GateAbstained: the decision ABSTAINed. Downstream modules must
	// treat this as "no answer" — they should neither allow nor block.
	GateAbstained
)

// String renders a GateDecision for logs and receipts.
func (g GateDecision) String() string {
	switch g {
	case GateAllowed:
		return "ALLOWED"
	case GateBlocked:
		return "BLOCKED"
	case GateAbstained:
		return "ABSTAINED"
	default:
		return "PENDING"
	}
}

// ChallengeWindow bundles the timings used by the challenge/slash flow.
type ChallengeWindow struct {
	// Duration is how long after commit the challenge window stays open.
	Duration time.Duration
}

// DefaultChallengeWindow gives the demo 5 seconds — enough for the
// reproducer script to file a successful and an unsuccessful challenge
// without long waits.
var DefaultChallengeWindow = ChallengeWindow{Duration: 5 * time.Second}

// ChallengeResult captures the outcome of a challenge against a commit.
type ChallengeResult struct {
	// Successful means the challenger presented valid counter-evidence.
	Successful bool
	// Reason is a short human-readable explanation.
	Reason string
	// At is the wall-clock moment the challenge was filed.
	At time.Time
}

// GateEvaluator maps a DecisionCommit + optional challenge outcome into a
// GateDecision.
type GateEvaluator struct {
	window ChallengeWindow
	// nowFn is injectable for deterministic tests. Defaults to time.Now.
	nowFn func() time.Time
}

// NewGateEvaluator returns an evaluator with the given challenge window
// and a real wall-clock now function.
func NewGateEvaluator(w ChallengeWindow) *GateEvaluator {
	return &GateEvaluator{window: w, nowFn: time.Now}
}

// WithClock returns a copy of the evaluator that uses nowFn instead of
// time.Now. Tests should use this to stay deterministic.
func (g *GateEvaluator) WithClock(nowFn func() time.Time) *GateEvaluator {
	c := *g
	c.nowFn = nowFn
	return &c
}

// ErrCommitNotJudged is returned when Evaluate is called on a commit
// whose Aggregate is VerdictUnknown.
var ErrCommitNotJudged = errors.New("decision: commit has no aggregate verdict")

// Evaluate returns the current GateDecision for a commit. A challenge that
// was filed AFTER the challenge window closed is treated as too-late and
// ignored; the caller is expected to enforce that same rule when reading
// authoritative on-chain state, but the local evaluator is defensive.
func (g *GateEvaluator) Evaluate(commit DecisionCommit, ch *ChallengeResult) (GateDecision, error) {
	if commit.Aggregate == VerdictUnknown {
		return GatePending, ErrCommitNotJudged
	}
	switch commit.Aggregate {
	case VerdictReject:
		return GateBlocked, nil
	case VerdictAbstain:
		return GateAbstained, nil
	}

	// From here we know Aggregate is APPROVE.
	closesAt := commit.Decision.SubmittedAt.Add(g.window.Duration)
	now := g.nowFn()

	// A challenge counts only if it landed before the window closed.
	if ch != nil && !ch.At.After(closesAt) {
		if ch.Successful {
			return GateBlocked, nil
		}
	}

	if now.Before(closesAt) {
		return GatePending, nil
	}
	return GateAllowed, nil
}
