package attest

import (
	"errors"
	"fmt"
	"sort"
)

// Byzantine-robust facet aggregation.
//
// The baseline Aggregate() treats every FacetVerdict as coming from a single
// trusted evaluator per FacetKind. That is fine for the demo fixture provider
// but is not defensible under adversarial assumptions: a compromised evaluator
// could produce a single REJECT on a critical facet and flip the outcome
// against the honest majority (or, symmetrically, a single high-confidence
// APPROVE could rubber-stamp a bad decision if we only counted one voter per
// facet).
//
// AggregateByzantineRobust() consumes MULTIPLE independent verdicts per
// FacetKind and, per kind, reduces them to a single "canonical" verdict using
// a majority-with-abstention rule that tolerates up to f Byzantine voters as
// long as at least (2f+1) verdicts per kind are supplied. The canonical
// per-kind verdicts are then fed into the existing Aggregate() so critical-
// veto and quorum semantics stay unchanged.
//
// Rules for the per-kind reduction:
//   * If (2f+1) voters are not supplied for a kind, that kind's reduced
//     verdict is ABSTAIN with a "insufficient-redundancy" reason.
//   * Otherwise let R = count(REJECT), A = count(APPROVE), S = count(ABSTAIN),
//     N = R+A+S = total voters for that kind.
//     - If R >= (f+1): kind reduces to REJECT. This is the honest-quorum lower
//       bound: even if all f Byzantine voters flipped, at least one honest
//       voter still said REJECT, so REJECT reflects at least one honest signal.
//     - Else if A >= (N - f):  kind reduces to APPROVE with the MEDIAN
//       confidence of the APPROVE voters (median is order-statistic robust to
//       f outliers on either tail). All honest voters agreed on APPROVE.
//     - Else: kind reduces to ABSTAIN with a "no-honest-quorum" reason.
//
// This is a conservative, deliberately simple rule; the point is that a
// single malicious voter cannot flip the aggregate. The stronger property
// (median-of-medians) is left as a follow-up.

// ByzantinePolicy configures per-kind redundancy tolerance.
type ByzantinePolicy struct {
	// F is the maximum number of Byzantine voters PER FacetKind that the
	// aggregation must tolerate. Callers must supply at least (2F+1)
	// verdicts per kind or that kind reduces to ABSTAIN.
	F int
}

// DefaultByzantinePolicy tolerates one Byzantine voter per facet, i.e.
// callers must supply at least 3 verdicts per FacetKind.
var DefaultByzantinePolicy = ByzantinePolicy{F: 1}

// ErrEmptyByzantineInput is returned when zero verdicts are passed in.
var ErrEmptyByzantineInput = errors.New("decision: empty byzantine input")

// AggregateByzantineRobust performs per-FacetKind Byzantine reduction and
// then defers to Aggregate for the final approve / abstain / reject verdict.
// It returns the final verdict, the FacetKind that caused a critical veto
// (empty if none), a diagnostic reason string, the reduced per-kind verdicts
// it computed (useful for audit), and an error.
func AggregateByzantineRobust(
	bz ByzantinePolicy,
	policy AggregationPolicy,
	verdicts []FacetVerdict,
) (Verdict, FacetKind, string, []FacetVerdict, error) {
	if len(verdicts) == 0 {
		return VerdictUnknown, "", "", nil, ErrEmptyByzantineInput
	}
	if bz.F < 0 {
		return VerdictUnknown, "", "", nil,
			fmt.Errorf("decision: negative Byzantine bound F=%d", bz.F)
	}
	required := 2*bz.F + 1

	byKind := make(map[FacetKind][]FacetVerdict)
	kindOrder := make([]FacetKind, 0, 8) // preserve first-seen order for determinism
	for _, v := range verdicts {
		if _, ok := byKind[v.Kind]; !ok {
			kindOrder = append(kindOrder, v.Kind)
		}
		byKind[v.Kind] = append(byKind[v.Kind], v)
	}

	reduced := make([]FacetVerdict, 0, len(kindOrder))
	for _, kind := range kindOrder {
		voters := byKind[kind]
		reduced = append(reduced, reducePerKind(kind, voters, bz.F, required))
	}

	final, vetoKind, reason, err := Aggregate(policy, reduced)
	return final, vetoKind, reason, reduced, err
}

func reducePerKind(kind FacetKind, voters []FacetVerdict, f, required int) FacetVerdict {
	n := len(voters)
	if n < required {
		return FacetVerdict{
			Kind:       kind,
			Verdict:    VerdictAbstain,
			Confidence: 0,
			Reason: fmt.Sprintf(
				"insufficient-redundancy: %d voters, need %d (F=%d)",
				n, required, f,
			),
		}
	}

	var (
		rejects       []FacetVerdict
		approves      []FacetVerdict
		abstains      int
	)
	for _, v := range voters {
		switch v.Verdict {
		case VerdictReject:
			rejects = append(rejects, v)
		case VerdictApprove:
			approves = append(approves, v)
		default:
			abstains++
		}
	}

	// Rule 1: at least f+1 REJECTs => at least one honest REJECT.
	if len(rejects) >= f+1 {
		// Cite the majority REJECT reason for auditability. Pick the highest-
		// confidence reject; ties broken by lexicographic reason for
		// determinism.
		sort.SliceStable(rejects, func(i, j int) bool {
			if rejects[i].Confidence != rejects[j].Confidence {
				return rejects[i].Confidence > rejects[j].Confidence
			}
			return rejects[i].Reason < rejects[j].Reason
		})
		pick := rejects[0]
		return FacetVerdict{
			Kind:    kind,
			Verdict: VerdictReject,
			// Confidence is the median confidence across all reject voters:
			// robust to a single flipped voter at either extreme.
			Confidence: medianConfidence(rejects),
			Reason: fmt.Sprintf("byzantine-robust reject: %d/%d rejects (F=%d), citing %q",
				len(rejects), n, f, pick.Reason),
		}
	}

	// Rule 2: at least (N - f) APPROVEs => every honest voter approved.
	if len(approves) >= n-f {
		return FacetVerdict{
			Kind:       kind,
			Verdict:    VerdictApprove,
			Confidence: medianConfidence(approves),
			Reason: fmt.Sprintf("byzantine-robust approve: %d/%d approves (F=%d), median conf",
				len(approves), n, f),
		}
	}

	// Rule 3: neither honest-quorum reached => ABSTAIN.
	return FacetVerdict{
		Kind:       kind,
		Verdict:    VerdictAbstain,
		Confidence: 0,
		Reason: fmt.Sprintf(
			"no-honest-quorum: rejects=%d approves=%d abstains=%d (F=%d, N=%d)",
			len(rejects), len(approves), abstains, f, n,
		),
	}
}

// medianConfidence returns the median confidence across the given verdicts.
// Median is robust to up to f outliers on each side, which is what we want
// under a Byzantine model.
func medianConfidence(vs []FacetVerdict) float64 {
	if len(vs) == 0 {
		return 0
	}
	confs := make([]float64, len(vs))
	for i, v := range vs {
		confs[i] = v.Confidence
	}
	sort.Float64s(confs)
	mid := len(confs) / 2
	if len(confs)%2 == 1 {
		return confs[mid]
	}
	return (confs[mid-1] + confs[mid]) / 2
}
