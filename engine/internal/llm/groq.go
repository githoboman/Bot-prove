package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GroqProvider implements Provider for Groq's OpenAI-compatible API.
// Groq is Tier 1 (fast): Llama-3.x on their LPU hardware answers in ~200-400ms.
//
// API docs: https://console.groq.com/docs/api-reference#chat-create
// Env: GROQ_API_KEY, GROQ2_API_KEY (round-robin via KeyRing).
type GroqProvider struct {
	ring    *KeyRing
	client  *http.Client
	model   string
	baseURL string // overridable for tests
}

// NewGroqProvider builds a Groq provider from a KeyRing.
// Default model: llama-3.1-8b-instant (fastest tier).
func NewGroqProvider(ring *KeyRing) *GroqProvider {
	return &GroqProvider{
		ring:    ring,
		client:  &http.Client{}, // per-call timeout comes from ctx
		model:   "llama-3.1-8b-instant",
		baseURL: "https://api.groq.com/openai/v1",
	}
}

// WithModel overrides the default Groq model. Chainable.
func (g *GroqProvider) WithModel(model string) *GroqProvider {
	g.model = model
	return g
}

// WithBaseURL overrides base URL (test hook).
func (g *GroqProvider) WithBaseURL(url string) *GroqProvider {
	g.baseURL = strings.TrimRight(url, "/")
	return g
}

// ID reports the stable provider identifier.
func (g *GroqProvider) ID() string { return "groq" }

// Tier reports TierFast (parallel fan-out band).
func (g *GroqProvider) Tier() Tier { return TierFast }

// KeyCount reports the number of usable keys the ring was built with.
func (g *GroqProvider) KeyCount() int { return g.ring.Len() }

// Complete performs a chat completion via Groq.
// Honors ctx deadline; picks the next available key from the ring; on 429
// rests the key and returns a retryable ProviderError.
func (g *GroqProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	if g.ring.Len() == 0 {
		return nil, &ProviderError{Provider: g.ID(), Cause: ErrNoKeys}
	}
	start := time.Now()

	key, keyIdx, err := g.ring.Next()
	if err != nil {
		return nil, &ProviderError{Provider: g.ID(), Cause: err, Retryable: false}
	}

	// Build OpenAI-compatible chat payload.
	model := req.ModelHint
	if model == "" || !isGroqModelHint(model) {
		model = g.model
	}
	body := groqRequest{
		Model:       model,
		Messages:    groqMessagesFromRequest(req),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	if body.MaxTokens == 0 {
		body.MaxTokens = 512
	}
	if req.JSONMode {
		body.ResponseFormat = &groqResponseFormat{Type: "json_object"}
	}
	if req.Seed != 0 {
		body.Seed = req.Seed
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, &ProviderError{Provider: g.ID(), Cause: fmt.Errorf("marshal: %w", err)}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, &ProviderError{Provider: g.ID(), Cause: fmt.Errorf("build request: %w", err)}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+key)

	resp, err := g.client.Do(httpReq)
	if err != nil {
		// Context cancellation / timeout / network. Not the key's fault, but the
		// runner may retry against another provider.
		return nil, &ProviderError{Provider: g.ID(), Cause: err, Retryable: true}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProviderError{Provider: g.ID(), Cause: fmt.Errorf("read: %w", err), Retryable: true}
	}

	latency := time.Since(start)

	// Non-2xx handling.
	if resp.StatusCode >= 400 {
		switch {
		case isRateLimitStatus(resp.StatusCode):
			// Rest this key; some providers set Retry-After but KeyRing only
			// supports its configured cooldown — so we ignore the exact value
			// and just call Rest().
			_ = parseRetryAfter(resp.Header.Get("Retry-After")) // reserved for future use
			g.ring.Rest(keyIdx)
			return nil, &ProviderError{
				Provider:   g.ID(),
				StatusCode: resp.StatusCode,
				Retryable:  true,
				Body:       truncateBody(raw),
			}
		case isAuthFailure(resp.StatusCode):
			// Bad key: rest it (KeyRing has no "disable forever", so cooldown
			// is the best we can do without expanding the API).
			g.ring.Rest(keyIdx)
			return nil, &ProviderError{
				Provider:   g.ID(),
				StatusCode: resp.StatusCode,
				Retryable:  false,
				Body:       truncateBody(raw),
			}
		default:
			return nil, &ProviderError{
				Provider:   g.ID(),
				StatusCode: resp.StatusCode,
				Retryable:  resp.StatusCode >= 500,
				Body:       truncateBody(raw),
			}
		}
	}

	var parsed groqResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, &ProviderError{Provider: g.ID(), Cause: fmt.Errorf("decode: %w", err), Retryable: false}
	}
	if len(parsed.Choices) == 0 {
		return nil, &ProviderError{Provider: g.ID(), Cause: fmt.Errorf("empty choices"), Retryable: false}
	}
	content := parsed.Choices[0].Message.Content

	return &Response{
		Content:          content,
		Provider:         g.ID(),
		Model:            parsed.Model,
		KeyIndex:         keyIdx,
		LatencyMs:        latency.Milliseconds(),
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		FinishReason:     parsed.Choices[0].FinishReason,
		RawJSON:          raw,
	}, nil
}

// isGroqModelHint reports whether the caller's ModelHint looks like a Groq
// model name (rather than an OpenAI-family name like "gpt-4o-mini"). Very
// coarse — if it starts with "llama", "mixtral", "gemma", or "deepseek",
// we forward it as-is.
func isGroqModelHint(hint string) bool {
	hint = strings.ToLower(strings.TrimSpace(hint))
	switch {
	case strings.HasPrefix(hint, "llama"),
		strings.HasPrefix(hint, "mixtral"),
		strings.HasPrefix(hint, "gemma"),
		strings.HasPrefix(hint, "deepseek"),
		strings.HasPrefix(hint, "qwen"):
		return true
	}
	return false
}

// --- Groq wire types (OpenAI-compatible subset) ---

type groqRequest struct {
	Model          string              `json:"model"`
	Messages       []groqMessage       `json:"messages"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
	Temperature    float64             `json:"temperature,omitempty"`
	Seed           int64               `json:"seed,omitempty"`
	ResponseFormat *groqResponseFormat `json:"response_format,omitempty"`
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqResponseFormat struct {
	Type string `json:"type"`
}

type groqResponse struct {
	Model   string       `json:"model"`
	Choices []groqChoice `json:"choices"`
	Usage   groqUsage    `json:"usage"`
}

type groqChoice struct {
	Message      groqMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type groqUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// groqMessagesFromRequest converts our generic Request into Groq-style messages.
// The Groq API uses OpenAI's shape: an array of {role, content} entries.
func groqMessagesFromRequest(req Request) []groqMessage {
	out := make([]groqMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		out = append(out, groqMessage{Role: string(m.Role), Content: m.Content})
	}
	return out
}
