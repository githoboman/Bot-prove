package api

// Merkle-recursion aggregation HTTP surface (Pack AS, backlog #2.12).
//
// Endpoints (public, always registered — no env gate — so the routes
// are always discoverable via /v1/routes; the scheme label makes the
// contract explicit):
//
//   POST /v1/aggregation/merkle-aggregate    build a Merkle tree over
//                                            leaf digests and return
//                                            (root, count, height)
//   POST /v1/aggregation/merkle-inclusion    build an O(log n)
//                                            inclusion proof for one
//                                            leaf index
//   POST /v1/aggregation/merkle-verify       verify a supplied
//                                            inclusion proof against
//                                            an aggregate root
//
// Scheme label on the wire: "merkle-recursion-v1" — see
// docs/MERKLE_RECURSION.md for the honesty disclosure (this is NOT
// real STARK recursion; membership check is O(log n) hashes, not a
// re-execution of the underlying proof).

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/aggregator"
)

// registerMerkleRecursionRoutes wires the /v1/aggregation/merkle-* endpoints.
func (s *Server) registerMerkleRecursionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/aggregation/merkle-aggregate", s.merkleAggregate)
	mux.HandleFunc("POST /v1/aggregation/merkle-inclusion", s.merkleInclusion)
	mux.HandleFunc("POST /v1/aggregation/merkle-verify", s.merkleVerify)
}

// merkleAggregateRequest carries leaves as hex strings.
type merkleAggregateRequest struct {
	Leaves []string `json:"leaves_hex"`
}

func decodeLeaves(hexes []string) ([][]byte, error) {
	if len(hexes) == 0 {
		return nil, errors.New("leaves_hex is required and must be non-empty")
	}
	out := make([][]byte, len(hexes))
	for i, s := range hexes {
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, errors.New("leaves_hex[" + itoa(i) + "] is not valid hex")
		}
		if len(b) == 0 {
			return nil, errors.New("leaves_hex[" + itoa(i) + "] is empty")
		}
		out[i] = b
	}
	return out, nil
}

// merkleAggregate builds the tree.
func (s *Server) merkleAggregate(w http.ResponseWriter, r *http.Request) {
	var req merkleAggregateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	leaves, err := decodeLeaves(req.Leaves)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	agg, err := aggregator.AggregateMerkleRecursion(leaves)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"scheme":          agg.Scheme,
		"count":           agg.Count,
		"tree_height":     agg.TreeHeight,
		"merkle_root_hex": agg.RootHex,
		"disclosure":      "merkle-recursion-v1 is a Merkle tree over proof-commitment digests. Membership is O(log n) SHA-256 hashes, NOT a re-execution of the underlying proof. See docs/MERKLE_RECURSION.md.",
	})
}

// merkleInclusionRequest asks for an inclusion path for one leaf.
type merkleInclusionRequest struct {
	Leaves    []string `json:"leaves_hex"`
	LeafIndex int      `json:"leaf_index"`
}

func (s *Server) merkleInclusion(w http.ResponseWriter, r *http.Request) {
	var req merkleInclusionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	leaves, err := decodeLeaves(req.Leaves)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	proof, err := aggregator.BuildInclusionProof(leaves, req.LeafIndex)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"leaf_index": proof.LeafIndex,
		"leaf_hex":   proof.LeafHex,
		"path_hex":   proof.Path,
		"positions":  proof.Positions,
	})
}

// merkleVerifyRequest bundles an aggregate + inclusion proof for a
// stateless verifier.
type merkleVerifyRequest struct {
	Aggregate struct {
		Scheme        string `json:"scheme"`
		Count         int    `json:"count"`
		TreeHeight    int    `json:"tree_height"`
		MerkleRootHex string `json:"merkle_root_hex"`
	} `json:"aggregate"`
	Proof struct {
		LeafIndex int      `json:"leaf_index"`
		LeafHex   string   `json:"leaf_hex"`
		PathHex   []string `json:"path_hex"`
		Positions []bool   `json:"positions"`
	} `json:"proof"`
}

func (s *Server) merkleVerify(w http.ResponseWriter, r *http.Request) {
	var req merkleVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	agg := aggregator.MerkleRecursionProof{
		Scheme:     req.Aggregate.Scheme,
		Count:      req.Aggregate.Count,
		TreeHeight: req.Aggregate.TreeHeight,
		RootHex:    req.Aggregate.MerkleRootHex,
	}
	proof := aggregator.MerkleInclusionProof{
		LeafIndex: req.Proof.LeafIndex,
		LeafHex:   req.Proof.LeafHex,
		Path:      req.Proof.PathHex,
		Positions: req.Proof.Positions,
	}
	valid, verr := aggregator.VerifyMerkleInclusion(agg, proof)
	resp := map[string]any{"valid": valid, "scheme": agg.Scheme}
	if verr != nil {
		resp["error"] = verr.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
