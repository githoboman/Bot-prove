package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// idemStubServer builds a minimal Server value just for middleware
// wiring — the middlewares under test don't touch other fields.
func idemStubServer() *Server {
	return &Server{}
}

func TestIdempotency_ReplayOnRetry(t *testing.T) {
	// Reset store between tests.
	idem.mu.Lock()
	idem.items = map[string]*idempotencyEntry{}
	idem.mu.Unlock()

	srv := idemStubServer()
	calls := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"n":` + strconv.Itoa(calls) + `,"echo":"` + string(body) + `"}`))
	})
	h := srv.idempotencyMiddleware(inner)

	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/proofs", bytes.NewReader([]byte(`{"x":1}`)))
		req.Header.Set(idempotencyHeader, "K1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	first := do()
	second := do()

	if calls != 1 {
		t.Fatalf("inner handler ran %d times, want 1", calls)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("replayed body differs:\nfirst=%s\nsecond=%s", first.Body, second.Body)
	}
	if second.Header().Get("X-Idempotency-Replay") != "true" {
		t.Fatalf("replay header missing on second call")
	}
}

func TestIdempotency_ConflictOnPayloadDrift(t *testing.T) {
	idem.mu.Lock()
	idem.items = map[string]*idempotencyEntry{}
	idem.mu.Unlock()

	srv := idemStubServer()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	h := srv.idempotencyMiddleware(inner)

	req1 := httptest.NewRequest("POST", "/proofs", bytes.NewReader([]byte(`{"x":1}`)))
	req1.Header.Set(idempotencyHeader, "K2")
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code != 200 {
		t.Fatalf("first call code=%d", rec1.Code)
	}

	req2 := httptest.NewRequest("POST", "/proofs", bytes.NewReader([]byte(`{"x":999}`)))
	req2.Header.Set(idempotencyHeader, "K2")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second call with different payload should be 409, got %d", rec2.Code)
	}
}

func TestIdempotency_NonMutatingPassthrough(t *testing.T) {
	idem.mu.Lock()
	idem.items = map[string]*idempotencyEntry{}
	idem.mu.Unlock()

	srv := idemStubServer()
	calls := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(200)
	})
	h := srv.idempotencyMiddleware(inner)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/proofs", nil)
		req.Header.Set(idempotencyHeader, "K3")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
	if calls != 3 {
		t.Fatalf("GET should never be deduped by idempotency middleware, calls=%d", calls)
	}
}
