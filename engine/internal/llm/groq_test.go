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

// happyGroqServer returns a stub Groq endpoint that echoes the last user
// message back as the assistant reply.
func happyGroqServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Errorf("missing bearer auth: %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		var req groqRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		last := ""
		for _, m := range req.Messages {
			if m.Role == "user" {
				last = m.Content
			}
		}
		resp := groqResponse{
			Model: req.Model,
			Choices: []groqChoice{{
				Message:      groqMessage{Role: "assistant", Content: "echo: " + last},
				FinishReason: "stop",
			}},
			Usage: groqUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestGroq_HappyPath(t *testing.T) {
	srv := happyGroqServer(t)
	defer srv.Close()

	ring := NewKeyRing([]string{"key-A"}, 5*time.Second)
	prov := NewGroqProvider(ring).WithBaseURL(srv.URL)

	if prov.ID() != "groq" {
		t.Errorf("ID = %q, want groq", prov.ID())
	}
	if prov.Tier() != TierFast {
		t.Errorf("Tier = %v, want TierFast", prov.Tier())
	}
	if prov.KeyCount() != 1 {
		t.Errorf("KeyCount = %d, want 1", prov.KeyCount())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := prov.Complete(ctx, Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "you are a judge"},
			{Role: RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "echo: hello" {
		t.Errorf("Content = %q, want echo: hello", resp.Content)
	}
	if resp.Provider != "groq" {
		t.Errorf("Provider = %q", resp.Provider)
	}
	if resp.PromptTokens != 10 || resp.CompletionTokens != 20 {
		t.Errorf("usage mismatch: %+v", resp)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q", resp.FinishReason)
	}
	if resp.LatencyMs < 0 {
		t.Errorf("LatencyMs = %d, want >= 0", resp.LatencyMs)
	}
	if resp.KeyIndex != 0 {
		t.Errorf("KeyIndex = %d, want 0", resp.KeyIndex)
	}
	if len(resp.RawJSON) == 0 {
		t.Error("RawJSON is empty")
	}
	// Canonical should be stable and content-only.
	canon, err := resp.Canonical()
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	if !strings.Contains(string(canon), `"provider":"groq"`) {
		t.Errorf("Canonical missing provider: %s", canon)
	}
	if strings.Contains(string(canon), "latency") {
		t.Errorf("Canonical must not include latency: %s", canon)
	}
}

func TestGroq_RateLimit_RestsKey(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"error":"rate_limited"}`)
	}))
	defer srv.Close()

	ring := NewKeyRing([]string{"key-A", "key-B"}, 2*time.Second)
	prov := NewGroqProvider(ring).WithBaseURL(srv.URL)

	ctx := context.Background()
	_, err := prov.Complete(ctx, Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err == nil {
		t.Fatal("expected error on 429")
	}
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("want ProviderError, got %T: %v", err, err)
	}
	if pe.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d", pe.StatusCode)
	}
	if !pe.Retryable {
		t.Error("expected Retryable on 429")
	}

	// One key should now be resting.
	snap := ring.HealthSnapshot()
	resting := 0
	for _, s := range snap {
		if s.Resting {
			resting++
		}
	}
	if resting != 1 {
		t.Errorf("resting count = %d, want 1", resting)
	}

	// Second call should hit the other key.
	_, _ = prov.Complete(ctx, Request{Messages: []Message{{Role: RoleUser, Content: "y"}}})

	// Both keys should now be resting.
	snap = ring.HealthSnapshot()
	resting = 0
	for _, s := range snap {
		if s.Resting {
			resting++
		}
	}
	if resting != 2 {
		t.Errorf("after second 429, resting count = %d, want 2", resting)
	}

	// Third call: all keys cooling, expect ErrAllKeysCooling wrapped.
	_, err = prov.Complete(ctx, Request{Messages: []Message{{Role: RoleUser, Content: "z"}}})
	if err == nil {
		t.Fatal("expected error when all keys cooling")
	}
	pe, ok = err.(*ProviderError)
	if !ok {
		t.Fatalf("want ProviderError, got %T: %v", err, err)
	}
	if pe.Cause != ErrAllKeysCooling {
		t.Errorf("Cause = %v, want ErrAllKeysCooling", pe.Cause)
	}
	if hits != 2 {
		t.Errorf("server hits = %d, want 2 (third should not reach the server)", hits)
	}
}

func TestGroq_AuthFailure_RestsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":"invalid_api_key"}`)
	}))
	defer srv.Close()

	ring := NewKeyRing([]string{"key-A"}, 30*time.Second)
	prov := NewGroqProvider(ring).WithBaseURL(srv.URL)

	_, err := prov.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("want ProviderError, got %T", err)
	}
	if pe.Retryable {
		t.Error("401 should not be retryable")
	}
	// Key should be resting.
	snap := ring.HealthSnapshot()
	if len(snap) != 1 || !snap[0].Resting {
		t.Errorf("expected key to be resting after 401: %+v", snap)
	}
}

func TestGroq_NoKeys(t *testing.T) {
	ring := NewKeyRing(nil, time.Second)
	prov := NewGroqProvider(ring).WithBaseURL("http://unused")
	if prov.KeyCount() != 0 {
		t.Errorf("KeyCount = %d, want 0", prov.KeyCount())
	}
	_, err := prov.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("want ProviderError, got %T", err)
	}
	if pe.Cause != ErrNoKeys {
		t.Errorf("Cause = %v, want ErrNoKeys", pe.Cause)
	}
}

func TestGroq_ContextCancel(t *testing.T) {
	// Server that sleeps 500ms; client cancels immediately.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	ring := NewKeyRing([]string{"k"}, time.Second)
	prov := NewGroqProvider(ring).WithBaseURL(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := prov.Complete(ctx, Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err == nil {
		t.Fatal("expected error on ctx timeout")
	}
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("want ProviderError, got %T: %v", err, err)
	}
	if !pe.Retryable {
		t.Error("network/timeout should be retryable")
	}
}

func TestGroq_ModelHint(t *testing.T) {
	tests := []struct {
		hint string
		want bool
	}{
		{"", false},
		{"llama-3.1-70b", true},
		{"Llama-3.1-8B-instant", true},
		{"gpt-4o-mini", false},
		{"claude-3-opus", false},
		{"mixtral-8x7b", true},
		{"gemma-9b", true},
		{"deepseek-r1", true},
		{"qwen-2.5", true},
	}
	for _, tc := range tests {
		if got := isGroqModelHint(tc.hint); got != tc.want {
			t.Errorf("isGroqModelHint(%q) = %v, want %v", tc.hint, got, tc.want)
		}
	}
}
