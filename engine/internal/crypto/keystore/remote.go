package keystore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
)

// RemoteKeystoreStub documents the HTTP contract an HSM/KMS gateway must
// implement for the engine to delegate signing without ever seeing raw
// private keys. It is a HARNESS ONLY — the real driver (AWS KMS, Google
// Cloud KMS, YubiHSM PKCS#11 shim, Nitrokey, HashiCorp Vault Transit) lives
// outside this repo and is provisioned per-deployment.
//
// Contract (JSON over HTTPS, Bearer auth):
//
//   POST {base}/keys                    body: {"algo":"..."}       -> KeyMeta
//   POST {base}/keys/{id}/rotate                                    -> KeyMeta (new)
//   GET  {base}/keys                                                -> []KeyMeta
//   GET  {base}/keys/{id}                                           -> KeyMeta
//   POST {base}/keys/{id}/sign          body: {"message_hex":"..."} -> {"signature_hex":"..."}
//   POST {base}/keys/{id}/verify        body: {"message_hex","signature_hex"} -> {"valid":true}
//   POST {base}/keys/migrate            body: {"old_key_id","message_hex","old_signature_hex","to_algo"}
//                                                                    -> {"signature_hex","new_key_id"}
//
// Every response is JSON. Non-2xx returns a JSON body {"error":"..."} that
// the stub surfaces as-is to the caller.
//
// The stub itself refuses to call an unconfigured endpoint (returns
// ErrNotSupported) so an accidental prod deploy without wiring can't
// silently drop to a no-op.
type RemoteKeystoreStub struct {
	baseURL string
	token   string
	client  *http.Client

	// The stub keeps a public-only mirror of the ring (populated on List
	// / GetMeta responses) so Verify can be answered locally without
	// round-tripping. Sign is ALWAYS delegated.
	mu       sync.RWMutex
	verifier *pqcrypto.KeyRing
}

// NewRemote constructs a stub. baseURL="" AND token="" leaves it in
// "not configured" mode — every call except Info returns ErrNotSupported.
func NewRemote(baseURL, token string) *RemoteKeystoreStub {
	return &RemoteKeystoreStub{
		baseURL:  strings.TrimRight(baseURL, "/"),
		token:    token,
		client:   &http.Client{Timeout: 15 * time.Second},
		verifier: pqcrypto.NewKeyRing(), // empty verify-only ring
	}
}

func (r *RemoteKeystoreStub) configured() bool {
	return r.baseURL != "" && r.token != ""
}

func (r *RemoteKeystoreStub) do(ctx context.Context, method, path string, body any, out any) error {
	if !r.configured() {
		return ErrNotSupported
	}
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("keystore/remote: marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("keystore/remote: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("keystore/remote: http: %w", err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("keystore/remote: %s %s -> %d: %s", method, path, resp.StatusCode, string(buf))
	}
	if out != nil {
		if err := json.Unmarshal(buf, out); err != nil {
			return fmt.Errorf("keystore/remote: parse response: %w", err)
		}
	}
	return nil
}

func (r *RemoteKeystoreStub) Info(_ context.Context) Info {
	backing := "HSM/KMS gateway at " + r.baseURL
	if !r.configured() {
		backing = "HSM/KMS gateway NOT configured (see docs/KEYSTORE.md)"
	}
	r.mu.RLock()
	count := len(r.verifier.List())
	r.mu.RUnlock()
	return Info{
		Kind:           KindRemote,
		Backing:        backing,
		KeyCount:       count,
		Persistent:     true,
		HardwareBacked: r.configured(),
	}
}

func (r *RemoteKeystoreStub) CreateKey(ctx context.Context, algo pqcrypto.Algo) (pqcrypto.KeyMeta, error) {
	var out pqcrypto.KeyMeta
	if err := r.do(ctx, "POST", "/keys", map[string]string{"algo": string(algo)}, &out); err != nil {
		return pqcrypto.KeyMeta{}, err
	}
	// Mirror public metadata into the local verify-only ring so Verify()
	// works without a round-trip.
	if err := r.mirrorMeta(out); err != nil {
		return out, fmt.Errorf("keystore/remote: mirror: %w", err)
	}
	return out, nil
}

func (r *RemoteKeystoreStub) RotateKey(ctx context.Context, algo pqcrypto.Algo) (pqcrypto.KeyMeta, error) {
	return r.CreateKey(ctx, algo)
}

func (r *RemoteKeystoreStub) ActiveKeyID(_ context.Context, algo pqcrypto.Algo) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.verifier.ActiveKeyID(algo)
}

func (r *RemoteKeystoreStub) GetMeta(ctx context.Context, id string) (pqcrypto.KeyMeta, error) {
	var out pqcrypto.KeyMeta
	if err := r.do(ctx, "GET", "/keys/"+id, nil, &out); err != nil {
		return pqcrypto.KeyMeta{}, err
	}
	return out, nil
}

func (r *RemoteKeystoreStub) List(ctx context.Context) []pqcrypto.KeyMeta {
	var out []pqcrypto.KeyMeta
	if err := r.do(ctx, "GET", "/keys", nil, &out); err != nil {
		return nil
	}
	return out
}

func (r *RemoteKeystoreStub) Sign(ctx context.Context, algo pqcrypto.Algo, message []byte) ([]byte, string, error) {
	// Two-step: resolve active ID, then sign under that ID. A real
	// gateway would likely offer POST /algos/{algo}:sign to fuse this
	// into one call; kept explicit here so the harness stays small.
	r.mu.RLock()
	id, ok := r.verifier.ActiveKeyID(algo)
	r.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("keystore/remote: no active key for algo %q", algo)
	}
	sig, err := r.SignWithKey(ctx, id, message)
	if err != nil {
		return nil, "", err
	}
	return sig, id, nil
}

func (r *RemoteKeystoreStub) SignWithKey(ctx context.Context, id string, message []byte) ([]byte, error) {
	var out struct {
		SignatureHex string `json:"signature_hex"`
	}
	req := map[string]string{"message_hex": hex.EncodeToString(message)}
	if err := r.do(ctx, "POST", "/keys/"+id+"/sign", req, &out); err != nil {
		return nil, err
	}
	sig, err := hex.DecodeString(out.SignatureHex)
	if err != nil {
		return nil, fmt.Errorf("keystore/remote: bad signature_hex: %w", err)
	}
	return sig, nil
}

func (r *RemoteKeystoreStub) Verify(_ context.Context, id string, message, signature []byte) (bool, error) {
	// Verify locally against the mirrored public key ring — no round-trip
	// needed because Verify uses only public material.
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.verifier.Verify(id, message, signature)
}

func (r *RemoteKeystoreStub) MigrateSignature(ctx context.Context, oldKeyID string, message, oldSig []byte, toAlgo pqcrypto.Algo) ([]byte, string, error) {
	var out struct {
		SignatureHex string `json:"signature_hex"`
		NewKeyID     string `json:"new_key_id"`
	}
	req := map[string]string{
		"old_key_id":        oldKeyID,
		"message_hex":       hex.EncodeToString(message),
		"old_signature_hex": hex.EncodeToString(oldSig),
		"to_algo":           string(toAlgo),
	}
	if err := r.do(ctx, "POST", "/keys/migrate", req, &out); err != nil {
		return nil, "", err
	}
	sig, err := hex.DecodeString(out.SignatureHex)
	if err != nil {
		return nil, "", fmt.Errorf("keystore/remote: bad signature_hex: %w", err)
	}
	return sig, out.NewKeyID, nil
}

// mirrorMeta writes a KeyMeta into the local verify-only ring so future
// Verify() calls succeed without a round-trip. Called after remote CreateKey.
func (r *RemoteKeystoreStub) mirrorMeta(m pqcrypto.KeyMeta) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Re-serialize the ring's current public snapshot, append this key,
	// reload. Cheap for the small key counts we deal with.
	snapshot, err := r.verifier.MarshalPublic()
	if err != nil {
		return err
	}
	var doc struct {
		Keys []pqcrypto.KeyMeta `json:"keys"`
	}
	if err := json.Unmarshal(snapshot, &doc); err != nil {
		return err
	}
	doc.Keys = append(doc.Keys, m)
	newSnap, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	updated, err := pqcrypto.LoadPublicKeyRing(newSnap)
	if err != nil {
		return err
	}
	r.verifier = updated
	return nil
}

// Compile-time interface assertion.
var _ Keystore = (*RemoteKeystoreStub)(nil)
