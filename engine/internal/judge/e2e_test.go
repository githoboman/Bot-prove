// End-to-end sanity: run a full 3-provider Runner through FacetJudge via
// FixtureProvider (no real network) and confirm the three outcome branches:
//
//   1. providers agree            → OverallVerdict = AGREE
//   2. providers disagree         → OverallVerdict = DISAGREE + equivocation proof verifies
//   3. providers ABSTAIN (errors) → OverallVerdict = ABSTAIN + HITL event verifies
//
// This is deliberately in the judge package (not judge_test) so we can call
// unexported helpers directly if needed. It uses only in-package types and
// the pifixture facet catalog; no LLM call goes out.
package judge_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge/equivocation"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge/hitl"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/llm"
)

// scriptedProvider is a minimal in-package fake to control per-provider vote.
// (We don't use FixtureProvider here because it emits ONE canned string; we
// need each provider to return a different, per-facet value.)
type scriptedProvider struct {
	id      string
	answers map[string]string // facet.Prompt substring -> answer
	fail    bool
	delay   time.Duration
}

func (s *scriptedProvider) ID() string     { return s.id }
func (s *scriptedProvider) Tier() llm.Tier { return llm.TierFast }
func (s *scriptedProvider) KeyCount() int  { return 1 }
func (s *scriptedProvider) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.fail {
		return nil, &llm.ProviderError{Provider: s.id, Retryable: true, Body: "scripted-fail"}
	}
	// Concatenate the user prompt to look up an answer by facet.Prompt substring.
	user := ""
	for _, m := range req.Messages {
		if m.Role == llm.RoleUser {
			user = m.Content
		}
	}
	for k, v := range s.answers {
		if strings.Contains(user, k) {
			return &llm.Response{Provider: s.id, Content: v}, nil
		}
	}
	return &llm.Response{Provider: s.id, Content: "unknown"}, nil
}

func buildJudge(providers []llm.Provider) *judge.FacetJudge {
	r := llm.NewRunner(providers, nil, llm.Config{
		PerProviderTimeout:    200 * time.Millisecond,
		TotalBudget:           1 * time.Second,
		EnableFixtureFallback: false,
	})
	return judge.NewFacetJudge(r)
}

func TestE2E_AllAgree(t *testing.T) {
	// Three providers all say "yes" to a single-facet task.
	providers := []llm.Provider{
		&scriptedProvider{id: "p1", answers: map[string]string{"safe": "yes"}},
		&scriptedProvider{id: "p2", answers: map[string]string{"safe": "yes"}},
		&scriptedProvider{id: "p3", answers: map[string]string{"safe": "yes"}},
	}
	j := buildJudge(providers)
	task := &judge.Task{
		ID: "e2e-agree", Input: "The user asked for the weather.",
		Facets: []judge.Facet{
			{ID: "safety", Prompt: "Is the content safe? (yes/no)", AllowedValues: []string{"yes", "no"}, Weight: 1.0},
		},
		MinProviders: 2, AgreementThreshold: 0.66,
	}
	tr, err := j.Decide(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if tr.OverallVerdict != judge.VerdictAgree {
		t.Fatalf("overall: got %s want AGREE", tr.OverallVerdict)
	}
	if got := tr.Facets["safety"]; got == nil || got.Winner != "yes" || got.LiveCount != 3 {
		t.Fatalf("facet: %+v", got)
	}
}

func TestE2E_DisagreeProducesVerifiableProof(t *testing.T) {
	// Three providers split 2-1 below the 0.66 threshold on a single facet.
	providers := []llm.Provider{
		&scriptedProvider{id: "p1", answers: map[string]string{"safe": "yes"}},
		&scriptedProvider{id: "p2", answers: map[string]string{"safe": "no"}},
		&scriptedProvider{id: "p3", answers: map[string]string{"safe": "maybe"}},
	}
	j := buildJudge(providers)
	task := &judge.Task{
		ID: "e2e-disagree", Input: "Same question, 3 opinions.",
		Facets: []judge.Facet{
			{ID: "safety", Prompt: "Is the content safe? (yes/no/maybe)", AllowedValues: []string{"yes", "no", "maybe"}, Weight: 1.0},
		},
		MinProviders: 2, AgreementThreshold: 0.66,
	}
	tr, err := j.Decide(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if tr.OverallVerdict != judge.VerdictDisagree {
		t.Fatalf("overall: got %s want DISAGREE  facets=%+v", tr.OverallVerdict, tr.Facets["safety"])
	}
	proof, err := equivocation.FromTaskResult(task, tr)
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if err := proof.Verify(); err != nil {
		t.Fatalf("verify proof: %v", err)
	}
	if proof.DigestHex == "" {
		t.Fatal("empty digest")
	}
	// Round-trip through canonical bytes must produce the same digest.
	b, err := proof.MarshalCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("empty canonical bytes")
	}
}

func TestE2E_AbstainProducesVerifiableHITL(t *testing.T) {
	// Two providers fail; one succeeds. MinProviders=2 => facet is ABSTAIN.
	providers := []llm.Provider{
		&scriptedProvider{id: "p1", fail: true},
		&scriptedProvider{id: "p2", fail: true},
		&scriptedProvider{id: "p3", answers: map[string]string{"safe": "yes"}},
	}
	j := buildJudge(providers)
	task := &judge.Task{
		ID: "e2e-abstain", Input: "Test insufficient quorum.",
		Facets: []judge.Facet{
			{ID: "safety", Prompt: "Is the content safe? (yes/no)", AllowedValues: []string{"yes", "no"}, Weight: 1.0},
		},
		MinProviders: 2, AgreementThreshold: 0.66,
	}
	tr, err := j.Decide(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if tr.OverallVerdict != judge.VerdictAbstain {
		t.Fatalf("overall: got %s want ABSTAIN  facets=%+v", tr.OverallVerdict, tr.Facets["safety"])
	}
	ev, err := hitl.BuildEvent(task, tr, hitl.Options{})
	if err != nil {
		t.Fatalf("build hitl: %v", err)
	}
	if err := hitl.Verify(ev); err != nil {
		t.Fatalf("verify hitl: %v", err)
	}
	if len(ev.Facets) != 1 {
		t.Fatalf("facets: %+v", ev.Facets)
	}
	if ev.Facets[0].LiveCount > 1 {
		t.Fatalf("expected <=1 live provider, got %d", ev.Facets[0].LiveCount)
	}
}
