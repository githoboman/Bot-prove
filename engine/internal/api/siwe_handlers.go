package api

import (
	"encoding/json"
	"net/http"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/api/siwe"
)

// siweChallengeReq / siweChallengeResp — issue a fresh SIWE-like
// challenge bound to (pubkey, purpose). The client is expected to
// sign the returned canonical `message` with its Ed25519 private
// key and post the signature to /auth/siwe/verify within the TTL
// window.
//
// STATUS: REAL. See engine/internal/api/siwe for the primitive
// contract. This is an authentication PRIMITIVE, not a session-
// management layer — one nonce, one operation.
//
// The endpoint is deliberately unauthenticated at the HTTP layer
// (an API-key requirement here would defeat the purpose: the SIWE
// path is what you use when you do NOT have an API-key but you do
// have an Ed25519 identity). The shared rate-limit middleware still
// applies.
type siweChallengeReq struct {
	Pubkey  string `json:"pubkey"`
	Purpose string `json:"purpose"`
}

type siweChallengeResp struct {
	Message  string `json:"message"`
	Nonce    string `json:"nonce"`
	Purpose  string `json:"purpose"`
	Pubkey   string `json:"pubkey"`
	TTLSecs  int64  `json:"ttl_seconds"`
	Warnings []string `json:"warnings,omitempty"`
}

func (s *Server) siweChallenge(w http.ResponseWriter, r *http.Request) {
	var req siweChallengeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	pub, err := siwe.ParsePubkeyHex(req.Pubkey)
	if err != nil {
		s.jsonError(w, "invalid pubkey (must be 32-byte hex Ed25519 public key)", http.StatusBadRequest)
		return
	}
	msg, nonceHex, err := s.siwe.Issue(pub, req.Purpose)
	if err != nil {
		switch err {
		case siwe.ErrInvalidInput:
			s.jsonError(w, "invalid purpose (lowercase letters/digits/hyphen, non-empty, max 64 chars)", http.StatusBadRequest)
		case siwe.ErrCapacityExceeded:
			s.jsonError(w, "too many outstanding challenges — retry later", http.StatusServiceUnavailable)
		default:
			s.jsonError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	resp := siweChallengeResp{
		Message: msg,
		Nonce:   nonceHex,
		Purpose: req.Purpose,
		Pubkey:  req.Pubkey,
		TTLSecs: int64(siwe.DefaultTTL.Seconds()),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// siweVerifyReq / siweVerifyResp — consume a challenge. On success
// the nonce is single-use (removed from the outstanding store); a
// second call with the same message returns 401. Note that the
// message field must be the EXACT canonical message returned by
// /auth/siwe/challenge — the server does not re-derive it because
// re-derivation would require the client to reconstruct the exact
// issued-at timestamp, which is deliberately server-controlled.
type siweVerifyReq struct {
	Pubkey    string `json:"pubkey"`
	Purpose   string `json:"purpose"`
	Message   string `json:"message"`
	Signature string `json:"signature"`
}

type siweVerifyResp struct {
	OK      bool   `json:"ok"`
	Purpose string `json:"purpose"`
	Pubkey  string `json:"pubkey"`
}

func (s *Server) siweVerify(w http.ResponseWriter, r *http.Request) {
	var req siweVerifyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	pub, err := siwe.ParsePubkeyHex(req.Pubkey)
	if err != nil {
		s.jsonError(w, "invalid pubkey (must be 32-byte hex Ed25519 public key)", http.StatusBadRequest)
		return
	}
	sig, err := siwe.ParseSignatureHex(req.Signature)
	if err != nil {
		s.jsonError(w, "invalid signature (must be 64-byte hex Ed25519 signature)", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		s.jsonError(w, "message is required", http.StatusBadRequest)
		return
	}
	if err := s.siwe.Verify(pub, req.Purpose, req.Message, sig); err != nil {
		switch err {
		case siwe.ErrExpired:
			s.jsonError(w, "challenge expired or unknown", http.StatusUnauthorized)
		case siwe.ErrPurposeMismatch:
			s.jsonError(w, "purpose mismatch", http.StatusUnauthorized)
		case siwe.ErrPubkeyMismatch:
			s.jsonError(w, "pubkey mismatch", http.StatusUnauthorized)
		case siwe.ErrSignatureInvalid:
			s.jsonError(w, "signature verification failed", http.StatusUnauthorized)
		case siwe.ErrInvalidInput:
			s.jsonError(w, "invalid input", http.StatusBadRequest)
		default:
			s.jsonError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	resp := siweVerifyResp{OK: true, Purpose: req.Purpose, Pubkey: req.Pubkey}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
