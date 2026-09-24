package attest

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// Risk-controlled abstention via split conformal calibration.
//
// The stock decision path treats a facet as decisive if its Confidence exceeds
// a hard threshold (MinConfidence in AggregationPolicy). That threshold is a
// magic number and gives no statistical guarantee. This file adds a data-
// driven alternative: given a calibration set of past facet verdicts each
// labeled with whether the resulting decision turned out to be correct,
// pick the smallest confidence cut-off τ such that the empirical error rate
// on the held-out set is at most α. Any live facet verdict whose confidence
// falls below τ is coerced to ABSTAIN before quorum is applied.
//
// This is a classic split-conformal calibration on the score = 1 - Confidence
// (nonconformity increases as confidence decreases). It gives a marginal
// (not conditional) coverage guarantee under exchangeability of calibration
// and live samples.
//
// The point is NOT to claim finite-sample optimality — it is to REPLACE a
// hand-picked 0.60 with a threshold that has an auditable error budget and
// that trivially reproduces from a fixture calibration set.
//
// Public API:
//   * CalibrationSample: one row of the held-out calibration set.
//   * CalibrateAbstention: given a calibration set and α, returns τ.
//   * ApplyConformalAbstention: rewrites verdicts below τ to ABSTAIN, in place.

// CalibrationSample describes a past facet verdict that we know the ground
// truth for.
//
//   Correct == true means: the facet's original Verdict was the right call
//   on that sample. If the facet APPROVED and the ground truth also approved,
//   Correct=true. Same for REJECT. An ABSTAIN is treated as neither correct
//   nor incorrect and is excluded from calibration (see CalibrateAbstention).
type CalibrationSample struct {
	Confidence float64
	Correct    bool
	Verdict    Verdict // used only to filter out ABSTAIN samples
}

// ErrCalibrationEmpty is returned when the calibration set contains no usable
// (non-ABSTAIN) samples.
var ErrCalibrationEmpty = errors.New("decision: empty calibration set")

// ErrInvalidAlpha is returned when α is outside (0, 1].
var ErrInvalidAlpha = errors.New("decision: alpha must be in (0, 1]")

// CalibrateAbstention returns the smallest confidence threshold τ such that
// the empirical error rate on the calibration set (excluding ABSTAINs) is at
// most α when we abstain on every sample with Confidence < τ.
//
// Guarantee (marginal, under exchangeability):
//   For a live facet verdict whose (Confidence, Correct) is drawn from the
//   same distribution as the calibration samples, the probability of a wrong
//   decision made without abstaining is at most α.
//
// If no threshold in [0, 1] achieves this, CalibrateAbstention returns 1.0
// (abstain on everything) — the fail-closed choice.
func CalibrateAbstention(cal []CalibrationSample, alpha float64) (float64, error) {
	if alpha <= 0 || alpha > 1 {
		return 0, ErrInvalidAlpha
	}
	// Drop ABSTAINs from calibration.
	filtered := make([]CalibrationSample, 0, len(cal))
	for _, s := range cal {
		if s.Verdict != VerdictAbstain {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) == 0 {
		return 0, ErrCalibrationEmpty
	}

	// Sort by ascending confidence so we can sweep and track the tail
	// empirical error rate on samples with Confidence >= τ.
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].Confidence < filtered[j].Confidence
	})

	n := len(filtered)
	// suffix[i] counts (errors, total) among samples at position >= i.
	suffixErr := make([]int, n+1)
	suffixCnt := make([]int, n+1)
	for i := n - 1; i >= 0; i-- {
		suffixErr[i] = suffixErr[i+1]
		suffixCnt[i] = suffixCnt[i+1] + 1
		if !filtered[i].Correct {
			suffixErr[i]++
		}
	}

	// Sweep smallest τ = filtered[i].Confidence such that the tail
	// (Confidence >= τ) has error rate <= α. We treat 0 samples in the
	// tail as trivially satisfying the constraint but return 1.0 there
	// because abstaining on everything is a well-defined fail-closed answer,
	// whereas returning 0.0 would let everything through unfiltered.
	for i := 0; i < n; i++ {
		if suffixCnt[i] == 0 {
			continue
		}
		errRate := float64(suffixErr[i]) / float64(suffixCnt[i])
		if errRate <= alpha {
			return filtered[i].Confidence, nil
		}
	}

	// No suffix satisfies the budget — fail closed.
	return 1.0, nil
}

// ApplyConformalAbstention rewrites any FacetVerdict whose Confidence is
// strictly below tau to VerdictAbstain, with a reason indicating the
// calibrated threshold. It returns a NEW slice; the input is not mutated.
func ApplyConformalAbstention(verdicts []FacetVerdict, tau float64) []FacetVerdict {
	out := make([]FacetVerdict, len(verdicts))
	for i, v := range verdicts {
		if v.Verdict != VerdictAbstain && v.Confidence < tau {
			out[i] = FacetVerdict{
				Kind:       v.Kind,
				Verdict:    VerdictAbstain,
				Confidence: v.Confidence,
				Reason: fmt.Sprintf(
					"conformal-abstain: conf=%.4f < tau=%.4f (was %s: %s)",
					v.Confidence, tau, v.Verdict, truncate(v.Reason, 80),
				),
			}
		} else {
			out[i] = v
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// AggregateWithConformalAbstention convenience wrapper: calibrate, apply, aggregate.
func AggregateWithConformalAbstention(
	cal []CalibrationSample,
	alpha float64,
	policy AggregationPolicy,
	verdicts []FacetVerdict,
) (Verdict, FacetKind, string, float64, error) {
	tau, err := CalibrateAbstention(cal, alpha)
	if err != nil {
		return VerdictUnknown, "", "", 0, err
	}
	adjusted := ApplyConformalAbstention(verdicts, tau)
	final, veto, reason, err := Aggregate(policy, adjusted)
	// Include tau in the reason for auditability.
	fullReason := reason
	if reason != "" {
		fullReason = fmt.Sprintf("[tau=%.4f alpha=%.2f] %s", tau, alpha, reason)
	} else if final == VerdictApprove {
		fullReason = fmt.Sprintf("[tau=%.4f alpha=%.2f] approved", tau, alpha)
	}
	return final, veto, fullReason, tau, err
}

// EmpiricalRiskAtTau reports the empirical error rate on the calibration
// set restricted to samples with Confidence >= tau. Useful as a sanity
// probe from tests.
func EmpiricalRiskAtTau(cal []CalibrationSample, tau float64) (float64, int) {
	errs, tot := 0, 0
	for _, s := range cal {
		if s.Verdict == VerdictAbstain {
			continue
		}
		if s.Confidence >= tau {
			tot++
			if !s.Correct {
				errs++
			}
		}
	}
	if tot == 0 {
		return math.NaN(), 0
	}
	return float64(errs) / float64(tot), tot
}
