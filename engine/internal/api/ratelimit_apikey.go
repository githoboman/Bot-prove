package api

import (
	"net/http"
	"sync"
	"time"
)

// perKeyLimiter is a per-API-key token bucket rate limiter.
//
// Item 7.3 from API hardening v2. Runs IN ADDITION to the existing
// per-IP limiter — an authenticated client with a single API key
// but many source IPs still gets throttled. Unauthenticated calls
// bypass this middleware entirely (per-IP limiter still applies).
//
// Bucket: 120 requests / minute per key, refilled continuously.
// Chosen as 2x the per-IP limit (60/min) — a legitimate multi-agent
// deployment on one API key should never trip it, but a leaked key
// used for scraping will.
type perKeyLimiter struct {
	mu      sync.Mutex
	buckets map[string]*keyBucket
	// clock injectable for tests.
	now func() time.Time

	maxTokens  float64
	refillRate float64 // tokens per second
}

type keyBucket struct {
	tokens   float64
	lastFill time.Time
}

var keyRL = newPerKeyLimiter(120, 120.0/60.0) // 120/min → 2/s refill

func newPerKeyLimiter(maxTokens, refillPerSec float64) *perKeyLimiter {
	return &perKeyLimiter{
		buckets:    make(map[string]*keyBucket),
		now:        time.Now,
		maxTokens:  maxTokens,
		refillRate: refillPerSec,
	}
}

// allow returns true if the caller with `key` may proceed. It
// deducts one token; if the bucket is empty the call is denied.
func (l *perKeyLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &keyBucket{tokens: l.maxTokens - 1, lastFill: now}
		return true
	}
	// Refill based on elapsed time.
	elapsed := now.Sub(b.lastFill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.refillRate
		if b.tokens > l.maxTokens {
			b.tokens = l.maxTokens
		}
		b.lastFill = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// perKeyRateLimitMiddleware enforces the per-API-key token bucket.
// It runs AFTER authMiddleware in the chain, so s.apiKey has already
// been validated. Only requests carrying an X-API-Key header (or one
// of the future scoped keys, item 7.11 → U) are throttled here.
func (s *Server) perKeyRateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" {
			// No key → per-IP limiter is the only guard.
			next.ServeHTTP(w, r)
			return
		}
		if !keyRL.allow(key) {
			w.Header().Set("Retry-After", "30")
			s.jsonError(w, "per-api-key rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
