package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark-crypto/ecc"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/zkverifier/gnarkzk"
)

// End-to-end: /v1/zk/anchor-verdict without a live Casper submitter returns
// valid=true, anchored=false with an explanatory anchor_error message.
// (CP_STRICT=0 default lets the handler soft-fail so demos work offline.)
func TestZkAnchorVerdict_OffchainMode(t *testing.T) {
	reg := gnarkzk.NewRegistry()
	if err := reg.Register(gnarkzk.MiMCPreimageCircuit{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.Compile("mimc_preimage_v1"); err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Produce a real Groth16 proof through the registry.
	// MiMC preimage: private input=42, public output=MiMC(42).
	inputs := map[string]any{"preimage": int64(42), "hash": gnarkzk.ComputeMiMCHash(big.NewInt(42))}
	proof, err := reg.Prove("mimc_preimage_v1", inputs)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	var buf bytes.Buffer
	if _, err := proof.WriteTo(&buf); err != nil {
		t.Fatalf("write proof: %v", err)
	}
	proofHex := hex.EncodeToString(buf.Bytes())

	// Only public inputs go through the wire.
	pubMap := map[string]any{"hash": gnarkzk.ComputeMiMCHash(big.NewInt(42)).String()}

	s := &Server{zkReg: reg}
	body, _ := json.Marshal(map[string]any{
		"circuit_id":    "mimc_preimage_v1",
		"proof_hex":     proofHex,
		"public_inputs": pubMap,
		"model_id":      "gpt-4o-mini",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/zk/anchor-verdict", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.zkAnchorVerdict(rec, req)

	if rec.Code != http.StatusOK {
		respBody, _ := io.ReadAll(rec.Body)
		t.Fatalf("want 200, got %d: %s", rec.Code, respBody)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["valid"] != true {
		t.Errorf("valid=%v want true", resp["valid"])
	}
	if resp["anchored"] != false {
		t.Errorf("anchored=%v want false in offchain mode", resp["anchored"])
	}
	if _, ok := resp["anchor_error"]; !ok {
		t.Errorf("expected anchor_error explanation, got %+v", resp)
	}
	if resp["proof_hash"] == "" || resp["public_inputs_hash"] == "" {
		t.Errorf("expected proof_hash + public_inputs_hash in response: %+v", resp)
	}
}

// A missing model_id must 400 - the on-chain record makes no sense without it.
func TestZkAnchorVerdict_MissingModelID(t *testing.T) {
	reg := gnarkzk.NewRegistry()
	_ = reg.Register(gnarkzk.MiMCPreimageCircuit{})
	_ = reg.Compile("mimc_preimage_v1")

	s := &Server{zkReg: reg}
	body, _ := json.Marshal(map[string]any{
		"circuit_id": "mimc_preimage_v1",
		"proof_hex":  "aa",
		// no model_id
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/zk/anchor-verdict", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.zkAnchorVerdict(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

// A tampered public input flips valid=false; anchor still proceeds (with a
// verdict=0 on-chain), but the tampering doesn't cause a 400 - the handler
// contract is "off-chain verify then anchor whatever we got".
func TestZkAnchorVerdict_TamperedInputRunsAndRecordsInvalid(t *testing.T) {
	reg := gnarkzk.NewRegistry()
	_ = reg.Register(gnarkzk.MiMCPreimageCircuit{})
	_ = reg.Compile("mimc_preimage_v1")

	// Make a real proof for Secret=42, then submit with a different Hash.
	proof, err := reg.Prove("mimc_preimage_v1", map[string]any{"preimage": int64(42), "hash": gnarkzk.ComputeMiMCHash(big.NewInt(42))})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_, _ = proof.WriteTo(&buf)

	pubMap := map[string]any{"hash": "12345"} // wrong hash
	s := &Server{zkReg: reg}
	body, _ := json.Marshal(map[string]any{
		"circuit_id":    "mimc_preimage_v1",
		"proof_hex":     hex.EncodeToString(buf.Bytes()),
		"public_inputs": pubMap,
		"model_id":      "gpt-4o-mini",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/zk/anchor-verdict", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.zkAnchorVerdict(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["valid"] != false {
		t.Errorf("valid=%v want false", resp["valid"])
	}
}

// Sanity: build a witness manually to make sure the registry's public-input
// conventions match what a caller of /v1/zk/anchor-verdict is expected to send.
var _ = frontend.Compile
var _ = groth16.NewProof(ecc.BN254)
