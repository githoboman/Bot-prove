package api

// Gate 4: real Groth16 (gnark BN254) is the PRIMARY ZK path;
// the conceptual hash-based /zk/verify-groth16{,-sim} and
// /zk/batch-verify{,-sim} are kept for legacy/demo and MUST advertise
// themselves as {simulation:true, deprecated:true} in the response body
// and via Warning/Deprecation/Sunset headers.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGate4_SimVerifyGroth16_AdvertisesSimulationAndDeprecation(t *testing.T) {
	s := newTestServer("")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/zk/verify-groth16",
		strings.NewReader(`{"proof":"abc","public_inputs":["1","2"],"vk_hash":"vk"}`))
	s.zkVerifyGroth16(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Response body flags
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["simulation"] != true {
		t.Fatalf("expected simulation:true in body, got %v", body)
	}
	if body["deprecated"] != true {
		t.Fatalf("expected deprecated:true in body, got %v", body)
	}
	if body["use"] != "/zk/groth16-real/verify" {
		t.Fatalf("expected use=/zk/groth16-real/verify, got %v", body["use"])
	}
	if note, _ := body["note"].(string); !strings.HasPrefix(note, "[sim]") {
		t.Fatalf("expected [sim]-prefixed note, got %q", note)
	}
	// Headers
	if w := rec.Header().Get("Warning"); !strings.Contains(w, "simulation") {
		t.Fatalf("expected Warning header mentioning simulation, got %q", w)
	}
	if rec.Header().Get("Deprecation") != "true" {
		t.Fatalf("expected Deprecation:true header, got %q", rec.Header().Get("Deprecation"))
	}
	if rec.Header().Get("Sunset") == "" {
		t.Fatalf("expected Sunset header set")
	}
}

func TestGate4_SimBatchVerify_AdvertisesSimulationAndDeprecation(t *testing.T) {
	s := newTestServer("")
	rec := httptest.NewRecorder()
	body := `{"proofs":[{"proof":"a","public_inputs":["1"]},{"proof":"b","public_inputs":["2"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/zk/batch-verify", strings.NewReader(body))
	s.zkBatchVerify(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["simulation"] != true || resp["deprecated"] != true {
		t.Fatalf("expected simulation:true, deprecated:true, got %v", resp)
	}
	if resp["use"] != "/zk/groth16-real/verify" {
		t.Fatalf("expected use=/zk/groth16-real/verify, got %v", resp["use"])
	}
	if rec.Header().Get("Deprecation") != "true" {
		t.Fatalf("expected Deprecation:true header")
	}
}

// Test that the real Groth16 endpoint (gnark, BN254 pairing checks) does
// NOT carry the simulation banner and produces a proof that actually
// verifies. This is the primary ZK path.
func TestGate4_RealGroth16Endpoint_ProveAndVerify_NoSimBanner(t *testing.T) {
	s := newTestServer("")
	if s.realZK == nil {
		t.Skip("real Groth16 setup unavailable in this test binary")
	}

	// Prove
	provRec := httptest.NewRecorder()
	provReq := httptest.NewRequest(http.MethodPost, "/zk/groth16-real/prove",
		strings.NewReader(`{"preimage":"42"}`))
	s.zkGroth16RealProve(provRec, provReq)

	if provRec.Code != http.StatusOK {
		t.Fatalf("prove: expected 200, got %d: %s", provRec.Code, provRec.Body.String())
	}
	// Real endpoint must NOT carry the deprecation/sim markers.
	if provRec.Header().Get("Deprecation") == "true" {
		t.Fatalf("real endpoint must not carry Deprecation header")
	}
	var provBody map[string]any
	if err := json.Unmarshal(provRec.Body.Bytes(), &provBody); err != nil {
		t.Fatalf("prove decode: %v", err)
	}
	if provBody["simulation"] == true {
		t.Fatalf("real endpoint must not set simulation:true, got %v", provBody)
	}
	if provBody["curve"] != "BN254" {
		t.Fatalf("expected curve BN254, got %v", provBody["curve"])
	}
	hash, _ := provBody["hash"].(string)
	proofHex, _ := provBody["proof_hex"].(string)
	if hash == "" || proofHex == "" {
		t.Fatalf("prove missing hash/proof_hex: %v", provBody)
	}

	// Verify — round-trip
	verRec := httptest.NewRecorder()
	verReq := httptest.NewRequest(http.MethodPost, "/zk/groth16-real/verify",
		strings.NewReader(`{"hash":"`+hash+`","proof_hex":"`+proofHex+`"}`))
	s.zkGroth16RealVerify(verRec, verReq)

	if verRec.Code != http.StatusOK {
		t.Fatalf("verify: expected 200, got %d: %s", verRec.Code, verRec.Body.String())
	}
	var verBody map[string]any
	if err := json.Unmarshal(verRec.Body.Bytes(), &verBody); err != nil {
		t.Fatalf("verify decode: %v", err)
	}
	if verBody["valid"] != true {
		t.Fatalf("real Groth16 round-trip should verify true, got %v", verBody)
	}
	if verBody["simulation"] == true {
		t.Fatalf("real endpoint must not set simulation:true, got %v", verBody)
	}
	if verRec.Header().Get("Deprecation") == "true" {
		t.Fatalf("real endpoint must not carry Deprecation header")
	}
}
