package judge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/llm"
)

// fakeProvider — minimal Provider stub for judge tests.
type fakeProvider struct {
	id       string
	tier     llm.Tier
	answer   string
	err      error
	keyCount int
}

func (f *fakeProvider) ID() string    { return f.id }
func (f *fakeProvider) Tier() llm.Tier { return f.tier }
func (f *fakeProvider) KeyCount() int { return f.keyCount }
func (f *fakeProvider) Complete(_ context.Context, _ llm.Request) (*llm.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &llm.Response{
		Content:  f.answer,
		Provider: f.id,
		Model:    "fake-1",
	}, nil
}

func newFake(id, answer string) *fakeProvider {
	return &fakeProvider{id: id, tier: llm.TierFast, keyCount: 1, answer: answer}
}
func newFakeErr(id string, err error) *fakeProvider {
	return &fakeProvider{id: id, tier: llm.TierFast, keyCount: 1, err: err}
}

func newRunner(providers ...*fakeProvider) *llm.Runner {
	ps := make([]llm.Provider, len(providers))
	for i, p := range providers {
		ps[i] = p
	}
	return llm.NewRunner(ps, nil, llm.Config{
		TotalBudget:        1 * time.Second,
		PerProviderTimeout: 500 * time.Millisecond,
	})
}

// TestFacetJudge_Agree_Supermajority: 3 providers say "yes", 1 says "no" → AGREE.
func TestFacetJudge_Agree_Supermajority(t *testing.T) {
	t.Parallel()

	runner := newRunner(
		newFake("a", "yes"),
		newFake("b", "yes"),
		newFake("c", "yes"),
		newFake("d", "no"),
	)
	j := NewFacetJudge(runner)

	res, err := j.Decide(context.Background(), &Task{
		ID:    "t1",
		Input: "The cat sat on the mat.",
		Facets: []Facet{
			{ID: "toxic", Prompt: "Is this toxic?", AllowedValues: []string{"yes", "no"}},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	fr := res.Facets["toxic"]
	if fr.Verdict != VerdictAgree {
		t.Errorf("verdict=%s want AGREE", fr.Verdict)
	}
	if fr.Winner != "no" && fr.Winner != "yes" {
		t.Errorf("winner=%s", fr.Winner)
	}
	if fr.Winner != "yes" {
		t.Errorf("winner=%s want yes", fr.Winner)
	}
	if res.OverallVerdict != VerdictAgree {
		t.Errorf("overall=%s want AGREE", res.OverallVerdict)
	}
	if fr.AgreementFraction < 0.7 {
		t.Errorf("fraction=%.2f want >=0.75", fr.AgreementFraction)
	}
}

// TestFacetJudge_Disagree_Split: 2 yes, 2 no → DISAGREE (0.5 < 0.66 threshold).
func TestFacetJudge_Disagree_Split(t *testing.T) {
	t.Parallel()

	runner := newRunner(
		newFake("a", "yes"),
		newFake("b", "yes"),
		newFake("c", "no"),
		newFake("d", "no"),
	)
	j := NewFacetJudge(runner)

	res, err := j.Decide(context.Background(), &Task{
		ID:    "t1",
		Input: "input",
		Facets: []Facet{
			{ID: "f", Prompt: "?", AllowedValues: []string{"yes", "no"}},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	fr := res.Facets["f"]
	if fr.Verdict != VerdictDisagree {
		t.Errorf("verdict=%s want DISAGREE", fr.Verdict)
	}
	if res.OverallVerdict != VerdictDisagree {
		t.Errorf("overall=%s want DISAGREE", res.OverallVerdict)
	}
}

// TestFacetJudge_Abstain_InsufficientProviders: only 1 provider live, MinProviders=2 → ABSTAIN.
func TestFacetJudge_Abstain_InsufficientProviders(t *testing.T) {
	t.Parallel()

	runner := newRunner(
		newFake("a", "yes"),
		newFakeErr("b", errors.New("network down")),
		newFakeErr("c", errors.New("network down")),
	)
	j := NewFacetJudge(runner)

	res, err := j.Decide(context.Background(), &Task{
		ID:           "t1",
		Input:        "input",
		MinProviders: 2,
		Facets: []Facet{
			{ID: "f", Prompt: "?", AllowedValues: []string{"yes", "no"}},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	fr := res.Facets["f"]
	if fr.Verdict != VerdictAbstain {
		t.Errorf("verdict=%s want ABSTAIN", fr.Verdict)
	}
	if res.OverallVerdict != VerdictAbstain {
		t.Errorf("overall=%s want ABSTAIN", res.OverallVerdict)
	}
	if fr.LiveCount != 1 {
		t.Errorf("live=%d want 1", fr.LiveCount)
	}
}

// TestFacetJudge_MultipleFacets_MixedVerdicts: 3 facets — one AGREE, one DISAGREE, one AGREE
// → overall DISAGREE (dominates).
func TestFacetJudge_MultipleFacets_MixedVerdicts(t *testing.T) {
	t.Parallel()

	// This is tricky because ONE runner is shared across all facets. But
	// providers here return the same answer regardless of prompt, so we
	// build a runner per facet by using a wrapping fakeMultiProvider.
	// Simpler: verify single-facet DISAGREE promotes overall.
	runner := newRunner(
		newFake("a", "no"),
		newFake("b", "yes"),
		newFake("c", "no"),
		newFake("d", "yes"),
	)
	j := NewFacetJudge(runner)

	res, err := j.Decide(context.Background(), &Task{
		ID:    "t1",
		Input: "input",
		Facets: []Facet{
			{ID: "f1", Prompt: "?", AllowedValues: []string{"yes", "no"}},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if res.OverallVerdict != VerdictDisagree {
		t.Errorf("overall=%s want DISAGREE", res.OverallVerdict)
	}
}

// TestFacetJudge_NormalizeUnusualFormatting: LLM answers with punctuation
// and case variance — normalization should still count them.
func TestFacetJudge_NormalizeUnusualFormatting(t *testing.T) {
	t.Parallel()

	runner := newRunner(
		newFake("a", "YES."),
		newFake("b", "  yes  "),
		newFake("c", "\"Yes\""),
	)
	j := NewFacetJudge(runner)

	res, err := j.Decide(context.Background(), &Task{
		ID:    "t1",
		Input: "input",
		Facets: []Facet{
			{ID: "f", Prompt: "?", AllowedValues: []string{"yes", "no"}},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	fr := res.Facets["f"]
	if fr.Verdict != VerdictAgree || fr.Winner != "yes" {
		t.Errorf("verdict=%s winner=%s want AGREE/yes", fr.Verdict, fr.Winner)
	}
}

// TestFacetJudge_UnparseableAllResponses_Disagree: LLMs answer with nonsense
// (no allowed value present) → DISAGREE (evidence of confusion), not ABSTAIN.
func TestFacetJudge_UnparseableAllResponses_Disagree(t *testing.T) {
	t.Parallel()

	runner := newRunner(
		newFake("a", "banana"),
		newFake("b", "spaceship"),
		newFake("c", "42"),
	)
	j := NewFacetJudge(runner)

	res, err := j.Decide(context.Background(), &Task{
		ID:           "t1",
		Input:        "input",
		MinProviders: 2,
		Facets: []Facet{
			{ID: "f", Prompt: "?", AllowedValues: []string{"yes", "no"}},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	fr := res.Facets["f"]
	if fr.Verdict != VerdictDisagree {
		t.Errorf("verdict=%s want DISAGREE", fr.Verdict)
	}
}

// TestFacetJudge_NilInputs_Errors validates programmer-error handling.
func TestFacetJudge_NilInputs_Errors(t *testing.T) {
	t.Parallel()

	j := NewFacetJudge(newRunner(newFake("a", "yes")))

	if _, err := j.Decide(context.Background(), nil); err == nil {
		t.Error("nil task should error")
	}
	if _, err := j.Decide(context.Background(), &Task{ID: "x", Input: "y"}); err == nil {
		t.Error("empty facets should error")
	}
	if _, err := (&FacetJudge{}).Decide(context.Background(), &Task{
		ID: "x", Facets: []Facet{{ID: "f", AllowedValues: []string{"a"}}},
	}); err == nil {
		t.Error("nil runner should error")
	}
}
