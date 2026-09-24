package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func buildMerkleMux(t *testing.T) *http.ServeMux {
	t.Helper()
	s := &Server{}
	mux := http.NewServeMux()
	s.registerMerkleRecursionRoutes(mux)
	return mux
}

func hexLeaves(n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = hex.EncodeToString([]byte(fmt.Sprintf("commit-%d", i)))
	}
	return out
}

func TestMerkleHTTP_AggregateHappyPath(t *testing.T) {
	mux := buildMerkleMux(t)
	body := map[string]any{"leaves_hex": hexLeaves(8)}
	resp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/merkle-aggregate", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("aggregate: %d body=%s", resp.Code, resp.Body.String())
	}
	var out struct {
		Scheme        string `json:"scheme"`
		Count         int    `json:"count"`
		TreeHeight    int    `json:"tree_height"`
		MerkleRootHex string `json:"merkle_root_hex"`
		Disclosure    string `json:"disclosure"`
	}
	_ = json.Unmarshal(resp.Body.Bytes(), &out)
	if out.Scheme != "merkle-recursion-v1" || out.Count != 8 || out.TreeHeight != 3 {
		t.Fatalf("bad shape: %+v", out)
	}
	if len(out.MerkleRootHex) != 64 || out.Disclosure == "" {
		t.Fatalf("bad shape: %+v", out)
	}
}

func TestMerkleHTTP_InclusionAndVerifyRoundTrip(t *testing.T) {
	mux := buildMerkleMux(t)
	leaves := hexLeaves(6)
	// Build aggregate.
	aggResp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/merkle-aggregate",
		map[string]any{"leaves_hex": leaves})
	if aggResp.Code != http.StatusOK {
		t.Fatalf("aggregate: %d", aggResp.Code)
	}
	var agg struct {
		Scheme        string `json:"scheme"`
		Count         int    `json:"count"`
		TreeHeight    int    `json:"tree_height"`
		MerkleRootHex string `json:"merkle_root_hex"`
	}
	_ = json.Unmarshal(aggResp.Body.Bytes(), &agg)

	// Build inclusion proof for index 2.
	incResp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/merkle-inclusion",
		map[string]any{"leaves_hex": leaves, "leaf_index": 2})
	if incResp.Code != http.StatusOK {
		t.Fatalf("inclusion: %d body=%s", incResp.Code, incResp.Body.String())
	}
	var proof struct {
		LeafIndex int      `json:"leaf_index"`
		LeafHex   string   `json:"leaf_hex"`
		PathHex   []string `json:"path_hex"`
		Positions []bool   `json:"positions"`
	}
	_ = json.Unmarshal(incResp.Body.Bytes(), &proof)
	if proof.LeafIndex != 2 || len(proof.PathHex) != agg.TreeHeight {
		t.Fatalf("bad inclusion: %+v", proof)
	}

	// Verify.
	verifyBody := map[string]any{
		"aggregate": agg,
		"proof":     proof,
	}
	vResp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/merkle-verify", verifyBody)
	if vResp.Code != http.StatusOK {
		t.Fatalf("verify: %d body=%s", vResp.Code, vResp.Body.String())
	}
	var vout struct {
		Valid  bool   `json:"valid"`
		Scheme string `json:"scheme"`
	}
	_ = json.Unmarshal(vResp.Body.Bytes(), &vout)
	if !vout.Valid || vout.Scheme != "merkle-recursion-v1" {
		t.Fatalf("verify failed: %+v — body=%s", vout, vResp.Body.String())
	}
}

func TestMerkleHTTP_VerifyRejectsTamperedRoot(t *testing.T) {
	mux := buildMerkleMux(t)
	leaves := hexLeaves(4)
	aggResp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/merkle-aggregate",
		map[string]any{"leaves_hex": leaves})
	var agg map[string]any
	_ = json.Unmarshal(aggResp.Body.Bytes(), &agg)

	incResp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/merkle-inclusion",
		map[string]any{"leaves_hex": leaves, "leaf_index": 1})
	var proof map[string]any
	_ = json.Unmarshal(incResp.Body.Bytes(), &proof)

	// Tamper the root.
	root, ok := agg["merkle_root_hex"].(string)
	if !ok {
		t.Fatalf("merkle_root_hex: expected string, got %T", agg["merkle_root_hex"])
	}
	rb, _ := hex.DecodeString(root)
	rb[0] ^= 0xff
	agg["merkle_root_hex"] = hex.EncodeToString(rb)

	vResp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/merkle-verify",
		map[string]any{"aggregate": agg, "proof": proof})
	if vResp.Code != http.StatusOK {
		t.Fatalf("verify http: %d", vResp.Code)
	}
	var vout struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(vResp.Body.Bytes(), &vout)
	if vout.Valid || vout.Error == "" {
		t.Fatalf("tampered root accepted: %+v", vout)
	}
}

func TestMerkleHTTP_InclusionOutOfRange(t *testing.T) {
	mux := buildMerkleMux(t)
	leaves := hexLeaves(3)
	resp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/merkle-inclusion",
		map[string]any{"leaves_hex": leaves, "leaf_index": 99})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestMerkleHTTP_RejectsBadHex(t *testing.T) {
	mux := buildMerkleMux(t)
	resp := keyringDoJSON(t, mux, "POST", "/v1/aggregation/merkle-aggregate",
		map[string]any{"leaves_hex": []string{"not-hex"}})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}
