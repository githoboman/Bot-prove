package api

import (
	"testing"
	"time"
)

func TestPerKeyLimiter_FirstRequestAllowed(t *testing.T) {
	l := newPerKeyLimiter(5, 1)
	l.now = func() time.Time { return time.Unix(1000, 0) }
	if !l.allow("k1") {
		t.Fatal("first request must be allowed")
	}
}

func TestPerKeyLimiter_BucketDrains(t *testing.T) {
	l := newPerKeyLimiter(3, 0.001) // very slow refill
	base := time.Unix(1000, 0)
	l.now = func() time.Time { return base }
	// 3 tokens (maxTokens=3): first call deducts one immediately at init (tokens=2),
	// then 2 more calls succeed (tokens=1, 0), 4th call denied.
	if !l.allow("k1") {
		t.Fatal("call 1 should succeed")
	}
	if !l.allow("k1") {
		t.Fatal("call 2 should succeed")
	}
	if !l.allow("k1") {
		t.Fatal("call 3 should succeed")
	}
	if l.allow("k1") {
		t.Fatal("call 4 should be denied (bucket empty)")
	}
}

func TestPerKeyLimiter_RefillsOverTime(t *testing.T) {
	l := newPerKeyLimiter(2, 1) // 1 token/sec, cap 2
	base := time.Unix(1000, 0)
	l.now = func() time.Time { return base }
	if !l.allow("k1") { // tokens: 1
		t.Fatal()
	}
	if !l.allow("k1") { // tokens: 0
		t.Fatal()
	}
	if l.allow("k1") { // denied
		t.Fatal("expected denial")
	}
	// advance 2 seconds → refills to 2 tokens
	l.now = func() time.Time { return base.Add(2 * time.Second) }
	if !l.allow("k1") {
		t.Fatal("post-refill call must succeed")
	}
	if !l.allow("k1") {
		t.Fatal("second post-refill call must succeed")
	}
}

func TestPerKeyLimiter_CapDoesNotExceedMax(t *testing.T) {
	l := newPerKeyLimiter(2, 100) // fast refill, small cap
	base := time.Unix(1000, 0)
	l.now = func() time.Time { return base }
	l.allow("k1") // seeds bucket at tokens=1

	// Advance 1 hour: refill would be huge, but must cap at maxTokens.
	l.now = func() time.Time { return base.Add(1 * time.Hour) }
	// Should get exactly maxTokens (2) allowed calls, not more.
	if !l.allow("k1") {
		t.Fatal("call 1 must succeed after long wait")
	}
	if !l.allow("k1") {
		t.Fatal("call 2 must succeed (cap = 2)")
	}
	if l.allow("k1") {
		t.Fatal("call 3 must be denied — bucket must have capped at 2")
	}
}

func TestPerKeyLimiter_KeysAreIndependent(t *testing.T) {
	l := newPerKeyLimiter(1, 0.001)
	l.now = func() time.Time { return time.Unix(1000, 0) }
	if !l.allow("k1") {
		t.Fatal()
	}
	if l.allow("k1") {
		t.Fatal("k1 must be exhausted")
	}
	if !l.allow("k2") {
		t.Fatal("k2 must have its own bucket")
	}
}
