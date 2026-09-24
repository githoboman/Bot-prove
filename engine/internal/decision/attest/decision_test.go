package attest

import (
	"strings"
	"testing"
	"time"
)

// facet is a compact test helper.
func facet(k FacetKind, v Verdict, conf float64) FacetVerdict {
	return FacetVerdict{Kind: k, Verdict: v, Confidence: conf, Reason: string(k)}
}

func TestAggregate_HappyPath_Approve(t *testing.T) {
	policy := AggregationPolicy{ApproveThreshold: 2, MinConfidence: 0.6}
	verdicts := []FacetVerdict{
		facet(FacetSafety, VerdictApprove, 1.0),
		facet(FacetEquivocation, VerdictApprove, 1.0),
		facet(FacetCorrectness, VerdictApprove, 0.9),
		facet(FacetSpecCompliance, VerdictApprove, 0.8),
	}
	got, veto, reason, err := Aggregate(policy, verdicts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != VerdictApprove {
		t.Fatalf("expected APPROVE, got %s (reason=%q)", got, reason)
	}
	if veto != "" {
		t.Fatalf("expected no veto, got %s", veto)
	}
}

func TestAggregate_CriticalVeto_Safety(t *testing.T) {
	verdicts := []FacetVerdict{
		facet(FacetSafety, VerdictReject, 1.0),
		facet(FacetEquivocation, VerdictApprove, 1.0),
		facet(FacetCorrectness, VerdictApprove, 1.0),
		facet(FacetSpecCompliance, VerdictApprove, 1.0),
	}
	got, veto, reason, err := Aggregate(DefaultAggregationPolicy, verdicts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != VerdictReject {
		t.Fatalf("expected REJECT, got %s", got)
	}
	if veto != FacetSafety {
		t.Fatalf("expected veto=safety, got %s", veto)
	}
	if !strings.Contains(reason, "critical-veto") {
		t.Fatalf("reason should mention critical-veto: %q", reason)
	}
}

func TestAggregate_CriticalVeto_Equivocation(t *testing.T) {
	// Even if safety and both correctness facets approve, equivocation
	// REJECT must still veto.
	verdicts := []FacetVerdict{
		facet(FacetSafety, VerdictApprove, 1.0),
		facet(FacetEquivocation, VerdictReject, 1.0),
		facet(FacetCorrectness, VerdictApprove, 1.0),
		facet(FacetSpecCompliance, VerdictApprove, 1.0),
	}
	got, veto, _, _ := Aggregate(DefaultAggregationPolicy, verdicts)
	if got != VerdictReject {
		t.Fatalf("expected REJECT, got %s", got)
	}
	if veto != FacetEquivocation {
		t.Fatalf("expected veto=equivocation, got %s", veto)
	}
}

func TestAggregate_CriticalAbstain_ForcesAbstain(t *testing.T) {
	// If a critical facet ABSTAINs (not enough evidence), the aggregate
	// must ABSTAIN too — never APPROVE — even with two strong approves.
	verdicts := []FacetVerdict{
		facet(FacetSafety, VerdictAbstain, 0.5),
		facet(FacetEquivocation, VerdictApprove, 1.0),
		facet(FacetCorrectness, VerdictApprove, 1.0),
		facet(FacetSpecCompliance, VerdictApprove, 1.0),
	}
	got, veto, reason, err := Aggregate(DefaultAggregationPolicy, verdicts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != VerdictAbstain {
		t.Fatalf("expected ABSTAIN, got %s", got)
	}
	if veto != "" {
		t.Fatalf("expected no veto, got %s", veto)
	}
	if !strings.Contains(reason, "safety") {
		t.Fatalf("reason should name the failing critical facet: %q", reason)
	}
}

func TestAggregate_QuorumBelowThreshold_IsAbstain(t *testing.T) {
	policy := AggregationPolicy{ApproveThreshold: 2, MinConfidence: 0.6}
	verdicts := []FacetVerdict{
		facet(FacetSafety, VerdictApprove, 1.0),
		facet(FacetEquivocation, VerdictApprove, 1.0),
		facet(FacetCorrectness, VerdictApprove, 0.9),   // counts
		facet(FacetSpecCompliance, VerdictApprove, 0.3), // dropped: below MinConfidence
	}
	got, _, reason, _ := Aggregate(policy, verdicts)
	if got != VerdictAbstain {
		t.Fatalf("expected ABSTAIN (1 valid approve, threshold 2), got %s reason=%q", got, reason)
	}
	if !strings.Contains(reason, "quorum") {
		t.Fatalf("reason should mention quorum: %q", reason)
	}
}

func TestAggregate_NonCriticalReject_Rejects(t *testing.T) {
	verdicts := []FacetVerdict{
		facet(FacetSafety, VerdictApprove, 1.0),
		facet(FacetEquivocation, VerdictApprove, 1.0),
		facet(FacetCorrectness, VerdictReject, 0.9),
		facet(FacetSpecCompliance, VerdictApprove, 0.8),
	}
	got, _, reason, _ := Aggregate(DefaultAggregationPolicy, verdicts)
	if got != VerdictReject {
		t.Fatalf("expected REJECT for a non-critical reject, got %s (reason=%q)", got, reason)
	}
}

func TestAggregate_EmptyVerdicts_ReturnsError(t *testing.T) {
	_, _, _, err := Aggregate(DefaultAggregationPolicy, nil)
	if err != ErrNoFacetVerdicts {
		t.Fatalf("expected ErrNoFacetVerdicts, got %v", err)
	}
}

func TestDecisionID_Deterministic(t *testing.T) {
	d1 := Decision{
		Submitter: "0xanna",
		SpecID:    "policy/v1",
		Payload:   []byte("hello"),
		Nonce:     42,
	}
	d2 := d1
	if d1.ID() != d2.ID() {
		t.Fatalf("expected identical decisions to hash equal: %s vs %s", d1.ID(), d2.ID())
	}

	// Length-prefixed encoding: concatenation ambiguity must be
	// impossible. "aa" || "bb" must not equal "aab" || "b".
	a := Decision{Submitter: "aa", SpecID: "bb"}
	b := Decision{Submitter: "aab", SpecID: "b"}
	if a.ID() == b.ID() {
		t.Fatalf("length-prefix collision: %s == %s", a.ID(), b.ID())
	}

	// Nonce differences must produce different IDs.
	d3 := d1
	d3.Nonce++
	if d1.ID() == d3.ID() {
		t.Fatal("differing nonce produced identical ID")
	}
}

func TestCommitDigest_IsOrderIndependent(t *testing.T) {
	base := Decision{
		Submitter:   "0xanna",
		SpecID:      "policy/v1",
		Payload:     []byte("payload"),
		Nonce:       1,
		SubmittedAt: time.Unix(1_000_000, 0),
	}
	c1 := DecisionCommit{
		Decision:   base,
		DecisionID: base.ID(),
		FacetVerdicts: []FacetVerdict{
			facet(FacetSafety, VerdictApprove, 1.0),
			facet(FacetEquivocation, VerdictApprove, 1.0),
			facet(FacetCorrectness, VerdictApprove, 0.9),
			facet(FacetSpecCompliance, VerdictApprove, 0.8),
		},
		Aggregate: VerdictApprove,
	}
	c2 := c1
	c2.FacetVerdicts = []FacetVerdict{
		facet(FacetSpecCompliance, VerdictApprove, 0.8),
		facet(FacetCorrectness, VerdictApprove, 0.9),
		facet(FacetEquivocation, VerdictApprove, 1.0),
		facet(FacetSafety, VerdictApprove, 1.0),
	}
	if c1.CommitDigest() != c2.CommitDigest() {
		t.Fatalf("commit digest should be order-independent: %s vs %s",
			c1.CommitDigest(), c2.CommitDigest())
	}
}
