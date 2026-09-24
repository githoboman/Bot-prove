package api

// BLS12-381 threshold quorum HTTP surface (Pack AS).
//
// Endpoints (all under scope `quorum:*`, opt-in via CP_QUORUM_ENABLE=1):
//   POST /v1/quorum/signers                   register a new signer (quorum:write)
//   GET  /v1/quorum/signers                   list signers, optionally filtered (quorum:read)
//   POST /v1/quorum/signers/{id}/slash        mark signer slashed (quorum:write)
//   POST /v1/quorum/signers/{id}/retire       mark signer retired (quorum:write)
//   POST /v1/quorum/verify                    verify quorum witness (quorum:read)
//   GET  /v1/quorum/threshold                 recommended Byzantine threshold (quorum:read)
//
// When CP_QUORUM_ENABLE is unset every endpoint returns 503. The
// service is deliberately a "bring your own committee" surface: the
// registry starts empty and rejects every verify call until the
// operator registers signers.

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/quorum"
)

// registerQuorumRoutes wires the /v1/quorum/* endpoints.
func (s *Server) registerQuorumRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/quorum/signers", s.quorumRegisterSigner)
	mux.HandleFunc("GET /v1/quorum/signers", s.quorumListSigners)
	mux.HandleFunc("POST /v1/quorum/signers/{id}/slash", s.quorumSlashSigner)
	mux.HandleFunc("POST /v1/quorum/signers/{id}/retire", s.quorumRetireSigner)
	mux.HandleFunc("POST /v1/quorum/verify", s.quorumVerify)
	mux.HandleFunc("GET /v1/quorum/threshold", s.quorumThreshold)
}

// ---------------------------------------------------------------------------
// Request / response wire types.
// ---------------------------------------------------------------------------

type quorumRegisterRequest struct {
	ID     string `json:"id"`
	PubKey string `json:"public_key_hex"`
	Bond   uint64 `json:"bond,omitempty"`
}

type quorumSignerWire struct {
	ID           string `json:"id"`
	PubKey       string `json:"public_key_hex"`
	Status       string `json:"status"`
	Bond         uint64 `json:"bond"`
	RegisteredAt string `json:"registered_at"`
	SlashReason  string `json:"slash_reason,omitempty"`
}

func signerWireFromRec(rec *quorum.SignerRecord) quorumSignerWire {
	if rec == nil {
		return quorumSignerWire{}
	}
	return quorumSignerWire{
		ID: rec.ID, PubKey: rec.PublicKeyHex,
		Status: string(rec.Status), Bond: rec.Bond,
		RegisteredAt: rec.RegisteredAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		SlashReason:  rec.SlashReason,
	}
}

type quorumListResponse struct {
	Signers     []quorumSignerWire `json:"signers"`
	Total       int                `json:"total"`
	ActiveCount int                `json:"active_count"`
}

type quorumSlashRequest struct {
	Reason string `json:"reason,omitempty"`
}

type quorumVerifyRequest struct {
	EvidenceRootHex string   `json:"evidence_root_hex"`
	SignerBitset    []string `json:"signer_bitset"`
	AggregateSigHex string   `json:"aggregate_sig_hex"`
	Threshold       int      `json:"threshold,omitempty"`
}

type quorumThresholdResponse struct {
	ActiveCount int `json:"active_count"`
	Threshold   int `json:"recommended_threshold"`
	Formula     string `json:"formula"`
}

// ---------------------------------------------------------------------------
// Handlers.
// ---------------------------------------------------------------------------

// disabled returns true and writes a 503 when the quorum service is off.
func (s *Server) quorumDisabled(w http.ResponseWriter) bool {
	if s.quorumRegistry == nil {
		s.jsonError(w, "quorum service disabled (set CP_QUORUM_ENABLE=1)", http.StatusServiceUnavailable)
		return true
	}
	return false
}

func (s *Server) quorumRegisterSigner(w http.ResponseWriter, r *http.Request) {
	if s.quorumDisabled(w) {
		return
	}
	var req quorumRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid json body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		s.jsonError(w, "missing id", http.StatusBadRequest)
		return
	}
	pk, err := quorum.PubKeyFromHex(req.PubKey)
	if err != nil {
		s.jsonError(w, "invalid public_key_hex: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.quorumRegistry.Register(req.ID, pk, req.Bond); err != nil {
		if errors.Is(err, quorum.ErrDuplicateSigner) {
			s.jsonError(w, err.Error(), http.StatusConflict)
			return
		}
		// registry.Register wraps duplicate as fmt.Errorf (not sentinel);
		// still map "already registered" prose to 409 for the caller.
		if rec := s.quorumRegistry.Get(req.ID); rec != nil {
			s.jsonError(w, err.Error(), http.StatusConflict)
			return
		}
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusCreated, signerWireFromRec(s.quorumRegistry.Get(req.ID)))
}

func (s *Server) quorumListSigners(w http.ResponseWriter, r *http.Request) {
	if s.quorumDisabled(w) {
		return
	}
	filter := quorum.SignerStatus(r.URL.Query().Get("status"))
	list := s.quorumRegistry.List(filter)
	out := make([]quorumSignerWire, 0, len(list))
	for _, rec := range list {
		out = append(out, signerWireFromRec(rec))
	}
	s.writeJSON(w, http.StatusOK, quorumListResponse{
		Signers:     out,
		Total:       len(out),
		ActiveCount: s.quorumRegistry.ActiveCount(),
	})
}

func (s *Server) quorumSlashSigner(w http.ResponseWriter, r *http.Request) {
	if s.quorumDisabled(w) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		s.jsonError(w, "missing id", http.StatusBadRequest)
		return
	}
	var req quorumSlashRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // reason is optional
	if err := s.quorumRegistry.Slash(id, req.Reason); err != nil {
		if errors.Is(err, quorum.ErrUnknownSigner) {
			s.jsonError(w, err.Error(), http.StatusNotFound)
			return
		}
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, signerWireFromRec(s.quorumRegistry.Get(id)))
}

func (s *Server) quorumRetireSigner(w http.ResponseWriter, r *http.Request) {
	if s.quorumDisabled(w) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		s.jsonError(w, "missing id", http.StatusBadRequest)
		return
	}
	if err := s.quorumRegistry.Retire(id); err != nil {
		if errors.Is(err, quorum.ErrUnknownSigner) {
			s.jsonError(w, err.Error(), http.StatusNotFound)
			return
		}
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, http.StatusOK, signerWireFromRec(s.quorumRegistry.Get(id)))
}

// quorumVerify runs the full pairing-based quorum check and returns
// the witness (with a SHA-256 commitment) on success.
//
// The signature is a REAL BLS aggregate over BLS12-381. The verifier
// does e(H(msg), agg_pk) == e(agg_sig, G2) — not a hash-based
// stand-in. See docs/BLS_QUORUM.md.
func (s *Server) quorumVerify(w http.ResponseWriter, r *http.Request) {
	if s.quorumDisabled(w) {
		return
	}
	var req quorumVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid json body: "+err.Error(), http.StatusBadRequest)
		return
	}
	root, err := hex.DecodeString(req.EvidenceRootHex)
	if err != nil {
		s.jsonError(w, "invalid evidence_root_hex: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.SignerBitset) == 0 {
		s.jsonError(w, "signer_bitset required", http.StatusBadRequest)
		return
	}
	sig, err := quorum.SigFromHex(req.AggregateSigHex)
	if err != nil {
		s.jsonError(w, "invalid aggregate_sig_hex: "+err.Error(), http.StatusBadRequest)
		return
	}
	witness, err := quorum.VerifyQuorum(s.quorumRegistry, root, req.SignerBitset, sig, req.Threshold)
	if err != nil {
		// Map domain errors to the right HTTP class so the client can
		// distinguish "committee not big enough" from "signature bad"
		// from "unknown signer" without parsing prose.
		switch {
		case errors.Is(err, quorum.ErrPairingCheckFail):
			s.jsonError(w, err.Error(), http.StatusUnprocessableEntity)
		case errors.Is(err, quorum.ErrThresholdNotMet),
			errors.Is(err, quorum.ErrDuplicateSigner),
			errors.Is(err, quorum.ErrInactiveSigner):
			s.jsonError(w, err.Error(), http.StatusForbidden)
		case errors.Is(err, quorum.ErrUnknownSigner):
			s.jsonError(w, err.Error(), http.StatusNotFound)
		default:
			s.jsonError(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	s.writeJSON(w, http.StatusOK, witness)
}

func (s *Server) quorumThreshold(w http.ResponseWriter, r *http.Request) {
	if s.quorumDisabled(w) {
		return
	}
	n := s.quorumRegistry.ActiveCount()
	s.writeJSON(w, http.StatusOK, quorumThresholdResponse{
		ActiveCount: n,
		Threshold:   quorum.ByzantineThreshold(n),
		Formula:     "floor(2n/3) + 1, clamped to [1, n]",
	})
}

