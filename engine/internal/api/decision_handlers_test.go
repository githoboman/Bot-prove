package api

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/decision/attest"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/hitl"
)

// enableDecisionPipeline mimics initDecisionPipeline() without touching env.
func enableDecisionPipeline(t *testing.T, s *Server, systemProvider attest.Provider) {
	t.Helper()
	pool := attest.NewProviderPool()
	if systemProvider == nil {
		systemProvider = attest.NewNamedFixtureProvider("cp-decision-system")
	}
	if err := pool.Register(attest.PooledProvider{
		Provider: systemProvider,
		Trust:    attest.TrustSystem,
		Capabilities: []attest.Capability{
			attest.CapSafety,
			attest.CapCorrectness,
			attest.CapSpecCompliance,
			attest.CapEquivocation,
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	s.decisionPool = pool
	s.decisionRouter = attest.NewRouter(pool)
	s.decisionJudge = attest.NewJudge(systemProvider, attest.DefaultAggregationPolicy)
	s.hitlService = hitl.NewService(hitl.DefaultPolicy, nil)
}

func TestDecisionEvaluateDisabledReturns503(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest(http.MethodPost, "/v1/decision/evaluate", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	s.decisionEvaluate(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDecisionEvaluateHappyPath(t *testing.T) {
	s := newTestServer("")

	// A deterministic provider that APPROVEs everything with high confidence.
	provider := attest.NewNamedFixtureProvider("test-system")
	dec := attest.Decision{Submitter: "sub-A", SpecID: "spec-A", Payload: []byte{0x01, 0x02}, Nonce: 42}
	provider.Register(dec.ID(), []attest.FacetVerdict{
		{Kind: attest.FacetSafety, Verdict: attest.VerdictApprove, Confidence: 0.95, Reason: "ok"},
		{Kind: attest.FacetEquivocation, Verdict: attest.VerdictApprove, Confidence: 0.95, Reason: "no conflict"},
		{Kind: attest.FacetCorrectness, Verdict: attest.VerdictApprove, Confidence: 0.9, Reason: "post-cond met"},
		{Kind: attest.FacetSpecCompliance, Verdict: attest.VerdictApprove, Confidence: 0.9, Reason: "spec ok"},
	})
	enableDecisionPipeline(t, s, provider)

	body, _ := json.Marshal(decisionEvaluateRequest{
		Submitter:  "sub-A",
		SpecID:     "spec-A",
		PayloadHex: hex.EncodeToString([]byte{0x01, 0x02}),
		Nonce:      42,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/decision/evaluate", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.decisionEvaluate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp decisionEvaluateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Aggregate != "APPROVE" {
		t.Fatalf("expected APPROVE, got %q", resp.Aggregate)
	}
	if resp.HITL.Action != hitl.ActionPass {
		t.Fatalf("expected HITL=pass, got %v (%s)", resp.HITL.Action, resp.HITL.Reason)
	}
	if resp.CommitDigest == "" {
		t.Fatalf("expected commit_digest to be set")
	}
	if len(resp.Providers) == 0 {
		t.Fatalf("expected providers surfaced")
	}
}

func TestDecisionEvaluateEscalatesToHITL(t *testing.T) {
	s := newTestServer("")
	// Provider abstains on safety → HITL must escalate.
	provider := attest.NewNamedFixtureProvider("test-abstain")
	dec := attest.Decision{Submitter: "sub-B", SpecID: "spec-B", Payload: []byte{0xAA}, Nonce: 1}
	provider.Register(dec.ID(), []attest.FacetVerdict{
		{Kind: attest.FacetSafety, Verdict: attest.VerdictAbstain, Reason: "insufficient evidence"},
		{Kind: attest.FacetEquivocation, Verdict: attest.VerdictApprove, Confidence: 0.9},
		{Kind: attest.FacetCorrectness, Verdict: attest.VerdictApprove, Confidence: 0.9},
		{Kind: attest.FacetSpecCompliance, Verdict: attest.VerdictApprove, Confidence: 0.9},
	})
	enableDecisionPipeline(t, s, provider)

	body, _ := json.Marshal(decisionEvaluateRequest{
		Submitter:  "sub-B",
		SpecID:     "spec-B",
		PayloadHex: hex.EncodeToString([]byte{0xAA}),
		Nonce:      1,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/decision/evaluate", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.decisionEvaluate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp decisionEvaluateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.HITL.Action != hitl.ActionEscalate {
		t.Fatalf("expected escalate, got %v (%s)", resp.HITL.Action, resp.HITL.Reason)
	}
	if resp.HITL.TicketID == "" {
		t.Fatalf("expected TicketID populated on escalate")
	}
	// Ticket must exist in the store.
	if _, err := s.hitlService.Store().Get(resp.HITL.TicketID); err != nil {
		t.Fatalf("ticket not found in store: %v", err)
	}
}

func TestHITLResolveEndpointHappyPath(t *testing.T) {
	s := newTestServer("")
	provider := attest.NewNamedFixtureProvider("test-abstain")
	dec := attest.Decision{Submitter: "sub-C", SpecID: "spec-C", Payload: []byte{0xFF}, Nonce: 2}
	provider.Register(dec.ID(), []attest.FacetVerdict{
		{Kind: attest.FacetSafety, Verdict: attest.VerdictAbstain, Reason: "unclear"},
	})
	enableDecisionPipeline(t, s, provider)

	// Trigger a ticket.
	body, _ := json.Marshal(decisionEvaluateRequest{Submitter: "sub-C", SpecID: "spec-C", PayloadHex: "ff", Nonce: 2})
	req := httptest.NewRequest(http.MethodPost, "/v1/decision/evaluate", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.decisionEvaluate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate: %d %s", rec.Code, rec.Body.String())
	}
	var resp decisionEvaluateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.HITL.TicketID == "" {
		t.Fatal("no ticket")
	}
	ticketID := resp.HITL.TicketID

	// Resolve endpoint with the correct mux path variable.
	resolveBody := `{"resolver":"reviewer-1","state":"approved","note":"looked ok"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/hitl/tickets/"+ticketID+"/resolve", strings.NewReader(resolveBody))
	req.SetPathValue("id", ticketID)
	rec = httptest.NewRecorder()
	s.hitlResolveTicket(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on resolve, got %d: %s", rec.Code, rec.Body.String())
	}

	// Fetch and verify state transition.
	req = httptest.NewRequest(http.MethodGet, "/v1/hitl/tickets/"+ticketID, nil)
	req.SetPathValue("id", ticketID)
	rec = httptest.NewRecorder()
	s.hitlGetTicket(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	var tk hitl.Ticket
	if err := json.Unmarshal(rec.Body.Bytes(), &tk); err != nil {
		t.Fatal(err)
	}
	if tk.State != "approved" || tk.Resolver != "reviewer-1" {
		t.Fatalf("unexpected resolved ticket: %+v", tk)
	}
}

func TestHITLResolveInvalidState(t *testing.T) {
	s := newTestServer("")
	enableDecisionPipeline(t, s, nil)
	// Seed a ticket directly.
	tk := hitl.Ticket{ID: "seed", State: "pending"}
	if err := s.hitlService.Store().Create(tk); err != nil {
		t.Fatalf("Create: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/hitl/tickets/seed/resolve", strings.NewReader(`{"resolver":"x","state":"whatever"}`))
	req.SetPathValue("id", "seed")
	rec := httptest.NewRecorder()
	s.hitlResolveTicket(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on invalid state, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDecisionPoolInfo(t *testing.T) {
	s := newTestServer("")
	enableDecisionPipeline(t, s, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/decision/pool", nil)
	rec := httptest.NewRecorder()
	s.decisionPoolInfo(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	b, _ := io.ReadAll(rec.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	count, ok := out["count"].(float64)
	if !ok || count != 1 {
		t.Fatalf("expected count=1, got %v", out["count"])
	}
}
