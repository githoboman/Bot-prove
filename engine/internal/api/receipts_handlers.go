package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/decision/attest"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/hitl"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/receipts"
)

// initReceiptsService wires the provenance-lineage receipt service.
// Prerequisite: the keystore is always populated (memory backend is
// the default). The service is safe to construct even when the
// decision pipeline is disabled — /v1/receipts/emit accepts a
// caller-supplied commit and never depends on decisionRouter.
//
// If CP_RECEIPTS_JSONL is set, a JSONLSink is attached; failure to
// open the file is logged but does not disable receipts (falls back
// to the noop sink) — a missing OTel drain is degraded observability,
// not a production stop.
func (s *Server) initReceiptsService() {
	store := receipts.NewInMemoryStore()
	svc := receipts.NewService(store, s.keystore)
	if did := os.Getenv("CP_RECEIPTS_ISSUER_DID"); did != "" {
		svc.IssuerDID = did
	}
	if path := os.Getenv("CP_RECEIPTS_JSONL"); path != "" {
		sink, err := receipts.NewJSONLSink(path)
		if err != nil {
			s.log.Warn("receipts JSONL sink open failed — using noop sink", "err", err, "path", path)
		} else {
			svc.Sink = sink
			s.receiptSink = sink
			s.log.Info("receipts JSONL sink attached", "path", path)
		}
	}
	s.receipts = svc
	s.log.Info("receipts service enabled")
}

// registerReceiptRoutes wires the /v1/receipts/* endpoints.
func (s *Server) registerReceiptRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/receipts/emit", s.receiptsEmit)
	mux.HandleFunc("GET /v1/receipts/{id}", s.receiptsGet)
	mux.HandleFunc("GET /v1/receipts/{id}/lineage", s.receiptsLineage)
	mux.HandleFunc("GET /v1/receipts/{id}/w3c-vc", s.receiptsW3CVC)
	mux.HandleFunc("GET /v1/receipts/{id}/agent-receipt", s.receiptsAgentReceipt)
}

// receiptsEmitRequest is the wire form of POST /v1/receipts/emit.
//
// The caller supplies a fully-formed decision commit — either the
// wire form of attest.DecisionCommit (with facets already
// evaluated) or the reference to a decision that has been evaluated
// through /v1/decision/evaluate. The service does NOT re-run the
// pool; that would let the caller substitute a different commit for
// the one the receipt signs. Instead, the emit path binds whatever
// commit the caller supplies.
type receiptsEmitRequest struct {
	// Commit is required. Every field must match the shape decision
	// package produces — facets, aggregate, submitter, spec_id, etc.
	Commit receiptsCommitWire `json:"commit"`
	// EvidenceRoot is optional (hex-encoded, 32-byte merkle root).
	EvidenceRoot string `json:"evidence_root,omitempty"`
	// ModelID is optional (hex or opaque string).
	ModelID string `json:"model_id,omitempty"`
	// Providers is optional lineage: upstream receipts consulted.
	Providers []providerReceiptWire `json:"provider_receipts,omitempty"`
	// HITL is optional; supply when the emit path is downstream of a
	// human-review resolution.
	HITL *hitlWire `json:"hitl,omitempty"`
}

type receiptsCommitWire struct {
	Submitter     string           `json:"submitter"`
	SpecID        string           `json:"spec_id"`
	PayloadHex    string           `json:"payload_hex"`
	Nonce         uint64           `json:"nonce"`
	Aggregate     string           `json:"aggregate"`
	VetoedBy      string           `json:"vetoed_by,omitempty"`
	AbstainReason string           `json:"abstain_reason,omitempty"`
	Facets        []facetWire      `json:"facets"`
}

type facetWire struct {
	Kind       string  `json:"kind"`
	Verdict    string  `json:"verdict"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason,omitempty"`
}

type providerReceiptWire struct {
	Provider    string `json:"provider"`
	TrustLevel  string `json:"trust_level"`
	ReceiptHash string `json:"receipt_hash"`
}

type hitlWire struct {
	Action   string `json:"action"`
	Reason   string `json:"reason"`
	TicketID string `json:"ticket_id,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`
}

// receiptsEmit accepts a commit + optional lineage, signs a receipt,
// stores it, and returns the canonical receipt JSON.
func (s *Server) receiptsEmit(w http.ResponseWriter, r *http.Request) {
	if s.receipts == nil {
		s.jsonError(w, "receipts service disabled (set CP_RECEIPTS_ENABLE=1)", http.StatusServiceUnavailable)
		return
	}
	var req receiptsEmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid json body: "+err.Error(), http.StatusBadRequest)
		return
	}
	commit, err := commitFromWire(req.Commit)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	in := receipts.EmitInput{
		Commit:       commit,
		EvidenceRoot: req.EvidenceRoot,
		ModelID:      req.ModelID,
	}
	for _, p := range req.Providers {
		in.ProviderReceipts = append(in.ProviderReceipts, receipts.ProviderReceipt{
			Provider:    p.Provider,
			TrustLevel:  p.TrustLevel,
			ReceiptHash: p.ReceiptHash,
		})
	}
	if req.HITL != nil {
		in.HITL = &hitl.Response{
			Action:   hitl.Action(req.HITL.Action),
			Reason:   req.HITL.Reason,
			TicketID: req.HITL.TicketID,
		}
		in.Reviewer = req.HITL.Reviewer
	}
	rec, err := s.receipts.Emit(r.Context(), in)
	if err != nil {
		// Emit returns partial receipt + non-nil err only when the
		// sink write fails. Store still succeeded — surface as 200
		// with a warning header so a downstream doesn't retry and
		// double-store.
		if rec.ID != "" {
			w.Header().Set("X-CP-Receipt-Warning", err.Error())
			s.writeJSON(w, http.StatusOK, rec)
			return
		}
		s.jsonError(w, "emit: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusCreated, rec)
}

// receiptsGet returns the canonical receipt JSON by id.
func (s *Server) receiptsGet(w http.ResponseWriter, r *http.Request) {
	if s.receipts == nil {
		s.jsonError(w, "receipts service disabled", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	rec, err := s.receipts.Store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, receipts.ErrNotFound) {
			s.jsonError(w, "receipt not found", http.StatusNotFound)
			return
		}
		s.jsonError(w, "store: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, rec)
}

// receiptsLineage returns the upstream lineage graph — one JSON object
// containing the root receipt and its ancestors, up to depth
// max_depth (default 8, hard cap 32).
func (s *Server) receiptsLineage(w http.ResponseWriter, r *http.Request) {
	if s.receipts == nil {
		s.jsonError(w, "receipts service disabled", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	depth := 8
	if v := r.URL.Query().Get("max_depth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			depth = n
		}
	}
	if depth > 32 {
		depth = 32
	}
	root, err := s.receipts.Store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, receipts.ErrNotFound) {
			s.jsonError(w, "receipt not found", http.StatusNotFound)
			return
		}
		s.jsonError(w, "store: "+err.Error(), http.StatusInternalServerError)
		return
	}
	ancs, err := s.receipts.Store.Ancestors(r.Context(), id, depth)
	if err != nil {
		s.jsonError(w, "lineage: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"root":     root,
		"ancestors": ancs,
		"depth":    depth,
	})
}

// receiptsW3CVC returns the W3C Verifiable Credentials 2.0 shape.
func (s *Server) receiptsW3CVC(w http.ResponseWriter, r *http.Request) {
	rec, ok := s.loadReceipt(w, r)
	if !ok {
		return
	}
	vc, err := receipts.ToW3CVC(rec)
	if err != nil {
		s.jsonError(w, "w3c-vc: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, vc)
}

// receiptsAgentReceipt returns the agentreceipts.org draft shape.
func (s *Server) receiptsAgentReceipt(w http.ResponseWriter, r *http.Request) {
	rec, ok := s.loadReceipt(w, r)
	if !ok {
		return
	}
	ar, err := receipts.ToAgentReceipt(rec)
	if err != nil {
		s.jsonError(w, "agent-receipt: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, ar)
}

func (s *Server) loadReceipt(w http.ResponseWriter, r *http.Request) (receipts.DecisionReceipt, bool) {
	if s.receipts == nil {
		s.jsonError(w, "receipts service disabled", http.StatusServiceUnavailable)
		return receipts.DecisionReceipt{}, false
	}
	id := r.PathValue("id")
	rec, err := s.receipts.Store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, receipts.ErrNotFound) {
			s.jsonError(w, "receipt not found", http.StatusNotFound)
			return receipts.DecisionReceipt{}, false
		}
		s.jsonError(w, "store: "+err.Error(), http.StatusInternalServerError)
		return receipts.DecisionReceipt{}, false
	}
	return rec, true
}

func commitFromWire(c receiptsCommitWire) (attest.DecisionCommit, error) {
	payload, err := hex.DecodeString(c.PayloadHex)
	if err != nil {
		return attest.DecisionCommit{}, err
	}
	verdict, err := parseVerdict(c.Aggregate)
	if err != nil {
		return attest.DecisionCommit{}, err
	}
	dec := attest.Decision{
		Submitter: c.Submitter,
		SpecID:    c.SpecID,
		Payload:   payload,
		Nonce:     c.Nonce,
	}
	fvs := make([]attest.FacetVerdict, 0, len(c.Facets))
	for _, f := range c.Facets {
		v, err := parseVerdict(f.Verdict)
		if err != nil {
			return attest.DecisionCommit{}, err
		}
		fvs = append(fvs, attest.FacetVerdict{
			Kind:       attest.FacetKind(f.Kind),
			Verdict:    v,
			Confidence: f.Confidence,
			Reason:     f.Reason,
		})
	}
	return attest.DecisionCommit{
		Decision:      dec,
		DecisionID:    dec.ID(),
		FacetVerdicts: fvs,
		Aggregate:     verdict,
		VetoedBy:      attest.FacetKind(c.VetoedBy),
		AbstainReason: c.AbstainReason,
	}, nil
}

func parseVerdict(s string) (attest.Verdict, error) {
	switch s {
	case "APPROVE":
		return attest.VerdictApprove, nil
	case "ABSTAIN":
		return attest.VerdictAbstain, nil
	case "REJECT":
		return attest.VerdictReject, nil
	default:
		return attest.VerdictUnknown, errors.New("unknown verdict: " + s)
	}
}

// writeJSON is the standard success writer for handlers that don't
// share the request-scoped helpers.
func (s *Server) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		s.log.Warn("json encode failed", "err", err)
	}
}

// closeReceipts released the receipt sink if one is attached. It was never
// wired into a graceful-shutdown path (cmd/casperprover/main.go has none
// today), so it was always dead code per the linter. Removed rather than
// kept unused; re-add + wire to a signal handler if/when the server gains
// graceful shutdown.
