// Package siwe implements a lightweight SIWE-like (Sign-In With
// Ed25519) challenge-response authentication mechanism for CasperProver.
//
// STATUS: REAL (Ed25519 signature verification, deterministic nonce
// binding). This is a defence-in-depth layer that complements the
// X-API-Key middleware — an API-key alone identifies the API client
// but not the underlying user/agent; a SIWE-like signed challenge
// binds an action to a specific Ed25519 public key without shipping
// the private key to the server.
//
// Scope for CasperProver:
//   - This package issues a random 128-bit nonce (challenge) bound
//     to the requesting client's public key (as hex) and a purpose
//     tag (domain separation).
//   - The client signs the canonicalised challenge message with the
//     Ed25519 private key corresponding to that public key.
//   - The server verifies the signature against the stored nonce
//     inside a bounded TTL window (default 5 minutes).
//   - Nonces are single-use (consumed on successful verify or on
//     TTL expiry) and stored only in memory.
//
// Non-goals (kept honest):
//   - This is NOT full EIP-4361 (SIWE) — CasperProver does not
//     require Ethereum semantics; we only borrow the challenge-nonce-
//     signature shape.
//   - This is NOT a session-management layer. It authenticates a
//     single operation. Session-scoped tokens are a separate concern
//     tracked in docs/OPS_RUNBOOKS.md.
//   - No cross-request state persistence: a server restart discards
//     outstanding challenges. That is by design — the TTL is short
//     and clients are expected to retry.
//   - No paid dependency, no side network call.
package siwe

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Canonical purpose tag prefix. Every challenge is a message of the
// form "cp:siwe:v1|<purpose>|<pubkey-hex>|<nonce-hex>|<issued-iso>".
// Purpose tags are the domain-separation surface — a nonce issued for
// purpose "revoke-proof" cannot be replayed to satisfy "submit-batch".
const (
	purposePrefix     = "cp:siwe:v1"
	DefaultTTL        = 5 * time.Minute
	NonceBytes        = 16 // 128-bit nonce
	MaxOutstanding    = 1024
	MaxPurposeLength  = 64
)

// ErrExpired is returned when a nonce is presented after its TTL.
var ErrExpired = errors.New("siwe: challenge expired or unknown")

// ErrPurposeMismatch is returned when the verify purpose does not
// match the purpose the nonce was issued for.
var ErrPurposeMismatch = errors.New("siwe: purpose mismatch")

// ErrPubkeyMismatch is returned when the verify pubkey does not match
// the pubkey the nonce was issued for.
var ErrPubkeyMismatch = errors.New("siwe: pubkey mismatch")

// ErrSignatureInvalid is returned when the signature does not verify.
var ErrSignatureInvalid = errors.New("siwe: signature verification failed")

// ErrInvalidInput is returned for malformed inputs (bad hex, wrong
// length, empty purpose, etc.).
var ErrInvalidInput = errors.New("siwe: invalid input")

// ErrCapacityExceeded is returned when the outstanding-challenge cap
// is hit. Bounded to prevent memory exhaustion by a rogue client.
var ErrCapacityExceeded = errors.New("siwe: too many outstanding challenges")

type challenge struct {
	pubkey    ed25519.PublicKey
	purpose   string
	nonce     []byte
	issuedAt  time.Time
	expiresAt time.Time
}

// Store holds outstanding challenges in memory. Safe for concurrent
// use.
type Store struct {
	ttl time.Duration
	mu  sync.Mutex
	// keyed by canonical message string
	items map[string]challenge
	now   func() time.Time // injectable for tests
}

// NewStore constructs a Store with the given TTL. Zero uses DefaultTTL.
func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{
		ttl:   ttl,
		items: make(map[string]challenge),
		now:   time.Now,
	}
}

// SetClock overrides the clock (test hook only). Concurrency-safe: must
// be called before the store is exposed to concurrent traffic.
func (s *Store) SetClock(f func() time.Time) {
	if f != nil {
		s.now = f
	}
}

func validatePurpose(purpose string) error {
	if purpose == "" || len(purpose) > MaxPurposeLength {
		return ErrInvalidInput
	}
	for _, r := range purpose {
		// allow lowercase letters, digits, and '-' only; keeps the
		// domain-separation string simple to reason about.
		if r == '-' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return ErrInvalidInput
	}
	return nil
}

// Issue creates a fresh challenge for the given pubkey and purpose,
// returning the canonical message the client must sign and the raw
// nonce (hex-encoded) for observability. The message is stable across
// calls only for the same nonce; a repeat Issue with the same pubkey
// and purpose returns a fresh nonce (no per-key rate-limiting is
// enforced here — that belongs at the middleware layer).
func (s *Store) Issue(pubkey ed25519.PublicKey, purpose string) (message string, nonceHex string, err error) {
	if len(pubkey) != ed25519.PublicKeySize {
		return "", "", ErrInvalidInput
	}
	if err := validatePurpose(purpose); err != nil {
		return "", "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	if len(s.items) >= MaxOutstanding {
		return "", "", ErrCapacityExceeded
	}
	nonce := make([]byte, NonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", "", fmt.Errorf("siwe: rng: %w", err)
	}
	issued := s.now().UTC().Truncate(time.Second)
	msg := canonicalMessage(pubkey, purpose, nonce, issued)
	s.items[msg] = challenge{
		pubkey:    append(ed25519.PublicKey(nil), pubkey...),
		purpose:   purpose,
		nonce:     append([]byte(nil), nonce...),
		issuedAt:  issued,
		expiresAt: issued.Add(s.ttl),
	}
	return msg, hex.EncodeToString(nonce), nil
}

// Verify checks a client-provided signature against a previously-issued
// challenge and consumes the nonce on success. Returns nil on success
// or one of the sentinel errors on failure. Consumption is atomic: on
// success the challenge is removed, on failure it is left in place so
// a legitimate retry with a corrected signature within the TTL window
// can still succeed.
func (s *Store) Verify(pubkey ed25519.PublicKey, purpose string, message string, signature []byte) error {
	if len(pubkey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize {
		return ErrInvalidInput
	}
	if err := validatePurpose(purpose); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	ch, ok := s.items[message]
	if !ok {
		return ErrExpired
	}
	if s.now().After(ch.expiresAt) {
		delete(s.items, message)
		return ErrExpired
	}
	if !equalBytes(ch.pubkey, pubkey) {
		return ErrPubkeyMismatch
	}
	if ch.purpose != purpose {
		return ErrPurposeMismatch
	}
	if !ed25519.Verify(pubkey, []byte(message), signature) {
		return ErrSignatureInvalid
	}
	delete(s.items, message)
	return nil
}

// Outstanding reports the current number of live challenges.
// Intended for /metrics exposure, not for auth decisions.
func (s *Store) Outstanding() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	return len(s.items)
}

func (s *Store) gcLocked() {
	now := s.now()
	for k, v := range s.items {
		if now.After(v.expiresAt) {
			delete(s.items, k)
		}
	}
}

func canonicalMessage(pubkey ed25519.PublicKey, purpose string, nonce []byte, issued time.Time) string {
	var b strings.Builder
	b.Grow(96)
	b.WriteString(purposePrefix)
	b.WriteByte('|')
	b.WriteString(purpose)
	b.WriteByte('|')
	b.WriteString(hex.EncodeToString(pubkey))
	b.WriteByte('|')
	b.WriteString(hex.EncodeToString(nonce))
	b.WriteByte('|')
	b.WriteString(issued.Format(time.RFC3339))
	return b.String()
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// ParsePubkeyHex decodes a hex-encoded Ed25519 public key from a
// user-supplied string. Rejects any non-hex or wrong-length input.
func ParsePubkeyHex(s string) (ed25519.PublicKey, error) {
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, ErrInvalidInput
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, ErrInvalidInput
	}
	return ed25519.PublicKey(b), nil
}

// ParseSignatureHex decodes a hex-encoded Ed25519 signature. Rejects
// any non-hex or wrong-length input.
func ParseSignatureHex(s string) ([]byte, error) {
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, ErrInvalidInput
	}
	if len(b) != ed25519.SignatureSize {
		return nil, ErrInvalidInput
	}
	return b, nil
}
