package api

// Nova / folding aggregation HTTP surface.
//
// Endpoints:
//
//   POST /v1/aggregation/fold           — fold a sequence of steps into one AggregateProof
//   POST /v1/aggregation/verify-fold    — reconstruct + check an AggregateProof against its steps
//
// The default folder is the hash-chain HashFolder in internal/aggregator.
// The public response ALWAYS labels its scheme (currently "hash-fold-v1")
// so downstream code can tell a real Nova aggregate from this stand-in
// once a real folding scheme is wired.
//
// See docs/roadmap/NOVA_HARNESS.md for the disclosure.

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/aggregator"
)

// foldStepDTO is the wire form of aggregator.FoldStep: hex strings instead
// of raw bytes so JSON stays legible.
type foldStepDTO struct {
	Instance      string `json:"instance"`       // hex OR plain — see decodeFoldStep
	InstanceUTF8  bool   `json:"instance_utf8,omitempty"`
	WitnessDigest string `json:"witness_digest"` // hex only
}

func decodeFoldStep(d foldStepDTO) (aggregator.FoldStep, error) {
	if d.Instance == "" {
		return aggregator.FoldStep{}, errors.New("instance is required")
	}
	if d.WitnessDigest == "" {
		return aggregator.FoldStep{}, errors.New("witness_digest is required (hex)")
	}
	var inst []byte
	if d.InstanceUTF8 {
		inst = []byte(d.Instance)
	} else {
		b, err := hex.DecodeString(d.Instance)
		if err != nil {
			return aggregator.FoldStep{}, errors.New("instance must be hex-encoded (or set instance_utf8=true)")
		}
		inst = b
	}
	wit, err := hex.DecodeString(d.WitnessDigest)
	if err != nil {
		return aggregator.FoldStep{}, errors.New("witness_digest must be hex-encoded")
	}
	return aggregator.FoldStep{Instance: inst, WitnessDigest: wit}, nil
}

// novaFold aggregates a step sequence.
//
// The optional "scheme" field selects between "hash-fold-v1" (default,
// hash-chain stand-in) and "pedersen-fold-v1" (BLS12-381 G1 Pedersen
// commitment sum — intermediate cryptographic upgrade, still not Nova).
// See docs/roadmap/NOVA_HARNESS.md and docs/PEDERSEN_FOLD.md.
func (s *Server) novaFold(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scheme string        `json:"scheme,omitempty"`
		Steps  []foldStepDTO `json:"steps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if len(req.Steps) == 0 {
		s.jsonError(w, "steps must be non-empty", http.StatusBadRequest)
		return
	}
	steps := make([]aggregator.FoldStep, 0, len(req.Steps))
	for i, dto := range req.Steps {
		fs, err := decodeFoldStep(dto)
		if err != nil {
			s.jsonError(w, "step "+itoa(i)+": "+err.Error(), http.StatusBadRequest)
			return
		}
		steps = append(steps, fs)
	}
	scheme := aggregator.FoldingScheme(req.Scheme)
	if scheme == "" {
		scheme = aggregator.SchemeHashFoldV1
	}
	var (
		agg aggregator.AggregateProof
		err error
		disclosure string
	)
	switch scheme {
	case aggregator.SchemeHashFoldV1:
		agg, err = aggregator.FoldAll(steps)
		disclosure = "hash-fold-v1 is a hash-chain stand-in, NOT a cryptographic folding scheme — see docs/roadmap/NOVA_HARNESS.md"
	case aggregator.SchemePedersenFoldV1:
		agg, err = aggregator.FoldAllPedersen(steps)
		disclosure = "pedersen-fold-v1 is a real BLS12-381 G1 Pedersen commitment sum — hiding + binding under DLP, homomorphic across splits, but NOT a Nova folding scheme (does not reduce R1CS instances). See docs/PEDERSEN_FOLD.md."
	default:
		s.jsonError(w, "unsupported scheme: "+string(scheme)+" (want hash-fold-v1 or pedersen-fold-v1)", http.StatusBadRequest)
		return
	}
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"scheme":          agg.Scheme,
		"steps":           agg.Steps,
		"root_hex":        agg.Root,
		"step_hashes_hex": agg.StepHashes,
		"disclosure":      disclosure,
	})
}

// novaVerifyFold reconstructs the accumulator from steps and compares.
func (s *Server) novaVerifyFold(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Steps     []foldStepDTO `json:"steps"`
		Aggregate struct {
			Scheme     string   `json:"scheme"`
			Steps      int      `json:"steps"`
			Root       string   `json:"root_hex"`
			StepHashes []string `json:"step_hashes_hex"`
		} `json:"aggregate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if len(req.Steps) == 0 {
		s.jsonError(w, "steps must be non-empty", http.StatusBadRequest)
		return
	}
	steps := make([]aggregator.FoldStep, 0, len(req.Steps))
	for i, dto := range req.Steps {
		fs, err := decodeFoldStep(dto)
		if err != nil {
			s.jsonError(w, "step "+itoa(i)+": "+err.Error(), http.StatusBadRequest)
			return
		}
		steps = append(steps, fs)
	}
	agg := aggregator.AggregateProof{
		Scheme:     aggregator.FoldingScheme(req.Aggregate.Scheme),
		Steps:      req.Aggregate.Steps,
		Root:       req.Aggregate.Root,
		StepHashes: req.Aggregate.StepHashes,
	}
	var (
		valid bool
		verr  error
	)
	switch agg.Scheme {
	case aggregator.SchemePedersenFoldV1:
		valid, verr = aggregator.VerifyAllPedersen(steps, agg)
	case aggregator.SchemeHashFoldV1, "":
		if agg.Scheme == "" {
			agg.Scheme = aggregator.SchemeHashFoldV1
		}
		valid, verr = aggregator.VerifyAll(steps, agg)
	default:
		s.jsonError(w, "unsupported scheme: "+string(agg.Scheme), http.StatusBadRequest)
		return
	}
	resp := map[string]any{"valid": valid, "scheme": string(agg.Scheme)}
	if verr != nil {
		resp["error"] = verr.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) registerNovaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/aggregation/fold", s.novaFold)
	mux.HandleFunc("POST /v1/aggregation/verify-fold", s.novaVerifyFold)
}

// itoa is a local single-digit-ish helper; keeps this file dep-free of strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
