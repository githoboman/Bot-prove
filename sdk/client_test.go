package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer spins up a fake CasperProver API for round-trip tests.
func newTestServer(t *testing.T, method, path string, respStatus int, respBody map[string]any, wantAuth string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+path, func(w http.ResponseWriter, r *http.Request) {
		if wantAuth != "" && r.Header.Get("X-API-Key") != wantAuth {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(respStatus)
		_ = json.NewEncoder(w).Encode(respBody)
	})
	return httptest.NewServer(mux)
}

func TestSubmitProof(t *testing.T) {
	srv := newTestServer(t, "POST", "/proofs", http.StatusOK, map[string]any{"id": "P-1", "valid": true}, "")
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	out, err := c.SubmitProof(context.Background(), SubmitProofRequest{
		Agent: "agent-1", Input: "in", Output: "out", Model: "model",
	})
	if err != nil {
		t.Fatalf("SubmitProof: %v", err)
	}
	if out["id"] != "P-1" {
		t.Fatalf("expected id P-1, got %v", out["id"])
	}
}

func TestVerifyProof(t *testing.T) {
	srv := newTestServer(t, "POST", "/verify", http.StatusOK, map[string]any{"proof_id": "P-1", "valid": true}, "")
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	out, err := c.VerifyProof(context.Background(), "P-1")
	if err != nil {
		t.Fatalf("VerifyProof: %v", err)
	}
	if out["valid"] != true {
		t.Fatalf("expected valid=true, got %v", out["valid"])
	}
}

func TestGetProofNotFound(t *testing.T) {
	srv := newTestServer(t, "GET", "/proofs/{id}", http.StatusNotFound, map[string]any{"error": "proof not found"}, "")
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	_, err := c.GetProof(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing proof, got nil")
	}
}

func TestAuthTokenSentOnRequests(t *testing.T) {
	srv := newTestServer(t, "POST", "/kyc/check", http.StatusOK, map[string]any{"passed": true}, "secret-key")
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithAuthToken("secret-key"))
	if _, err := c.KYCCheck(context.Background(), "P-1"); err != nil {
		t.Fatalf("expected auth to succeed, got: %v", err)
	}

	c2 := NewClient(WithBaseURL(srv.URL)) // no token set
	if _, err := c2.KYCCheck(context.Background(), "P-1"); err == nil {
		t.Fatal("expected request without auth token to fail")
	}
}

func TestHealthAndStats(t *testing.T) {
	srv := newTestServer(t, "GET", "/health", http.StatusOK, map[string]any{"status": "ok"}, "")
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	out, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if out["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", out["status"])
	}
}
