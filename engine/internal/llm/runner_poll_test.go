package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRunner_Poll_AllSucceed verifies every provider is called in parallel
// and every response comes back.
func TestRunner_Poll_AllSucceed(t *testing.T) {
	t.Parallel()

	fastA := newFast("fastA", 0, &Response{Content: "A", Provider: "fastA"}, nil)
	fastB := newFast("fastB", 0, &Response{Content: "B", Provider: "fastB"}, nil)
	rel := newReliable("rel", 0, &Response{Content: "R", Provider: "rel"}, nil)

	r := NewRunner([]Provider{fastA, fastB, rel}, nil, Config{
		TotalBudget:        1 * time.Second,
		PerProviderTimeout: 500 * time.Millisecond,
	})

	results := r.Poll(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Every result should have a non-nil Resp.
	seen := make(map[string]bool)
	for _, res := range results {
		if res.Resp == nil {
			t.Errorf("provider %s returned nil resp: %s", res.Provider, res.Attempt.Err)
			continue
		}
		seen[res.Provider] = true
	}
	if !seen["fastA"] || !seen["fastB"] || !seen["rel"] {
		t.Errorf("missing provider in results: %+v", seen)
	}
}

// TestRunner_Poll_MixedResults verifies failed providers are captured with
// error info while successful ones return responses.
func TestRunner_Poll_MixedResults(t *testing.T) {
	t.Parallel()

	good := newFast("good", 0, &Response{Content: "ok", Provider: "good"}, nil)
	bad := newFast("bad", 0, nil, errors.New("boom"))

	r := NewRunner([]Provider{good, bad}, nil, Config{
		TotalBudget:        1 * time.Second,
		PerProviderTimeout: 500 * time.Millisecond,
	})

	results := r.Poll(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		switch res.Provider {
		case "good":
			if res.Resp == nil {
				t.Errorf("good provider should have Resp")
			}
			if !res.Attempt.Success {
				t.Errorf("good provider attempt should be success")
			}
		case "bad":
			if res.Resp != nil {
				t.Errorf("bad provider should have nil Resp")
			}
			if res.Attempt.Success {
				t.Errorf("bad provider attempt should be failure")
			}
			if res.Attempt.Err == "" {
				t.Errorf("bad provider attempt should carry Err text")
			}
		}
	}
}

// TestRunner_Poll_FixtureFallback verifies fixture is only invoked when
// zero real providers answered.
func TestRunner_Poll_FixtureFallback(t *testing.T) {
	t.Parallel()

	// All real providers fail.
	badA := newFast("badA", 0, nil, errors.New("boom"))
	badB := newFast("badB", 0, nil, errors.New("boom"))

	fx := NewFixtureProvider(FixtureConfig{Canned: "fixture-answer"})

	r := NewRunner([]Provider{badA, badB}, fx, Config{
		TotalBudget:           1 * time.Second,
		PerProviderTimeout:    500 * time.Millisecond,
		EnableFixtureFallback: true,
	})

	results := r.Poll(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})

	if len(results) != 3 {
		t.Fatalf("expected 3 results (2 real + fixture), got %d", len(results))
	}
	// Last result should be fixture.
	last := results[len(results)-1]
	if last.Provider != fx.ID() {
		t.Errorf("last result should be fixture, got %s", last.Provider)
	}
	if last.Resp == nil || !last.Resp.Fixture {
		t.Errorf("fixture result should have Resp.Fixture=true")
	}
}

// TestRunner_Poll_FixtureNotCalled_WhenAnyRealSucceeds verifies the fixture
// is NOT polled when at least one real provider answers — the judge must
// only see real signals in that case.
func TestRunner_Poll_FixtureNotCalled_WhenAnyRealSucceeds(t *testing.T) {
	t.Parallel()

	good := newFast("good", 0, &Response{Content: "real", Provider: "good"}, nil)
	bad := newFast("bad", 0, nil, errors.New("boom"))

	fx := NewFixtureProvider(FixtureConfig{Canned: "fixture-answer"})

	r := NewRunner([]Provider{good, bad}, fx, Config{
		TotalBudget:           1 * time.Second,
		PerProviderTimeout:    500 * time.Millisecond,
		EnableFixtureFallback: true,
	})

	results := r.Poll(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 results (fixture must NOT be appended), got %d", len(results))
	}
	for _, res := range results {
		if res.Resp != nil && res.Resp.Fixture {
			t.Errorf("fixture should not appear when a real provider succeeded")
		}
	}
}
