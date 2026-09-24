package llm

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// KeyRing is a thread-safe round-robin selector over a set of API keys with
// per-key cooldown when the provider returns 429/5xx. A key that just hit a
// rate limit sits out for `cooldown` before it's eligible again; if every key
// is cooling, the ring returns ErrAllKeysCooling immediately (so the runner
// can fail this provider fast and let another provider carry the load).
type KeyRing struct {
	keys     []string
	next     uint64 // atomic cursor for round-robin
	cooldown time.Duration

	mu           sync.Mutex
	restingUntil []time.Time // per-key resting deadlines
}

// ErrAllKeysCooling is returned when every key in the ring is currently
// serving a cooldown.
var ErrAllKeysCooling = errors.New("llm: all keys are cooling")

// ErrNoKeys is returned when the ring was constructed with an empty set.
var ErrNoKeys = errors.New("llm: no keys configured")

// NewKeyRing constructs a ring. `cooldown` is how long a key sits out after
// a 429/5xx. Zero cooldown means keys are never rested (not recommended).
func NewKeyRing(keys []string, cooldown time.Duration) *KeyRing {
	filtered := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != "" {
			filtered = append(filtered, k)
		}
	}
	return &KeyRing{
		keys:         filtered,
		cooldown:     cooldown,
		restingUntil: make([]time.Time, len(filtered)),
	}
}

// Len is the number of usable keys the ring was built with.
func (r *KeyRing) Len() int {
	if r == nil {
		return 0
	}
	return len(r.keys)
}

// Next returns the next key that isn't currently cooling, together with its
// index (so the caller can Rest() it later on a 429). It advances the ring
// cursor. Callers get a fresh best-effort round-robin without holding a lock
// on the fast path — the cooldown check briefly takes the mutex.
func (r *KeyRing) Next() (key string, index int, err error) {
	if r.Len() == 0 {
		return "", -1, ErrNoKeys
	}

	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Walk at most len(keys) slots looking for one that's not resting.
	for i := 0; i < len(r.keys); i++ {
		idx := int(atomic.AddUint64(&r.next, 1)-1) % len(r.keys)
		if r.restingUntil[idx].IsZero() || now.After(r.restingUntil[idx]) {
			return r.keys[idx], idx, nil
		}
	}
	return "", -1, ErrAllKeysCooling
}

// Rest marks the key at `index` as cooling for the configured cooldown.
// Safe to call from any goroutine; a nil ring is a no-op.
func (r *KeyRing) Rest(index int) {
	if r == nil || r.cooldown == 0 || index < 0 || index >= len(r.restingUntil) {
		return
	}
	r.mu.Lock()
	r.restingUntil[index] = time.Now().Add(r.cooldown)
	r.mu.Unlock()
}

// HealthSnapshot returns per-key resting status for diagnostics. The returned
// slice is a copy; the caller is free to mutate it.
func (r *KeyRing) HealthSnapshot() []KeyStatus {
	if r == nil {
		return nil
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]KeyStatus, len(r.keys))
	for i := range r.keys {
		out[i] = KeyStatus{
			Index:   i,
			Resting: !r.restingUntil[i].IsZero() && now.Before(r.restingUntil[i]),
		}
		if out[i].Resting {
			out[i].RestingUntil = r.restingUntil[i]
		}
	}
	return out
}

// KeyStatus is a diagnostic snapshot of one key's health.
type KeyStatus struct {
	Index        int       `json:"index"`
	Resting      bool      `json:"resting"`
	RestingUntil time.Time `json:"resting_until,omitempty"`
}
