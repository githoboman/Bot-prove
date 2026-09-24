package judge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/llm"
)

// FacetJudge is the default facet-based Judge implementation.
// It uses an llm.Runner to poll every configured provider for each facet
// independently, then aggregates votes into AGREE / DISAGREE / ABSTAIN.
type FacetJudge struct {
	runner *llm.Runner
}

// NewFacetJudge builds a FacetJudge over an existing llm.Runner.
func NewFacetJudge(runner *llm.Runner) *FacetJudge {
	return &FacetJudge{runner: runner}
}

// Decide implements Judge. See package docs for verdict semantics.
func (j *FacetJudge) Decide(ctx context.Context, task *Task) (*TaskResult, error) {
	if task == nil {
		return nil, errors.New("judge: task is nil")
	}
	if len(task.Facets) == 0 {
		return nil, errors.New("judge: task has zero facets")
	}
	if j.runner == nil {
		return nil, errors.New("judge: runner is nil")
	}

	minProviders := task.MinProviders
	if minProviders <= 0 {
		minProviders = 2
	}
	threshold := task.AgreementThreshold
	if threshold <= 0 {
		threshold = 0.66
	}

	res := &TaskResult{
		TaskID:    task.ID,
		Facets:    make(map[string]*FacetResult, len(task.Facets)),
		StartedAt: time.Now(),
	}

	for _, facet := range task.Facets {
		fr := j.decideFacet(ctx, task, facet, minProviders, threshold)
		res.Facets[facet.ID] = fr
	}
	res.CompletedAt = time.Now()

	res.OverallVerdict = aggregateOverall(res.Facets)
	return res, nil
}

// decideFacet polls every provider for one facet and returns a FacetResult.
func (j *FacetJudge) decideFacet(
	ctx context.Context,
	task *Task,
	facet Facet,
	minProviders int,
	threshold float64,
) *FacetResult {
	// Build the LLM request. System message is a strict instruction to answer
	// with ONE of the allowed values, nothing else.
	sys := buildFacetSystemPrompt(task.SystemMsg, facet)
	user := buildFacetUserPrompt(task.Input, facet)

	req := llm.Request{
		Messages: buildFacetMessages(sys, user),
		Temperature: 0.0, // determinism matters for facet agreement.
		MaxTokens:   64,  // categorical answers are short.
	}

	polls := j.runner.Poll(ctx, req)

	votes := make([]ProviderVote, 0, len(polls))
	tally := make(map[string]int)
	live := 0
	for _, p := range polls {
		vote := ProviderVote{
			ProviderID: p.Provider,
			Latency:    time.Duration(p.Attempt.LatencyMs) * time.Millisecond,
		}
		if p.Resp == nil {
			vote.Err = p.Attempt.Err
			if vote.Err == "" {
				vote.Err = "provider returned no response"
			}
			votes = append(votes, vote)
			continue
		}
		vote.Raw = p.Resp.Content
		normalized := normalizeVote(p.Resp.Content, facet.AllowedValues)
		vote.Value = normalized
		votes = append(votes, vote)
		if normalized == "" {
			// Response was unparseable / off-menu — counts as live but not
			// contributing to any bucket. We still increment live so that a
			// facet with N garbage responses hits DISAGREE, not ABSTAIN.
			live++
			continue
		}
		tally[normalized]++
		live++
	}

	fr := &FacetResult{
		FacetID:   facet.ID,
		Votes:     votes,
		LiveCount: live,
	}

	if live < minProviders {
		fr.Verdict = VerdictAbstain
		return fr
	}

	// Find the top value.
	var top string
	topCount := 0
	for v, c := range tally {
		if c > topCount {
			top = v
			topCount = c
		}
	}

	if topCount == 0 {
		// All live responses were unparseable — DISAGREE (evidence of confusion).
		fr.Verdict = VerdictDisagree
		return fr
	}

	fraction := float64(topCount) / float64(live)
	fr.AgreementFraction = fraction

	if fraction >= threshold {
		fr.Verdict = VerdictAgree
		fr.Winner = top
	} else {
		fr.Verdict = VerdictDisagree
	}
	return fr
}

// aggregateOverall combines per-facet verdicts into one task verdict.
// DISAGREE dominates ABSTAIN dominates AGREE.
func aggregateOverall(facets map[string]*FacetResult) Verdict {
	sawDisagree := false
	sawAbstain := false
	for _, fr := range facets {
		switch fr.Verdict {
		case VerdictDisagree:
			sawDisagree = true
		case VerdictAbstain:
			sawAbstain = true
		}
	}
	switch {
	case sawDisagree:
		return VerdictDisagree
	case sawAbstain:
		return VerdictAbstain
	default:
		return VerdictAgree
	}
}

// buildFacetMessages assembles the llm.Message pair from the strict
// system message and user prompt. Exported callers must use this to keep
// FixtureProvider key derivation aligned with what decideFacet actually sends.
func buildFacetMessages(sys, user string) []llm.Message {
	return []llm.Message{
		{Role: llm.RoleSystem, Content: sys},
		{Role: llm.RoleUser, Content: user},
	}
}

// BuildFacetSystemPrompt is the exported counterpart of buildFacetSystemPrompt.
// External corpora (see internal/judge/pifixture) use it to construct fixture
// tables keyed on the exact bytes the judge will send at eval time.
func BuildFacetSystemPrompt(userSys string, facet Facet) string {
	return buildFacetSystemPrompt(userSys, facet)
}

// BuildFacetUserPrompt is the exported counterpart of buildFacetUserPrompt.
func BuildFacetUserPrompt(input string, facet Facet) string {
	return buildFacetUserPrompt(input, facet)
}

// buildFacetSystemPrompt constructs the strict-format system message.
func buildFacetSystemPrompt(userSys string, facet Facet) string {
	var b strings.Builder
	if strings.TrimSpace(userSys) != "" {
		b.WriteString(userSys)
		b.WriteString("\n\n")
	}
	b.WriteString("You are a strict classification oracle. ")
	b.WriteString("You will be asked one question about the input. ")
	b.WriteString("Reply with EXACTLY ONE of these values and nothing else: ")
	b.WriteString(strings.Join(facet.AllowedValues, ", "))
	b.WriteString(".\n")
	b.WriteString("Do not explain. Do not add punctuation. Do not repeat the question. ")
	b.WriteString("If the input is ambiguous, still choose the single best-fit value.")
	return b.String()
}

// buildFacetUserPrompt constructs the input+question pair.
func buildFacetUserPrompt(input string, facet Facet) string {
	return fmt.Sprintf("Input:\n%s\n\nQuestion: %s", input, facet.Prompt)
}

// normalizeVote maps a raw LLM answer to one of the AllowedValues, or "" if
// none match. Match is case-insensitive and trims surrounding whitespace,
// periods, and quotes. Exact-token match is preferred; if the raw contains
// exactly one allowed value as a substring, that also counts.
func normalizeVote(raw string, allowed []string) string {
	if len(allowed) == 0 {
		// No allowlist — return the raw stripped answer.
		return strings.TrimSpace(raw)
	}
	stripped := strings.ToLower(strings.Trim(strings.TrimSpace(raw), ".!?\"'`"))
	// Exact match wins.
	for _, v := range allowed {
		if strings.ToLower(v) == stripped {
			return v
		}
	}
	// Look for the first allowed value that appears as a whole word.
	lowerRaw := " " + strings.ToLower(raw) + " "
	for _, v := range allowed {
		needle := " " + strings.ToLower(v) + " "
		if strings.Contains(lowerRaw, needle) {
			return v
		}
	}
	return ""
}
