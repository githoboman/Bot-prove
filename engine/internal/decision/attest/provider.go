package attest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Provider is the pluggable evaluator that turns a Decision into a slice
// of FacetVerdicts. Implementations must be deterministic: two calls with
// the same Decision must return byte-identical verdicts (order of verdicts
// need not match — Judge sorts by kind for hashing). Non-determinism
// breaks reproducibility of the on-chain commitment.
//
// The interface is designed so that a real hosted evaluator (RPC to a
// signed judge service, threshold BLS quorum, on-chain oracle) can be
// dropped in without touching the aggregation, veto, or proof binding.
type Provider interface {
	// Name identifies the provider in receipts and logs.
	Name() string
	// Evaluate returns one FacetVerdict per FacetKind the provider
	// covers. Missing kinds are treated as ABSTAIN by the Judge.
	Evaluate(ctx context.Context, d Decision) ([]FacetVerdict, error)
}

// FixtureProvider is a deterministic provider that returns a canned set of
// verdicts per decision ID. It exists so that the demo, the reproducer
// script and CI can exercise APPROVE / ABSTAIN / REJECT paths without
// depending on any external service.
//
// The fixture is keyed on Decision.ID() so that the same submitter, spec
// and payload always produce the same verdicts across processes.
type FixtureProvider struct {
	mu       sync.RWMutex
	name     string
	verdicts map[string][]FacetVerdict
	// fallback is used when a decision ID is not in verdicts. It defaults
	// to a full ABSTAIN across all kinds so unknown inputs cannot silently
	// approve.
	fallback []FacetVerdict
}

// NewFixtureProvider returns an empty fixture provider named "fixture".
func NewFixtureProvider() *FixtureProvider {
	return NewNamedFixtureProvider("fixture")
}

// NewNamedFixtureProvider returns a fixture provider with a caller-chosen
// name (useful when running two fixtures side by side in tests).
func NewNamedFixtureProvider(name string) *FixtureProvider {
	fb := make([]FacetVerdict, 0, len(AllFacetKinds))
	for _, k := range AllFacetKinds {
		fb = append(fb, FacetVerdict{
			Kind:       k,
			Verdict:    VerdictAbstain,
			Confidence: 0.0,
			Reason:     "no fixture registered",
		})
	}
	return &FixtureProvider{
		name:     name,
		verdicts: make(map[string][]FacetVerdict),
		fallback: fb,
	}
}

// Name returns the provider name.
func (p *FixtureProvider) Name() string { return p.name }

// Register installs the fixture verdict slice under the given decision ID.
// It replaces any previous registration for the same ID. Passing an empty
// slice is legal and means "use fallback (all ABSTAIN)".
func (p *FixtureProvider) Register(decisionID string, verdicts []FacetVerdict) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(verdicts) == 0 {
		delete(p.verdicts, decisionID)
		return
	}
	cp := make([]FacetVerdict, len(verdicts))
	copy(cp, verdicts)
	p.verdicts[decisionID] = cp
}

// Evaluate implements Provider. It never returns an error and always
// returns a deterministic verdict set (the registered one if any, else
// the fallback all-ABSTAIN set).
func (p *FixtureProvider) Evaluate(_ context.Context, d Decision) ([]FacetVerdict, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.verdicts[d.ID()]; ok {
		out := make([]FacetVerdict, len(v))
		copy(out, v)
		return out, nil
	}
	out := make([]FacetVerdict, len(p.fallback))
	copy(out, p.fallback)
	return out, nil
}

// Judge orchestrates a single evaluation: it invokes the provider, fills
// in any missing facet kinds as ABSTAIN, runs Aggregate, and returns a
// fully populated DecisionCommit ready for proof binding.
type Judge struct {
	provider Provider
	policy   AggregationPolicy
}

// NewJudge constructs a Judge with the given provider and policy. A nil
// provider or an unset policy (ApproveThreshold == 0) will produce
// ErrInvalidJudge on Evaluate.
func NewJudge(p Provider, policy AggregationPolicy) *Judge {
	return &Judge{provider: p, policy: policy}
}

// ErrInvalidJudge is returned when Evaluate is called on a Judge that was
// constructed with a nil provider or a zero policy.
var ErrInvalidJudge = errors.New("decision: judge misconfigured (nil provider or zero policy)")

// Evaluate runs the provider, normalises the verdicts (ensures one entry
// per FacetKind, ABSTAIN for anything missing), and returns the final
// DecisionCommit.
func (j *Judge) Evaluate(ctx context.Context, d Decision) (DecisionCommit, error) {
	if j == nil || j.provider == nil || j.policy.ApproveThreshold == 0 {
		return DecisionCommit{}, ErrInvalidJudge
	}
	raw, err := j.provider.Evaluate(ctx, d)
	if err != nil {
		return DecisionCommit{}, fmt.Errorf("provider %s: %w", j.provider.Name(), err)
	}
	verdicts := normaliseVerdicts(raw)
	agg, veto, reason, err := Aggregate(j.policy, verdicts)
	if err != nil {
		return DecisionCommit{}, err
	}
	commit := DecisionCommit{
		Decision:      d,
		DecisionID:    d.ID(),
		FacetVerdicts: verdicts,
		Aggregate:     agg,
		VetoedBy:      veto,
	}
	if agg == VerdictAbstain {
		commit.AbstainReason = reason
	}
	return commit, nil
}

// normaliseVerdicts guarantees exactly one verdict per FacetKind in
// AllFacetKinds. If the provider omitted a kind it is inserted as
// ABSTAIN with confidence 0 and a machine-readable reason. Duplicates are
// resolved by keeping the LAST occurrence, which is convenient for the
// fixture provider (it lets a caller override a base fixture).
func normaliseVerdicts(in []FacetVerdict) []FacetVerdict {
	byKind := make(map[FacetKind]FacetVerdict, len(AllFacetKinds))
	for _, fv := range in {
		byKind[fv.Kind] = fv
	}
	out := make([]FacetVerdict, 0, len(AllFacetKinds))
	for _, k := range AllFacetKinds {
		if fv, ok := byKind[k]; ok {
			out = append(out, fv)
			continue
		}
		out = append(out, FacetVerdict{
			Kind:       k,
			Verdict:    VerdictAbstain,
			Confidence: 0.0,
			Reason:     "no verdict from provider",
		})
	}
	// Also preserve any provider-supplied verdicts whose Kind is outside
	// AllFacetKinds — dropping them silently would hide provider bugs.
	// Sort tail by kind for determinism.
	extras := extraVerdicts(byKind)
	sort.Slice(extras, func(i, j int) bool { return extras[i].Kind < extras[j].Kind })
	return append(out, extras...)
}

func extraVerdicts(byKind map[FacetKind]FacetVerdict) []FacetVerdict {
	known := make(map[FacetKind]struct{}, len(AllFacetKinds))
	for _, k := range AllFacetKinds {
		known[k] = struct{}{}
	}
	var out []FacetVerdict
	for k, v := range byKind {
		if _, ok := known[k]; ok {
			continue
		}
		out = append(out, v)
	}
	return out
}
