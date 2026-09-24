package attest

import (
	"context"
	"errors"
	"testing"
)

func mustDecision(t *testing.T, submitter, spec, payload string, nonce uint64) Decision {
	t.Helper()
	return Decision{
		Submitter: submitter,
		SpecID:    spec,
		Payload:   []byte(payload),
		Nonce:     nonce,
	}
}

func TestFixtureProvider_Deterministic(t *testing.T) {
	p := NewFixtureProvider()
	d := mustDecision(t, "0xanna", "policy/v1", "approve me", 1)
	p.Register(d.ID(), []FacetVerdict{
		facet(FacetSafety, VerdictApprove, 1.0),
		facet(FacetEquivocation, VerdictApprove, 1.0),
		facet(FacetCorrectness, VerdictApprove, 0.9),
		facet(FacetSpecCompliance, VerdictApprove, 0.9),
	})

	ctx := context.Background()
	v1, err := p.Evaluate(ctx, d)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	v2, err := p.Evaluate(ctx, d)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(v1) != len(v2) {
		t.Fatalf("verdict count changed between calls")
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("verdict %d changed: %+v vs %+v", i, v1[i], v2[i])
		}
	}
}

func TestFixtureProvider_UnknownFallsBackToAbstain(t *testing.T) {
	p := NewFixtureProvider()
	d := mustDecision(t, "0xanna", "policy/v1", "unregistered", 7)
	v, err := p.Evaluate(context.Background(), d)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(v) != len(AllFacetKinds) {
		t.Fatalf("fallback should cover all kinds, got %d", len(v))
	}
	for _, fv := range v {
		if fv.Verdict != VerdictAbstain {
			t.Fatalf("fallback verdict must be ABSTAIN, got %s for %s", fv.Verdict, fv.Kind)
		}
	}
}

func TestJudge_ApprovePath(t *testing.T) {
	p := NewFixtureProvider()
	d := mustDecision(t, "0xanna", "policy/v1", "increase gate limit to 100", 1)
	p.Register(d.ID(), []FacetVerdict{
		facet(FacetSafety, VerdictApprove, 1.0),
		facet(FacetEquivocation, VerdictApprove, 1.0),
		facet(FacetCorrectness, VerdictApprove, 0.9),
		facet(FacetSpecCompliance, VerdictApprove, 0.9),
	})

	j := NewJudge(p, DefaultAggregationPolicy)
	commit, err := j.Evaluate(context.Background(), d)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if commit.Aggregate != VerdictApprove {
		t.Fatalf("expected APPROVE, got %s (reason=%q)", commit.Aggregate, commit.AbstainReason)
	}
	if commit.CommitDigest() == "" {
		t.Fatal("commit digest is empty")
	}
}

func TestJudge_ProviderMissingKind_IsAbstainedIn(t *testing.T) {
	// Provider returns only 2 facets. Judge must fill safety+equivocation
	// as ABSTAIN, and because a critical facet did not approve the
	// aggregate must be ABSTAIN, not APPROVE.
	p := NewFixtureProvider()
	d := mustDecision(t, "0xanna", "policy/v1", "partial verdicts", 2)
	p.Register(d.ID(), []FacetVerdict{
		facet(FacetCorrectness, VerdictApprove, 0.9),
		facet(FacetSpecCompliance, VerdictApprove, 0.9),
	})

	j := NewJudge(p, DefaultAggregationPolicy)
	commit, err := j.Evaluate(context.Background(), d)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if commit.Aggregate != VerdictAbstain {
		t.Fatalf("expected ABSTAIN, got %s", commit.Aggregate)
	}
	// Must have exactly len(AllFacetKinds) verdicts, no more, no less.
	if len(commit.FacetVerdicts) != len(AllFacetKinds) {
		t.Fatalf("normalised verdicts should have %d entries, got %d",
			len(AllFacetKinds), len(commit.FacetVerdicts))
	}
}

func TestJudge_MisconfiguredReturnsError(t *testing.T) {
	// Nil provider.
	j := NewJudge(nil, DefaultAggregationPolicy)
	_, err := j.Evaluate(context.Background(), Decision{})
	if !errors.Is(err, ErrInvalidJudge) {
		t.Fatalf("expected ErrInvalidJudge with nil provider, got %v", err)
	}

	// Zero policy.
	j2 := NewJudge(NewFixtureProvider(), AggregationPolicy{})
	_, err = j2.Evaluate(context.Background(), Decision{Submitter: "x"})
	if !errors.Is(err, ErrInvalidJudge) {
		t.Fatalf("expected ErrInvalidJudge with zero policy, got %v", err)
	}
}

type errProvider struct{}

func (errProvider) Name() string { return "err" }
func (errProvider) Evaluate(context.Context, Decision) ([]FacetVerdict, error) {
	return nil, errors.New("boom")
}

func TestJudge_PropagatesProviderError(t *testing.T) {
	j := NewJudge(errProvider{}, DefaultAggregationPolicy)
	_, err := j.Evaluate(context.Background(), Decision{Submitter: "x"})
	if err == nil {
		t.Fatal("expected error from provider to propagate")
	}
}
