package attest

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// singleFacetProvider is a deterministic test provider that covers
// exactly one facet with a fixed verdict.
type singleFacetProvider struct {
	name    string
	kind    FacetKind
	verdict Verdict
	conf    float64
	err     error
}

func (p *singleFacetProvider) Name() string { return p.name }
func (p *singleFacetProvider) Evaluate(_ context.Context, _ Decision) ([]FacetVerdict, error) {
	if p.err != nil {
		return nil, p.err
	}
	return []FacetVerdict{{Kind: p.kind, Verdict: p.verdict, Confidence: p.conf, Reason: p.name}}, nil
}

func TestProviderPoolRegisterRejectsNilAndDuplicates(t *testing.T) {
	pool := NewProviderPool()
	if err := pool.Register(PooledProvider{}); err == nil {
		t.Fatalf("expected error for nil provider")
	}
	p := &singleFacetProvider{name: "safety-1", kind: FacetSafety, verdict: VerdictApprove, conf: 0.9}
	if err := pool.Register(PooledProvider{Provider: p, Trust: TrustSystem, Capabilities: []Capability{CapSafety}}); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	// Duplicate name rejected.
	dup := &singleFacetProvider{name: "safety-1", kind: FacetCorrectness, verdict: VerdictApprove, conf: 0.5}
	if err := pool.Register(PooledProvider{Provider: dup, Trust: TrustSystem, Capabilities: []Capability{CapCorrectness}}); err == nil {
		t.Fatalf("expected duplicate-name rejection")
	}
	// No capabilities rejected.
	if err := pool.Register(PooledProvider{Provider: &singleFacetProvider{name: "x", kind: FacetSafety}, Trust: TrustSystem}); err == nil {
		t.Fatalf("expected no-capability rejection")
	}
	if pool.Len() != 1 {
		t.Fatalf("expected pool len 1, got %d", pool.Len())
	}
}

func TestRouterEmptyPool(t *testing.T) {
	r := NewRouter(NewProviderPool())
	if _, err := r.Route(context.Background(), Decision{}, nil); !errors.Is(err, ErrNoProviders) {
		t.Fatalf("expected ErrNoProviders, got %v", err)
	}
}

func TestRouterRoutesByCapability(t *testing.T) {
	pool := NewProviderPool()
	// Safety-only, correctness-only, and unrelated providers.
	safety := &singleFacetProvider{name: "safety", kind: FacetSafety, verdict: VerdictApprove, conf: 0.9}
	corr := &singleFacetProvider{name: "correctness", kind: FacetCorrectness, verdict: VerdictApprove, conf: 0.9}
	spec := &singleFacetProvider{name: "spec", kind: FacetSpecCompliance, verdict: VerdictApprove, conf: 0.9}
	must(t, pool.Register(PooledProvider{Provider: safety, Trust: TrustSystem, Capabilities: []Capability{CapSafety}}))
	must(t, pool.Register(PooledProvider{Provider: corr, Trust: TrustSystem, Capabilities: []Capability{CapCorrectness}}))
	must(t, pool.Register(PooledProvider{Provider: spec, Trust: TrustDelegated, Capabilities: []Capability{CapSpecCompliance}}))

	r := NewRouter(pool)
	out, err := r.Route(context.Background(), Decision{Submitter: "s", SpecID: "spec", Nonce: 1}, []FacetKind{FacetSafety, FacetCorrectness})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	// Only safety and correctness providers should be consulted.
	if len(out.Providers) != 2 {
		t.Fatalf("expected 2 providers routed, got %d: %v", len(out.Providers), out.Providers)
	}
	if out.Providers[0] != "correctness" || out.Providers[1] != "safety" {
		t.Fatalf("expected sorted [correctness, safety], got %v", out.Providers)
	}
	if len(out.Verdicts) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(out.Verdicts))
	}
}

func TestRouterDowngradesObservationalCriticalVote(t *testing.T) {
	pool := NewProviderPool()
	// Observational provider tries to APPROVE the safety facet — must be downgraded.
	obs := &singleFacetProvider{name: "obs", kind: FacetSafety, verdict: VerdictApprove, conf: 0.99}
	must(t, pool.Register(PooledProvider{Provider: obs, Trust: TrustObservational, Capabilities: []Capability{CapSafety}}))

	r := NewRouter(pool)
	out, err := r.Route(context.Background(), Decision{Submitter: "s", SpecID: "sp", Nonce: 1}, nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(out.Verdicts) != 1 {
		t.Fatalf("expected 1 verdict, got %d", len(out.Verdicts))
	}
	if out.Verdicts[0].Verdict != VerdictAbstain {
		t.Fatalf("expected observational critical vote to be downgraded to ABSTAIN, got %v", out.Verdicts[0].Verdict)
	}
	if !strings.Contains(out.Verdicts[0].Reason, "trust-downgrade") {
		t.Fatalf("expected trust-downgrade reason, got %q", out.Verdicts[0].Reason)
	}
}

func TestRouterErrorFromProviderIsIsolated(t *testing.T) {
	pool := NewProviderPool()
	good := &singleFacetProvider{name: "good", kind: FacetCorrectness, verdict: VerdictApprove, conf: 0.9}
	bad := &singleFacetProvider{name: "bad", kind: FacetCorrectness, err: errors.New("boom")}
	must(t, pool.Register(PooledProvider{Provider: good, Trust: TrustSystem, Capabilities: []Capability{CapCorrectness}}))
	must(t, pool.Register(PooledProvider{Provider: bad, Trust: TrustSystem, Capabilities: []Capability{CapCorrectness}}))

	r := NewRouter(pool)
	out, err := r.Route(context.Background(), Decision{Submitter: "s", SpecID: "s", Nonce: 1}, []FacetKind{FacetCorrectness})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(out.Providers) != 1 || out.Providers[0] != "good" {
		t.Fatalf("expected only good provider surfaced, got %v", out.Providers)
	}
	if _, ok := out.ErrorsByProvider["bad"]; !ok {
		t.Fatalf("expected bad provider error to appear in ErrorsByProvider, got %v", out.ErrorsByProvider)
	}
	if len(out.Verdicts) != 1 {
		t.Fatalf("expected 1 verdict, got %d", len(out.Verdicts))
	}
}

func TestRouterNoRoutedProviders(t *testing.T) {
	pool := NewProviderPool()
	safety := &singleFacetProvider{name: "safety", kind: FacetSafety, verdict: VerdictApprove, conf: 0.9}
	must(t, pool.Register(PooledProvider{Provider: safety, Trust: TrustSystem, Capabilities: []Capability{CapSafety}}))
	r := NewRouter(pool)
	if _, err := r.Route(context.Background(), Decision{}, []FacetKind{FacetCorrectness}); !errors.Is(err, ErrNoRoutedProviders) {
		t.Fatalf("expected ErrNoRoutedProviders, got %v", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
