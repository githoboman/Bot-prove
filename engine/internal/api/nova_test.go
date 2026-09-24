package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
)

func buildNovaMux(t *testing.T) *http.ServeMux {
	t.Helper()
	s := &Server{}
	mux := http.NewServeMux()
	s.registerNovaRoutes(mux)
	return mux
}

func TestNova_FoldAndVerify_RoundTrip(t *testing.T) {
	mux := buildNovaMux(t)

	body := map[string]any{
		"steps": []map[string]any{
			{"instance": "step-a", "instance_utf8": true, "witness_digest": "01020304"},
			{"instance": "step-b", "instance_utf8": true, "witness_digest": "abcd"},
			{"instance": hex.EncodeToString([]byte("step-c")), "witness_digest": "ffeeddcc"},
		},
	}
	resp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/fold", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("fold: %d body=%s", resp.Code, resp.Body.String())
	}
	var agg struct {
		Scheme        string   `json:"scheme"`
		Steps         int      `json:"steps"`
		Root          string   `json:"root_hex"`
		StepHashes    []string `json:"step_hashes_hex"`
		Disclosure    string   `json:"disclosure"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &agg); err != nil {
		t.Fatalf("decode fold: %v", err)
	}
	if agg.Scheme != "hash-fold-v1" {
		t.Fatalf("wrong scheme label: %s", agg.Scheme)
	}
	if agg.Steps != 3 || len(agg.StepHashes) != 3 || len(agg.Root) != 64 {
		t.Fatalf("bad aggregate shape: %+v", agg)
	}
	if agg.Disclosure == "" {
		t.Fatalf("disclosure text must be present")
	}

	// Verify — should pass.
	verifyBody := map[string]any{
		"steps":     body["steps"],
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
	var verifyOut struct {
		Valid bool `json:"valid"`
	}
	_ = json.Unmarshal(resp.Body.Bytes(), &verifyOut)
	if !verifyOut.Valid {
		t.Fatalf("verify must return valid:true, got %s", resp.Body.String())
	}
}

func TestNova_VerifyRejectsTamperedRoot(t *testing.T) {
	mux := buildNovaMux(t)
	body := map[string]any{
		"steps": []map[string]any{
			{"instance": "s", "instance_utf8": true, "witness_digest": "11"},
			{"instance": "t", "instance_utf8": true, "witness_digest": "22"},
		},
	}
	resp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/fold", body)
	var agg struct {
		Scheme     string   `json:"scheme"`
		Steps      int      `json:"steps"`
		Root       string   `json:"root_hex"`
		StepHashes []string `json:"step_hashes_hex"`
	}
	_ = json.Unmarshal(resp.Body.Bytes(), &agg)

	// Flip a byte of the root.
	tamperedRoot := "00" + agg.Root[2:]
	verifyBody := map[string]any{
		"steps":     body["steps"],
		"aggregate": map[string]any{
			"scheme":          agg.Scheme,
			"steps":           agg.Steps,
			"root_hex":        tamperedRoot,
			"step_hashes_hex": agg.StepHashes,
		},
	}
	resp = keyringDoJSON(t, mux, "POST", "/v1/aggregation/verify-fold", verifyBody)
	if resp.Code != http.StatusOK {
		t.Fatalf("verify: %d body=%s", resp.Code, resp.Body.String())
	}
	var out struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(resp.Body.Bytes(), &out)
	if out.Valid {
		t.Fatalf("tampered root must yield valid:false")
	}
	if out.Error == "" {
		t.Fatalf("verify must report an error string on tamper")
	}
}

func TestNova_FoldRejectsEmpty(t *testing.T) {
	mux := buildNovaMux(t)
	resp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/fold", map[string]any{"steps": []any{}})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("empty steps must be 400, got %d", resp.Code)
	}
}

func TestNova_FoldRejectsBadHex(t *testing.T) {
	mux := buildNovaMux(t)
	resp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/fold", map[string]any{
		"steps": []map[string]any{
			{"instance": "not-hex-and-not-flagged", "witness_digest": "01"},
		},
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("non-hex instance without utf8 flag must be 400, got %d body=%s", resp.Code, resp.Body.String())
	}
}
