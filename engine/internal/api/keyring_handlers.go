package api

// PQ key rotation + versioning HTTP surface.
//
// Endpoints (all under scope pq:*):
//
//   POST /v1/pq/keys                       — create a key for {algo}
//   POST /v1/pq/keys/{algo}/rotate         — rotate active key for algo
//   GET  /v1/pq/keys                       — list all keys
//   GET  /v1/pq/keys/{id}                  — get one key's public metadata
//   POST /v1/pq/keys/sign                  — sign under a key (active or by id)
//   POST /v1/pq/keys/verify                — verify under a key by id
//   POST /v1/pq/keys/migrate               — verify old sig then re-sign under toAlgo
//
// Every write-side handler is gated by CP_KEYRING_ENABLE=1. When the gate
// is off, calls return 503 with a plain error explaining the gate — this
// exists so that the in-memory keyring, whose private keys are lost on
// every process restart, cannot be silently used to sign production
// artefacts. In strict mode (CP_STRICT=1) the gate is enforced even on
// read-only endpoints.

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
)

func (s *Server) keyRingEnabled() bool {
	return os.Getenv("CP_KEYRING_ENABLE") == "1"
}

func (s *Server) requireKeyRing(w http.ResponseWriter, write bool) bool {
	if s.keyRingEnabled() {
		return true
	}
	if write || s.strict {
		s.jsonError(w, "pq keyring is disabled — set CP_KEYRING_ENABLE=1 to opt in (see docs/roadmap/KEY_MANAGEMENT.md — private keys live in memory only)", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// -----------------------------------------------------------------------------
// Create / rotate
// -----------------------------------------------------------------------------

func (s *Server) pqKeyCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireKeyRing(w, true) {
		return
	}
	var req struct {
		Algo string `json:"algo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	algo, err := pqcrypto.ParseAlgo(req.Algo)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	meta, err := s.keystore.CreateKey(r.Context(), algo)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(meta)
}

func (s *Server) pqKeyRotate(w http.ResponseWriter, r *http.Request) {
	if !s.requireKeyRing(w, true) {
		return
	}
	algo, err := pqcrypto.ParseAlgo(r.PathValue("algo"))
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	prev, hadPrev := s.keystore.ActiveKeyID(r.Context(), algo)
	meta, err := s.keystore.RotateKey(r.Context(), algo)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := map[string]any{
		"new_key": meta,
	}
	if hadPrev {
		out["retired_key_id"] = prev
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// -----------------------------------------------------------------------------
// List / get
// -----------------------------------------------------------------------------

func (s *Server) pqKeyList(w http.ResponseWriter, r *http.Request) {
	if !s.requireKeyRing(w, false) {
		return
	}
	// Optional ?algo= filter.
	all := s.keystore.List(r.Context())
	if algoStr := r.URL.Query().Get("algo"); algoStr != "" {
		algo, err := pqcrypto.ParseAlgo(algoStr)
		if err != nil {
			s.jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		filtered := all[:0:0]
		for _, m := range all {
			if m.Algo == algo {
				filtered = append(filtered, m)
			}
		}
		all = filtered
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": all, "count": len(all)})
}

func (s *Server) pqKeyGet(w http.ResponseWriter, r *http.Request) {
	if !s.requireKeyRing(w, false) {
		return
	}
	id := r.PathValue("id")
	meta, err := s.keystore.GetMeta(r.Context(), id)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(meta)
}

// -----------------------------------------------------------------------------
// Sign / verify / migrate
// -----------------------------------------------------------------------------

// pqKeySign signs a message. Body must specify either {algo} (use active
// key) or {key_id} (use a specific key — enables re-signing under a retired
// key, mostly for testing).
func (s *Server) pqKeySign(w http.ResponseWriter, r *http.Request) {
	if !s.requireKeyRing(w, true) {
		return
	}
	var req struct {
		Algo    string `json:"algo,omitempty"`
		KeyID   string `json:"key_id,omitempty"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		s.jsonError(w, "message is required", http.StatusBadRequest)
		return
	}
	if req.Algo == "" && req.KeyID == "" {
		s.jsonError(w, "one of {algo, key_id} is required", http.StatusBadRequest)
		return
	}

	var (
		sig []byte
		id  string
		err error
	)
	if req.KeyID != "" {
		sig, err = s.keystore.SignWithKey(r.Context(), req.KeyID, []byte(req.Message))
		id = req.KeyID
	} else {
		algo, aerr := pqcrypto.ParseAlgo(req.Algo)
		if aerr != nil {
			s.jsonError(w, aerr.Error(), http.StatusBadRequest)
			return
		}
		sig, id, err = s.keystore.Sign(r.Context(), algo, []byte(req.Message))
	}
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"signature": hex.EncodeToString(sig),
		"key_id":    id,
	})
}

// pqKeyVerify verifies a signature against the public key of a specific ID.
func (s *Server) pqKeyVerify(w http.ResponseWriter, r *http.Request) {
	if !s.requireKeyRing(w, false) {
		return
	}
	var req struct {
		KeyID     string `json:"key_id"`
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.KeyID == "" || req.Message == "" || req.Signature == "" {
		s.jsonError(w, "key_id, message and signature are required", http.StatusBadRequest)
		return
	}
	sig, err := hex.DecodeString(strings.TrimSpace(req.Signature))
	if err != nil {
		s.jsonError(w, "signature must be hex-encoded", http.StatusBadRequest)
		return
	}
	valid, verr := s.keystore.Verify(r.Context(), req.KeyID, []byte(req.Message), sig)
	if verr != nil {
		// Unknown key ID → 404; any other failure → 400.
		if strings.Contains(verr.Error(), "unknown key id") {
			s.jsonError(w, verr.Error(), http.StatusNotFound)
			return
		}
		s.jsonError(w, verr.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"valid":  valid,
		"key_id": req.KeyID,
	})
}

// pqKeyMigrate verifies an existing signature under old_key_id, then produces
// a fresh signature under the currently-active key of to_algo. Used to
// upgrade anchored signatures to a stronger PQ scheme without regenerating
// the underlying proof.
func (s *Server) pqKeyMigrate(w http.ResponseWriter, r *http.Request) {
	if !s.requireKeyRing(w, true) {
		return
	}
	var req struct {
		OldKeyID     string `json:"old_key_id"`
		Message      string `json:"message"`
		OldSignature string `json:"old_signature"`
		ToAlgo       string `json:"to_algo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.OldKeyID == "" || req.Message == "" || req.OldSignature == "" || req.ToAlgo == "" {
		s.jsonError(w, "old_key_id, message, old_signature, to_algo are required", http.StatusBadRequest)
		return
	}
	oldSig, err := hex.DecodeString(strings.TrimSpace(req.OldSignature))
	if err != nil {
		s.jsonError(w, "old_signature must be hex-encoded", http.StatusBadRequest)
		return
	}
	toAlgo, err := pqcrypto.ParseAlgo(req.ToAlgo)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	newSig, newID, err := s.keystore.MigrateSignature(r.Context(), req.OldKeyID, []byte(req.Message), oldSig, toAlgo)
	if err != nil {
		// Signal old-sig invalid vs. any other failure.
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "did not verify") {
			status = http.StatusUnprocessableEntity
		}
		s.jsonError(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"new_signature": hex.EncodeToString(newSig),
		"new_key_id":    newID,
		"to_algo":       string(toAlgo),
	})
}

// pqKeystoreInfo reports the current keystore backend, backing, and key
// count. Read-only; not gated by CP_KEYRING_ENABLE because it's diagnostic.
func (s *Server) pqKeystoreInfo(w http.ResponseWriter, r *http.Request) {
	info := s.keystore.Info(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

// registerKeyRingRoutes wires the /v1/pq/keys/* endpoints and their scopes.
func (s *Server) registerKeyRingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/pq/keys", s.pqKeyCreate)
	mux.HandleFunc("POST /v1/pq/keys/{algo}/rotate", s.pqKeyRotate)
	mux.HandleFunc("GET /v1/pq/keys", s.pqKeyList)
	mux.HandleFunc("GET /v1/pq/keys/{id}", s.pqKeyGet)
	mux.HandleFunc("POST /v1/pq/keys/sign", s.pqKeySign)
	mux.HandleFunc("POST /v1/pq/keys/verify", s.pqKeyVerify)
	mux.HandleFunc("POST /v1/pq/keys/migrate", s.pqKeyMigrate)
	mux.HandleFunc("GET /v1/pq/keystore/info", s.pqKeystoreInfo)
}

// Note: PQ keyring scopes are registered directly in scopes.go's routeScopes
// map — no additional registration hook is needed here.
