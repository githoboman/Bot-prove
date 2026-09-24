package equivocation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge/equivocation"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge/pifixture"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/llm"
)

// runDisagreeCase spins the pifixture "system-override" case through the
// pipeline and returns a real judge.TaskResult for the proof to consume.
func runDisagreeCase(t *testing.T, name string) (*judge.Task, *judge.TaskResult) {
	t.Helper()
	var c pifixture.Case
	for _, cc := range pifixture.Corpus() {
		if cc.Name == name {
			c = cc
			break
		}
	}
	if c.Task == nil {
		t.Fatalf("case %q not in corpus", name)
	}
	fixtures := pifixture.SeederFor(c)
	provs := make([]llm.Provider, 0, len(fixtures))
	for _, f := range fixtures {
		provs = append(provs, f)
	}
	runner := llm.NewRunner(provs, nil, llm.Config{
		TotalBudget:        2 * time.Second,
		PerProviderTimeout: 500 * time.Millisecond,
	})
	j := judge.NewFacetJudge(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := j.Decide(ctx, c.Task)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	return c.Task, res
}

func TestFromTaskResult_DisagreeCase(t *testing.T) {
	task, res := runDisagreeCase(t, "system-override-toxic")
	if res.OverallVerdict != judge.VerdictDisagree {
		t.Fatalf("precondition: expected DISAGREE, got %s", res.OverallVerdict)
	}
	proof, err := equivocation.FromTaskResult(task, res)
	if err != nil {
		t.Fatalf("FromTaskResult: %v", err)
	}
	if proof.OverallVerdict != string(judge.VerdictDisagree) {
		t.Errorf("proof.OverallVerdict = %s, want DISAGREE", proof.OverallVerdict)
	}
	if proof.Version != 1 {
		t.Errorf("proof.Version = %d, want 1", proof.Version)
	}
	if proof.TaskID != task.ID {
		t.Errorf("proof.TaskID = %s, want %s", proof.TaskID, task.ID)
	}
	if len(proof.Facets) != len(res.Facets) {
		t.Errorf("proof.Facets = %d entries, want %d", len(proof.Facets), len(res.Facets))
	}
	if proof.DigestHex == "" {
		t.Errorf("proof.DigestHex is empty")
	}
	if len(proof.DigestHex) != 64 {
		t.Errorf("proof.DigestHex length = %d, want 64 hex chars", len(proof.DigestHex))
	}
}

func TestFromTaskResult_RejectsAgree(t *testing.T) {
	task, res := runDisagreeCase(t, "clean-content-consensus")
	if res.OverallVerdict != judge.VerdictAgree {
		t.Fatalf("precondition: expected AGREE, got %s", res.OverallVerdict)
	}
	_, err := equivocation.FromTaskResult(task, res)
	if err == nil {
		t.Fatalf("expected error rejecting AGREE, got nil")
	}
	if !strings.Contains(err.Error(), "DISAGREE") {
		t.Errorf("error should mention DISAGREE requirement: %v", err)
	}
}

func TestFromTaskResult_RejectsAbstain(t *testing.T) {
	task, res := runDisagreeCase(t, "provider-outage-abstain")
	if res.OverallVerdict != judge.VerdictAbstain {
		t.Fatalf("precondition: expected ABSTAIN, got %s", res.OverallVerdict)
	}
	_, err := equivocation.FromTaskResult(task, res)
	if err == nil {
		t.Fatalf("expected error rejecting ABSTAIN, got nil")
	}
}

func TestCanonicalMarshal_Deterministic(t *testing.T) {
	task, res := runDisagreeCase(t, "system-override-toxic")
	p1, err := equivocation.FromTaskResult(task, res)
	if err != nil {
		t.Fatalf("build p1: %v", err)
	}
	p2, err := equivocation.FromTaskResult(task, res)
	if err != nil {
		t.Fatalf("build p2: %v", err)
	}
	b1, err := p1.MarshalCanonical()
	if err != nil {
		t.Fatalf("marshal p1: %v", err)
	}
	b2, err := p2.MarshalCanonical()
	if err != nil {
		t.Fatalf("marshal p2: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Errorf("canonical marshal drift:\np1=%s\np2=%s", b1, b2)
	}
	// Sanity: valid JSON and keys sorted.
	var asMap map[string]any
	if err := json.Unmarshal(b1, &asMap); err != nil {
		t.Fatalf("canonical output is not valid JSON: %v", err)
	}
	// Digest and OverallVerdict both start with 'd' and 'o'; digest_hex should
	// come before overall_verdict in the canonical output.
	digestIdx := bytes.Index(b1, []byte("digest_hex"))
	overallIdx := bytes.Index(b1, []byte("overall_verdict"))
	if digestIdx < 0 || overallIdx < 0 {
		t.Fatalf("expected keys missing")
	}
	if digestIdx >= overallIdx {
		t.Errorf("expected digest_hex before overall_verdict (alphabetical); got %d vs %d", digestIdx, overallIdx)
	}
}

func TestVerify_MatchesDigest(t *testing.T) {
	task, res := runDisagreeCase(t, "system-override-toxic")
	p, err := equivocation.FromTaskResult(task, res)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := p.Verify(); err != nil {
		t.Errorf("Verify on fresh proof: %v", err)
	}
}

func TestVerify_DetectsTampering(t *testing.T) {
	task, res := runDisagreeCase(t, "system-override-toxic")
	p, err := equivocation.FromTaskResult(task, res)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// Flip a vote — must invalidate.
	p.Facets[0].Votes[0].Value = "tampered"
	err = p.Verify()
	if err == nil {
		t.Errorf("expected verify failure after tampering, got nil")
	}
}

func TestVerify_TamperingWithDigestAlone(t *testing.T) {
	task, res := runDisagreeCase(t, "system-override-toxic")
	p, err := equivocation.FromTaskResult(task, res)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// Just corrupt DigestHex — must fail.
	p.DigestHex = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := p.Verify(); err == nil {
		t.Errorf("expected verify failure on tampered digest, got nil")
	}
}

func TestFromTaskResult_NilArgs(t *testing.T) {
	if _, err := equivocation.FromTaskResult(nil, &judge.TaskResult{OverallVerdict: judge.VerdictDisagree}); err == nil {
		t.Errorf("expected error on nil task")
	}
	if _, err := equivocation.FromTaskResult(&judge.Task{ID: "x"}, nil); err == nil {
		t.Errorf("expected error on nil result")
	}
}
