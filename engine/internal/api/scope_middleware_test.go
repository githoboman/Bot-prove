package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScopeRegistrySetLookup(t *testing.T) {
	r := NewScopeRegistry()
	r.Set("k1", APIKeyRecord{TenantID: "t-1", Scopes: []string{"proof:write"}})

	got, ok := r.Lookup("k1")
	if !ok {
		t.Fatal("Lookup missed just-set key")
	}
	if got.TenantID != "t-1" || len(got.Scopes) != 1 {
		t.Fatalf("bad record: %+v", got)
	}

	if _, ok := r.Lookup("does-not-exist"); ok {
		t.Fatal("Lookup returned ok for missing key")
	}
}

func TestScopeRegistrySetMany(t *testing.T) {
	r := NewScopeRegistry()
	r.SetMany(map[string]APIKeyRecord{
		"k1": {TenantID: "t-1", Scopes: []string{"proof:write"}},
		"k2": {TenantID: "t-2", Scopes: []string{"kyc:admin"}},
	})
	if _, ok := r.Lookup("k1"); !ok {
		t.Fatal("k1 missing")
	}
	if _, ok := r.Lookup("k2"); !ok {
		t.Fatal("k2 missing")
	}
}

func TestHasScopeMatch(t *testing.T) {
	rec := APIKeyRecord{Scopes: []string{"proof:write", "kyc:admin"}}
	if !rec.HasScope("proof:write") {
		t.Fatal("HasScope(proof:write) should be true")
	}
	if rec.HasScope("wallet:sign") {
		t.Fatal("HasScope(wallet:sign) should be false")
	}
}

func TestHasScopeWildcard(t *testing.T) {
	rec := APIKeyRecord{Scopes: []string{"*"}}
	if !rec.HasScope("anything") {
		t.Fatal("wildcard should grant any scope")
	}
}

func TestScopeGateDisabledWhenRegistryNil(t *testing.T) {
	s := &Server{scopeReg: nil}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := s.scopeGate("proof:write", final)

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("nil registry should pass through; got %d", rec.Code)
	}
}

func TestScopeGatePassesUnknownKeys(t *testing.T) {
	reg := NewScopeRegistry()
	// registry populated but not with this key
	reg.Set("k1", APIKeyRecord{TenantID: "t-1", Scopes: []string{"proof:write"}})

	s := &Server{scopeReg: reg}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := s.scopeGate("proof:write", final)

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("X-API-Key", "not-in-registry")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("unknown key should pass through (auth gates instead); got %d", rec.Code)
	}
}

func TestScopeGateBlocksMissingScope(t *testing.T) {
	reg := NewScopeRegistry()
	reg.Set("k1", APIKeyRecord{TenantID: "t-1", Scopes: []string{"kyc:read"}})

	s := &Server{scopeReg: reg}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := s.scopeGate("proof:write", final)

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("X-API-Key", "k1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing scope must 403; got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if want := "proof:write"; !contains(string(body), want) {
		t.Fatalf("body should mention required scope; got %s", string(body))
	}
}

func TestScopeGateAllowsMatchingScope(t *testing.T) {
	reg := NewScopeRegistry()
	reg.Set("k1", APIKeyRecord{TenantID: "t-1", Scopes: []string{"proof:write"}})

	s := &Server{scopeReg: reg}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tid := TenantIDFromContext(r.Context()); tid != "t-1" {
			t.Fatalf("tenant id not propagated to handler; got %q", tid)
		}
		w.WriteHeader(http.StatusTeapot)
	})
	h := s.scopeGate("proof:write", final)

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("X-API-Key", "k1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("matching scope should be allowed; got %d", rec.Code)
	}
}

func TestScopeGateAllowsReadOnly(t *testing.T) {
	reg := NewScopeRegistry()
	reg.Set("k1", APIKeyRecord{TenantID: "t-1", Scopes: []string{"kyc:read"}})

	s := &Server{scopeReg: reg}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := s.scopeGate("proof:write", final)

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		req := httptest.NewRequest(method, "/x", nil)
		req.Header.Set("X-API-Key", "k1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusTeapot {
			t.Fatalf("%s should be exempt from scope gate; got %d", method, rec.Code)
		}
	}
}

func TestTenantIDFromContextDefault(t *testing.T) {
	if v := TenantIDFromContext(context.Background()); v != "" {
		t.Fatalf("default context should have empty tenant id; got %q", v)
	}
}

// tiny helper so we don't drag in strings for the whole file
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
