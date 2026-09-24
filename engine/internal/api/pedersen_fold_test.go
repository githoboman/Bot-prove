package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestPedersenFold_HTTP_RoundTrip drives the /v1/aggregation/fold
// endpoint with scheme="pedersen-fold-v1" end-to-end and re-checks
// with /v1/aggregation/verify-fold.
func TestPedersenFold_HTTP_RoundTrip(t *testing.T) {
	mux := buildNovaMux(t)

	body := map[string]any{
		"scheme": "pedersen-fold-v1",
		"steps": []map[string]any{
			{"instance": "alpha", "instance_utf8": true, "witness_digest": "01020304"},
			{"instance": "beta", "instance_utf8": true, "witness_digest": "abcd"},
			{"instance": "gamma", "instance_utf8": true, "witness_digest": "ffeeddcc"},
		},
	}
	resp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/fold", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("fold: %d body=%s", resp.Code, resp.Body.String())
	}
	var agg struct {
		Scheme     string   `json:"scheme"`
		Steps      int      `json:"steps"`
		Root       string   `json:"root_hex"`
		StepHashes []string `json:"step_hashes_hex"`
		Disclosure string   `json:"disclosure"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &agg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if agg.Scheme != "pedersen-fold-v1" {
		t.Fatalf("scheme = %q, want pedersen-fold-v1", agg.Scheme)
	}
	if agg.Steps != 3 || len(agg.StepHashes) != 3 {
		t.Fatalf("bad shape: %+v", agg)
	}
	// BLS12-381 G1 compressed = 48 bytes = 96 hex chars.
	if len(agg.Root) != 96 {
		t.Fatalf("root_hex len = %d, want 96 (48-byte compressed G1)", len(agg.Root))
	}
	if agg.Disclosure == "" {
		t.Fatal("disclosure must be present")
	}

	verifyBody := map[string]any{
		"steps": body["steps"],
		"aggregate": map[string]any{
			"scheme":          agg.Scheme,
			"steps":           agg.Steps,
			"root_hex":        agg.Root,
			"step_hashes_hex": agg.StepHashes,
		},
	}
	resp = keyringDoJSON(t, mux, "POST", "/v1/aggregation/verify-fold", verifyBody)
	if resp.Code != http.StatusOK {
		t.Fatalf("verify: %d body=%s", resp.Code, resp.Body.String())
	}
	var vout struct {
		Valid  bool   `json:"valid"`
		Scheme string `json:"scheme"`
	}
	_ = json.Unmarshal(resp.Body.Bytes(), &vout)
	if !vout.Valid || vout.Scheme != "pedersen-fold-v1" {
		t.Fatalf("verify: %+v — body=%s", vout, resp.Body.String())
	}
}

// TestPedersenFold_HTTP_RejectsUnknownScheme guards the label surface.
func TestPedersenFold_HTTP_RejectsUnknownScheme(t *testing.T) {
	mux := buildNovaMux(t)
	body := map[string]any{
		"scheme": "made-up-scheme-v99",
		"steps": []map[string]any{
			{"instance": "x", "instance_utf8": true, "witness_digest": "aa"},
		},
	}
	resp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/fold", body)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

// TestPedersenFold_HTTP_HashFoldStillDefault confirms the default path
// keeps working; scheme omission means hash-fold-v1.
func TestPedersenFold_HTTP_HashFoldStillDefault(t *testing.T) {
	mux := buildNovaMux(t)
	body := map[string]any{
		"steps": []map[string]any{
			{"instance": "x", "instance_utf8": true, "witness_digest": "aa"},
		},
	}
	resp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/fold", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("fold default: %d body=%s", resp.Code, resp.Body.String())
	}
	var agg struct {
		Scheme string `json:"scheme"`
	}
	_ = json.Unmarshal(resp.Body.Bytes(), &agg)
	if agg.Scheme != "hash-fold-v1" {
		t.Fatalf("default scheme = %q, want hash-fold-v1", agg.Scheme)
	}
}
