package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHasScopeGrammar(t *testing.T) {
	cases := []struct {
		granted  []string
		required string
		want     bool
	}{
		{[]string{"*"}, "proofs:write", true},
		{[]string{"proofs:*"}, "proofs:write", true},
		{[]string{"proofs:*"}, "proofs:read", true},
		{[]string{"proofs:*"}, "verify", false},
		{[]string{"proofs:write"}, "proofs:write", true},
		{[]string{"proofs:write"}, "proofs:read", false},
		{[]string{"verify", "stats"}, "verify", true},
		{[]string{}, "proofs:write", false},
		{[]string{"proofs:write"}, "", true}, // no scope required
	}
	for _, tc := range cases {
		if got := hasScope(tc.granted, tc.required); got != tc.want {
			t.Errorf("hasScope(%v, %q): got %v want %v", tc.granted, tc.required, got, tc.want)
		}
	}
}

func TestScopeRegistryLoadAndEnforce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	payload := `{"keys":[{"key":"sk_prover","tenant_id":"acme","scopes":["proofs:*","verify"]},{"key":"sk_ro","tenant_id":"monitor","scopes":["proofs:read"]}]}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := newScopeRegistry()
	if err := reg.loadFromFile(path); err != nil {
		t.Fatal(err)
	}
	if reg.lookup("sk_prover") == nil {
		t.Fatal("sk_prover missing")
	}
	if reg.lookup("sk_missing") != nil {
		t.Fatal("sk_missing should be nil")
	}
	if !hasScope(reg.lookup("sk_prover").Scopes, "proofs:write") {
		t.Fatal("sk_prover should have proofs:write")
	}
	if hasScope(reg.lookup("sk_ro").Scopes, "proofs:write") {
		t.Fatal("sk_ro must not have proofs:write")
	}
}

func TestScopeRegistryMissingFileIsSilent(t *testing.T) {
	reg := newScopeRegistry()
	if err := reg.loadFromFile("/definitely/not/here.json"); err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(reg.keys) != 0 {
		t.Fatal("registry should be empty")
	}
}

func TestEnforceScopeAllowsWhenSubsystemOff(t *testing.T) {
	s := newTestServer("")
	if !s.enforceScope(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/proofs", nil), "proofs:read") {
		t.Fatal("expected allow when scopes disabled")
	}
}

func TestEnforceScopeDeniesInsufficient(t *testing.T) {
	s := newTestServer("")
	s.scopes = newScopeRegistry()
	s.scopes.keys["sk_ro"] = &scopedKey{Key: "sk_ro", TenantID: "monitor", Scopes: []string{"proofs:read"}}
	req := httptest.NewRequest(http.MethodPost, "/proofs", nil)
	req.Header.Set("X-API-Key", "sk_ro")
	rec := httptest.NewRecorder()
	if s.enforceScope(rec, req, "proofs:write") {
		t.Fatal("expected deny")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] == nil {
		t.Fatal("expected error body")
	}
}

func TestEnforceScopeUnscopedKeyPassesThrough(t *testing.T) {
	// A key that isn't in the scoped registry falls back to blanket
	// auth (authMiddleware). enforceScope must not deny it here.
	s := newTestServer("")
	s.scopes = newScopeRegistry()
	req := httptest.NewRequest(http.MethodPost, "/proofs", nil)
	req.Header.Set("X-API-Key", "unrecognized-blanket-key")
	if !s.enforceScope(httptest.NewRecorder(), req, "proofs:write") {
		t.Fatal("unscoped key must not be blocked at scope layer")
	}
}
