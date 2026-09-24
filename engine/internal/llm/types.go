// Package llm provides a multi-provider LLM adapter with parallel fan-out,
// key rotation, deterministic fixture fallback, and unified response schema
// suitable for downstream facet-based judge decisions.
//
// Design goals:
//   - Speed first: parallel fan-out to 2-3 providers, first-non-error wins.
//   - Reliability: multiple keys per provider (round-robin on rate limit),
//     multiple providers per tier (fast tier vs. reliability tier).
//   - Determinism at demo time: if every provider fails, a deterministic
//     fixture responder returns a canned answer so the demo pipeline never
//     hard-fails.
//   - Trust-worthy schema: every response carries the provider/model/latency
//     it came from, plus a stable canonical form the judge can hash.
package llm

import (
	"context"
	"encoding/json"
	"time"
)

// Role is a chat message role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is one turn in a chat request.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// Request is a provider-agnostic chat completion request.
// The adapter maps this onto each provider's native format.
type Request struct {
	// Messages is the chat history; the last one is the user turn to answer.
	Messages []Message `json:"messages"`

	// ModelHint is an optional preferred model family (e.g. "llama-3.1-70b",
	// "gpt-4o-mini"). Each provider maps this to its closest available model.
	// Empty means "use the provider's default fast model".
	ModelHint string `json:"model_hint,omitempty"`

	// MaxTokens caps the response length. Zero means the provider default.
	MaxTokens int `json:"max_tokens,omitempty"`

	// Temperature (0.0 = deterministic, 1.0 = creative). Default 0.0 for
	// judge use cases so re-runs are reproducible.
	Temperature float64 `json:"temperature"`

	// JSONMode requests the provider return strict JSON (when supported).
	// Providers that lack a native JSON mode fall back to prompt-side hinting.
	JSONMode bool `json:"json_mode,omitempty"`

	// Seed pins the sampler where the provider supports it. 0 = unset.
	Seed int64 `json:"seed,omitempty"`
}

// Response is the unified reply returned by every provider path.
// It is intentionally small and hashable so a downstream judge can commit
// to (provider, model, latency, tokens, content) as part of a facet claim.
type Response struct {
	// Content is the assistant's reply text.
	Content string `json:"content"`

	// Provider is the provider ID that answered ("groq", "cerebras", ...).
	Provider string `json:"provider"`

	// Model is the exact model string the provider reported.
	Model string `json:"model"`

	// KeyIndex identifies which key slot answered (0-based). Useful for
	// diagnostics but never contains the key itself.
	KeyIndex int `json:"key_index"`

	// LatencyMs is the wall-clock duration for this provider's call.
	LatencyMs int64 `json:"latency_ms"`

	// PromptTokens / CompletionTokens are provider-reported; 0 if unknown.
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`

	// FinishReason is the provider's reported stop reason ("stop",
	// "length", "content_filter", ...). Empty if not surfaced.
	FinishReason string `json:"finish_reason,omitempty"`

	// Fixture is true when the fixture fallback served this response.
	// Never true for a real provider hit.
	Fixture bool `json:"fixture,omitempty"`

	// RawJSON is the provider's raw response envelope, kept for audit
	// evidence. Judges may hash this and pin it to the on-chain claim.
	RawJSON json.RawMessage `json:"raw_json,omitempty"`
}

// Canonical returns a stable JSON form of the response suitable for hashing.
// It intentionally omits Latency and RawJSON (transport-level noise) so the
// same content answered by different runs collides on the hash.
func (r *Response) Canonical() ([]byte, error) {
	type canonical struct {
		Content          string `json:"content"`
		Provider         string `json:"provider"`
		Model            string `json:"model"`
		PromptTokens     int    `json:"prompt_tokens"`
		CompletionTokens int    `json:"completion_tokens"`
		FinishReason     string `json:"finish_reason"`
		Fixture          bool   `json:"fixture"`
	}
	return json.Marshal(canonical{
		Content:          r.Content,
		Provider:         r.Provider,
		Model:            r.Model,
		PromptTokens:     r.PromptTokens,
		CompletionTokens: r.CompletionTokens,
		FinishReason:     r.FinishReason,
		Fixture:          r.Fixture,
	})
}

// Provider is the interface every real LLM backend implements.
// Implementations are expected to be safe for concurrent use.
type Provider interface {
	// ID is a stable short identifier ("groq", "openai", ...) used in
	// responses and logs.
	ID() string

	// Tier reports the intended priority band: TierFast for low-latency
	// providers that go into the parallel fan-out, TierReliability for
	// fallbacks (higher cost / higher latency, called only when the fast
	// tier fails or is rate-limited).
	Tier() Tier

	// Complete executes a chat completion. It must honor ctx cancellation.
	// The Response.Provider field must equal ID().
	Complete(ctx context.Context, req Request) (*Response, error)

	// KeyCount is the number of keys this provider was configured with.
	// Zero means the provider is disabled (no keys supplied).
	KeyCount() int
}

// Tier picks how the runner uses the provider.
type Tier int

const (
	// TierFast providers go into the parallel fan-out (first-non-error wins).
	TierFast Tier = iota
	// TierReliability providers run only if the fast tier failed entirely.
	TierReliability
)

// Config carries the runner's timing budget and fallback policy.
type Config struct {
	// PerProviderTimeout caps each individual provider call.
	PerProviderTimeout time.Duration

	// TotalBudget caps the whole Complete() call including fallback tiers.
	// Must be >= PerProviderTimeout.
	TotalBudget time.Duration

	// EnableFixtureFallback returns the canned deterministic fixture when
	// every real provider fails. Must be true for demo continuity.
	EnableFixtureFallback bool

	// FixtureContent overrides the canned fixture text. Empty = use the
	// package default.
	FixtureContent string

	// forceFixture (private) is set by LoadConfig when LLM_FIXTURE_MODE=1.
	// The runner skips real providers when true. Callers read it via
	// Config.ForceFixture().
	forceFixture bool
}

// DefaultConfig returns sensible defaults for demo/hackathon workloads.
func DefaultConfig() Config {
	return Config{
		PerProviderTimeout:    3 * time.Second,
		TotalBudget:           8 * time.Second,
		EnableFixtureFallback: true,
	}
}
