package hitl_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge/hitl"
)

func fixedNow() time.Time {
	return time.Date(2026, 7, 20, 8, 30, 0, 0, time.UTC)
}

func makeTask(facets []judge.Facet) *judge.Task {
	return &judge.Task{
		ID:                 "t-abstain-1",
		Input:              "Is this content acceptable?",
		SystemMsg:          "You are a strict content judge.",
		Facets:             facets,
		MinProviders:       2,
		AgreementThreshold: 0.66,
	}
}

func facet(id, prompt string, allowed ...string) judge.Facet {
	return judge.Facet{ID: id, Prompt: prompt, AllowedValues: allowed, Weight: 1.0}
}

func vote(pid, val, raw string, ms int, err string) judge.ProviderVote {
	return judge.ProviderVote{ProviderID: pid, Value: val, Raw: raw, Latency: time.Duration(ms) * time.Millisecond, Err: err}
}

// abstain: one AGREE facet, one ABSTAIN facet -> low severity
func TestBuildEvent_LowSeverity(t *testing.T) {
	facets := []judge.Facet{
		facet("safe", "Is it safe?", "yes", "no"),
		facet("severe", "How severe?", "low", "medium", "high"),
	}
	task := makeTask(facets)
	tr := &judge.TaskResult{
		TaskID:         task.ID,
		OverallVerdict: judge.VerdictAbstain,
		Facets: map[string]*judge.FacetResult{
			"safe": {
				FacetID: "safe", Verdict: judge.VerdictAgree, Winner: "yes",
				LiveCount: 3, AgreementFraction: 1.0,
				Votes: []judge.ProviderVote{
					vote("groq/llama", "yes", "yes.", 200, ""),
					vote("openrouter/mixtral", "yes", "yes", 300, ""),
					vote("gemini/flash", "yes", "yes indeed", 250, ""),
				},
			},
			"severe": {
				FacetID: "severe", Verdict: judge.VerdictAbstain,
				LiveCount: 1, AgreementFraction: 0,
				Votes: []judge.ProviderVote{
					vote("groq/llama", "low", "low", 200, ""),
					vote("openrouter/mixtral", "", "", 0, "context length exceeded"),
					vote("gemini/flash", "", "", 0, "429 rate limit"),
				},
			},
		},
	}
	ev, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}
	if ev.Severity != hitl.SeverityLow {
		t.Fatalf("severity: got %s want low", ev.Severity)
	}
	if len(ev.Facets) != 2 {
		t.Fatalf("facets: got %d want 2", len(ev.Facets))
	}
	if ev.Facets[0].FacetID != "safe" || ev.Facets[1].FacetID != "severe" {
		t.Fatalf("facets not sorted by ID: %v", []string{ev.Facets[0].FacetID, ev.Facets[1].FacetID})
	}
	if ev.Facets[1].VoteHistogram["low"] != 1 {
		t.Fatalf("histogram unexpected: %v", ev.Facets[1].VoteHistogram)
	}
	if ev.Digest == "" {
		t.Fatal("digest empty")
	}
	if err := hitl.Verify(ev); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// abstain with 2 ABSTAIN facets -> medium severity
func TestBuildEvent_MediumSeverity(t *testing.T) {
	task := makeTask([]judge.Facet{
		facet("f1", "?", "yes", "no"), facet("f2", "?", "yes", "no"), facet("f3", "?", "yes", "no"),
	})
	tr := &judge.TaskResult{
		TaskID:         task.ID,
		OverallVerdict: judge.VerdictAbstain,
		Facets: map[string]*judge.FacetResult{
			"f1": {FacetID: "f1", Verdict: judge.VerdictAbstain, Votes: []judge.ProviderVote{}},
			"f2": {FacetID: "f2", Verdict: judge.VerdictAbstain, Votes: []judge.ProviderVote{}},
			"f3": {FacetID: "f3", Verdict: judge.VerdictAgree, Winner: "yes", Votes: []judge.ProviderVote{vote("p", "yes", "yes", 10, "")}},
		},
	}
	ev, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}
	if ev.Severity != hitl.SeverityMedium {
		t.Fatalf("severity: got %s want medium", ev.Severity)
	}
}

// abstain with 1 DISAGREE facet mixed in -> high severity
func TestBuildEvent_HighSeverity(t *testing.T) {
	task := makeTask([]judge.Facet{
		facet("f1", "?", "yes", "no"),
		facet("f2", "?", "yes", "no"),
		facet("f3", "?", "yes", "no"),
	})
	tr := &judge.TaskResult{
		TaskID:         task.ID,
		OverallVerdict: judge.VerdictAbstain, // deliberately still ABSTAIN (edge case)
		Facets: map[string]*judge.FacetResult{
			"f1": {FacetID: "f1", Verdict: judge.VerdictAgree, Winner: "yes", Votes: []judge.ProviderVote{vote("p", "yes", "yes", 10, "")}},
			"f2": {FacetID: "f2", Verdict: judge.VerdictAgree, Winner: "yes", Votes: []judge.ProviderVote{vote("p", "yes", "yes", 10, "")}},
			"f3": {FacetID: "f3", Verdict: judge.VerdictDisagree, Votes: []judge.ProviderVote{vote("p", "yes", "yes", 10, ""), vote("q", "no", "no", 10, "")}},
		},
	}
	ev, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}
	if ev.Severity != hitl.SeverityHigh {
		t.Fatalf("severity: got %s want high", ev.Severity)
	}
}

// abstain with >50% DISAGREE -> critical (indicates judge should have gone DISAGREE)
func TestBuildEvent_CriticalSeverity(t *testing.T) {
	task := makeTask([]judge.Facet{
		facet("f1", "?", "y", "n"), facet("f2", "?", "y", "n"), facet("f3", "?", "y", "n"),
	})
	tr := &judge.TaskResult{
		TaskID:         task.ID,
		OverallVerdict: judge.VerdictAbstain,
		Facets: map[string]*judge.FacetResult{
			"f1": {FacetID: "f1", Verdict: judge.VerdictDisagree, Votes: []judge.ProviderVote{vote("p", "y", "y", 10, ""), vote("q", "n", "n", 10, "")}},
			"f2": {FacetID: "f2", Verdict: judge.VerdictDisagree, Votes: []judge.ProviderVote{vote("p", "y", "y", 10, ""), vote("q", "n", "n", 10, "")}},
			"f3": {FacetID: "f3", Verdict: judge.VerdictAgree, Winner: "y", Votes: []judge.ProviderVote{vote("p", "y", "y", 10, "")}},
		},
	}
	ev, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}
	if ev.Severity != hitl.SeverityCritical {
		t.Fatalf("severity: got %s want critical", ev.Severity)
	}
}

// AGREE overall -> refuse to build
func TestBuildEvent_RefusesAgree(t *testing.T) {
	task := makeTask([]judge.Facet{facet("f1", "?", "y", "n")})
	tr := &judge.TaskResult{TaskID: task.ID, OverallVerdict: judge.VerdictAgree, Facets: map[string]*judge.FacetResult{}}
	if _, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow}); err == nil {
		t.Fatal("expected error for AGREE overall")
	}
}

// DISAGREE overall -> refuse to build (equivocation package's job)
func TestBuildEvent_RefusesDisagree(t *testing.T) {
	task := makeTask([]judge.Facet{facet("f1", "?", "y", "n")})
	tr := &judge.TaskResult{TaskID: task.ID, OverallVerdict: judge.VerdictDisagree, Facets: map[string]*judge.FacetResult{}}
	_, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err == nil || !strings.Contains(err.Error(), "DISAGREE") {
		t.Fatalf("expected refuse for DISAGREE, got %v", err)
	}
}

// nil inputs
func TestBuildEvent_NilInputs(t *testing.T) {
	if _, err := hitl.BuildEvent(nil, &judge.TaskResult{}, hitl.Options{}); err == nil {
		t.Fatal("expected error for nil task")
	}
	if _, err := hitl.BuildEvent(&judge.Task{}, nil, hitl.Options{}); err == nil {
		t.Fatal("expected error for nil result")
	}
}

// Digest is deterministic across multiple builds of the same input
func TestBuildEvent_DigestDeterministic(t *testing.T) {
	task := makeTask([]judge.Facet{
		facet("safe", "?", "yes", "no"),
		facet("severe", "?", "low", "med", "high"),
	})
	tr := &judge.TaskResult{
		TaskID: task.ID, OverallVerdict: judge.VerdictAbstain,
		Facets: map[string]*judge.FacetResult{
			"safe":   {FacetID: "safe", Verdict: judge.VerdictAgree, Winner: "yes", Votes: []judge.ProviderVote{vote("a", "yes", "yes", 10, ""), vote("b", "yes", "yes", 20, "")}},
			"severe": {FacetID: "severe", Verdict: judge.VerdictAbstain, Votes: []judge.ProviderVote{vote("a", "low", "low", 10, ""), vote("b", "", "", 0, "err")}},
		},
	}
	e1, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	e2, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if e1.Digest != e2.Digest {
		t.Fatalf("digest not deterministic: %s vs %s", e1.Digest, e2.Digest)
	}
	// change one vote value -> digest must change
	tr.Facets["safe"].Votes[0].Value = "no"
	e3, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if e1.Digest == e3.Digest {
		t.Fatal("digest did not change after vote tamper")
	}
}

// Verify catches tampering
func TestVerify_CatchesTamper(t *testing.T) {
	task := makeTask([]judge.Facet{facet("safe", "?", "yes", "no")})
	tr := &judge.TaskResult{
		TaskID: task.ID, OverallVerdict: judge.VerdictAbstain,
		Facets: map[string]*judge.FacetResult{
			"safe": {FacetID: "safe", Verdict: judge.VerdictAbstain, Votes: []judge.ProviderVote{vote("a", "yes", "yes", 10, "")}},
		},
	}
	ev, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if err := hitl.Verify(ev); err != nil {
		t.Fatalf("verify pristine: %v", err)
	}
	// tamper input
	ev.Input = "different"
	if err := hitl.Verify(ev); err == nil {
		t.Fatal("expected verify to fail after tamper")
	}
}

// JSON round-trip preserves digest verification
func TestBuildEvent_JSONRoundtrip(t *testing.T) {
	task := makeTask([]judge.Facet{facet("safe", "?", "yes", "no")})
	tr := &judge.TaskResult{
		TaskID: task.ID, OverallVerdict: judge.VerdictAbstain,
		Facets: map[string]*judge.FacetResult{
			"safe": {FacetID: "safe", Verdict: judge.VerdictAbstain, Votes: []judge.ProviderVote{vote("a", "yes", "yes", 10, "")}},
		},
	}
	ev, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var back hitl.EscalationEvent
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if err := hitl.Verify(back); err != nil {
		t.Fatalf("verify after roundtrip: %v", err)
	}
}

// MultiSink fan-out: every sink is called, first error is returned but no delivery is skipped
func TestMultiSink_FanOut(t *testing.T) {
	task := makeTask([]judge.Facet{facet("safe", "?", "yes", "no")})
	tr := &judge.TaskResult{
		TaskID: task.ID, OverallVerdict: judge.VerdictAbstain,
		Facets: map[string]*judge.FacetResult{
			"safe": {FacetID: "safe", Verdict: judge.VerdictAbstain, Votes: []judge.ProviderVote{vote("a", "yes", "yes", 10, "")}},
		},
	}
	ev, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	failing := hitl.SinkFunc(func(ctx context.Context, e hitl.EscalationEvent) error {
		calls = append(calls, "failing")
		return errors.New("boom")
	})
	ok1 := hitl.SinkFunc(func(ctx context.Context, e hitl.EscalationEvent) error {
		calls = append(calls, "ok1")
		return nil
	})
	ok2 := hitl.SinkFunc(func(ctx context.Context, e hitl.EscalationEvent) error {
		calls = append(calls, "ok2")
		return nil
	})
	m := &hitl.MultiSink{Sinks: []hitl.Sink{failing, ok1, ok2}}
	err = m.Deliver(context.Background(), ev)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom, got %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("expected all sinks called: %v", calls)
	}
}

// Missing facet result is treated as ABSTAIN, not a panic
func TestBuildEvent_MissingFacetResult(t *testing.T) {
	task := makeTask([]judge.Facet{facet("declared_but_missing", "?", "y", "n")})
	tr := &judge.TaskResult{TaskID: task.ID, OverallVerdict: judge.VerdictAbstain, Facets: map[string]*judge.FacetResult{}}
	ev, err := hitl.BuildEvent(task, tr, hitl.Options{Now: fixedNow})
	if err != nil {
		t.Fatalf("expected graceful handling, got %v", err)
	}
	if len(ev.Facets) != 1 || ev.Facets[0].Verdict != judge.VerdictAbstain {
		t.Fatalf("unexpected facets: %+v", ev.Facets)
	}
}
