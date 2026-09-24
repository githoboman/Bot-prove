// Package api — idempotency middleware.
//
// Task 7.6 from backlog: server-side X-Idempotency-Key support for mutating
// endpoints. When a client (SDK or user) provides the header on a POST, the
// server stores the response body + status and returns the same cached
// response on retries within the TTL window. Keeps deploy submitters and
// SDK callers safe against network double-fires.
//
// Store is in-process only (LRU-ish with time expiry). Cross-node dedup is
// out of scope for the hackathon; see api CHANGELOG entry.

package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	idempotencyHeader = "X-Idempotency-Key"
	idempotencyTTL    = 15 * time.Minute
	idempotencyMax    = 4096 // hard cap on retained entries
)

type idempotencyEntry struct {
	status      int
	body        []byte
	contentType string
	requestHash string // sha256 of method+path+raw body
	expiresAt   time.Time
}

type idempotencyStore struct {
	mu    sync.Mutex
	items map[string]*idempotencyEntry
}

var idem = &idempotencyStore{items: make(map[string]*idempotencyEntry)}

// bufferedResponseWriter captures a downstream handler's write so we can
// replay it on retries.
type bufferedResponseWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (b *bufferedResponseWriter) WriteHeader(code int) {
	b.status = code
	b.ResponseWriter.WriteHeader(code)
}

func (b *bufferedResponseWriter) Write(p []byte) (int, error) {
	b.buf.Write(p)
	return b.ResponseWriter.Write(p)
}

// idempotencyMiddleware wraps mutating requests. Rules:
//   - Only POST/PUT/PATCH/DELETE are considered.
//   - No X-Idempotency-Key => passthrough.
//   - Cached entry present AND request hash matches => replay.
//   - Cached entry present BUT request hash differs => 409 Conflict
//     (same key, different payload — a genuine client bug we shouldn't hide).
//   - No cached entry => run handler, buffer response, cache on 2xx only.
//
// Keeping the middleware in-memory means a restart resets the state; the
// TTL cap plus idempotencyMax entry cap bound worst-case memory to a few MB.
func (s *Server) idempotencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutating(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		key := r.Header.Get(idempotencyHeader)
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Read full body so we can hash + restore.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.jsonError(w, "read body", http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))

		reqHash := hashReq(r.Method, r.URL.Path, body)

		idem.mu.Lock()
		idem.sweepExpiredLocked()
		if entry, ok := idem.items[key]; ok {
			idem.mu.Unlock()
			if entry.requestHash != reqHash {
				// Same key, different payload — client bug.
				s.jsonError(w, "idempotency-key reused with different payload", http.StatusConflict)
				return
			}
			// Replay cached response.
			if entry.contentType != "" {
				w.Header().Set("Content-Type", entry.contentType)
			}
			w.Header().Set("X-Idempotency-Replay", "true")
			w.WriteHeader(entry.status)
			_, _ = w.Write(entry.body)
			return
		}
		idem.mu.Unlock()

		buf := &bufferedResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(buf, r)

		// Cache only successful mutations. Errors are safe to retry.
		if buf.status >= 200 && buf.status < 300 {
			idem.mu.Lock()
			// Enforce cap: drop oldest by expiry if over.
			if len(idem.items) >= idempotencyMax {
				idem.evictOldestLocked()
			}
			idem.items[key] = &idempotencyEntry{
				status:      buf.status,
				body:        append([]byte(nil), buf.buf.Bytes()...),
				contentType: w.Header().Get("Content-Type"),
				requestHash: reqHash,
				expiresAt:   time.Now().Add(idempotencyTTL),
			}
			idem.mu.Unlock()
		}
	})
}

func isMutating(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func hashReq(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{0})
	h.Write([]byte(path))
	h.Write([]byte{0})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func (s *idempotencyStore) sweepExpiredLocked() {
	now := time.Now()
	for k, v := range s.items {
		if now.After(v.expiresAt) {
			delete(s.items, k)
		}
	}
}

func (s *idempotencyStore) evictOldestLocked() {
	var oldestKey string
	var oldestT time.Time
	first := true
	for k, v := range s.items {
		if first || v.expiresAt.Before(oldestT) {
			oldestKey = k
			oldestT = v.expiresAt
			first = false
		}
	}
	if oldestKey != "" {
		delete(s.items, oldestKey)
	}
}
