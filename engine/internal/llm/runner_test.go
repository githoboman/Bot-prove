package llm

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// fakeProvider is a controllable Provider used only in tests.
type fakeProvider struct {
	id       string
	tier     Tier
	keyCount int
	// delay before returning
	delay time.Duration
	// what to return
	resp *Response
	err  error
	// calls counts invocations
	calls int32
	// cancelled is set true if ctx was cancelled while sleeping
	cancelled int32
}

func (f *fakeProvider) ID() string    { return f.id }
func (f *fakeProvider) Tier() Tier    { return f.tier }
func (f *fakeProvider) KeyCount() int { return f.keyCount }

func (f *fakeProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			atomic.StoreInt32(&f.cancelled, 1)
			return nil, &ProviderError{Provider: f.id, Cause: ctx.Err(), Retryable: true}
		}
	}
	// After the delay elapses, check ctx one more time — Go's select may
	// pick time.After when both are ready, but we want cancellation to win.
	if err := ctx.Err(); err != nil {
		atomic.StoreInt32(&f.cancelled, 1)
		return nil, &ProviderError{Provider: f.id, Cause: err, Retryable: true}
	}
	if f.err != nil {
		return nil, f.err
	}
	// Clone so KeyIndex etc survive round-trip.
	c := *f.resp
	c.Provider = f.id
	return &c, nil
}

func newFast(id string, delay time.Duration, resp *Response, err error) *fakeProvider {
	return &fakeProvider{id: id, tier: TierFast, keyCount: 1, delay: delay, resp: resp, err: err}
}

func newReliable(id string, delay time.Duration, resp *Response, err error) *fakeProvider {
	return &fakeProvider{id: id, tier: TierReliability, keyCount: 1, delay: delay, resp: resp, err: err}
}

func TestRunner_FanOut_FirstWins(t *testing.T) {
	winner := &Response{Content: "fast", KeyIndex: 0}
	slow := &Response{Content: "slow", KeyIndex: 0}

	fastA := newFast("a", 5*time.Millisecond, winner, nil)
	fastB := newFast("b", 100*time.Millisecond, slow, nil)

	cfg := Config{PerProviderTimeout: 1 * time.Second, TotalBudget: 2 * time.Second}
	r := NewRunner([]Provider{fastA, fastB}, nil, cfg)

	resp, report, err := r.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Provider != "a" {
		t.Errorf("winner = %q, want a", resp.Provider)
	}
	if report.Winner != "a" {
		t.Errorf("report.Winner = %q", report.Winner)
	}
	if report.FastFanOutSize != 2 {
		t.Errorf("FastFanOutSize = %d", report.FastFanOutSize)
	}
	// Slow provider should have been cancelled — give the goroutine a moment.
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&fastB.cancelled) != 1 {
		t.Error("slow provider was not cancelled after a winner")
	}
}

func TestRunner_FanOut_AllFail_FallsToReliability(t *testing.T) {
	fastA := newFast("a", 0, nil, &ProviderError{Provider: "a", StatusCode: 500, Retryable: true})
	fastB := newFast("b", 0, nil, &ProviderError{Provider: "b", StatusCode: 500, Retryable: true})
	rel := newReliable("rel", 0, &Response{Content: "rel-ok"}, nil)

	cfg := Config{PerProviderTimeout: 1 * time.Second, TotalBudget: 2 * time.Second}
	r := NewRunner([]Provider{fastA, fastB, rel}, nil, cfg)

	resp, report, err := r.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Provider != "rel" {
		t.Errorf("winner = %q, want rel", resp.Provider)
	}
	// Report should show 3 attempts: 2 failed fast + 1 successful reliable.
	if got := len(report.Attempts); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
	// Both fast attempts should be marked failed.
	fastFailed := 0
	for _, a := range report.Attempts {
		if !a.Success && (a.Provider == "a" || a.Provider == "b") {
			fastFailed++
		}
	}
	if fastFailed != 2 {
		t.Errorf("failed fast attempts = %d, want 2", fastFailed)
	}
}

func TestRunner_AllFail_UsesFixture(t *testing.T) {
	fast := newFast("a", 0, nil, &ProviderError{Provider: "a", StatusCode: 500, Retryable: true})
	rel := newReliable("rel", 0, nil, &ProviderError{Provider: "rel", StatusCode: 500, Retryable: true})
	fx := NewFixtureProvider(FixtureConfig{Canned: "fx-out"})

	cfg := Config{PerProviderTimeout: 200 * time.Millisecond, TotalBudget: 500 * time.Millisecond, EnableFixtureFallback: true}
	r := NewRunner([]Provider{fast, rel}, fx, cfg)

	resp, report, err := r.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !resp.Fixture {
		t.Error("expected Fixture=true")
	}
	if resp.Content != "fx-out" {
		t.Errorf("Content = %q, want fx-out", resp.Content)
	}
	if !report.Fixture || report.Winner != "fixture" {
		t.Errorf("report should be fixture: %+v", report)
	}
}

func TestRunner_AllFail_NoFixture_Errors(t *testing.T) {
	fast := newFast("a", 0, nil, &ProviderError{Provider: "a", StatusCode: 500, Retryable: true})
	cfg := Config{PerProviderTimeout: 200 * time.Millisecond, TotalBudget: 500 * time.Millisecond}
	r := NewRunner([]Provider{fast}, nil, cfg)

	_, _, err := r.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if !errors.Is(err, ErrAllProvidersFailed) {
		t.Errorf("err = %v, want ErrAllProvidersFailed", err)
	}
}

func TestRunner_ForceFixture_SkipsReal(t *testing.T) {
	fast := newFast("a", 0, &Response{Content: "should not appear"}, nil)
	fx := NewFixtureProvider(FixtureConfig{Canned: "fx"})

	cfg := Config{PerProviderTimeout: 1 * time.Second, TotalBudget: 2 * time.Second, forceFixture: true}
	r := NewRunner([]Provider{fast}, fx, cfg)

	resp, _, err := r.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Provider != "fixture" || !resp.Fixture {
		t.Errorf("expected fixture, got %+v", resp)
	}
	if atomic.LoadInt32(&fast.calls) != 0 {
		t.Error("real provider was called under forceFixture")
	}
}

func TestRunner_TotalBudgetExhausted(t *testing.T) {
	slow := newFast("slow", 500*time.Millisecond, &Response{Content: "late"}, nil)
	fx := NewFixtureProvider(FixtureConfig{Canned: "fx"})
	// TotalBudget is the primary bound; the runner is expected to clamp
	// PerProviderTimeout down when it exceeds TotalBudget.
	cfg := Config{
		PerProviderTimeout:    1 * time.Second,
		TotalBudget:           50 * time.Millisecond,
		EnableFixtureFallback: true,
	}
	r := NewRunner([]Provider{slow}, fx, cfg)

	start := time.Now()
	resp, report, err := r.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("Complete: %v (report=%s)", err, report)
	}
	// Should have given up on slow and served the fixture.
	if !resp.Fixture {
		t.Errorf("expected fixture after budget exhaustion, got %+v (report=%s)", resp, report)
	}
	if dur > 800*time.Millisecond {
		t.Errorf("runner took too long: %v", dur)
	}
}

func TestRunner_ProviderWithZeroKeys_Dropped(t *testing.T) {
	realFast := newFast("real", 0, &Response{Content: "ok"}, nil)
	noKeys := &fakeProvider{id: "empty", tier: TierFast, keyCount: 0}
	cfg := Config{PerProviderTimeout: time.Second, TotalBudget: 2 * time.Second}
	r := NewRunner([]Provider{realFast, noKeys}, nil, cfg)

	resp, report, err := r.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Provider != "real" {
		t.Errorf("winner = %q", resp.Provider)
	}
	// FastFanOutSize should be 1 (empty was dropped).
	if report.FastFanOutSize != 1 {
		t.Errorf("FastFanOutSize = %d, want 1 (zero-key dropped)", report.FastFanOutSize)
	}
	if atomic.LoadInt32(&noKeys.calls) != 0 {
		t.Error("zero-key provider was called")
	}
}

func TestRunner_ReportString(t *testing.T) {
	r := &RunReport{
		Winner: "a", TotalLatencyMs: 42, FastFanOutSize: 2,
		Attempts: []providerAttempt{
			{Provider: "a", LatencyMs: 42, Success: true, KeyIndex: 0},
			{Provider: "b", LatencyMs: 41, Success: false, Status: 429, Retryable: true, Err: "rate"},
		},
	}
	s := r.String()
	if s == "" {
		t.Error("empty report")
	}
	// Just make sure critical fields appear.
	for _, want := range []string{"a", "b", "429", "42"} {
		if !containsStr(s, want) {
			t.Errorf("report missing %q: %s", want, s)
		}
	}
}

func containsStr(s, sub string) bool {
	return len(sub) == 0 || len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Sanity: fixture provider serves the expected content on a table hit.
func TestFixture_TableHit(t *testing.T) {
	key := FixtureKeyFromMessages([]Message{{Role: RoleUser, Content: "hello"}})
	fx := NewFixtureProvider(FixtureConfig{
		Table:  map[string]string{key: "hi back"},
		Canned: "default",
	})
	resp, err := fx.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "hi back" {
		t.Errorf("Content = %q, want 'hi back'", resp.Content)
	}
	// Miss falls to canned.
	resp2, _ := fx.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "other"}}})
	if resp2.Content != "default" {
		t.Errorf("miss Content = %q", resp2.Content)
	}
	// Sanity string.
	if got := fmt.Sprint(fx); got == "" {
		t.Error("String() empty")
	}
}
