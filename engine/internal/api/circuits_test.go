package api

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/zkverifier/gnarkzk"
)

// helper: pull a JSON response body into a map for spot-checks.
func decodeResp(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v; body=%s", err, string(body))
	}
	return out
}

// TestCircuits_List_ReturnsBundledCircuits — /v1/circuits publishes both
// registered circuits with populated key digests.
func TestCircuits_List_ReturnsBundledCircuits(t *testing.T) {
	s := newTestServer("")
	if s.zkReg == nil {
		t.Skip("zkReg not initialized on this build")
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/circuits", nil)
	rec := httptest.NewRecorder()
	s.circuitsList(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	out := decodeResp(t, rec.Body.Bytes())
	list, _ := out["circuits"].([]any)
	if len(list) < 2 {
		t.Fatalf("expected \u22652 circuits, got %d: %v", len(list), out)
	}
	ids := map[string]bool{}
	for _, c := range list {
		m, ok := c.(map[string]any)
		if !ok {
			t.Fatalf("circuit entry: expected map[string]any, got %T", c)
		}
		if id, ok := m["id"].(string); ok {
			ids[id] = true
		}
		if _, ok := m["key_digest"].(string); !ok || m["key_digest"] == "" {
			t.Fatalf("expected non-empty key_digest on %v", m)
		}
	}
	if !ids[gnarkzk.MiMCPreimageID] || !ids[gnarkzk.ModelInferenceID] {
		t.Fatalf("expected both bundled circuit ids in %v", ids)
	}
}

// TestCircuits_Get_UnknownIsNotFound
func TestCircuits_Get_UnknownIsNotFound(t *testing.T) {
	s := newTestServer("")
	if s.zkReg == nil {
		t.Skip("zkReg not initialized")
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/circuits/does-not-exist", nil)
	req.SetPathValue("id", "does-not-exist")
	rec := httptest.NewRecorder()
	s.circuitsGet(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// TestCircuits_GetVK_ReturnsHex
func TestCircuits_GetVK_ReturnsHex(t *testing.T) {
	s := newTestServer("")
	if s.zkReg == nil {
		t.Skip("zkReg not initialized")
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/circuits/"+gnarkzk.MiMCPreimageID+"/vk", nil)
	req.SetPathValue("id", gnarkzk.MiMCPreimageID)
	rec := httptest.NewRecorder()
	s.circuitsGetVK(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	out := decodeResp(t, rec.Body.Bytes())
	vkHex, _ := out["vk_hex"].(string)
	if len(vkHex) < 64 {
		t.Fatalf("expected non-empty vk_hex, got %q", vkHex)
	}
	if _, err := hex.DecodeString(vkHex); err != nil {
		t.Fatalf("vk_hex not hex: %v", err)
	}
	if _, ok := out["vk_sha256"].(string); !ok {
		t.Fatalf("expected vk_sha256 in response, got %v", out)
	}
}

// TestZKGeneric_ProveVerify_MiMCPreimage — end-to-end via HTTP surface:
// prove returns a proof_hex, verify accepts it with the correct hash, and
// rejects a tampered hash.
func TestZKGeneric_ProveVerify_MiMCPreimage(t *testing.T) {
	s := newTestServer("")
	if s.zkReg == nil {
		t.Skip("zkReg not initialized")
	}

	// Prove
	preimage := "12345"
	body := `{"circuit_id":"` + gnarkzk.MiMCPreimageID + `","inputs":{"preimage":"` + preimage + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/zk/prove", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.zkProveGeneric(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("prove expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	proveOut := decodeResp(t, rec.Body.Bytes())
	proofHex, _ := proveOut["proof_hex"].(string)
	if proofHex == "" {
		t.Fatalf("expected proof_hex, got %v", proveOut)
	}

	hash := gnarkzk.ComputeMiMCHash(new(big.Int).SetInt64(12345))
	// Verify (happy)
	vBody := `{"circuit_id":"` + gnarkzk.MiMCPreimageID + `","public_inputs":{"hash":"` + hash.String() + `"},"proof_hex":"` + proofHex + `"}`
	vReq := httptest.NewRequest(http.MethodPost, "/v1/zk/verify", strings.NewReader(vBody))
	vRec := httptest.NewRecorder()
	s.zkVerifyGeneric(vRec, vReq)
	if vRec.Code != http.StatusOK {
		t.Fatalf("verify expected 200, got %d: %s", vRec.Code, vRec.Body.String())
	}
	vOut := decodeResp(t, vRec.Body.Bytes())
	if v, _ := vOut["valid"].(bool); !v {
		t.Fatalf("expected valid=true, got %v", vOut)
	}

	// Verify (tampered) — different hash → valid=false
	wrong := new(big.Int).Add(hash, big.NewInt(1))
	tBody := `{"circuit_id":"` + gnarkzk.MiMCPreimageID + `","public_inputs":{"hash":"` + wrong.String() + `"},"proof_hex":"` + proofHex + `"}`
	tReq := httptest.NewRequest(http.MethodPost, "/v1/zk/verify", strings.NewReader(tBody))
	tRec := httptest.NewRecorder()
	s.zkVerifyGeneric(tRec, tReq)
	if tRec.Code != http.StatusOK {
		t.Fatalf("verify tampered expected 200, got %d", tRec.Code)
	}
	tOut := decodeResp(t, tRec.Body.Bytes())
	if v, _ := tOut["valid"].(bool); v {
		t.Fatalf("expected valid=false for tampered input, got %v", tOut)
	}
}

// TestZKGeneric_ProveVerify_ModelInference — same round-trip against the
// second bundled circuit, exercising the multi-public-input path.
func TestZKGeneric_ProveVerify_ModelInference(t *testing.T) {
	s := newTestServer("")
	if s.zkReg == nil {
		t.Skip("zkReg not initialized")
	}
	modelCommit := big.NewInt(111)
	input := big.NewInt(222)
	outHash := gnarkzk.ComputeModelOutputHash(modelCommit, input)

	body := `{"circuit_id":"` + gnarkzk.ModelInferenceID + `","inputs":{"model_commit":"` + modelCommit.String() + `","input":"` + input.String() + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/zk/prove", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.zkProveGeneric(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("prove expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	pOut := decodeResp(t, rec.Body.Bytes())
	proofHex, _ := pOut["proof_hex"].(string)
	if proofHex == "" {
		t.Fatalf("expected proof_hex in %v", pOut)
	}

	vBody := `{"circuit_id":"` + gnarkzk.ModelInferenceID + `","public_inputs":{"model_commit":"` + modelCommit.String() + `","output_hash":"` + outHash.String() + `"},"proof_hex":"` + proofHex + `"}`
	vReq := httptest.NewRequest(http.MethodPost, "/v1/zk/verify", strings.NewReader(vBody))
	vRec := httptest.NewRecorder()
	s.zkVerifyGeneric(vRec, vReq)
	if vRec.Code != http.StatusOK {
		t.Fatalf("verify expected 200, got %d: %s", vRec.Code, vRec.Body.String())
	}
	if v, _ := decodeResp(t, vRec.Body.Bytes())["valid"].(bool); !v {
		t.Fatalf("expected valid=true")
	}
}

// TestZKGeneric_MissingInput_400
func TestZKGeneric_MissingInput_400(t *testing.T) {
	s := newTestServer("")
	if s.zkReg == nil {
		t.Skip("zkReg not initialized")
	}
	// MiMCPreimage requires "preimage"; sending empty inputs should 400.
	body := `{"circuit_id":"` + gnarkzk.MiMCPreimageID + `","inputs":{"unrelated":"1"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/zk/prove", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.zkProveGeneric(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestZKGeneric_UnknownCircuit_404
func TestZKGeneric_UnknownCircuit_404(t *testing.T) {
	s := newTestServer("")
	if s.zkReg == nil {
		t.Skip("zkReg not initialized")
	}
	body := `{"circuit_id":"nope","inputs":{"preimage":"1"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/zk/prove", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.zkProveGeneric(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestZKGeneric_DefaultCircuitApplied — omitting circuit_id uses the
// registry default (MiMCPreimage — first registered).
func TestZKGeneric_DefaultCircuitApplied(t *testing.T) {
	s := newTestServer("")
	if s.zkReg == nil {
		t.Skip("zkReg not initialized")
	}
	body := `{"inputs":{"preimage":"7"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/zk/prove", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.zkProveGeneric(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	out := decodeResp(t, rec.Body.Bytes())
	if id, _ := out["circuit_id"].(string); id != gnarkzk.MiMCPreimageID {
		t.Fatalf("expected default circuit_id=%s, got %v", gnarkzk.MiMCPreimageID, out)
	}
}
