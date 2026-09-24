package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stubOpenAICompatServer returns an OpenAI-compatible /chat/completions
// endpoint that captures the incoming request for assertions.
func stubOpenAICompatServer(t *testing.T, capture *http.Request) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			// Copy interesting fields into the caller's slot.
			*capture = *r.Clone(context.Background())
		}
		resp := groqResponse{
			Model: "test-model",
			Choices: []groqChoice{{
				Message:      groqMessage{Role: "assistant", Content: "ok"},
				FinishReason: "stop",
			}},
			Usage: groqUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestOpenRouter_ExtraHeaders(t *testing.T) {
	var captured http.Request
	srv := stubOpenAICompatServer(t, &captured)
	defer srv.Close()

	ring := NewKeyRing([]string{"k"}, time.Second)
	prov := NewOpenRouter(ring).WithBaseURL(srv.URL)

	if prov.ID() != "openrouter" {
		t.Errorf("ID = %q", prov.ID())
	}
	if prov.Tier() != TierReliability {
		t.Errorf("Tier = %v, want TierReliability", prov.Tier())
	}

	_, err := prov.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := captured.Header.Get("HTTP-Referer"); got != "https://casperprover.ai" {
		t.Errorf("HTTP-Referer = %q", got)
	}
	if got := captured.Header.Get("X-Title"); got != "CasperProver Judge" {
		t.Errorf("X-Title = %q", got)
	}
	if got := captured.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
		t.Errorf("Authorization = %q", got)
	}
}

func TestZAI_Basics(t *testing.T) {
	srv := stubOpenAICompatServer(t, nil)
	defer srv.Close()

	ring := NewKeyRing([]string{"k"}, time.Second)
	prov := NewZAI(ring).WithBaseURL(srv.URL)

	if prov.ID() != "zai" {
		t.Errorf("ID = %q", prov.ID())
	}
	if prov.Tier() != TierFast {
		t.Errorf("Tier = %v, want TierFast", prov.Tier())
	}
	resp, err := prov.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.Provider != "zai" {
		t.Errorf("Provider = %q", resp.Provider)
	}
}

func TestOpenAICompat_500Retryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":"boom"}`)
	}))
	defer srv.Close()

	ring := NewKeyRing([]string{"k"}, time.Second)
	prov := NewZAI(ring).WithBaseURL(srv.URL)
	_, err := prov.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(*ProviderError)
	if !ok || !pe.Retryable {
		t.Errorf("want retryable ProviderError, got %v", err)
	}
	if pe.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d", pe.StatusCode)
	}
}
