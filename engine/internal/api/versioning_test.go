package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestV1Alias_StripsPrefix(t *testing.T) {
	srv := &Server{}
	seenPath := ""
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.WriteHeader(200)
	})
	h := srv.v1AliasMiddleware(inner)

	req := httptest.NewRequest("GET", "/v1/proofs/abc", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if seenPath != "/proofs/abc" {
		t.Fatalf("prefix not stripped, inner saw %q", seenPath)
	}
	if rec.Header().Get("X-CP-API-Version") != "v1" {
		t.Fatalf("X-CP-API-Version header missing on v1 request")
	}
	if rec.Header().Get("X-CP-Deprecation") != "" {
		t.Fatalf("v1 request must NOT carry deprecation header")
	}
}

func TestV1Alias_EmitsDeprecationOnLegacyPath(t *testing.T) {
	srv := &Server{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	h := srv.v1AliasMiddleware(inner)

	req := httptest.NewRequest("POST", "/proofs", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("X-CP-Deprecation") == "" {
		t.Fatalf("legacy path must carry X-CP-Deprecation header")
	}
	if rec.Header().Get("Sunset") == "" {
		t.Fatalf("legacy path must carry Sunset header (RFC 8594)")
	}
	if got := rec.Header().Get("Link"); got == "" {
		t.Fatalf("legacy path must carry Link header pointing at /v1 successor, got empty")
	}
}

func TestV1Alias_HealthNotDeprecated(t *testing.T) {
	srv := &Server{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	h := srv.v1AliasMiddleware(inner)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("X-CP-Deprecation") != "" {
		t.Fatalf("/health should be exempt from deprecation")
	}
}
