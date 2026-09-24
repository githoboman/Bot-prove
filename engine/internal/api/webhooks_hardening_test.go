package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// jitteredBackoff — full-jitter behaviour
// -----------------------------------------------------------------------------

func TestJitteredBackoffStaysInsideCap(t *testing.T) {
	// Cap for attempt N is initial<<(N-1), min(webhookMaxBackoff).
	cases := []struct {
		attempt int
		cap     time.Duration
	}{
		{1, webhookInitialBackoff},
		{2, 2 * webhookInitialBackoff},
		{3, 4 * webhookInitialBackoff},
		{4, 8 * webhookInitialBackoff},
		{20, webhookMaxBackoff}, // saturated
	}
	for _, tc := range cases {
		for i := 0; i < 200; i++ {
			got := jitteredBackoff(tc.attempt)
			if got < 0 || got >= tc.cap {
				t.Fatalf("attempt=%d cap=%s got %s out of [0,cap)", tc.attempt, tc.cap, got)
			}
		}
	}
}

func TestJitteredBackoffAttemptZeroTreatedAsOne(t *testing.T) {
	// A defensive zero/negative attempt must not panic or return
	// nonsense; we treat it as attempt=1 (cap = initial backoff).
	for i := 0; i < 100; i++ {
		got := jitteredBackoff(0)
		if got < 0 || got >= webhookInitialBackoff {
			t.Fatalf("attempt=0 got %s, want in [0,%s)", got, webhookInitialBackoff)
		}
	}
	for i := 0; i < 100; i++ {
		got := jitteredBackoff(-5)
		if got < 0 || got >= webhookInitialBackoff {
			t.Fatalf("attempt=-5 got %s, want in [0,%s)", got, webhookInitialBackoff)
		}
	}
}

func TestJitteredBackoffProducesSpread(t *testing.T) {
	// Fixed attempt, many draws — expect at least two distinct values.
	// A degenerate implementation returning the cap every time would
	// fail this. The chance of 200 identical draws from ~4 billion
	// ns is astronomically small.
	seen := map[time.Duration]struct{}{}
	for i := 0; i < 200; i++ {
		seen[jitteredBackoff(4)] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatalf("jittered backoff collapsed to a single value: %v", seen)
	}
}

// -----------------------------------------------------------------------------
// parseRetryAfter
// -----------------------------------------------------------------------------

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"empty", "", 0},
		{"whitespace", "   ", 0},
		{"negative_seconds", "-3", 0},
		{"seconds", "30", 30 * time.Second},
		{"seconds_padded", "  45  ", 45 * time.Second},
		{"malformed", "later", 0},
		{"past_http_date", now.Add(-5 * time.Minute).UTC().Format(http.TimeFormat), 0},
		{"future_http_date", now.Add(90 * time.Second).UTC().Format(http.TimeFormat), 90 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRetryAfter(tc.in, now)
			// HTTP dates round to seconds; allow ±1 s slop.
			if diff := tc.want - got; diff < -time.Second || diff > time.Second {
				t.Fatalf("parseRetryAfter(%q) = %s, want ~%s", tc.in, got, tc.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Retry-After honoured on 429/503
// -----------------------------------------------------------------------------

func TestRetryAfterHonouredOn429(t *testing.T) {
	// Receiver returns 429 with Retry-After: 90. Expect NextTryAt to
	// be pushed at least 90 s into the future regardless of the
	// jittered baseline (which caps at 1 s for attempt 1).
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "90")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	fixed := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	store := newWebhookStore()
	store.now = func() time.Time { return fixed }
	if _, err := store.register("owner", srv.URL, "s3cret", []string{EventProofVerified}); err != nil {
		t.Fatalf("register: %v", err)
	}
	store.enqueue(EventProofVerified, mustJSON(t, map[string]string{"hello": "world"}))
	store.deliverOnce(context.Background())
	if hits.Load() != 1 {
		t.Fatalf("expected 1 hit, got %d", hits.Load())
	}
	store.mu.RLock()
	if len(store.queue) != 1 {
		store.mu.RUnlock()
		t.Fatalf("expected event requeued, got queue len %d", len(store.queue))
	}
	next := store.queue[0].NextTryAt
	store.mu.RUnlock()
	wantMin := fixed.Add(90 * time.Second)
	if next.Before(wantMin) {
		t.Fatalf("NextTryAt=%s want >= %s (retry-after: 90s)", next, wantMin)
	}
}

func TestRetryAfterCappedByCeiling(t *testing.T) {
	// Receiver asks for a wildly long Retry-After; we cap it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", strconv.Itoa(int((24 * time.Hour).Seconds())))
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	fixed := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	store := newWebhookStore()
	store.now = func() time.Time { return fixed }
	if _, err := store.register("owner", srv.URL, "", []string{EventProofVerified}); err != nil {
		t.Fatalf("register: %v", err)
	}
	store.enqueue(EventProofVerified, mustJSON(t, map[string]string{"x": "y"}))
	store.deliverOnce(context.Background())
	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.queue) != 1 {
		t.Fatalf("expected event requeued, got queue len %d", len(store.queue))
	}
	delay := store.queue[0].NextTryAt.Sub(fixed)
	if delay > webhookRetryAfterCeiling+time.Second {
		t.Fatalf("retry-after not capped: delay=%s ceiling=%s", delay, webhookRetryAfterCeiling)
	}
}

// -----------------------------------------------------------------------------
// Circuit breaker — trips after N consecutive failures, deliverOnce skips
// -----------------------------------------------------------------------------

func TestCircuitBreakerTripsAndSkips(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	fixed := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	store := newWebhookStore()
	store.now = func() time.Time { return fixed }
	sub, err := store.register("owner", srv.URL, "", []string{EventProofVerified})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Enqueue circuitBreakerThreshold events and drive them all to
	// failure by advancing "now" past each NextTryAt.
	for i := 0; i < circuitBreakerThreshold; i++ {
		store.enqueue(EventProofVerified, mustJSON(t, map[string]int{"n": i}))
	}
	// deliver until every event has fired once (they're all NextTryAt=now).
	store.deliverOnce(context.Background())
	if got := int(hits.Load()); got != circuitBreakerThreshold {
		t.Fatalf("expected %d hits on first pass, got %d", circuitBreakerThreshold, got)
	}
	// Sub should now be in the tripped state.
	store.mu.RLock()
	gotCF := sub.ConsecutiveFailures
	gotOpen := sub.CircuitOpenUntil
	gotTripped := sub.CircuitTrippedTotal
	store.mu.RUnlock()
	if gotCF < circuitBreakerThreshold {
		t.Fatalf("consecutive failures = %d, want >= %d", gotCF, circuitBreakerThreshold)
	}
	if !gotOpen.After(fixed) {
		t.Fatalf("circuit open until = %s, want > %s", gotOpen, fixed)
	}
	if gotTripped != 1 {
		t.Fatalf("circuit tripped total = %d, want 1", gotTripped)
	}

	// Advance "now" only slightly (still inside cool-down) and enqueue
	// a fresh event. deliverOnce must NOT hit the receiver.
	store.now = func() time.Time { return fixed.Add(30 * time.Second) }
	store.enqueue(EventProofVerified, mustJSON(t, map[string]string{"during": "cooldown"}))
	before := hits.Load()
	store.deliverOnce(context.Background())
	if hits.Load() != before {
		t.Fatalf("delivery fired during cool-down: before=%d after=%d", before, hits.Load())
	}

	// Advance past the cool-down and a successful receiver; circuit resets.
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()
	// Swap the sub URL in-place; only tests do this.
	store.mu.Lock()
	sub.URL = okSrv.URL
	// Reset NextTryAt on every queued event to "now" so they fire.
	after := fixed.Add(circuitBreakerCooldown + time.Second)
	for _, ev := range store.queue {
		ev.NextTryAt = after
	}
	store.mu.Unlock()
	store.now = func() time.Time { return after }
	store.deliverOnce(context.Background())
	store.mu.RLock()
	if sub.ConsecutiveFailures != 0 {
		store.mu.RUnlock()
		t.Fatalf("consecutive failures not reset after 2xx: %d", sub.ConsecutiveFailures)
	}
	if !sub.CircuitOpenUntil.IsZero() {
		store.mu.RUnlock()
		t.Fatalf("circuit open until not cleared after 2xx: %s", sub.CircuitOpenUntil)
	}
	store.mu.RUnlock()
}

// -----------------------------------------------------------------------------
// Dead-letter TTL eviction
// -----------------------------------------------------------------------------

func TestDeadLetterTTLEviction(t *testing.T) {
	fixed := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	store := newWebhookStore()
	store.now = func() time.Time { return fixed }
	// Seed the DLQ with two entries — one old, one recent.
	store.mu.Lock()
	store.dead = append(store.dead,
		&deadLetter{
			Event:    webhookEvent{SubID: "wh_old", Kind: EventProofVerified},
			FailedAt: fixed.Add(-webhookDeadLetterTTL - time.Minute),
			URL:      "https://old.invalid",
		},
		&deadLetter{
			Event:    webhookEvent{SubID: "wh_new", Kind: EventProofVerified},
			FailedAt: fixed.Add(-time.Minute),
			URL:      "https://new.invalid",
		},
	)
	store.mu.Unlock()

	// A deliverOnce tick with an empty queue still runs the eviction.
	store.deliverOnce(context.Background())
	got := store.deadLetters()
	if len(got) != 1 {
		t.Fatalf("expected 1 dead-letter after eviction, got %d", len(got))
	}
	if got[0].Event.SubID != "wh_new" {
		t.Fatalf("wrong survivor: %+v", got[0])
	}
}

// -----------------------------------------------------------------------------
// Idempotency key header — deterministic per (sub, attempt, body)
// -----------------------------------------------------------------------------

func TestIdempotencyKeyDeterministic(t *testing.T) {
	body := []byte(`{"proof":"x"}`)
	got1 := idempotencyKey("wh_abc", 3, body)
	got2 := idempotencyKey("wh_abc", 3, body)
	if got1 != got2 {
		t.Fatalf("idempotency key not stable: %q vs %q", got1, got2)
	}
	if got := idempotencyKey("wh_abc", 4, body); got == got1 {
		t.Fatalf("attempt bump did not change key: %q", got)
	}
	if got := idempotencyKey("wh_abc", 3, []byte(`{"proof":"y"}`)); got == got1 {
		t.Fatalf("body change did not change key: %q", got)
	}
	// 8 hex chars = 4 bytes of SHA-256 body-prefix
	want := "wh_abc-3-"
	if got1[:len(want)] != want {
		t.Fatalf("prefix = %q, want %q", got1[:len(want)], want)
	}
	suffix := got1[len(want):]
	if _, err := hex.DecodeString(suffix); err != nil || len(suffix) != 8 {
		t.Fatalf("bad suffix %q (err=%v)", suffix, err)
	}
}

func TestDeliveryEmitsIdempotencyHeader(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-CP-Idempotency-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := newWebhookStore()
	fixed := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixed }
	sub, err := store.register("owner", srv.URL, "", []string{EventProofVerified})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	body := mustJSON(t, map[string]string{"id": "42"})
	store.enqueue(EventProofVerified, body)
	store.deliverOnce(context.Background())
	if gotKey == "" {
		t.Fatalf("no X-CP-Idempotency-Key emitted")
	}
	want := idempotencyKey(sub.ID, 1, body)
	if gotKey != want {
		t.Fatalf("idempotency key = %q, want %q", gotKey, want)
	}
}

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// force a compile-time reference so unused-import checkers do not
// prune fmt when the file is trimmed by a future refactor.
var _ = fmt.Sprintf
