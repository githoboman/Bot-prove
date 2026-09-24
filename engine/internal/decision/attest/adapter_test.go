package attest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPProviderAdapterUnconfiguredFallsBackToFixture(t *testing.T) {
	fb := NewNamedFixtureProvider("fb")
	fb.Register("some-id", []FacetVerdict{{Kind: FacetSafety, Verdict: VerdictApprove, Confidence: 0.9, Reason: "test"}})
	a := NewHTTPProviderAdapter(HTTPProviderConfig{Fallback: fb})
	if a.Configured() {
		t.Fatalf("expected unconfigured")
	}
	if err := a.RemoteRequired(); !errors.Is(err, ErrRemoteUnconfigured) {
		t.Fatalf("expected ErrRemoteUnconfigured, got %v", err)
	}
	// Any decision goes through fallback → returns fixture's default ABSTAIN
	// because we didn't register the specific ID.
	vs, err := a.Evaluate(context.Background(), Decision{Submitter: "x", SpecID: "y", Nonce: 1})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(vs) == 0 {
		t.Fatalf("fallback returned no verdicts")
	}
}

func TestHTTPProviderAdapterAgainstMockServer(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(evaluateResponse{
			Verdicts: []evaluateResponseFacet{
				{Kind: string(FacetSafety), Verdict: "APPROVE", Confidence: 0.95, Reason: "remote-safety"},
				{Kind: string(FacetCorrectness), Verdict: "REJECT", Confidence: 0.7, Reason: "remote-correctness"},
				{Kind: "unknown-kind", Verdict: "APPROVE", Confidence: 1.0, Reason: "should-be-dropped-by-allowlist"},
				{Kind: string(FacetSpecCompliance), Verdict: "not-a-real-verdict", Reason: "malformed"},
			},
		})
	}))
	defer srv.Close()

	a := NewHTTPProviderAdapter(HTTPProviderConfig{
		Name:          "unit-remote",
		Endpoint:      srv.URL,
		Token:         "test-token",
		Timeout:       500 * time.Millisecond,
		AllowedFacets: []FacetKind{FacetSafety, FacetCorrectness, FacetSpecCompliance},
	})
	if !a.Configured() {
		t.Fatalf("expected configured")
	}

	dec := Decision{Submitter: "sub", SpecID: "spec", Payload: []byte("hello"), Nonce: 7}
	vs, err := a.Evaluate(context.Background(), dec)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// Expect exactly 2 verdicts: unknown-kind dropped, malformed dropped.
	if len(vs) != 2 {
		t.Fatalf("expected 2 verdicts (after allowlist+skip malformed), got %d: %+v", len(vs), vs)
	}
	// Round-trip request body carried the canonical DecisionID.
	if got := gotBody["decision_id"]; got != dec.ID() {
		t.Fatalf("expected decision_id %s in request, got %v", dec.ID(), got)
	}
	if got := gotBody["submitter"]; got != "sub" {
		t.Fatalf("expected submitter 'sub' in request, got %v", got)
	}
}

func TestHTTPProviderAdapterFallsBackOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	fb := NewNamedFixtureProvider("fb")
	a := NewHTTPProviderAdapter(HTTPProviderConfig{
		Name:     "n",
		Endpoint: srv.URL,
		Fallback: fb,
		Timeout:  200 * time.Millisecond,
	})
	vs, err := a.Evaluate(context.Background(), Decision{Submitter: "s", SpecID: "s", Nonce: 1})
	if err != nil {
		t.Fatalf("expected fallback path, got err %v", err)
	}
	if len(vs) == 0 {
		t.Fatalf("expected fallback verdicts, got none")
	}
	// Every verdict should be ABSTAIN (fixture default) with fallback reason.
	for _, v := range vs {
		if v.Verdict != VerdictAbstain {
			t.Fatalf("expected ABSTAIN from fallback, got %v", v.Verdict)
		}
	}
}

func TestParseAndSerializeVerdictString(t *testing.T) {
	cases := map[string]Verdict{
		"APPROVE":     VerdictApprove,
		"abstain":     VerdictAbstain,
		"  reject  ":  VerdictReject,
	}
	for in, want := range cases {
		got, err := parseVerdictString(in)
		if err != nil {
			t.Fatalf("parseVerdictString(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("parseVerdictString(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := parseVerdictString("nonsense"); err == nil {
		t.Fatalf("expected parse error")
	}
	if s := verdictWireString(VerdictApprove); s != "APPROVE" {
		t.Fatalf("verdictWireString(APPROVE) = %q", s)
	}
	if s := verdictWireString(Verdict(99)); !strings.EqualFold(s, "UNKNOWN") {
		t.Fatalf("verdictWireString(unknown) = %q", s)
	}
}
