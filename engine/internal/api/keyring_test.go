package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto/keystore"
)

// buildKeyRingServer builds a minimal Server wired with just the keyring
// endpoints — no DB, no submitter, no worker. Sufficient for HTTP round-trip
// testing of the PQ keyring surface.
func buildKeyRingServer(t *testing.T, enable bool) (*Server, *http.ServeMux) {
	t.Helper()
	if enable {
		t.Setenv("CP_KEYRING_ENABLE", "1")
	} else {
		t.Setenv("CP_KEYRING_ENABLE", "")
	}
	ring := pqcrypto.NewKeyRing()
	s := &Server{keyRing: ring, keystore: keystore.NewMemory(ring)}
	mux := http.NewServeMux()
	s.registerKeyRingRoutes(mux)
	return s, mux
}

func keyringDoJSON(t *testing.T, mux *http.ServeMux, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestPQKeyRing_HTTPRoundTrip_CreateSignVerify(t *testing.T) {
	_, mux := buildKeyRingServer(t, true)

	// Create ed25519 key.
	resp := keyringDoJSON(t, mux, "POST", "/v1/pq/keys", map[string]string{"algo": "ed25519"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d body=%s", resp.Code, resp.Body.String())
	}
	var created struct {
		ID      string `json:"id"`
		Algo    string `json:"algo"`
		Version int    `json:"version"`
		Active  bool   `json:"active"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Algo != "ed25519" || created.Version != 1 || !created.Active {
		t.Fatalf("bad create response: %+v", created)
	}

	// Sign under the active key.
	resp = keyringDoJSON(t, mux, "POST", "/v1/pq/keys/sign", map[string]string{
		"algo":    "ed25519",
		"message": "hello",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("sign: expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	var signed struct {
		Signature string `json:"signature"`
		KeyID     string `json:"key_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &signed); err != nil {
		t.Fatalf("decode sign: %v", err)
	}
	if signed.KeyID != created.ID {
		t.Fatalf("sign used unexpected key: %s vs %s", signed.KeyID, created.ID)
	}
	if _, err := hex.DecodeString(signed.Signature); err != nil {
		t.Fatalf("signature must be hex: %v", err)
	}

	// Verify — pass.
	resp = keyringDoJSON(t, mux, "POST", "/v1/pq/keys/verify", map[string]string{
		"key_id":    signed.KeyID,
		"message":   "hello",
		"signature": signed.Signature,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("verify: expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "\"valid\":true") {
		t.Fatalf("expected valid:true, got %s", resp.Body.String())
	}

	// Verify — tampered message fails cleanly with valid:false, not error.
	resp = keyringDoJSON(t, mux, "POST", "/v1/pq/keys/verify", map[string]string{
		"key_id":    signed.KeyID,
		"message":   "hi",
		"signature": signed.Signature,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("verify tampered: expected 200 with valid:false, got %d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "\"valid\":false") {
		t.Fatalf("tampered must be valid:false, got %s", resp.Body.String())
	}
}

func TestPQKeyRing_HTTPRotation(t *testing.T) {
	_, mux := buildKeyRingServer(t, true)

	// v1
	resp := keyringDoJSON(t, mux, "POST", "/v1/pq/keys", map[string]string{"algo": "mldsa65"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create v1: %d body=%s", resp.Code, resp.Body.String())
	}
	// rotate → v2
	resp = keyringDoJSON(t, mux, "POST", "/v1/pq/keys/mldsa65/rotate", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("rotate: %d body=%s", resp.Code, resp.Body.String())
	}
	var rot struct {
		NewKey struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
			Active  bool   `json:"active"`
		} `json:"new_key"`
		RetiredKeyID string `json:"retired_key_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &rot); err != nil {
		t.Fatalf("decode rotate: %v body=%s", err, resp.Body.String())
	}
	if rot.NewKey.Version != 2 || !rot.NewKey.Active {
		t.Fatalf("bad rotate response: %+v", rot)
	}
	if rot.RetiredKeyID == "" {
		t.Fatalf("rotate must report retired key id")
	}

	// List should carry both.
	resp = keyringDoJSON(t, mux, "GET", "/v1/pq/keys?algo=mldsa65", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("list: %d body=%s", resp.Code, resp.Body.String())
	}
	var list struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(resp.Body.Bytes(), &list)
	if list.Count != 2 {
		t.Fatalf("expected 2 mldsa65 keys, got %d body=%s", list.Count, resp.Body.String())
	}
}

func TestPQKeyRing_HTTPMigrate(t *testing.T) {
	_, mux := buildKeyRingServer(t, true)

	// Create ed25519, sign a message.
	_ = keyringDoJSON(t, mux, "POST", "/v1/pq/keys", map[string]string{"algo": "ed25519"})
	resp := keyringDoJSON(t, mux, "POST", "/v1/pq/keys/sign", map[string]string{
		"algo":    "ed25519",
		"message": "anchored:proof:hash",
	})
	var signed struct {
		Signature string `json:"signature"`
		KeyID     string `json:"key_id"`
	}
	_ = json.Unmarshal(resp.Body.Bytes(), &signed)

	// Create a hybrid key.
	_ = keyringDoJSON(t, mux, "POST", "/v1/pq/keys", map[string]string{"algo": "hybrid_ed25519_mldsa65"})

	// Migrate.
	resp = keyringDoJSON(t, mux, "POST", "/v1/pq/keys/migrate", map[string]string{
		"old_key_id":    signed.KeyID,
		"message":       "anchored:proof:hash",
		"old_signature": signed.Signature,
		"to_algo":       "hybrid_ed25519_mldsa65",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("migrate: %d body=%s", resp.Code, resp.Body.String())
	}
	var mig struct {
		NewSignature string `json:"new_signature"`
		NewKeyID     string `json:"new_key_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &mig); err != nil {
		t.Fatalf("decode migrate: %v", err)
	}
	if mig.NewKeyID == signed.KeyID {
		t.Fatalf("migrate must issue a new key id")
	}

	// The new signature should verify under the new key.
	resp = keyringDoJSON(t, mux, "POST", "/v1/pq/keys/verify", map[string]string{
		"key_id":    mig.NewKeyID,
		"message":   "anchored:proof:hash",
		"signature": mig.NewSignature,
	})
	if !strings.Contains(resp.Body.String(), "\"valid\":true") {
		t.Fatalf("migrated signature must verify, got %s", resp.Body.String())
	}

	// Migrating a bogus signature → 422.
	resp = keyringDoJSON(t, mux, "POST", "/v1/pq/keys/migrate", map[string]string{
		"old_key_id":    signed.KeyID,
		"message":       "anchored:proof:hash",
		"old_signature": "00ff00ff", // clearly wrong length
		"to_algo":       "hybrid_ed25519_mldsa65",
	})
	if resp.Code != http.StatusBadRequest && resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bogus migrate must fail, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestPQKeyRing_DisabledGate(t *testing.T) {
	// Explicitly ensure the env is not set.
	_ = os.Unsetenv("CP_KEYRING_ENABLE")
	_, mux := buildKeyRingServer(t, false)

	resp := keyringDoJSON(t, mux, "POST", "/v1/pq/keys", map[string]string{"algo": "ed25519"})
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when CP_KEYRING_ENABLE unset, got %d body=%s", resp.Code, resp.Body.String())
	}
	// List (read) is allowed to succeed in non-strict mode — verifying that
	// happens implicitly by List returning 200 with an empty ring.
	resp = keyringDoJSON(t, mux, "GET", "/v1/pq/keys", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected list to work in non-strict mode, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestPQKeyRing_UnknownAlgo(t *testing.T) {
	_, mux := buildKeyRingServer(t, true)
	resp := keyringDoJSON(t, mux, "POST", "/v1/pq/keys", map[string]string{"algo": "kyber-space-fluff"})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", resp.Code, resp.Body.String())
	}
}
