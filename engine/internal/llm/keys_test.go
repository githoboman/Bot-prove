package llm

import (
	"errors"
	"testing"
	"time"
)

func TestKeyRing_EmptyIsErr(t *testing.T) {
	r := NewKeyRing(nil, time.Second)
	if r.Len() != 0 {
		t.Fatalf("expected len 0, got %d", r.Len())
	}
	if _, _, err := r.Next(); !errors.Is(err, ErrNoKeys) {
		t.Fatalf("expected ErrNoKeys, got %v", err)
	}
}

func TestKeyRing_SkipsEmptyStrings(t *testing.T) {
	r := NewKeyRing([]string{"", "a", "", "b"}, time.Second)
	if r.Len() != 2 {
		t.Fatalf("expected 2 usable keys, got %d", r.Len())
	}
}

func TestKeyRing_RoundRobin(t *testing.T) {
	r := NewKeyRing([]string{"a", "b", "c"}, time.Second)
	seen := map[string]int{}
	for i := 0; i < 6; i++ {
		k, _, err := r.Next()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		seen[k]++
	}
	if seen["a"] != 2 || seen["b"] != 2 || seen["c"] != 2 {
		t.Fatalf("expected balanced round-robin, got %v", seen)
	}
}

func TestKeyRing_RestSkipsCoolingKeys(t *testing.T) {
	r := NewKeyRing([]string{"a", "b"}, 50*time.Millisecond)

	_, idxA, _ := r.Next()
	r.Rest(idxA)

	// Next call must not return the resting key.
	for i := 0; i < 4; i++ {
		k, _, err := r.Next()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if k == r.keys[idxA] {
			t.Fatalf("resting key %q was returned before cooldown expired", k)
		}
	}
}

func TestKeyRing_AllCooling(t *testing.T) {
	r := NewKeyRing([]string{"a", "b"}, 5*time.Second)
	_, i0, _ := r.Next()
	_, i1, _ := r.Next()
	r.Rest(i0)
	r.Rest(i1)
	if _, _, err := r.Next(); !errors.Is(err, ErrAllKeysCooling) {
		t.Fatalf("expected ErrAllKeysCooling, got %v", err)
	}
}

func TestKeyRing_CoolingExpires(t *testing.T) {
	r := NewKeyRing([]string{"a"}, 20*time.Millisecond)
	_, i, _ := r.Next()
	r.Rest(i)
	time.Sleep(30 * time.Millisecond)
	if _, _, err := r.Next(); err != nil {
		t.Fatalf("expected key to be usable after cooldown, got err %v", err)
	}
}

func TestKeyRing_HealthSnapshot(t *testing.T) {
	r := NewKeyRing([]string{"a", "b"}, time.Second)
	_, i, _ := r.Next()
	r.Rest(i)
	snap := r.HealthSnapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(snap))
	}
	restingCount := 0
	for _, s := range snap {
		if s.Resting {
			restingCount++
		}
	}
	if restingCount != 1 {
		t.Fatalf("expected exactly 1 resting key, got %d", restingCount)
	}
}
