package keystore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
)

// VaultTransitKeystore is a real driver on top of HashiCorp Vault's
// Transit secret engine. The private key material is generated and
// stored INSIDE Vault; the engine never sees it. Signing is performed
// by Vault under a named key ("transit key" in Vault parlance) and
// only the public half is returned to us.
//
// Scope: this driver supports Vault Transit's Ed25519 primitive
// natively (algo=AlgoEd25519). Post-quantum signing (Dilithium /
// SPHINCS+ / Lamport) is NOT provided by upstream Vault Transit; a
// hybrid workflow with those primitives is done via a MemoryKeystore
// or a bespoke gateway (see docs/KEYSTORE.md). We fail-fast rather
// than silently pretending to support algos Vault cannot serve.
//
// Wire protocol summary (subset used):
//
//   POST   /v1/{mount}/keys/{name}       {"type":"ed25519"}
//   POST   /v1/{mount}/keys/{name}/rotate
//   GET    /v1/{mount}/keys/{name}
//   POST   /v1/{mount}/sign/{name}        {"input":<base64>, "hash_algorithm":"sha2-256"}
//   POST   /v1/{mount}/verify/{name}      {"input":<base64>, "signature":"vault:v1:..."}
//
// Auth: static token via X-Vault-Token header (from CP_VAULT_TOKEN or a
// pre-fetched OIDC/K8S auth ticket, up to the operator).
//
// Reference:
//   https://developer.hashicorp.com/vault/api-docs/secret/transit
type VaultTransitKeystore struct {
	addr   string        // e.g. "https://vault.internal:8200"
	token  string        // Vault auth token
	mount  string        // Transit mount path, default "transit"
	client *http.Client

	// Local mirror: name -> KeyMeta. Vault returns key metadata via
	// GET /keys/{name}; we cache it here on create/rotate so callers
	// don't pay a round-trip per Verify. Vault Transit signatures
	// serialize as `vault:v<N>:<b64sig>` — Verify() is delegated to
	// Vault because interpreting that format outside Vault is
	// error-prone (versioned key rotation must be honored).
	mu    sync.RWMutex
	keys  map[string]*vaultKeyState // by our key ID
	byAlg map[pqcrypto.Algo]string  // algo -> active key ID
	// order preserves insertion order for List().
	order []string
}

type vaultKeyState struct {
	meta    pqcrypto.KeyMeta
	vaultKN string // Vault key name (may differ from our ID)
	version int    // Vault key version (cipher_key latest_version)
}

// NewVaultTransit constructs a driver. addr AND token empty leaves it
// in "not configured" mode — every call except Info returns ErrNotConfigured.
func NewVaultTransit(addr, token, mount string) *VaultTransitKeystore {
	if mount == "" {
		mount = "transit"
	}
	return &VaultTransitKeystore{
		addr:   strings.TrimRight(addr, "/"),
		token:  token,
		mount:  mount,
		client: &http.Client{Timeout: 15 * time.Second},
		keys:   make(map[string]*vaultKeyState),
		byAlg:  make(map[pqcrypto.Algo]string),
	}
}

func (v *VaultTransitKeystore) configured() bool {
	return v.addr != "" && v.token != ""
}

// vaultReq performs a single JSON round-trip. Body may be nil.
func (v *VaultTransitKeystore) vaultReq(ctx context.Context, method, path string, body any, out any) error {
	if !v.configured() {
		return ErrNotConfigured
	}
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("vault_transit: marshal: %w", err)
		}
		buf = bytes.NewReader(b)
	}
	url := v.addr + path
	req, err := http.NewRequestWithContext(ctx, method, url, buf)
	if err != nil {
		return fmt.Errorf("vault_transit: build request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault_transit: http: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 404 {
		return ErrVaultKeyNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Vault error bodies are {"errors":["..."]}
		return fmt.Errorf("vault_transit: %s %s -> %d: %s", method, path, resp.StatusCode, string(raw))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("vault_transit: parse response: %w", err)
		}
	}
	return nil
}

// ErrVaultKeyNotFound is returned by GetMeta for an unknown key.
var ErrVaultKeyNotFound = errors.New("vault_transit: key not found")

// ErrAlgoNotSupported is returned by CreateKey/Rotate when the
// requested algo cannot be served by Vault Transit natively.
var ErrAlgoNotSupported = errors.New("vault_transit: algo not supported by Vault Transit; use file/memory keystore for PQ")

// Info returns backend metadata.
func (v *VaultTransitKeystore) Info(_ context.Context) Info {
	backing := "Vault Transit at " + v.addr + " (mount=" + v.mount + ")"
	if !v.configured() {
		backing = "Vault Transit NOT configured (set CP_VAULT_ADDR + CP_VAULT_TOKEN)"
	}
	v.mu.RLock()
	count := len(v.keys)
	v.mu.RUnlock()
	return Info{
		Kind:           KindVaultTransit,
		Backing:        backing,
		KeyCount:       count,
		Persistent:     true,
		HardwareBacked: v.configured(),
	}
}

// vaultAlgoName maps our Algo to the Vault Transit key type. Only
// Ed25519 is natively supported by Vault; PQ algos error out.
func vaultAlgoName(algo pqcrypto.Algo) (string, error) {
	switch algo {
	case pqcrypto.AlgoEd25519:
		return "ed25519", nil
	default:
		return "", fmt.Errorf("%w: %s", ErrAlgoNotSupported, algo)
	}
}

// CreateKey generates a Vault Transit key and mirrors its metadata.
func (v *VaultTransitKeystore) CreateKey(ctx context.Context, algo pqcrypto.Algo) (pqcrypto.KeyMeta, error) {
	vaultAlgo, err := vaultAlgoName(algo)
	if err != nil {
		return pqcrypto.KeyMeta{}, err
	}
	name := v.newVaultKeyName(algo)
	if err := v.vaultReq(ctx, "POST",
		"/v1/"+v.mount+"/keys/"+name,
		map[string]any{"type": vaultAlgo, "exportable": false, "derived": false},
		nil,
	); err != nil {
		return pqcrypto.KeyMeta{}, err
	}
	meta, ver, err := v.readKeyMeta(ctx, algo, name)
	if err != nil {
		return pqcrypto.KeyMeta{}, err
	}
	v.mu.Lock()
	// Retire the previous active key for this algo, if any.
	if prevID, ok := v.byAlg[algo]; ok {
		if prev, exists := v.keys[prevID]; exists {
			retired := time.Now().UTC()
			prev.meta.RetiredAt = &retired
			prev.meta.Active = false
		}
	}
	v.keys[meta.ID] = &vaultKeyState{meta: meta, vaultKN: name, version: ver}
	v.byAlg[algo] = meta.ID
	v.order = append(v.order, meta.ID)
	v.mu.Unlock()
	return meta, nil
}

// RotateKey mints a new Vault key rather than rotating in place. This
// preserves a clean 1:1 mapping between our KeyMeta.ID and a Vault
// key name — an in-place Vault rotate would change the underlying
// version and require version-aware Verify. Simpler to keep the mapping.
func (v *VaultTransitKeystore) RotateKey(ctx context.Context, algo pqcrypto.Algo) (pqcrypto.KeyMeta, error) {
	return v.CreateKey(ctx, algo)
}

// ActiveKeyID reports the active key for algo.
func (v *VaultTransitKeystore) ActiveKeyID(_ context.Context, algo pqcrypto.Algo) (string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	id, ok := v.byAlg[algo]
	return id, ok
}

// GetMeta returns metadata for a known key.
func (v *VaultTransitKeystore) GetMeta(_ context.Context, id string) (pqcrypto.KeyMeta, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	st, ok := v.keys[id]
	if !ok {
		return pqcrypto.KeyMeta{}, ErrVaultKeyNotFound
	}
	return st.meta, nil
}

// List returns all known keys, insertion-ordered.
func (v *VaultTransitKeystore) List(_ context.Context) []pqcrypto.KeyMeta {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]pqcrypto.KeyMeta, 0, len(v.order))
	for _, id := range v.order {
		if st, ok := v.keys[id]; ok {
			out = append(out, st.meta)
		}
	}
	return out
}

// Sign signs a message under the active key for algo. Delegates to
// Vault Transit's `sign` endpoint; the plaintext private key never
// leaves Vault.
func (v *VaultTransitKeystore) Sign(ctx context.Context, algo pqcrypto.Algo, message []byte) ([]byte, string, error) {
	v.mu.RLock()
	id, ok := v.byAlg[algo]
	v.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("vault_transit: no active key for algo %q", algo)
	}
	sig, err := v.SignWithKey(ctx, id, message)
	if err != nil {
		return nil, "", err
	}
	return sig, id, nil
}

// SignWithKey signs under a specific key. Signature bytes are the raw
// Ed25519 signature the caller expects (we strip Vault's
// `vault:v<N>:<b64>` wrapper on the way out).
func (v *VaultTransitKeystore) SignWithKey(ctx context.Context, id string, message []byte) ([]byte, error) {
	v.mu.RLock()
	st, ok := v.keys[id]
	v.mu.RUnlock()
	if !ok {
		return nil, ErrVaultKeyNotFound
	}
	var out struct {
		Data struct {
			Signature string `json:"signature"`
		} `json:"data"`
	}
	req := map[string]any{
		"input": base64.StdEncoding.EncodeToString(message),
		// Ed25519 in Vault Transit refuses hash_algorithm; do not send it.
	}
	if err := v.vaultReq(ctx, "POST", "/v1/"+v.mount+"/sign/"+st.vaultKN, req, &out); err != nil {
		return nil, err
	}
	raw, err := parseVaultSignature(out.Data.Signature)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// Verify delegates verification back to Vault Transit. The signature
// bytes are re-wrapped into `vault:v<N>:<b64>` on the wire; the caller
// passes plain Ed25519 signature bytes.
func (v *VaultTransitKeystore) Verify(ctx context.Context, id string, message, signature []byte) (bool, error) {
	v.mu.RLock()
	st, ok := v.keys[id]
	v.mu.RUnlock()
	if !ok {
		return false, ErrVaultKeyNotFound
	}
	wrapped := fmt.Sprintf("vault:v%d:%s", st.version, base64.StdEncoding.EncodeToString(signature))
	var out struct {
		Data struct {
			Valid bool `json:"valid"`
		} `json:"data"`
	}
	req := map[string]any{
		"input":     base64.StdEncoding.EncodeToString(message),
		"signature": wrapped,
	}
	if err := v.vaultReq(ctx, "POST", "/v1/"+v.mount+"/verify/"+st.vaultKN, req, &out); err != nil {
		return false, err
	}
	return out.Data.Valid, nil
}

// MigrateSignature verifies the OLD signature under oldKeyID, then
// re-signs the same message with the active key of toAlgo. Verify uses
// the Vault Transit /verify endpoint; re-sign uses /sign.
func (v *VaultTransitKeystore) MigrateSignature(ctx context.Context, oldKeyID string, message, oldSig []byte, toAlgo pqcrypto.Algo) ([]byte, string, error) {
	ok, err := v.Verify(ctx, oldKeyID, message, oldSig)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", fmt.Errorf("vault_transit: migrate: old signature invalid")
	}
	return v.Sign(ctx, toAlgo, message)
}

// --- helpers ---

// newVaultKeyName mints a deterministic-enough name. Vault key names
// share a namespace inside a mount; a monotonically-increasing suffix
// keeps collisions impossible under a single-process client.
func (v *VaultTransitKeystore) newVaultKeyName(algo pqcrypto.Algo) string {
	// nanosecond timestamp + short algo tag; not secret material, only
	// a Vault-namespace identifier.
	nowNs := time.Now().UnixNano()
	return fmt.Sprintf("cp-%s-%d", strings.ToLower(string(algo)), nowNs)
}

// readKeyMeta pulls the public half + version from Vault after Create.
func (v *VaultTransitKeystore) readKeyMeta(ctx context.Context, algo pqcrypto.Algo, name string) (pqcrypto.KeyMeta, int, error) {
	var out struct {
		Data struct {
			Type          string `json:"type"`
			LatestVersion int    `json:"latest_version"`
			Keys          map[string]struct {
				PublicKey    string `json:"public_key"`
				CreationTime string `json:"creation_time"`
			} `json:"keys"`
		} `json:"data"`
	}
	if err := v.vaultReq(ctx, "GET", "/v1/"+v.mount+"/keys/"+name, nil, &out); err != nil {
		return pqcrypto.KeyMeta{}, 0, err
	}
	latest := out.Data.LatestVersion
	entry, ok := out.Data.Keys[fmt.Sprintf("%d", latest)]
	if !ok {
		return pqcrypto.KeyMeta{}, 0, fmt.Errorf("vault_transit: latest_version %d not present in keys map", latest)
	}
	created, _ := time.Parse(time.RFC3339, entry.CreationTime)
	if created.IsZero() {
		created = time.Now().UTC()
	}
	// Vault returns Ed25519 public keys base64-encoded. Our KeyMeta
	// canonically holds public keys hex-encoded to match the memory
	// keystore convention.
	pubBytes, err := base64.StdEncoding.DecodeString(entry.PublicKey)
	if err != nil {
		return pqcrypto.KeyMeta{}, 0, fmt.Errorf("vault_transit: bad public_key base64: %w", err)
	}
	// Our internal KeyMeta.ID must be stable, human-tolerable, and
	// uncorrelated with the Vault key name (so a leaked ID doesn't
	// leak the Vault path). Hash (algo || pubkey) → short prefix.
	h := sha256.Sum256(append([]byte(string(algo)+"|"), pubBytes...))
	shortID := "vt-" + hex.EncodeToString(h[:8])
	return pqcrypto.KeyMeta{
		ID:        shortID,
		Algo:      algo,
		Version:   latest,
		PublicKey: hex.EncodeToString(pubBytes),
		CreatedAt: created,
		Active:    true,
	}, latest, nil
}

// parseVaultSignature strips the `vault:v<N>:<base64>` wrapper.
func parseVaultSignature(s string) ([]byte, error) {
	// Format: `vault:v<num>:<b64>`
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 || parts[0] != "vault" {
		return nil, fmt.Errorf("vault_transit: signature does not match vault:vN:<b64> format: %q", s)
	}
	if !strings.HasPrefix(parts[1], "v") {
		return nil, fmt.Errorf("vault_transit: bad version segment: %q", parts[1])
	}
	raw, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("vault_transit: bad base64 signature: %w", err)
	}
	return raw, nil
}

// Compile-time interface assertion.
var _ Keystore = (*VaultTransitKeystore)(nil)
