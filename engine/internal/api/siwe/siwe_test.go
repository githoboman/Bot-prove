package siwe

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"
)

func newKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	return pub, priv
}

func TestIssueVerify_HappyPath(t *testing.T) {
	s := NewStore(0)
	pub, priv := newKeypair(t)
	msg, nonceHex, err := s.Issue(pub, "submit-batch")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if len(nonceHex) != 2*NonceBytes {
		t.Fatalf("nonce hex length %d, want %d", len(nonceHex), 2*NonceBytes)
	}
	if !strings.HasPrefix(msg, purposePrefix+"|submit-batch|") {
		t.Fatalf("canonical message prefix mismatch: %q", msg)
	}
	sig := ed25519.Sign(priv, []byte(msg))
	if err := s.Verify(pub, "submit-batch", msg, sig); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// Consumed on success.
	if err := s.Verify(pub, "submit-batch", msg, sig); err != ErrExpired {
		t.Fatalf("expected ErrExpired on replay, got %v", err)
	}
}

func TestVerify_WrongSignature(t *testing.T) {
	s := NewStore(0)
	pub, _ := newKeypair(t)
	_, otherPriv := newKeypair(t)
	msg, _, err := s.Issue(pub, "revoke-proof")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// Sign with a different key entirely.
	sig := ed25519.Sign(otherPriv, []byte(msg))
	if err := s.Verify(pub, "revoke-proof", msg, sig); err != ErrSignatureInvalid {
		t.Fatalf("expected ErrSignatureInvalid, got %v", err)
	}
	// Bad sig does NOT consume the nonce — a legitimate retry within
	// the TTL must still work.
	sig = ed25519.Sign(otherPriv, []byte("some other message"))
	if err := s.Verify(pub, "revoke-proof", msg, sig); err != ErrSignatureInvalid {
		t.Fatalf("expected ErrSignatureInvalid on second bad sig, got %v", err)
	}
}

func TestVerify_PurposeMismatch(t *testing.T) {
	s := NewStore(0)
	pub, priv := newKeypair(t)
	msg, _, err := s.Issue(pub, "submit-batch")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	sig := ed25519.Sign(priv, []byte(msg))
	if err := s.Verify(pub, "revoke-proof", msg, sig); err != ErrPurposeMismatch {
		t.Fatalf("expected ErrPurposeMismatch, got %v", err)
	}
}

func TestVerify_PubkeyMismatch(t *testing.T) {
	s := NewStore(0)
	pub, priv := newKeypair(t)
	otherPub, _ := newKeypair(t)
	msg, _, err := s.Issue(pub, "submit-batch")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	sig := ed25519.Sign(priv, []byte(msg))
	if err := s.Verify(otherPub, "submit-batch", msg, sig); err != ErrPubkeyMismatch {
		t.Fatalf("expected ErrPubkeyMismatch, got %v", err)
	}
}

func TestVerify_Expired(t *testing.T) {
	s := NewStore(1 * time.Second)
	now := time.Unix(1_700_000_000, 0)
	s.SetClock(func() time.Time { return now })
	pub, priv := newKeypair(t)
	msg, _, err := s.Issue(pub, "submit-batch")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// Advance past TTL.
	now = now.Add(2 * time.Second)
	sig := ed25519.Sign(priv, []byte(msg))
	if err := s.Verify(pub, "submit-batch", msg, sig); err != ErrExpired {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}

func TestIssue_InvalidPurpose(t *testing.T) {
	s := NewStore(0)
	pub, _ := newKeypair(t)
	cases := []string{"", "UPPER", "has spaces", strings.Repeat("a", MaxPurposeLength+1), "a/b"}
	for _, p := range cases {
		if _, _, err := s.Issue(pub, p); err != ErrInvalidInput {
			t.Fatalf("purpose %q: expected ErrInvalidInput, got %v", p, err)
		}
	}
}

func TestIssue_InvalidPubkeyLen(t *testing.T) {
	s := NewStore(0)
	if _, _, err := s.Issue(make(ed25519.PublicKey, 16), "submit"); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for short pubkey, got %v", err)
	}
}

func TestParsePubkeyHex(t *testing.T) {
	pub, _ := newKeypair(t)
	s := hex.EncodeToString(pub)
	got, err := ParsePubkeyHex("  " + s + "  ")
	if err != nil {
		t.Fatalf("parse pubkey: %v", err)
	}
	if !equalBytes(got, pub) {
		t.Fatalf("roundtrip mismatch")
	}
	if _, err := ParsePubkeyHex("zzzz"); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput on garbage, got %v", err)
	}
	if _, err := ParsePubkeyHex(""); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput on empty, got %v", err)
	}
	if _, err := ParsePubkeyHex(hex.EncodeToString(make([]byte, 16))); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput on short bytes, got %v", err)
	}
}

func TestParseSignatureHex(t *testing.T) {
	sig := make([]byte, ed25519.SignatureSize)
	got, err := ParseSignatureHex(hex.EncodeToString(sig))
	if err != nil {
		t.Fatalf("parse sig: %v", err)
	}
	if len(got) != ed25519.SignatureSize {
		t.Fatalf("length mismatch")
	}
	if _, err := ParseSignatureHex("zzz"); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput on garbage")
	}
}

func TestOutstanding_And_GC(t *testing.T) {
	s := NewStore(500 * time.Millisecond)
	now := time.Unix(1_700_000_000, 0)
	s.SetClock(func() time.Time { return now })
	pub, _ := newKeypair(t)
	for i := 0; i < 5; i++ {
		if _, _, err := s.Issue(pub, "submit-batch"); err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
	}
	if s.Outstanding() != 5 {
		t.Fatalf("outstanding=%d, want 5", s.Outstanding())
	}
	// Advance well past TTL — Outstanding calls gcLocked.
	now = now.Add(10 * time.Second)
	if s.Outstanding() != 0 {
		t.Fatalf("expected GC to purge expired, got %d", s.Outstanding())
	}
}

func TestCapacity(t *testing.T) {
	s := NewStore(10 * time.Minute)
	pub, _ := newKeypair(t)
	for i := 0; i < MaxOutstanding; i++ {
		if _, _, err := s.Issue(pub, "submit-batch"); err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
	}
	if _, _, err := s.Issue(pub, "submit-batch"); err != ErrCapacityExceeded {
		t.Fatalf("expected ErrCapacityExceeded, got %v", err)
	}
}

func TestConcurrent_IssueVerify(t *testing.T) {
	s := NewStore(30 * time.Second)
	var wg sync.WaitGroup
	// 50 concurrent goroutines each doing issue+verify.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Errorf("keygen: %v", err)
				return
			}
			msg, _, err := s.Issue(pub, "submit-batch")
			if err != nil {
				t.Errorf("issue: %v", err)
				return
			}
			sig := ed25519.Sign(priv, []byte(msg))
			if err := s.Verify(pub, "submit-batch", msg, sig); err != nil {
				t.Errorf("verify: %v", err)
				return
			}
		}()
	}
	wg.Wait()
}

func TestCanonicalMessage_DomainSeparation(t *testing.T) {
	// Same pubkey + same nonce + different purpose -> different message.
	pub, _ := newKeypair(t)
	nonce := make([]byte, NonceBytes)
	issued := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	a := canonicalMessage(pub, "submit-batch", nonce, issued)
	b := canonicalMessage(pub, "revoke-proof", nonce, issued)
	if a == b {
		t.Fatalf("purpose does not domain-separate the message")
	}
	if !strings.Contains(a, "|submit-batch|") || !strings.Contains(b, "|revoke-proof|") {
		t.Fatalf("purpose not embedded in message")
	}
}
