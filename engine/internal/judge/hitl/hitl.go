// Package hitl implements the Human-In-The-Loop escalation hook.
//
// When a FacetJudge returns OverallVerdict == ABSTAIN, the judgment is
// inconclusive: providers were split or too many were unreachable to reach a
// verdict. That is exactly the case where a human reviewer must adjudicate.
//
// This package converts an ABSTAIN task result into a stable, canonical
// EscalationEvent that can be delivered to any downstream sink (webhook,
// PagerDuty, GitHub issue, DB). The event carries enough evidence for a human
// to reproduce the decision without leaking provider secrets.
package hitl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge"
)

// Severity ranks the urgency of the escalation for the sink to route on.
type Severity string

const (
	SeverityLow      Severity = "low"      // one ABSTAIN facet, rest AGREE
	SeverityMedium   Severity = "medium"   // several ABSTAIN facets, no DISAGREE
	SeverityHigh     Severity = "high"     // at least one DISAGREE facet, overall still ABSTAIN (edge case)
	SeverityCritical Severity = "critical" // majority DISAGREE, overall ABSTAIN — indicates possible judge bug
)

// VoteSummary is the redacted vote line shown to a reviewer.
type VoteSummary struct {
	ProviderID string `json:"provider_id"`
	Value      string `json:"value"`
	LatencyMs  int64  `json:"latency_ms"`
	Err        string `json:"err,omitempty"`
}

// FacetSummary is the per-facet slice that a human reviewer sees.
type FacetSummary struct {
	FacetID           string         `json:"facet_id"`
	Prompt            string         `json:"prompt"`
	AllowedValues     []string       `json:"allowed_values"`
	Verdict           judge.Verdict  `json:"verdict"`
	Winner            string         `json:"winner,omitempty"`
	LiveCount         int            `json:"live_count"`
	AgreementFraction float64        `json:"agreement_fraction"`
	Votes             []VoteSummary  `json:"votes"`
	VoteHistogram     map[string]int `json:"vote_histogram"`
}

// EscalationEvent is the canonical payload delivered to sinks.
//
// Digest is a sha256 over the canonical JSON (see canonicalize) of every
// non-Digest field. Downstream systems can persist Digest for dedup and for
// tamper-evidence.
type EscalationEvent struct {
	Version   string         `json:"version"`
	Timestamp time.Time      `json:"timestamp"`
	TaskID    string         `json:"task_id"`
	Input     string         `json:"input"`
	SystemMsg string         `json:"system_msg,omitempty"`
	Overall   judge.Verdict  `json:"overall"`
	Severity  Severity       `json:"severity"`
	Reason    string         `json:"reason"`
	Facets    []FacetSummary `json:"facets"`
	Digest    string         `json:"digest"`
}

// Options tunes escalation building.
type Options struct {
	// Now returns the timestamp; if nil, time.Now is used. Useful for tests.
	Now func() time.Time
}

// BuildEvent turns a task + result into an EscalationEvent. It only succeeds
// for OverallVerdict == ABSTAIN — for AGREE the caller should proceed, for
// DISAGREE the equivocation package should emit slashing proof.
func BuildEvent(task *judge.Task, tr *judge.TaskResult, opts Options) (EscalationEvent, error) {
	if task == nil {
		return EscalationEvent{}, errors.New("hitl: nil task")
	}
	if tr == nil {
		return EscalationEvent{}, errors.New("hitl: nil task result")
	}
	if tr.OverallVerdict != judge.VerdictAbstain {
		return EscalationEvent{}, fmt.Errorf("hitl: expected overall=ABSTAIN, got %s", tr.OverallVerdict)
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}

	facets := make([]FacetSummary, 0, len(task.Facets))
	agree, abstain, disagree := 0, 0, 0
	for _, f := range task.Facets {
		fr, ok := tr.Facets[f.ID]
		if !ok || fr == nil {
			// Judge returned no result for a declared facet — treat as ABSTAIN
			// evidence rather than crashing.
			facets = append(facets, FacetSummary{
				FacetID:       f.ID,
				Prompt:        f.Prompt,
				AllowedValues: append([]string(nil), f.AllowedValues...),
				Verdict:       judge.VerdictAbstain,
				VoteHistogram: map[string]int{},
			})
			abstain++
			continue
		}
		hist := make(map[string]int, len(fr.Votes))
		votes := make([]VoteSummary, 0, len(fr.Votes))
		for _, v := range fr.Votes {
			votes = append(votes, VoteSummary{
				ProviderID: v.ProviderID,
				Value:      v.Value,
				LatencyMs:  v.Latency.Milliseconds(),
				Err:        v.Err,
			})
			if v.Err == "" {
				hist[v.Value]++
			}
		}
		sort.Slice(votes, func(i, j int) bool { return votes[i].ProviderID < votes[j].ProviderID })
		facets = append(facets, FacetSummary{
			FacetID:           f.ID,
			Prompt:            f.Prompt,
			AllowedValues:     append([]string(nil), f.AllowedValues...),
			Verdict:           fr.Verdict,
			Winner:            fr.Winner,
			LiveCount:         fr.LiveCount,
			AgreementFraction: fr.AgreementFraction,
			Votes:             votes,
			VoteHistogram:     hist,
		})
		switch fr.Verdict {
		case judge.VerdictAgree:
			agree++
		case judge.VerdictAbstain:
			abstain++
		case judge.VerdictDisagree:
			disagree++
		}
	}
	sort.Slice(facets, func(i, j int) bool { return facets[i].FacetID < facets[j].FacetID })

	sev := classifySeverity(agree, abstain, disagree)
	reason := buildReason(agree, abstain, disagree)

	ev := EscalationEvent{
		Version:   "hitl.v1",
		Timestamp: now().UTC(),
		TaskID:    task.ID,
		Input:     task.Input,
		SystemMsg: task.SystemMsg,
		Overall:   tr.OverallVerdict,
		Severity:  sev,
		Reason:    reason,
		Facets:    facets,
	}
	digest, err := computeDigest(ev)
	if err != nil {
		return EscalationEvent{}, err
	}
	ev.Digest = digest
	return ev, nil
}

func classifySeverity(agree, abstain, disagree int) Severity {
	total := agree + abstain + disagree
	if total == 0 {
		return SeverityLow
	}
	if disagree*2 > total {
		return SeverityCritical
	}
	if disagree > 0 {
		return SeverityHigh
	}
	if abstain > 1 {
		return SeverityMedium
	}
	return SeverityLow
}

func buildReason(agree, abstain, disagree int) string {
	return fmt.Sprintf(
		"human review required: %d AGREE, %d ABSTAIN, %d DISAGREE facets — no dominant verdict",
		agree, abstain, disagree,
	)
}

// canonicalize returns byte-stable JSON: sorted map keys, no HTML escapes.
// Same contract as equivocation package.
func canonicalize(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	return canonicalizeValue(normalized)
}

func canonicalizeValue(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf := []byte{'{'}
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			buf = append(buf, kb...)
			buf = append(buf, ':')
			vb, err := canonicalizeValue(t[k])
			if err != nil {
				return nil, err
			}
			buf = append(buf, vb...)
		}
		buf = append(buf, '}')
		return buf, nil
	case []any:
		buf := []byte{'['}
		for i, e := range t {
			if i > 0 {
				buf = append(buf, ',')
			}
			eb, err := canonicalizeValue(e)
			if err != nil {
				return nil, err
			}
			buf = append(buf, eb...)
		}
		buf = append(buf, ']')
		return buf, nil
	default:
		return json.Marshal(v)
	}
}

func computeDigest(ev EscalationEvent) (string, error) {
	ev.Digest = ""
	b, err := canonicalize(ev)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Verify recomputes the digest and returns nil if it matches the stored one.
func Verify(ev EscalationEvent) error {
	if ev.Digest == "" {
		return errors.New("hitl: event has no digest")
	}
	got, err := computeDigest(ev)
	if err != nil {
		return err
	}
	if got != ev.Digest {
		return fmt.Errorf("hitl: digest mismatch: got %s want %s", got, ev.Digest)
	}
	return nil
}

// Sink is the delivery interface. Implementations must be idempotent on Digest
// so re-delivery is safe.
type Sink interface {
	Deliver(ctx context.Context, ev EscalationEvent) error
}

// SinkFunc adapts a function into a Sink.
type SinkFunc func(ctx context.Context, ev EscalationEvent) error

func (f SinkFunc) Deliver(ctx context.Context, ev EscalationEvent) error {
	return f(ctx, ev)
}

// MultiSink fans an event out to every configured sink. It attempts every
// delivery and returns the first error encountered.
type MultiSink struct {
	Sinks []Sink
}

func (m *MultiSink) Deliver(ctx context.Context, ev EscalationEvent) error {
	var firstErr error
	for _, s := range m.Sinks {
		if err := s.Deliver(ctx, ev); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
