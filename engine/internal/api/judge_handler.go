package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge/equivocation"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge/hitl"
)

// JudgeService is the minimal interface the /inference/judge endpoint needs.
// It is satisfied by *judge.FacetJudge in production and by fakes in tests.
type JudgeService interface {
	Decide(ctx context.Context, task *judge.Task) (*judge.TaskResult, error)
}

// SetJudge wires a judge service into the server. Kept in a dedicated method
// so main can install it without touching the New() signature, and tests can
// inject a stub without a full Runner wired up.
func (s *Server) SetJudge(j JudgeService) {
	s.judge = j
}

// judgeRequest is the wire schema for POST /inference/judge.
//
// The request never carries provider secrets — those live in server-side env
// (see internal/llm). Frontend supplies only the task shape.
type judgeRequest struct {
	TaskID    string          `json:"task_id"`
	Input     string          `json:"input"`
	SystemMsg string          `json:"system_msg,omitempty"`
	Facets    []judgeReqFacet `json:"facets"`

	// Optional tuning; server-side defaults apply if omitted.
	MinProviders       int     `json:"min_providers,omitempty"`
	AgreementThreshold float64 `json:"agreement_threshold,omitempty"`
}

type judgeReqFacet struct {
	ID            string   `json:"id"`
	Prompt        string   `json:"prompt"`
	AllowedValues []string `json:"allowed_values"`
	Weight        float64  `json:"weight,omitempty"`
}

// judgeResponse is deliberately verdict-forward: the frontend gets the overall
// answer + per-facet verdict summary, plus one of two evidence blobs depending
// on outcome. Raw provider votes are only in the HITL escalation payload,
// which itself is redacted (no API keys, no raw model text besides the vote).
type judgeResponse struct {
	TaskID      string                    `json:"task_id"`
	Overall     judge.Verdict             `json:"overall"`
	Facets      []judgeRespFacet          `json:"facets"`
	Equivocated *equivocation.Proof       `json:"equivocation_proof,omitempty"`
	Escalation  *hitl.EscalationEvent     `json:"hitl_escalation,omitempty"`
}

type judgeRespFacet struct {
	ID                string        `json:"id"`
	Verdict           judge.Verdict `json:"verdict"`
	Winner            string        `json:"winner,omitempty"`
	LiveCount         int           `json:"live_count"`
	AgreementFraction float64       `json:"agreement_fraction"`
}

// judgeHandler serves POST /inference/judge. It runs the multi-provider judge,
// then routes the result:
//   - AGREE     → verdict-only response
//   - DISAGREE  → verdict + equivocation.Proof (deterministic slashing evidence)
//   - ABSTAIN   → verdict + hitl.EscalationEvent + deliver to configured sinks
func (s *Server) judgeHandler(w http.ResponseWriter, r *http.Request) {
	if s.judge == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "judge not configured")
		return
	}
	var req judgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	task, err := buildJudgeTask(req)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}

	tr, err := s.judge.Decide(r.Context(), task)
	if err != nil {
		writeJSONErr(w, http.StatusBadGateway, "judge: "+err.Error())
		return
	}

	resp := judgeResponse{
		TaskID:  tr.TaskID,
		Overall: tr.OverallVerdict,
		Facets:  make([]judgeRespFacet, 0, len(task.Facets)),
	}
	for _, f := range task.Facets {
		fr, ok := tr.Facets[f.ID]
		if !ok || fr == nil {
			resp.Facets = append(resp.Facets, judgeRespFacet{ID: f.ID, Verdict: judge.VerdictAbstain})
			continue
		}
		resp.Facets = append(resp.Facets, judgeRespFacet{
			ID:                fr.FacetID,
			Verdict:           fr.Verdict,
			Winner:            fr.Winner,
			LiveCount:         fr.LiveCount,
			AgreementFraction: fr.AgreementFraction,
		})
	}

	switch tr.OverallVerdict {
	case judge.VerdictDisagree:
		proof, perr := equivocation.FromTaskResult(task, tr)
		if perr != nil {
			writeJSONErr(w, http.StatusInternalServerError, "equivocation: "+perr.Error())
			return
		}
		resp.Equivocated = proof
	case judge.VerdictAbstain:
		ev, herr := hitl.BuildEvent(task, tr, hitl.Options{})
		if herr != nil {
			writeJSONErr(w, http.StatusInternalServerError, "hitl: "+herr.Error())
			return
		}
		resp.Escalation = &ev
		if s.hitlSink != nil {
			// Best-effort delivery; failure is logged but does not fail the request
			// because the escalation event is already in the response body and the
			// caller can retry delivery.
			if derr := s.hitlSink.Deliver(r.Context(), ev); derr != nil {
				s.log.Warn("hitl sink delivery failed", "err", derr, "task_id", task.ID)
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// SetHITLSink wires a delivery sink for ABSTAIN events. Optional — if unset,
// escalation events are returned in the response body only.
func (s *Server) SetHITLSink(sink hitl.Sink) {
	s.hitlSink = sink
}

func buildJudgeTask(req judgeRequest) (*judge.Task, error) {
	if strings.TrimSpace(req.Input) == "" {
		return nil, errors.New("input is required")
	}
	if len(req.Facets) == 0 {
		return nil, errors.New("at least one facet is required")
	}
	if len(req.Facets) > 32 {
		return nil, fmt.Errorf("too many facets: %d (max 32)", len(req.Facets))
	}
	facets := make([]judge.Facet, 0, len(req.Facets))
	seen := make(map[string]bool, len(req.Facets))
	for _, rf := range req.Facets {
		id := strings.TrimSpace(rf.ID)
		if id == "" {
			return nil, errors.New("facet.id is required")
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate facet id: %q", id)
		}
		seen[id] = true
		if strings.TrimSpace(rf.Prompt) == "" {
			return nil, fmt.Errorf("facet %q: prompt is required", id)
		}
		if len(rf.AllowedValues) < 2 {
			return nil, fmt.Errorf("facet %q: at least 2 allowed_values required", id)
		}
		w := rf.Weight
		if w == 0 {
			w = 1.0
		}
		facets = append(facets, judge.Facet{
			ID: id, Prompt: rf.Prompt,
			AllowedValues: append([]string(nil), rf.AllowedValues...),
			Weight:        w,
		})
	}
	task := &judge.Task{
		ID:                 req.TaskID,
		Input:              req.Input,
		SystemMsg:          req.SystemMsg,
		Facets:             facets,
		MinProviders:       req.MinProviders,
		AgreementThreshold: req.AgreementThreshold,
	}
	return task, nil
}


func writeJSONErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
