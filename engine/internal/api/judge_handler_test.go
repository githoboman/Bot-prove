package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge/hitl"
)

// stubJudge is a fake JudgeService that returns a pre-canned TaskResult.
// Lets us exercise every branch of the handler (AGREE / DISAGREE / ABSTAIN)
// without wiring a real Runner.
type stubJudge struct {
	next *judge.TaskResult
	err  error
}

func (s stubJudge) Decide(ctx context.Context, task *judge.Task) (*judge.TaskResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	// Copy TaskID from task to mimic real judge behavior.
	out := *s.next
	if out.TaskID == "" {
		out.TaskID = task.ID
	}
	return &out, nil
}

// captureSink records every delivered event for assertions.
type captureSink struct {
	events []hitl.EscalationEvent
}

func (c *captureSink) Deliver(ctx context.Context, ev hitl.EscalationEvent) error {
	c.events = append(c.events, ev)
	return nil
}

// buildTestServer returns a minimal *Server with a mux that only serves the
// judge endpoint. We skip the full New() constructor because it drags in the
// prover, gnark setup, and Postgres.
func buildTestServer(t *testing.T, j JudgeService, sink hitl.Sink) *httptest.Server {
	t.Helper()
	s := &Server{}
	s.SetJudge(j)
	if sink != nil {
		s.SetHITLSink(sink)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /inference/judge", s.judgeHandler)
	return httptest.NewServer(mux)
}

func doPost(t *testing.T, url string, body any) (*http.Response, []byte) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url+"/inference/judge", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	return resp, buf.Bytes()
}

func vote(pid, val string) judge.ProviderVote {
	return judge.ProviderVote{ProviderID: pid, Value: val, Raw: val, Latency: 50 * time.Millisecond}
}

func TestJudgeHandler_UnwiredReturns503(t *testing.T) {
	ts := buildTestServer(t, nil, nil)
	defer ts.Close()
	resp, _ := doPost(t, ts.URL, map[string]any{
		"input":  "x",
		"facets": []map[string]any{{"id": "f1", "prompt": "?", "allowed_values": []string{"y", "n"}}},
	})
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503", resp.StatusCode)
	}
}

func TestJudgeHandler_RejectsBadJSON(t *testing.T) {
	stub := stubJudge{next: &judge.TaskResult{OverallVerdict: judge.VerdictAgree, Facets: map[string]*judge.FacetResult{}}}
	ts := buildTestServer(t, stub, nil)
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/inference/judge", "application/json", strings.NewReader("{not json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
}

func TestJudgeHandler_ValidatesInputs(t *testing.T) {
	stub := stubJudge{next: &judge.TaskResult{OverallVerdict: judge.VerdictAgree, Facets: map[string]*judge.FacetResult{}}}
	ts := buildTestServer(t, stub, nil)
	defer ts.Close()

	cases := []struct {
		name string
		body map[string]any
	}{
		{"empty input", map[string]any{"input": "", "facets": []map[string]any{{"id": "f", "prompt": "?", "allowed_values": []string{"y", "n"}}}}},
		{"no facets", map[string]any{"input": "x", "facets": []map[string]any{}}},
		{"facet no id", map[string]any{"input": "x", "facets": []map[string]any{{"id": "", "prompt": "?", "allowed_values": []string{"y", "n"}}}}},
		{"facet duplicate", map[string]any{"input": "x", "facets": []map[string]any{{"id": "a", "prompt": "?", "allowed_values": []string{"y", "n"}}, {"id": "a", "prompt": "?", "allowed_values": []string{"y", "n"}}}}},
		{"facet no prompt", map[string]any{"input": "x", "facets": []map[string]any{{"id": "a", "prompt": "", "allowed_values": []string{"y", "n"}}}}},
		{"facet 1 value", map[string]any{"input": "x", "facets": []map[string]any{{"id": "a", "prompt": "?", "allowed_values": []string{"y"}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := doPost(t, ts.URL, tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status: got %d want 400", resp.StatusCode)
			}
		})
	}
}

func TestJudgeHandler_AgreePath(t *testing.T) {
	stub := stubJudge{next: &judge.TaskResult{
		TaskID:         "t-agree",
		OverallVerdict: judge.VerdictAgree,
		Facets: map[string]*judge.FacetResult{
			"safe": {FacetID: "safe", Verdict: judge.VerdictAgree, Winner: "yes", LiveCount: 3, AgreementFraction: 1.0,
				Votes: []judge.ProviderVote{vote("a", "yes"), vote("b", "yes"), vote("c", "yes")}},
		},
	}}
	ts := buildTestServer(t, stub, nil)
	defer ts.Close()
	resp, body := doPost(t, ts.URL, map[string]any{
		"task_id": "t-agree",
		"input":   "hello",
		"facets":  []map[string]any{{"id": "safe", "prompt": "?", "allowed_values": []string{"yes", "no"}}},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200 body=%s", resp.StatusCode, body)
	}
	var got judgeResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Overall != judge.VerdictAgree {
		t.Fatalf("overall: %s", got.Overall)
	}
	if got.Equivocated != nil || got.Escalation != nil {
		t.Fatal("agree path must not emit equivocation or hitl payload")
	}
	if len(got.Facets) != 1 || got.Facets[0].Winner != "yes" {
		t.Fatalf("facets: %+v", got.Facets)
	}
}

func TestJudgeHandler_DisagreePathEmitsProof(t *testing.T) {
	stub := stubJudge{next: &judge.TaskResult{
		TaskID:         "t-dis",
		OverallVerdict: judge.VerdictDisagree,
		Facets: map[string]*judge.FacetResult{
			"safe": {FacetID: "safe", Verdict: judge.VerdictDisagree, LiveCount: 2, AgreementFraction: 0.5,
				Votes: []judge.ProviderVote{vote("a", "yes"), vote("b", "no")}},
		},
		StartedAt: time.Now().UTC(), CompletedAt: time.Now().UTC(),
	}}
	ts := buildTestServer(t, stub, nil)
	defer ts.Close()
	resp, body := doPost(t, ts.URL, map[string]any{
		"task_id": "t-dis",
		"input":   "hello",
		"facets":  []map[string]any{{"id": "safe", "prompt": "?", "allowed_values": []string{"yes", "no"}}},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200 body=%s", resp.StatusCode, body)
	}
	var got judgeResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Overall != judge.VerdictDisagree {
		t.Fatalf("overall: %s", got.Overall)
	}
	if got.Equivocated == nil {
		t.Fatal("expected equivocation proof")
	}
	if got.Escalation != nil {
		t.Fatal("disagree must not emit hitl payload")
	}
	if got.Equivocated.DigestHex == "" {
		t.Fatal("expected non-empty digest")
	}
	if err := got.Equivocated.Verify(); err != nil {
		t.Fatalf("proof verify: %v", err)
	}
}

func TestJudgeHandler_AbstainPathEmitsHITLAndCallsSink(t *testing.T) {
	stub := stubJudge{next: &judge.TaskResult{
		TaskID:         "t-abs",
		OverallVerdict: judge.VerdictAbstain,
		Facets: map[string]*judge.FacetResult{
			"safe": {FacetID: "safe", Verdict: judge.VerdictAbstain, LiveCount: 1, AgreementFraction: 0,
				Votes: []judge.ProviderVote{vote("a", "yes"), {ProviderID: "b", Err: "timeout"}}},
		},
	}}
	sink := &captureSink{}
	ts := buildTestServer(t, stub, sink)
	defer ts.Close()
	resp, body := doPost(t, ts.URL, map[string]any{
		"task_id": "t-abs",
		"input":   "hello",
		"facets":  []map[string]any{{"id": "safe", "prompt": "?", "allowed_values": []string{"yes", "no"}}},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200 body=%s", resp.StatusCode, body)
	}
	var got judgeResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Overall != judge.VerdictAbstain {
		t.Fatalf("overall: %s", got.Overall)
	}
	if got.Escalation == nil {
		t.Fatal("expected hitl escalation")
	}
	if got.Equivocated != nil {
		t.Fatal("abstain must not emit equivocation")
	}
	if err := hitl.Verify(*got.Escalation); err != nil {
		t.Fatalf("hitl verify: %v", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("sink events: got %d want 1", len(sink.events))
	}
	if sink.events[0].Digest != got.Escalation.Digest {
		t.Fatal("sink digest != response digest")
	}
}

// Facet count cap
func TestJudgeHandler_RejectsTooManyFacets(t *testing.T) {
	stub := stubJudge{next: &judge.TaskResult{OverallVerdict: judge.VerdictAgree, Facets: map[string]*judge.FacetResult{}}}
	ts := buildTestServer(t, stub, nil)
	defer ts.Close()
	facets := make([]map[string]any, 33)
	for i := range facets {
		facets[i] = map[string]any{"id": "f" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)), "prompt": "?", "allowed_values": []string{"y", "n"}}
	}
	resp, _ := doPost(t, ts.URL, map[string]any{"input": "x", "facets": facets})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
}
