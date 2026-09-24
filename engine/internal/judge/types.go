// Package judge implements facet-based verdict aggregation over multiple LLM providers.
//
// A judge task is a structured question with N independent facets. Each facet is
// answered by K LLM providers in parallel via the llm.Runner. Verdicts are then
// aggregated:
//
//   - AGREE  — supermajority (>= AgreementThreshold) of live providers agree on the same value
//   - DISAGREE — live providers split; equivocation evidence is emitted
//   - ABSTAIN — insufficient live providers to reach a quorum (< QuorumMin)
//
// The judge never fabricates a verdict when providers are unreachable. ABSTAIN is
// a first-class outcome that flows to HITL escalation, not a silent fallback.
package judge

import (
	"context"
	"time"
)

// Facet is one dimension of a structured verdict.
//
// Example: for a "toxic content?" task, facets might be
//   - "contains_slurs" (yes/no)
//   - "targeted_harassment" (yes/no)
//   - "severity" (low/medium/high)
type Facet struct {
	// ID is a stable identifier for this facet (e.g. "toxic.slurs").
	ID string

	// Prompt is the exact question sent to each LLM. Should be phrased to elicit
	// a short, categorical answer from the AllowedValues set.
	Prompt string

	// AllowedValues restricts the verdict space. Answers outside this set are
	// normalized to "unknown" and count against agreement.
	AllowedValues []string

	// Weight controls how heavily this facet influences the overall task verdict.
	// Defaults to 1.0 if zero.
	Weight float64
}

// Task is a full judge job: input plus one or more facets to evaluate.
type Task struct {
	ID        string
	Input     string
	SystemMsg string
	Facets    []Facet

	// MinProviders is the minimum number of live provider responses required per
	// facet before a verdict can be issued. Below this, the facet resolves to
	// ABSTAIN. Defaults to 2.
	MinProviders int

	// AgreementThreshold is the fraction of live providers that must return the
	// same value for AGREE. Below this (but at or above MinProviders), the facet
	// resolves to DISAGREE. Defaults to 0.66.
	AgreementThreshold float64
}

// Verdict is one of three outcomes for a facet or task.
type Verdict string

const (
	VerdictAgree    Verdict = "AGREE"
	VerdictDisagree Verdict = "DISAGREE"
	VerdictAbstain  Verdict = "ABSTAIN"
)

// ProviderVote is one provider's answer to one facet.
type ProviderVote struct {
	ProviderID string
	Value      string
	Raw        string        // raw model output (for audit)
	Latency    time.Duration // provider latency
	Err        string        // non-empty if this provider errored (Value will be "")
}

// FacetResult is the aggregated outcome for one facet across all providers.
type FacetResult struct {
	FacetID string

	// Verdict is AGREE / DISAGREE / ABSTAIN.
	Verdict Verdict

	// Winner is the value chosen when Verdict == AGREE. Empty otherwise.
	Winner string

	// Votes contains every provider's raw vote (including errors).
	Votes []ProviderVote

	// LiveCount is the number of providers that answered (Err == "").
	LiveCount int

	// AgreementFraction is Winner-vote-count / LiveCount when Verdict != ABSTAIN.
	// Zero on ABSTAIN.
	AgreementFraction float64
}

// TaskResult is the outcome of a full judge task.
type TaskResult struct {
	TaskID string

	// OverallVerdict is:
	//   AGREE   — every facet is AGREE
	//   DISAGREE — at least one facet is DISAGREE (equivocation)
	//   ABSTAIN — at least one facet is ABSTAIN AND no facet is DISAGREE
	//
	// DISAGREE takes precedence over ABSTAIN because equivocation is stronger
	// evidence of adversarial or defective behavior than unavailability.
	OverallVerdict Verdict

	// Facets maps FacetID -> FacetResult.
	Facets map[string]*FacetResult

	// StartedAt / CompletedAt for latency reporting.
	StartedAt   time.Time
	CompletedAt time.Time
}

// Judge is the facet-based decision engine.
type Judge interface {
	// Decide evaluates the task across all facets and returns the aggregated result.
	// Never returns error for provider failures — those become ABSTAIN. Only returns
	// error for programmer errors (nil task, no facets, etc).
	Decide(ctx context.Context, task *Task) (*TaskResult, error)
}
