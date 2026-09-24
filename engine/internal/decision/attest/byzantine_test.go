package attest

import (
	"testing"
)

// Helper to build a verdict fast.
func v(kind FacetKind, ver Verdict, conf float64, reason string) FacetVerdict {
	return FacetVerdict{Kind: kind, Verdict: ver, Confidence: conf, Reason: reason}
}

func TestByz_HonestSafetyMajorityRejects(t *testing.T) {
	// 3 voters on safety facet, 2 REJECT + 1 APPROVE (attacker).
	// F=1 -> require 3 voters, need >=2 rejects to reject. Passes.
	verdicts := []FacetVerdict{
		v(FacetSafety, VerdictReject, 0.9, "policy violation A"),
		v(FacetSafety, VerdictReject, 0.85, "policy violation B"),
		v(FacetSafety, VerdictApprove, 0.99, "attacker approve"),
	}
	got, veto, reason, _, err := AggregateByzantineRobust(
		DefaultByzantinePolicy, DefaultAggregationPolicy, verdicts,
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != VerdictReject {
		t.Fatalf("want REJECT, got %s (reason=%s)", got, reason)
	}
	if veto != FacetSafety {
		t.Fatalf("want vetoKind=safety, got %s", veto)
	}
}

func TestByz_SingleAttackerCannotFlipApprove(t *testing.T) {
	// Non-critical facets (compliance + finance) each with 3 voters.
	// Attacker rejects finance once, honest majority approves.
	// Also supply approving safety facet voters so critical facet quorum passes.
	verdicts := []FacetVerdict{
		// critical: safety — all approve
		v(FacetSafety, VerdictApprove, 0.9, "safe A"),
		v(FacetSafety, VerdictApprove, 0.9, "safe B"),
		v(FacetSafety, VerdictApprove, 0.9, "safe C"),
		// non-critical: compliance — 3 approves
		v(FacetCorrectness, VerdictApprove, 0.85, "kyc ok A"),
		v(FacetCorrectness, VerdictApprove, 0.85, "kyc ok B"),
		v(FacetCorrectness, VerdictApprove, 0.85, "kyc ok C"),
		// non-critical: finance — 2 approve, 1 malicious reject
		v(FacetSpecCompliance, VerdictApprove, 0.8, "budget ok A"),
		v(FacetSpecCompliance, VerdictApprove, 0.8, "budget ok B"),
		v(FacetSpecCompliance, VerdictReject, 0.99, "malicious reject"),
	}
	got, _, reason, reduced, err := AggregateByzantineRobust(
		DefaultByzantinePolicy, DefaultAggregationPolicy, verdicts,
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != VerdictApprove {
		t.Fatalf("want APPROVE despite malicious voter, got %s (reason=%s reduced=%+v)",
			got, reason, reduced)
	}
}

func TestByz_InsufficientRedundancyForcesAbstain(t *testing.T) {
	// Only 2 voters on safety with F=1 -> require 3.
	verdicts := []FacetVerdict{
		v(FacetSafety, VerdictApprove, 0.9, "safe A"),
		v(FacetSafety, VerdictApprove, 0.9, "safe B"),
		// give compliance/finance enough for their part but safety is short
		v(FacetCorrectness, VerdictApprove, 0.9, "c A"),
		v(FacetCorrectness, VerdictApprove, 0.9, "c B"),
		v(FacetCorrectness, VerdictApprove, 0.9, "c C"),
		v(FacetSpecCompliance, VerdictApprove, 0.9, "f A"),
		v(FacetSpecCompliance, VerdictApprove, 0.9, "f B"),
		v(FacetSpecCompliance, VerdictApprove, 0.9, "f C"),
	}
	got, _, reason, reduced, err := AggregateByzantineRobust(
		DefaultByzantinePolicy, DefaultAggregationPolicy, verdicts,
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != VerdictAbstain {
		t.Fatalf("want ABSTAIN (safety underspecified), got %s (reason=%s)", got, reason)
	}
	// The safety reduction must expose "insufficient-redundancy".
	foundSafety := false
	for _, r := range reduced {
		if r.Kind == FacetSafety {
			foundSafety = true
			if r.Verdict != VerdictAbstain {
				t.Fatalf("safety not reduced to ABSTAIN: %+v", r)
			}
			if !contains(r.Reason, "insufficient-redundancy") {
				t.Fatalf("expected insufficient-redundancy reason, got %q", r.Reason)
			}
		}
	}
	if !foundSafety {
		t.Fatal("no safety verdict in reduced set")
	}
}

func TestByz_MedianConfidenceIgnoresOutlier(t *testing.T) {
	// 3 approves for a non-critical facet with confidences 0.7, 0.7, 0.99.
	// Median is 0.7 (not 0.79 mean).
	verdicts := []FacetVerdict{
		// give critical safety a clean approving quorum
		v(FacetSafety, VerdictApprove, 0.9, ""),
		v(FacetSafety, VerdictApprove, 0.9, ""),
		v(FacetSafety, VerdictApprove, 0.9, ""),
		v(FacetCorrectness, VerdictApprove, 0.7, "a"),
		v(FacetCorrectness, VerdictApprove, 0.7, "b"),
		v(FacetCorrectness, VerdictApprove, 0.99, "outlier"), // adversarial high conf
		v(FacetSpecCompliance, VerdictApprove, 0.9, "c"),
		v(FacetSpecCompliance, VerdictApprove, 0.9, "d"),
		v(FacetSpecCompliance, VerdictApprove, 0.9, "e"),
	}
	_, _, _, reduced, err := AggregateByzantineRobust(
		DefaultByzantinePolicy, DefaultAggregationPolicy, verdicts,
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, r := range reduced {
		if r.Kind == FacetCorrectness {
			if r.Confidence < 0.69 || r.Confidence > 0.71 {
				t.Fatalf("compliance reduced confidence should be median ~0.70, got %.3f", r.Confidence)
			}
		}
	}
}

func TestByz_NoHonestQuorumForcesAbstain(t *testing.T) {
	// safety: 3 voters split 1 reject + 1 approve + 1 abstain. F=1 needs
	// either >=2 rejects (no) or >=(3-1)=2 approves (no). => ABSTAIN.
	verdicts := []FacetVerdict{
		v(FacetSafety, VerdictReject, 0.9, "r"),
		v(FacetSafety, VerdictApprove, 0.9, "a"),
		v(FacetSafety, VerdictAbstain, 0.0, "s"),
		v(FacetCorrectness, VerdictApprove, 0.9, ""),
		v(FacetCorrectness, VerdictApprove, 0.9, ""),
		v(FacetCorrectness, VerdictApprove, 0.9, ""),
		v(FacetSpecCompliance, VerdictApprove, 0.9, ""),
		v(FacetSpecCompliance, VerdictApprove, 0.9, ""),
		v(FacetSpecCompliance, VerdictApprove, 0.9, ""),
	}
	got, _, reason, _, err := AggregateByzantineRobust(
		DefaultByzantinePolicy, DefaultAggregationPolicy, verdicts,
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != VerdictAbstain {
		t.Fatalf("want ABSTAIN, got %s (reason=%s)", got, reason)
	}
}

func TestByz_FGreaterThanOneToleratesMoreAttackers(t *testing.T) {
	// F=2 -> require 5 voters per kind. 3 attackers reject safety with high
	// confidence, 2 honest approve. len(rejects)=3 >= F+1=3 => REJECT (matches
	// spec since even 2 honest voters could have been the flipped attackers).
	// This is intentional: to survive 2 attackers you'd need 2F+1 = 5 honest
	// voters, and safety only has 2 honest, so we correctly fail closed.
	verdicts := []FacetVerdict{
		v(FacetSafety, VerdictReject, 0.9, "r1"),
		v(FacetSafety, VerdictReject, 0.9, "r2"),
		v(FacetSafety, VerdictReject, 0.9, "r3"),
		v(FacetSafety, VerdictApprove, 0.9, "a1"),
		v(FacetSafety, VerdictApprove, 0.9, "a2"),
	}
	pol := ByzantinePolicy{F: 2}
	got, _, _, _, err := AggregateByzantineRobust(pol, DefaultAggregationPolicy, verdicts)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != VerdictReject {
		t.Fatalf("want REJECT with F=2 and 3/5 rejects, got %s", got)
	}
}

func TestByz_EmptyInputErrors(t *testing.T) {
	_, _, _, _, err := AggregateByzantineRobust(DefaultByzantinePolicy, DefaultAggregationPolicy, nil)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestByz_NegativeFErrors(t *testing.T) {
	_, _, _, _, err := AggregateByzantineRobust(
		ByzantinePolicy{F: -1}, DefaultAggregationPolicy,
		[]FacetVerdict{v(FacetSafety, VerdictApprove, 0.9, "")},
	)
	if err == nil {
		t.Fatal("expected error for negative F")
	}
}

// contains is a tiny substring helper to avoid an import.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
