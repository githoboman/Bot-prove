package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGemini_HappyPath(t *testing.T) {
	var captured geminiRequest
	var capturedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.URL.Query().Get("key")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "answer part 1 "}, {Text: "part 2"}},
				},
				FinishReason: "STOP",
			}},
			UsageMetadata: geminiUsageMetadata{
				PromptTokenCount: 12, CandidatesTokenCount: 34, TotalTokenCount: 46,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ring := NewKeyRing([]string{"gem-key"}, time.Second)
	prov := NewGemini(ring).WithBaseURL(srv.URL)

	if prov.ID() != "gemini" {
		t.Errorf("ID = %q", prov.ID())
	}
	if prov.Tier() != TierReliability {
		t.Errorf("Tier = %v, want TierReliability", prov.Tier())
	}

	resp, err := prov.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "you are a judge"},
			{Role: RoleUser, Content: "verdict?"},
		},
		Temperature: 0.2,
		MaxTokens:   100,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "answer part 1 part 2" {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.PromptTokens != 12 || resp.CompletionTokens != 34 {
		t.Errorf("tokens: %+v", resp)
	}
	if resp.FinishReason != "STOP" {
		t.Errorf("FinishReason = %q", resp.FinishReason)
	}
	if capturedKey != "gem-key" {
		t.Errorf("URL key = %q, want gem-key", capturedKey)
	}
	// System instruction should be lifted out.
	if captured.SystemInstruction == nil || len(captured.SystemInstruction.Parts) == 0 {
		t.Fatal("missing system_instruction")
	}
	if captured.SystemInstruction.Parts[0].Text != "you are a judge" {
		t.Errorf("system text = %q", captured.SystemInstruction.Parts[0].Text)
	}
	// Contents should only have the user turn.
	if len(captured.Contents) != 1 || captured.Contents[0].Role != "user" {
		t.Errorf("contents = %+v", captured.Contents)
	}
	if captured.GenerationConfig == nil || captured.GenerationConfig.MaxOutputTokens != 100 {
		t.Errorf("generation_config = %+v", captured.GenerationConfig)
	}
}

func TestGemini_RateLimit_RestsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"error":"quota"}`)
	}))
	defer srv.Close()
	ring := NewKeyRing([]string{"k"}, time.Second)
	prov := NewGemini(ring).WithBaseURL(srv.URL)
	_, err := prov.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	pe, ok := err.(*ProviderError)
	if !ok || !pe.Retryable || pe.StatusCode != 429 {
		t.Errorf("want retryable 429 ProviderError, got %v", err)
	}
	snap := ring.HealthSnapshot()
	if !snap[0].Resting {
		t.Error("expected key to be resting after 429")
	}
}

func TestGemini_JSONMode(t *testing.T) {
	var captured geminiRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(geminiResponse{
			Candidates: []geminiCandidate{{
				Content:      geminiContent{Parts: []geminiPart{{Text: `{"v":1}`}}},
				FinishReason: "STOP",
			}},
		})
	}))
	defer srv.Close()
	ring := NewKeyRing([]string{"k"}, time.Second)
	prov := NewGemini(ring).WithBaseURL(srv.URL)
	_, err := prov.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "x"}},
		JSONMode: true,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if captured.GenerationConfig == nil ||
		!strings.EqualFold(captured.GenerationConfig.ResponseMimeType, "application/json") {
		t.Errorf("expected responseMimeType=application/json, got %+v", captured.GenerationConfig)
	}
}
