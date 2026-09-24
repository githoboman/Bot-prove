package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GeminiProvider talks to Google's Gemini API (generativelanguage.googleapis.com).
// The wire format differs from OpenAI (contents/parts instead of messages);
// it's kept in its own file so the shape is explicit.
//
// API docs: https://ai.google.dev/api/generate-content
// Env: GEMINI_API_KEY (single key; free-tier gets 15 rpm / 1M tpd).
type GeminiProvider struct {
	ring    *KeyRing
	client  *http.Client
	model   string
	baseURL string
}

// NewGemini builds a Gemini provider. Default model: gemini-2.0-flash-exp
// (fast, high context, free tier friendly).
func NewGemini(ring *KeyRing) *GeminiProvider {
	return &GeminiProvider{
		ring:    ring,
		client:  &http.Client{},
		model:   "gemini-2.0-flash-exp",
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
	}
}

// WithModel overrides the default model.
func (g *GeminiProvider) WithModel(m string) *GeminiProvider { g.model = m; return g }

// WithBaseURL overrides base URL (test hook).
func (g *GeminiProvider) WithBaseURL(u string) *GeminiProvider {
	g.baseURL = strings.TrimRight(u, "/")
	return g
}

// ID reports "gemini".
func (g *GeminiProvider) ID() string { return "gemini" }

// Tier reports TierReliability (Gemini is our long-context fallback).
func (g *GeminiProvider) Tier() Tier { return TierReliability }

// KeyCount reports the number of usable keys.
func (g *GeminiProvider) KeyCount() int { return g.ring.Len() }

// Complete performs a Gemini generateContent call and maps it back into the
// unified Response shape.
func (g *GeminiProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	if g.ring.Len() == 0 {
		return nil, &ProviderError{Provider: g.ID(), Cause: ErrNoKeys}
	}
	start := time.Now()

	key, keyIdx, err := g.ring.Next()
	if err != nil {
		return nil, &ProviderError{Provider: g.ID(), Cause: err, Retryable: false}
	}

	model := req.ModelHint
	if model == "" || !strings.HasPrefix(model, "gemini") {
		model = g.model
	}

	// Split system from user/assistant turns — Gemini has a dedicated
	// system_instruction field.
	body := geminiRequest{}
	var systemText string
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			if systemText != "" {
				systemText += "\n"
			}
			systemText += m.Content
		case RoleUser, RoleAssistant:
			role := "user"
			if m.Role == RoleAssistant {
				role = "model"
			}
			body.Contents = append(body.Contents, geminiContent{
				Role:  role,
				Parts: []geminiPart{{Text: m.Content}},
			})
		}
	}
	if systemText != "" {
		body.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: systemText}}}
	}
	if req.MaxTokens > 0 || req.Temperature > 0 || req.JSONMode {
		body.GenerationConfig = &geminiGenerationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
		}
		if req.JSONMode {
			body.GenerationConfig.ResponseMimeType = "application/json"
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, &ProviderError{Provider: g.ID(), Cause: fmt.Errorf("marshal: %w", err)}
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		g.baseURL, model, url.QueryEscape(key))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, &ProviderError{Provider: g.ID(), Cause: fmt.Errorf("build request: %w", err)}
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Provider: g.ID(), Cause: err, Retryable: true}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProviderError{Provider: g.ID(), Cause: fmt.Errorf("read: %w", err), Retryable: true}
	}
	latency := time.Since(start)

	if resp.StatusCode >= 400 {
		switch {
		case isRateLimitStatus(resp.StatusCode):
			g.ring.Rest(keyIdx)
			return nil, &ProviderError{Provider: g.ID(), StatusCode: resp.StatusCode, Retryable: true, Body: truncateBody(raw)}
		case isAuthFailure(resp.StatusCode):
			g.ring.Rest(keyIdx)
			return nil, &ProviderError{Provider: g.ID(), StatusCode: resp.StatusCode, Retryable: false, Body: truncateBody(raw)}
		default:
			return nil, &ProviderError{Provider: g.ID(), StatusCode: resp.StatusCode, Retryable: resp.StatusCode >= 500, Body: truncateBody(raw)}
		}
	}

	var parsed geminiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, &ProviderError{Provider: g.ID(), Cause: fmt.Errorf("decode: %w", err), Retryable: false}
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, &ProviderError{Provider: g.ID(), Cause: fmt.Errorf("empty candidates"), Retryable: false}
	}
	// Concatenate all text parts of the first candidate.
	var buf strings.Builder
	for _, p := range parsed.Candidates[0].Content.Parts {
		buf.WriteString(p.Text)
	}

	return &Response{
		Content:          buf.String(),
		Provider:         g.ID(),
		Model:            model, // Gemini doesn't echo model in response
		KeyIndex:         keyIdx,
		LatencyMs:        latency.Milliseconds(),
		PromptTokens:     parsed.UsageMetadata.PromptTokenCount,
		CompletionTokens: parsed.UsageMetadata.CandidatesTokenCount,
		FinishReason:     parsed.Candidates[0].FinishReason,
		RawJSON:          raw,
	}, nil
}

// --- Gemini wire types ---

type geminiRequest struct {
	Contents          []geminiContent         `json:"contents,omitempty"`
	SystemInstruction *geminiContent          `json:"system_instruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generation_config,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
	Temperature      float64 `json:"temperature,omitempty"`
	ResponseMimeType string  `json:"responseMimeType,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate   `json:"candidates"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}
