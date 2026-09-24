// Package decision implements the verifiable decision attestation layer.
//
// A submitter (any principal holding a wallet key) commits a decision — an
// opaque payload plus a fixed set of structured facets — to the on-chain
// registry. The commitment is judged by an independent quorum of facet
// evaluators (deterministic policy functions in this implementation) and the
// aggregated verdict is bound into a real off-chain Groth16 proof. The final
// verdict may be one of APPROVE, ABSTAIN or REJECT. A critical-facet veto
// short-circuits aggregation to REJECT regardless of the numerical quorum, so
// safety-relevant facets can never be out-voted by pure counting.
//
// Nothing in this package touches large language models, autonomous execution
// loops, or non-deterministic providers at runtime: providers are pluggable
// and, for hackathon reproducibility, the default provider is a deterministic
// fixture. Real hosted providers can be wired via the Provider interface
// without any change to the aggregation, veto, or proof logic.
package attest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Verdict is the outcome of a single facet or the aggregated decision.
type Verdict uint8

const (
	// VerdictUnknown is the zero value; it never crosses a public boundary
	// and always indicates a programmer error.
	VerdictUnknown Verdict = iota
	// VerdictApprove means the facet or aggregate accepts the decision.
	VerdictApprove
	// VerdictAbstain means the facet or aggregate refuses to take sides
	// (typically because confidence is below a policy threshold or a
	// fixture explicitly abstains).
	VerdictAbstain
	// VerdictReject means the facet or aggregate rejects the decision.
	VerdictReject
)

// String renders a Verdict for logs, receipts and error strings.
func (v Verdict) String() string {
	switch v {
	case VerdictApprove:
		return "APPROVE"
	case VerdictAbstain:
		return "ABSTAIN"
	case VerdictReject:
		return "REJECT"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON emits the string form so JSON receipts are human-readable.
// Unmarshalling is not implemented — receipts are produced, not consumed,
// by CasperProver.
func (v Verdict) MarshalJSON() ([]byte, error) {
	return []byte(`"` + v.String() + `"`), nil
}

// FacetKind identifies a structured evaluation dimension. Kinds are a small
// closed set on purpose — every kind has a documented meaning in
// docs/DECISION_LAYER.md and a corresponding evaluator implementation.
type FacetKind string

const (
	// FacetSafety inspects whether the decision violates a hard policy
	// invariant (prompt injection, ex-filtration attempt, unsafe payload).
	// It is a critical facet: a REJECT here vetoes the whole decision.
	FacetSafety FacetKind = "safety"
	// FacetCorrectness inspects whether the decision satisfies its own
	// declared post-condition (e.g. numeric bounds, referenced hash exists).
	FacetCorrectness FacetKind = "correctness"
	// FacetSpecCompliance inspects whether the decision matches a declared
	// spec_id (schema, versioned rule set).
	FacetSpecCompliance FacetKind = "spec_compliance"
	// FacetEquivocation inspects whether the same signer previously
	// committed a conflicting decision within the current window. It is a
	// critical facet: a REJECT here vetoes the whole decision.
	FacetEquivocation FacetKind = "equivocation"
)

// AllFacetKinds lists every facet kind in the canonical order used for
// deterministic hashing of a DecisionCommit.
var AllFacetKinds = []FacetKind{
	FacetSafety,
	FacetCorrectness,
	FacetSpecCompliance,
	FacetEquivocation,
}

// isCritical reports whether a REJECT from this facet short-circuits the
// aggregation to REJECT (critical-veto).
func (k FacetKind) isCritical() bool {
	return k == FacetSafety || k == FacetEquivocation
}

// Decision is the payload a principal wants to commit. Payload is opaque
// bytes; the layer never interprets them beyond hashing. SpecID names the
// versioned rule set the payload should comply with.
type Decision struct {
	// Submitter is the hex-encoded public key of the wallet committing.
	Submitter string
	// SpecID identifies the versioned rule set (e.g. "policy/v1").
	SpecID string
	// Payload is opaque bytes committed by the submitter.
	Payload []byte
	// Nonce prevents collisions between otherwise-identical payloads and
	// gives replay protection at the equivocation layer.
	Nonce uint64
	// SubmittedAt is the wall-clock submission time; the layer uses it for
	// challenge-window bookkeeping only.
	SubmittedAt time.Time
}

// ID returns the canonical decision identifier: sha256 over
// submitter||spec_id||payload||nonce (all length-prefixed, big-endian). The
// same identifier is committed on-chain and bound into the ZK proof, so it
// must be reproducible byte-for-byte across processes and languages.
func (d Decision) ID() string {
	h := sha256.New()
	writeLP(h, []byte(d.Submitter))
	writeLP(h, []byte(d.SpecID))
	writeLP(h, d.Payload)
	writeU64(h, d.Nonce)
	return hex.EncodeToString(h.Sum(nil))
}

// FacetVerdict is one facet evaluator's judgement.
type FacetVerdict struct {
	Kind    FacetKind
	Verdict Verdict
	// Confidence is in [0.0, 1.0] and is used by policy-abstention. It is
	// only meaningful for non-critical facets; for critical facets a REJECT
	// is a hard veto regardless of confidence.
	Confidence float64
	// Reason is a short human-readable explanation, included in receipts.
	Reason string
}

// DecisionCommit is the fully-judged decision, ready for proof binding.
type DecisionCommit struct {
	Decision      Decision
	DecisionID    string
	FacetVerdicts []FacetVerdict
	Aggregate     Verdict
	// VetoedBy names the critical facet that forced a REJECT, if any.
	VetoedBy FacetKind
	// AbstainReason names the reason for an ABSTAIN, if any.
	AbstainReason string
}

// CommitDigest returns sha256 over the canonical commit representation.
// The digest is what the on-chain proof-registry stores and what the ZK
// proof binds as public input.
func (c DecisionCommit) CommitDigest() string {
	h := sha256.New()
	writeLP(h, []byte(c.DecisionID))
	// Sort facet verdicts by kind to make the digest deterministic
	// regardless of evaluation order.
	sorted := append([]FacetVerdict(nil), c.FacetVerdicts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Kind < sorted[j].Kind })
	for _, fv := range sorted {
		writeLP(h, []byte(fv.Kind))
		h.Write([]byte{byte(fv.Verdict)})
	}
	h.Write([]byte{byte(c.Aggregate)})
	writeLP(h, []byte(c.VetoedBy))
	return hex.EncodeToString(h.Sum(nil))
}

// AggregationPolicy configures the numeric quorum used when no critical veto
// has fired.
type AggregationPolicy struct {
	// ApproveThreshold is the minimum number of non-critical facets that
	// must return APPROVE for the aggregate to be APPROVE. Non-critical
	// facets that ABSTAIN do not count for or against.
	ApproveThreshold int
	// MinConfidence is the minimum confidence a facet must have for its
	// APPROVE to count. Facets below this threshold are treated as ABSTAIN
	// for the purpose of aggregation.
	MinConfidence float64
}

// DefaultAggregationPolicy is a conservative policy suitable for the demo:
// at least 2 non-critical facets must APPROVE with confidence ≥ 0.6.
var DefaultAggregationPolicy = AggregationPolicy{
	ApproveThreshold: 2,
	MinConfidence:    0.6,
}

// ErrNoFacetVerdicts is returned by Aggregate when the caller passed no
// facet verdicts at all — an obvious programmer error.
var ErrNoFacetVerdicts = errors.New("decision: no facet verdicts to aggregate")

// Aggregate runs critical-veto first, then quorum, and returns the outcome
// alongside a diagnostic reason string. It never panics on legal input.
func Aggregate(policy AggregationPolicy, verdicts []FacetVerdict) (Verdict, FacetKind, string, error) {
	if len(verdicts) == 0 {
		return VerdictUnknown, "", "", ErrNoFacetVerdicts
	}

	// Critical-veto pass. If any critical facet rejects, we reject.
	for _, fv := range verdicts {
		if fv.Kind.isCritical() && fv.Verdict == VerdictReject {
			return VerdictReject, fv.Kind, "critical-veto: " + fv.Reason, nil
		}
	}

	// Any critical facet that did not itself reject but also did not
	// approve (e.g. ABSTAIN because we lack evidence) forces the aggregate
	// to ABSTAIN — a critical dimension being unknown is not a green light.
	for _, fv := range verdicts {
		if fv.Kind.isCritical() && fv.Verdict != VerdictApprove {
			return VerdictAbstain, "",
				fmt.Sprintf("critical facet %s did not approve (%s)", fv.Kind, fv.Verdict), nil
		}
	}

	// Quorum pass over non-critical facets. APPROVE with insufficient
	// confidence is treated as ABSTAIN.
	approves := 0
	for _, fv := range verdicts {
		if fv.Kind.isCritical() {
			continue
		}
		if fv.Verdict == VerdictReject {
			return VerdictReject, "", "non-critical reject: " + fv.Reason, nil
		}
		if fv.Verdict == VerdictApprove && fv.Confidence >= policy.MinConfidence {
			approves++
		}
	}

	if approves >= policy.ApproveThreshold {
		return VerdictApprove, "", "", nil
	}
	return VerdictAbstain, "",
		fmt.Sprintf("quorum not reached: %d/%d approves at conf ≥ %.2f",
			approves, policy.ApproveThreshold, policy.MinConfidence), nil
}
