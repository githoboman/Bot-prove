// Fail-closed authentication tests for CP_STRICT=1 + empty API_KEY
// (feat/cp-api-key-fail-closed, closes CP_AGENT_SPEC v2 Gate 1.2 item
// "startup fails or prominently degrades if API_KEY missing").
//
// Under CP_STRICT=1, an empty API_KEY must cause api.New() to return an
// error rather than a running-but-anonymous server. That error is what
// main.go turns into os.Exit(1). Without CP_STRICT, an empty API_KEY
// stays a warning (dev / demo mode) so we do not break local
// development.
//
// The tests here exercise the api.New() precondition matrix directly.
// They also cross-check that the /health endpoint reports the auth
// posture in a machine-readable form so verify.sh and the frontend can
// gate on it without parsing the log stream.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
)

// newEng builds a ProofEngine for use across the precondition-matrix
// tests. api.New treats eng as opaque; the tests here are purely about
// the (CP_STRICT, API_KEY) precondition surface, not the engine.
func newEng(t *testing.T) *prover.ProofEngine {
	t.Helper()
	return prover.New()
}

// -----------------------------------------------------------------------
// Precondition matrix: 4 combinations of (CP_STRICT, API_KEY).
// t.Setenv scopes the env change to the test and restores automatically.
// -----------------------------------------------------------------------

func TestNew_FailClosed_StrictAndEmptyKey(t *testing.T) {
	// The target failure mode -- production operator forgot to set
	// API_KEY but opted into CP_STRICT. api.New must return an error
	// rather than a running server.
	t.Setenv("CP_STRICT", "1")
	t.Setenv("API_KEY", "")

	eng := newEng(t)
	srv, err := New(eng, 0, nil)
	if err == nil {
		t.Fatalf("expected error, got srv=%v", srv)
	}
	if !strings.Contains(err.Error(), "CP_STRICT") {
		t.Errorf("error should mention CP_STRICT; got: %v", err)
	}
	if !strings.Contains(err.Error(), "API_KEY") {
		t.Errorf("error should mention API_KEY; got: %v", err)
	}
	if srv != nil {
		t.Errorf("server should be nil on error; got %v", srv)
	}
}

func TestNew_OK_StrictAndKey(t *testing.T) {
	// Strict on, key set -- the production configuration. Must boot.
	t.Setenv("CP_STRICT", "1")
	t.Setenv("API_KEY", "abc123")

	eng := newEng(t)
	srv, err := New(eng, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv == nil {
		t.Fatal("expected server")
	}
	if !srv.strict {
		t.Error("strict flag should be true")
	}
	if srv.apiKey != "abc123" {
		t.Errorf("apiKey mismatch: got %q", srv.apiKey)
	}
}

func TestNew_OK_LooseAndEmptyKey(t *testing.T) {
	// Loose mode, no key -- local dev / demo. Must still boot; warning
	// logged instead. This preserves the existing dev experience.
	t.Setenv("CP_STRICT", "0")
	t.Setenv("API_KEY", "")

	eng := newEng(t)
	srv, err := New(eng, 0, nil)
	if err != nil {
		t.Fatalf("loose mode + empty key should NOT error; got: %v", err)
	}
	if srv == nil {
		t.Fatal("expected server")
	}
	if srv.strict {
		t.Error("strict flag should be false")
	}
	if srv.apiKey != "" {
		t.Errorf("apiKey should be empty; got %q", srv.apiKey)
	}
}

func TestNew_OK_LooseAndKey(t *testing.T) {
	// Loose mode, key set. Boots, auth enforced but not strict.
	t.Setenv("CP_STRICT", "0")
	t.Setenv("API_KEY", "abc123")

	eng := newEng(t)
	srv, err := New(eng, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv.strict {
		t.Error("strict flag should be false")
	}
	if srv.apiKey != "abc123" {
		t.Errorf("apiKey mismatch: got %q", srv.apiKey)
	}
}

// -----------------------------------------------------------------------
// /health endpoint -- structured auth block.
//
// verify.sh keys off `auth.mode` / `auth.enforced` / `auth.strict` to
// report the deployment posture without parsing free-text status
// strings. These tests pin the JSON shape so a future refactor of
// health() cannot silently break the operator surface.
// -----------------------------------------------------------------------

func TestHealth_Auth_EnabledAndEnforced(t *testing.T) {
	// A running server with API_KEY set must report auth.mode=enabled
	// and auth.enforced=true, regardless of strict.
	t.Setenv("CP_STRICT", "0")
	t.Setenv("API_KEY", "abc123")

	eng := newEng(t)
	srv, err := New(eng, 0, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	srv.health(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health body: %v", err)
	}
	auth, ok := body["auth"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing auth block; got %v", body)
	}
	if auth["mode"] != "enabled" {
		t.Errorf("auth.mode: want %q, got %v", "enabled", auth["mode"])
	}
	if auth["enforced"] != true {
		t.Errorf("auth.enforced: want true, got %v", auth["enforced"])
	}
	if auth["strict"] != false {
		t.Errorf("auth.strict: want false, got %v", auth["strict"])
	}
}

func TestHealth_Auth_DisabledInLooseMode(t *testing.T) {
	// Loose mode + empty key: mode="disabled", enforced=false,
	// strict=false. This is the state a dev / demo runs in.
	t.Setenv("CP_STRICT", "0")
	t.Setenv("API_KEY", "")

	eng := newEng(t)
	srv, err := New(eng, 0, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	srv.health(rr, req)
	var body map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	auth, ok := body["auth"].(map[string]interface{})
	if !ok {
		t.Fatalf("auth: expected map[string]interface{}, got %T", body["auth"])
	}
	if auth["mode"] != "disabled" {
		t.Errorf("auth.mode: want %q, got %v", "disabled", auth["mode"])
	}
	if auth["enforced"] != false {
		t.Errorf("auth.enforced: want false, got %v", auth["enforced"])
	}
}

func TestHealth_Auth_EnabledAndStrictInProdConfig(t *testing.T) {
	// The full production config: CP_STRICT=1 + API_KEY set. The auth
	// block reports the whole picture so verify.sh can green-light it
	// with a single field check.
	t.Setenv("CP_STRICT", "1")
	t.Setenv("API_KEY", "prod-key")

	eng := newEng(t)
	srv, err := New(eng, 0, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	srv.health(rr, req)
	var body map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	auth, ok := body["auth"].(map[string]interface{})
	if !ok {
		t.Fatalf("auth: expected map[string]interface{}, got %T", body["auth"])
	}
	if auth["mode"] != "enabled" || auth["enforced"] != true || auth["strict"] != true {
		t.Errorf("prod-config auth block wrong: %v", auth)
	}
}
