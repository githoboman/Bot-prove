// Package api — HTTP surface for auditable decision logging (backlog 3.2).
//
// Endpoints exposed:
//
//	POST /v1/decisions/log
//	    Body: {agent_id, model_id, model_version, request, response,
//	           metadata, verdict, risk_tier, policy_id, parent_id?,
//	           preview_opt_in?}
//	    Writes an immutable, chain-rooted record. Request/response are
//	    hashed at write time; the raw payloads are NEVER persisted.
//
//	GET  /v1/decisions/log/{id}
//	    Return one record + a boolean whether its chain root still
//	    verifies (tamper check).
//
//	GET  /v1/decisions/log/{id}/lineage
//	    Return the ancestor chain for one record, root first.
//
//	GET  /v1/decisions/log
//	    Return recent records (default 100).

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/decision"
)

// auditSink is process-wide. Server initializes it lazily on first use.
// Kept package-level so the Server struct stays untouched (no wiring
// change through the constructor).
var (
	auditOnce sync.Once
	auditSink *decision.InMemorySink
)

func getAuditSink() *decision.InMemorySink {
	auditOnce.Do(func() {
		auditSink = decision.NewInMemorySink(4096)
	})
	return auditSink
}

type logDecisionReq struct {
	AgentID        string            `json:"agent_id"`
	ModelID        string            `json:"model_id"`
	ModelVersion   string            `json:"model_version"`
	Request        string            `json:"request"`
	Response       string            `json:"response"`
	Metadata       map[string]string `json:"metadata"`
	Verdict        string            `json:"verdict"`
	RiskTier       string            `json:"risk_tier"`
	PolicyID       string            `json:"policy_id"`
	ParentID       string            `json:"parent_id"`
	PreviewOptIn   bool              `json:"preview_opt_in"`
}

type logDecisionResp struct {
	Record decision.Record `json:"record"`
}

func (s *Server) decisionLog(w http.ResponseWriter, r *http.Request) {
	var req logDecisionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.AgentID == "" || req.ModelID == "" || req.Verdict == "" {
		s.jsonError(w, "agent_id, model_id, verdict are required", http.StatusBadRequest)
		return
	}
	id := fmt.Sprintf("dec-%d", time.Now().UnixNano())
	rec := decision.BuildRecord(
		id, req.AgentID, req.ModelID, req.ModelVersion,
		[]byte(req.Request), []byte(req.Response),
		req.Metadata,
		decision.Verdict(req.Verdict), req.RiskTier, req.PolicyID, req.ParentID,
		req.PreviewOptIn,
	)
	if err := getAuditSink().Append(rec); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, logDecisionResp{Record: rec})
}

func (s *Server) decisionGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, ok, err := getAuditSink().Get(id)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		s.jsonError(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"record":   rec,
		"verified": decision.VerifyRecord(rec),
	})
}

func (s *Server) decisionLineage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	depth := 32
	if q := r.URL.Query().Get("depth"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			depth = n
		}
	}
	chain, err := getAuditSink().Lineage(id, depth)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chain": chain})
}

func (s *Server) decisionRecent(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}
	recs, err := getAuditSink().Recent(limit)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": recs, "count": len(recs)})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
