package api

// Pack AQ — A2A provider pool + real HTTP provider adapter + HITL policy.
//
// Endpoints (all under scope `decision:*`, opt-in via CP_DECISION_ENABLE=1):
//   POST /v1/decision/evaluate          → run Router.Route + Judge + HITL.Decide
//   GET  /v1/decision/pool              → list registered providers (audit)
//   POST /v1/hitl/tickets/{id}/resolve  → resolve an escalation
//   GET  /v1/hitl/tickets               → list tickets (query: state, limit)
//   GET  /v1/hitl/tickets/{id}          → fetch one ticket
//
// The pipeline is deliberately opt-in: when CP_DECISION_ENABLE is unset,
// New() leaves srv.decisionRouter nil and every handler returns 503. This
// keeps the existing test surface stable while giving operators a single
// toggle to switch on the A2A/HITL layer.

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/decision/attest"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/hitl"
)

// initDecisionPipeline builds the pool, adapter, judge and HITL
// service. It is called from Server.New when CP_DECISION_ENABLE=1.
func (s *Server) initDecisionPipeline() {
	pool := attest.NewProviderPool()

	// The system-trust provider is either the HTTP adapter (real
	// remote evaluator) OR its fixture fallback — but always present
	// so the pool is non-empty and the router has something to route
	// to. In either mode the provider is deterministic.
	adapter := attest.NewHTTPProviderAdapterFromEnv()
	systemName := "cp-decision-system"
	if adapter.Configured() {
		// Re-tag the adapter so receipts identify it as the system
		// provider without leaking its underlying URL.
		adapter = attest.NewHTTPProviderAdapter(attest.HTTPProviderConfig{
			Name:     systemName,
			Endpoint: os.Getenv("CP_DECISION_PROVIDER_URL"),
			Token:    os.Getenv("CP_DECISION_PROVIDER_TOKEN"),
			Fallback: attest.NewNamedFixtureProvider(systemName + "-fallback"),
		})
	}
	_ = pool.Register(attest.PooledProvider{
		Provider: adapter,
		Trust:    attest.TrustSystem,
		Capabilities: []attest.Capability{
			attest.CapSafety,
			attest.CapCorrectness,
			attest.CapSpecCompliance,
			attest.CapEquivocation,
		},
	})

	router := attest.NewRouter(pool)
	judge := attest.NewJudge(adapter, attest.DefaultAggregationPolicy)

	hs := hitl.NewService(hitl.DefaultPolicy, nil)

	s.decisionPool = pool
	s.decisionRouter = router
	s.decisionJudge = judge
	s.hitlService = hs
	s.log.Info("decision pipeline enabled",
		"pool_size", pool.Len(), "system_provider_configured", adapter.Configured())
}

// registerDecisionRoutes wires the /v1/decision/* endpoints.
func (s *Server) registerDecisionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/decision/evaluate", s.decisionEvaluate)
	mux.HandleFunc("GET /v1/decision/pool", s.decisionPoolInfo)
}

// registerHITLRoutes wires the /v1/hitl/* endpoints.
func (s *Server) registerHITLRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/hitl/tickets", s.hitlListTickets)
	mux.HandleFunc("GET /v1/hitl/tickets/{id}", s.hitlGetTicket)
	mux.HandleFunc("POST /v1/hitl/tickets/{id}/resolve", s.hitlResolveTicket)
}

// evaluateRequest is the wire form of POST /v1/decision/evaluate.
type decisionEvaluateRequest struct {
	Submitter  string   `json:"submitter"`
	SpecID     string   `json:"spec_id"`
	PayloadHex string   `json:"payload_hex"`
	Nonce      uint64   `json:"nonce"`
	Facets     []string `json:"facets,omitempty"` // if empty, all facets
}

// decisionEvaluateResponse is the wire form of the evaluate response.
type decisionEvaluateResponse struct {
	DecisionID    string                  `json:"decision_id"`
	CommitDigest  string                  `json:"commit_digest"`
	Aggregate     string                  `json:"aggregate"`
	VetoedBy      string                  `json:"vetoed_by,omitempty"`
	AbstainReason string                  `json:"abstain_reason,omitempty"`
	Facets        []facetVerdictWire      `json:"facets"`
	Providers     []string                `json:"providers"`
	Errors        map[string]string       `json:"provider_errors,omitempty"`
	HITL          hitl.Response           `json:"hitl"`
}

type facetVerdictWire struct {
	Kind       string  `json:"kind"`
	Verdict    string  `json:"verdict"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// decisionEvaluate: Route the decision through the pool → Byzantine
// aggregation → HITL policy. On error, returns a well-formed JSON error
// body plus the appropriate 4xx/5xx status.
func (s *Server) decisionEvaluate(w http.ResponseWriter, r *http.Request) {
	if s.decisionRouter == nil || s.hitlService == nil {
		s.jsonError(w, "decision pipeline disabled (set CP_DECISION_ENABLE=1)", http.StatusServiceUnavailable)
		return
	}
	var req decisionEvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid json body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Submitter) == "" || strings.TrimSpace(req.SpecID) == "" {
		s.jsonError(w, "submitter and spec_id are required", http.StatusBadRequest)
		return
	}
	payload, err := hex.DecodeString(strings.TrimPrefix(req.PayloadHex, "0x"))
	if err != nil {
		s.jsonError(w, "invalid payload_hex: "+err.Error(), http.StatusBadRequest)
		return
	}

	dec := attest.Decision{
		Submitter:   req.Submitter,
		SpecID:      req.SpecID,
		Payload:     payload,
		Nonce:       req.Nonce,
		SubmittedAt: time.Now().UTC(),
	}
	facets, err := parseFacets(req.Facets)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	outcome, err := s.decisionRouter.Route(r.Context(), dec, facets)
	if err != nil {
		s.jsonError(w, "router: "+err.Error(), http.StatusInternalServerError)
		return
	}

	agg, veto, reason, err := attest.Aggregate(attest.DefaultAggregationPolicy, outcome.Verdicts)
	if err != nil {
		s.jsonError(w, "aggregate: "+err.Error(), http.StatusInternalServerError)
		return
	}
	commit := attest.DecisionCommit{
		Decision:      dec,
		DecisionID:    dec.ID(),
		FacetVerdicts: outcome.Verdicts,
		Aggregate:     agg,
		VetoedBy:      veto,
	}
	if agg == attest.VerdictAbstain {
		commit.AbstainReason = reason
	}

	hitlResp, err := s.hitlService.Decide(commit)
	if err != nil {
		s.jsonError(w, "hitl: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := decisionEvaluateResponse{
		DecisionID:    commit.DecisionID,
		CommitDigest:  commit.CommitDigest(),
		Aggregate:     agg.String(),
		VetoedBy:      string(veto),
		AbstainReason: commit.AbstainReason,
		Facets:        facetsToWire(outcome.Verdicts),
		Providers:     outcome.Providers,
		HITL:          hitlResp,
	}
	if len(outcome.ErrorsByProvider) > 0 {
		resp.Errors = make(map[string]string, len(outcome.ErrorsByProvider))
		for k, v := range outcome.ErrorsByProvider {
			resp.Errors[k] = v.Error()
		}
	}
	writeJSONOK(w, resp)
}

// decisionPoolInfo returns the registered providers (names + count).
// Trust levels and capabilities are not surfaced to avoid leaking
// operational config; add a scoped endpoint later if needed.
func (s *Server) decisionPoolInfo(w http.ResponseWriter, r *http.Request) {
	if s.decisionPool == nil {
		s.jsonError(w, "decision pipeline disabled", http.StatusServiceUnavailable)
		return
	}
	writeJSONOK(w, map[string]any{
		"count":     s.decisionPool.Len(),
		"providers": s.decisionPool.Names(),
	})
}

// hitlListTickets: GET /v1/hitl/tickets?state=pending&limit=50
func (s *Server) hitlListTickets(w http.ResponseWriter, r *http.Request) {
	if s.hitlService == nil {
		s.jsonError(w, "hitl disabled", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	filter := hitl.ListFilter{State: q.Get("state")}
	if limStr := q.Get("limit"); limStr != "" {
		n, err := strconv.Atoi(limStr)
		if err != nil || n < 0 {
			s.jsonError(w, "invalid limit", http.StatusBadRequest)
			return
		}
		filter.Limit = n
	}
	tickets, err := s.hitlService.Store().List(filter)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONOK(w, map[string]any{"tickets": tickets})
}

// hitlGetTicket: GET /v1/hitl/tickets/{id}
func (s *Server) hitlGetTicket(w http.ResponseWriter, r *http.Request) {
	if s.hitlService == nil {
		s.jsonError(w, "hitl disabled", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	t, err := s.hitlService.Store().Get(id)
	if err != nil {
		if errors.Is(err, hitl.ErrTicketNotFound) {
			s.jsonError(w, "ticket not found", http.StatusNotFound)
			return
		}
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONOK(w, t)
}

// hitlResolveTicket: POST /v1/hitl/tickets/{id}/resolve
type hitlResolveRequest struct {
	Resolver string `json:"resolver"`
	State    string `json:"state"`
	Note     string `json:"note,omitempty"`
}

func (s *Server) hitlResolveTicket(w http.ResponseWriter, r *http.Request) {
	if s.hitlService == nil {
		s.jsonError(w, "hitl disabled", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		s.jsonError(w, "missing ticket id", http.StatusBadRequest)
		return
	}
	var req hitlResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Resolver) == "" {
		s.jsonError(w, "resolver is required", http.StatusBadRequest)
		return
	}
	t, err := s.hitlService.Store().Resolve(id, req.Resolver, req.State, req.Note)
	if err != nil {
		switch {
		case errors.Is(err, hitl.ErrTicketNotFound):
			s.jsonError(w, "ticket not found", http.StatusNotFound)
		case errors.Is(err, hitl.ErrAlreadyResolved):
			s.jsonError(w, "ticket already resolved", http.StatusConflict)
		case errors.Is(err, hitl.ErrInvalidState):
			s.jsonError(w,
				fmt.Sprintf("state must be one of approved|rejected|stale_escalation, got %q", req.State),
				http.StatusBadRequest)
		default:
			s.jsonError(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSONOK(w, t)
}

// writeJSONOK writes a 200 OK JSON response.
func writeJSONOK(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// parseFacets converts wire-form facet names to attest.FacetKind.
// Empty or missing ⇒ nil (Router treats nil as "all facets").
func parseFacets(names []string) ([]attest.FacetKind, error) {
	if len(names) == 0 {
		return nil, nil
	}
	out := make([]attest.FacetKind, 0, len(names))
	for _, n := range names {
		k := attest.FacetKind(strings.TrimSpace(n))
		switch k {
		case attest.FacetSafety, attest.FacetCorrectness,
			attest.FacetSpecCompliance, attest.FacetEquivocation:
			out = append(out, k)
		default:
			return nil, fmt.Errorf("unknown facet kind: %q", n)
		}
	}
	return out, nil
}

// facetsToWire converts internal FacetVerdicts to the wire form.
func facetsToWire(verdicts []attest.FacetVerdict) []facetVerdictWire {
	out := make([]facetVerdictWire, len(verdicts))
	for i, v := range verdicts {
		out[i] = facetVerdictWire{
			Kind:       string(v.Kind),
			Verdict:    v.Verdict.String(),
			Confidence: v.Confidence,
			Reason:     v.Reason,
		}
	}
	return out
}
