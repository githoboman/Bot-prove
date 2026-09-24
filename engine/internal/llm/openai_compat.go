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

// OpenAICompatProvider is a generic OpenAI-compatible chat provider used for
// OpenRouter, Z.AI, and any other provider that speaks the OpenAI
// /chat/completions wire format. Groq is *not* built on top of this because
// its model-hint mapping differs; keeping Groq separate keeps its file self-
// contained and the OpenAI-compat path uncoupled from Groq quirks.
type OpenAICompatProvider struct {
	id      string
	tier    Tier
	ring    *KeyRing
	client  *http.Client
	baseURL string
	model   string
	// extraHeaders are provider-specific headers set on every request.
	// OpenRouter for instance wants "HTTP-Referer" and "X-Title".
	extraHeaders map[string]string
}

// OpenAICompatConfig configures a provider variant.
type OpenAICompatConfig struct {
	ID           string            // provider ID, e.g. "openrouter"
	Tier         Tier              // TierFast or TierReliability
	BaseURL      string            // e.g. https://openrouter.ai/api/v1
	Model        string            // default model
	ExtraHeaders map[string]string // provider-specific static headers
}

// NewOpenAICompatProvider builds an OpenAI-compatible provider.
func NewOpenAICompatProvider(cfg OpenAICompatConfig, ring *KeyRing) *OpenAICompatProvider {
	if cfg.ID == "" {
		cfg.ID = "openai-compat"
	}
	return &OpenAICompatProvider{
		id:           cfg.ID,
		tier:         cfg.Tier,
		ring:         ring,
		client:       &http.Client{},
		baseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		model:        cfg.Model,
		extraHeaders: cfg.ExtraHeaders,
	}
}

// ID reports the stable provider ID.
func (p *OpenAICompatProvider) ID() string { return p.id }

// Tier reports the configured tier.
func (p *OpenAICompatProvider) Tier() Tier { return p.tier }

// KeyCount reports the number of usable keys.
func (p *OpenAICompatProvider) KeyCount() int { return p.ring.Len() }

// WithBaseURL overrides base URL (test hook).
func (p *OpenAICompatProvider) WithBaseURL(url string) *OpenAICompatProvider {
	p.baseURL = strings.TrimRight(url, "/")
	return p
}

// Complete performs a chat completion. See GroqProvider.Complete for the
// error/rest-on-429 semantics — they are identical here.
func (p *OpenAICompatProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	if p.ring.Len() == 0 {
		return nil, &ProviderError{Provider: p.id, Cause: ErrNoKeys}
	}
	start := time.Now()

	key, keyIdx, err := p.ring.Next()
	if err != nil {
		return nil, &ProviderError{Provider: p.id, Cause: err, Retryable: false}
	}

	model := req.ModelHint
	if model == "" {
		model = p.model
	}

	body := groqRequest{ // identical wire shape
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
		return nil, &ProviderError{Provider: p.id, Cause: fmt.Errorf("marshal: %w", err)}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, &ProviderError{Provider: p.id, Cause: fmt.Errorf("build request: %w", err)}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+key)
	for k, v := range p.extraHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Provider: p.id, Cause: err, Retryable: true}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProviderError{Provider: p.id, Cause: fmt.Errorf("read: %w", err), Retryable: true}
	}
	latency := time.Since(start)

	if resp.StatusCode >= 400 {
		switch {
		case isRateLimitStatus(resp.StatusCode):
			p.ring.Rest(keyIdx)
			return nil, &ProviderError{
				Provider: p.id, StatusCode: resp.StatusCode, Retryable: true, Body: truncateBody(raw),
			}
		case isAuthFailure(resp.StatusCode):
			p.ring.Rest(keyIdx)
			return nil, &ProviderError{
				Provider: p.id, StatusCode: resp.StatusCode, Retryable: false, Body: truncateBody(raw),
			}
		default:
			return nil, &ProviderError{
				Provider: p.id, StatusCode: resp.StatusCode, Retryable: resp.StatusCode >= 500, Body: truncateBody(raw),
			}
		}
	}

	var parsed groqResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, &ProviderError{Provider: p.id, Cause: fmt.Errorf("decode: %w", err), Retryable: false}
	}
	if len(parsed.Choices) == 0 {
		return nil, &ProviderError{Provider: p.id, Cause: fmt.Errorf("empty choices"), Retryable: false}
	}
	content := parsed.Choices[0].Message.Content

	return &Response{
		Content:          content,
		Provider:         p.id,
		Model:            parsed.Model,
		KeyIndex:         keyIdx,
		LatencyMs:        latency.Milliseconds(),
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		FinishReason:     parsed.Choices[0].FinishReason,
		RawJSON:          raw,
	}, nil
}

// --- concrete provider constructors ---

// NewOpenRouter builds an OpenRouter provider (Tier reliability by default —
// it's an aggregator, higher latency but very reliable and gives access to
// Claude/GPT-family models without direct keys).
func NewOpenRouter(ring *KeyRing) *OpenAICompatProvider {
	return NewOpenAICompatProvider(OpenAICompatConfig{
		ID:      "openrouter",
		Tier:    TierReliability,
		BaseURL: "https://openrouter.ai/api/v1",
		Model:   "meta-llama/llama-3.1-8b-instruct", // fast + cheap default
		ExtraHeaders: map[string]string{
			// OpenRouter uses these for attribution/analytics.
			"HTTP-Referer": "https://casperprover.ai",
			"X-Title":      "CasperProver Judge",
		},
	}, ring)
}

// NewZAI builds a Z.AI (bigmodel.cn) provider. Their /paas/v4/ endpoint is
// OpenAI-compatible. Default model glm-4-flash is fast + free-tier friendly.
func NewZAI(ring *KeyRing) *OpenAICompatProvider {
	return NewOpenAICompatProvider(OpenAICompatConfig{
		ID:      "zai",
		Tier:    TierFast,
		BaseURL: "https://api.z.ai/api/paas/v4",
		Model:   "glm-4-flash",
	}, ring)
}

// NewNVIDIA builds an NVIDIA NIM provider. Their integrate.api.nvidia.com/v1
// endpoint is OpenAI-compatible. Fast Llama-3.1 inference on datacenter GPUs;
// free tier available with per-account rate limits.
func NewNVIDIA(ring *KeyRing) *OpenAICompatProvider {
	return NewOpenAICompatProvider(OpenAICompatConfig{
		ID:      "nvidia",
		Tier:    TierFast,
		BaseURL: "https://integrate.api.nvidia.com/v1",
		Model:   "meta/llama-3.1-8b-instruct",
	}, ring)
}
