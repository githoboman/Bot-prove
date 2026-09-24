package attest

import (
	"math"
	"math/rand"
	"testing"
)

// Small calibration set with an obvious high-confidence cluster of correct
// predictions and a low-confidence cluster of wrong predictions. The
// calibrated tau must land in the middle.
func TestConformal_CalibrateSeparableCluster(t *testing.T) {
	cal := []CalibrationSample{
		// low-confidence, mostly wrong
		{Confidence: 0.1, Correct: false, Verdict: VerdictApprove},
		{Confidence: 0.2, Correct: false, Verdict: VerdictApprove},
		{Confidence: 0.3, Correct: false, Verdict: VerdictApprove},
		{Confidence: 0.4, Correct: true, Verdict: VerdictApprove},
		// high-confidence, mostly right
		{Confidence: 0.7, Correct: true, Verdict: VerdictApprove},
		{Confidence: 0.8, Correct: true, Verdict: VerdictApprove},
		{Confidence: 0.9, Correct: true, Verdict: VerdictApprove},
		{Confidence: 0.95, Correct: true, Verdict: VerdictApprove},
	}
	tau, err := CalibrateAbstention(cal, 0.05)
	if err != nil {
		t.Fatalf("calibrate: %v", err)
	}
	if tau < 0.4 {
		t.Fatalf("tau should reject low-confidence errors, got %.3f", tau)
	}
	if tau > 0.7 {
		t.Fatalf("tau should not exclude honest high-confidence cluster, got %.3f", tau)
	}
	// Verify the empirical error above tau really is <= alpha.
	rate, n := EmpiricalRiskAtTau(cal, tau)
	if n == 0 {
		t.Fatal("expected at least one sample above tau")
	}
	if rate > 0.05 {
		t.Fatalf("empirical error %.3f above tau=%.3f exceeds alpha=0.05", rate, tau)
	}
}

// Test the fail-closed branch: an all-wrong calibration set for a very tight
// alpha means no tau can satisfy the budget. Must return 1.0 and abstain
// on live data.
func TestConformal_FailClosedNoValidTau(t *testing.T) {
	cal := []CalibrationSample{
		{Confidence: 0.5, Correct: false, Verdict: VerdictApprove},
		{Confidence: 0.9, Correct: false, Verdict: VerdictApprove},
	}
	tau, err := CalibrateAbstention(cal, 0.01)
	if err != nil {
		t.Fatalf("calibrate: %v", err)
	}
	if tau != 1.0 {
		t.Fatalf("expected fail-closed tau=1.0, got %.3f", tau)
	}
	// Live verdicts below 1.0 all get coerced to ABSTAIN.
	live := []FacetVerdict{
		{Kind: FacetCorrectness, Verdict: VerdictApprove, Confidence: 0.99, Reason: "x"},
	}
	got := ApplyConformalAbstention(live, tau)
	if got[0].Verdict != VerdictAbstain {
		t.Fatalf("expected ABSTAIN under tau=1.0, got %s", got[0].Verdict)
	}
}

// Feed a random population with a known error/confidence relationship and
// confirm that the empirical risk after calibration on a HELD-OUT sample
// respects alpha with high probability.
func TestPBT_Conformal_ExchangeableGuarantee(t *testing.T) {
	seed := int64(20260727)
	rng := rand.New(rand.NewSource(seed))
	alpha := 0.10

	// Ground-truth data-generating process: probability of correct scales
	// with confidence; below 0.5 confidence, mostly wrong.
	makeSample := func() CalibrationSample {
		c := rng.Float64()
		correctProb := c
		if c < 0.5 {
			correctProb = c * 0.4
		}
		return CalibrationSample{
			Confidence: c,
			Correct:    rng.Float64() < correctProb,
			Verdict:    VerdictApprove,
		}
	}

	// 500 calibration + 500 held-out. Trials: 10.
	trials := 10
	respected := 0
	for tr := 0; tr < trials; tr++ {
		cal := make([]CalibrationSample, 500)
		for i := range cal {
			cal[i] = makeSample()
		}
		tau, err := CalibrateAbstention(cal, alpha)
		if err != nil {
			t.Fatalf("calibrate: %v", err)
		}

		held := make([]CalibrationSample, 500)
		for i := range held {
			held[i] = makeSample()
		}
		rate, n := EmpiricalRiskAtTau(held, tau)
		if n == 0 || math.IsNaN(rate) {
			continue // trial with no admitted samples, skip
		}
		// Marginal conformal coverage guarantee allows some slack.
		// Accept rate <= alpha + 0.05 (calibration is only 500 samples,
		// so a small deviation is expected).
		if rate <= alpha+0.05 {
			respected++
		}
	}
	if respected < 8 { // at least 8/10 trials respect the budget
		t.Fatalf("only %d/%d trials respected alpha; calibration guarantee weak", respected, trials)
	}
}

func TestConformal_ApplyRespectsExistingAbstain(t *testing.T) {
	live := []FacetVerdict{
		{Kind: FacetSafety, Verdict: VerdictAbstain, Confidence: 0.0, Reason: "no evidence"},
		{Kind: FacetCorrectness, Verdict: VerdictApprove, Confidence: 0.4, Reason: "weak"},
		{Kind: FacetSpecCompliance, Verdict: VerdictApprove, Confidence: 0.9, Reason: "ok"},
	}
	got := ApplyConformalAbstention(live, 0.7)
	if got[0].Verdict != VerdictAbstain {
		t.Fatal("existing ABSTAIN was disturbed")
	}
	if got[0].Reason != "no evidence" {
		t.Fatalf("existing ABSTAIN reason mutated: %q", got[0].Reason)
	}
	if got[1].Verdict != VerdictAbstain {
		t.Fatal("low-confidence APPROVE not coerced to ABSTAIN")
	}
	if got[2].Verdict != VerdictApprove {
		t.Fatal("high-confidence APPROVE incorrectly coerced")
	}
}

func TestConformal_InvalidAlpha(t *testing.T) {
	_, err := CalibrateAbstention([]CalibrationSample{{Confidence: 0.5, Verdict: VerdictApprove, Correct: true}}, 0.0)
	if err == nil {
		t.Fatal("expected error for alpha=0")
	}
	_, err = CalibrateAbstention([]CalibrationSample{{Confidence: 0.5, Verdict: VerdictApprove, Correct: true}}, 1.5)
	if err == nil {
		t.Fatal("expected error for alpha>1")
	}
}

func TestConformal_EmptyCalibration(t *testing.T) {
	_, err := CalibrateAbstention(nil, 0.1)
	if err == nil {
		t.Fatal("expected error for empty cal")
	}
	// A calibration set of only ABSTAINs is also empty for our purposes.
	_, err = CalibrateAbstention([]CalibrationSample{
		{Confidence: 0.9, Correct: true, Verdict: VerdictAbstain},
	}, 0.1)
	if err == nil {
		t.Fatal("expected error when cal has only ABSTAIN samples")
	}
}

func TestConformal_AggregateWrapperEndToEnd(t *testing.T) {
	// Calibration says confidence < 0.7 is unreliable at alpha=0.05.
	cal := []CalibrationSample{
		{Confidence: 0.3, Correct: false, Verdict: VerdictApprove},
		{Confidence: 0.4, Correct: false, Verdict: VerdictApprove},
		{Confidence: 0.5, Correct: false, Verdict: VerdictApprove},
		{Confidence: 0.6, Correct: false, Verdict: VerdictApprove},
		{Confidence: 0.7, Correct: true, Verdict: VerdictApprove},
		{Confidence: 0.8, Correct: true, Verdict: VerdictApprove},
		{Confidence: 0.9, Correct: true, Verdict: VerdictApprove},
	}
	// Live decision: safety high confidence, other facets low confidence.
	// Non-critical facets get coerced to ABSTAIN, quorum fails -> ABSTAIN.
	live := []FacetVerdict{
		{Kind: FacetSafety, Verdict: VerdictApprove, Confidence: 0.9, Reason: "ok"},
		{Kind: FacetCorrectness, Verdict: VerdictApprove, Confidence: 0.5, Reason: "weak"},
		{Kind: FacetSpecCompliance, Verdict: VerdictApprove, Confidence: 0.55, Reason: "weak"},
	}
	got, _, reason, tau, err := AggregateWithConformalAbstention(
		cal, 0.05, DefaultAggregationPolicy, live,
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if tau < 0.65 || tau > 0.75 {
		t.Fatalf("unexpected tau=%.3f", tau)
	}
	if got != VerdictAbstain {
		t.Fatalf("expected ABSTAIN under conformal coercion, got %s (reason=%s)", got, reason)
	}
}
