package attestor

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
)

// Every backend must report Available()==false in the hackathon build.
func TestAllBackendsUnavailable(t *testing.T) {
	backends := []Attestor{
		NewTPMStub(),
		NewSGXStub(),
		NewSEVSNPStub(),
		NewTrustZoneStub(),
		NewStubAll(),
	}
	for _, b := range backends {
		t.Run(string(b.Kind()), func(t *testing.T) {
			if b.Available() {
				t.Fatalf("%s Available()=true, expected false in stub build", b.Kind())
			}
		})
	}
}

// Every stub's Attest must return an error and never a Quote.
func TestAttestReturnsUnavailable(t *testing.T) {
	backends := []Attestor{
		NewTPMStub(),
		NewSGXStub(),
		NewSEVSNPStub(),
		NewTrustZoneStub(),
		NewStubAll(),
	}
	for _, b := range backends {
		t.Run(string(b.Kind()), func(t *testing.T) {
			q, err := b.Attest([]byte("challenge-data"))
			if q != nil {
				t.Fatalf("%s returned non-nil quote: %+v", b.Kind(), q)
			}
			if err == nil {
				t.Fatalf("%s returned nil error", b.Kind())
			}
			if !errors.Is(err, ErrAttestorUnavailable) {
				t.Fatalf("%s error does not wrap ErrAttestorUnavailable: %v", b.Kind(), err)
			}
			if !strings.Contains(err.Error(), "unavailable") {
				t.Fatalf("%s error missing 'unavailable' literal: %v", b.Kind(), err)
			}
		})
	}
}

// Every stub's Verify must refuse.
func TestVerifyRefuses(t *testing.T) {
	backends := []Attestor{
		NewTPMStub(),
		NewSGXStub(),
		NewSEVSNPStub(),
		NewTrustZoneStub(),
		NewStubAll(),
	}
	// A well-formed *Quote (but Simulated) must still be refused.
	q := unavailableQuote(KindTPM, "test", []byte("c"))
	for _, b := range backends {
		t.Run(string(b.Kind()), func(t *testing.T) {
			if err := b.Verify(q); err == nil {
				t.Fatalf("%s Verify returned nil, expected refusal", b.Kind())
			}
			// Nil quote also refused.
			if err := b.Verify(nil); err == nil {
				t.Fatalf("%s Verify(nil) returned nil, expected refusal", b.Kind())
			}
		})
	}
}

// The Default() selector must return a StubAll today. This is a
// canary: if a future refactor accidentally wires a real backend in
// on the default path, this test starts failing.
func TestDefaultIsStubAll(t *testing.T) {
	d := Default()
	if d == nil {
		t.Fatal("Default() returned nil")
	}
	if d.Available() {
		t.Fatal("Default().Available() = true, expected false in stub build")
	}
	if _, ok := d.(*StubAll); !ok {
		t.Fatalf("Default() = %T, want *StubAll (a real backend was wired in on the default path — audit this change)", d)
	}
}

// unavailableQuote is a caller-facing helper for tests / debugging;
// it MUST set Simulated=true. This test pins that invariant.
func TestUnavailableQuoteSimulatedTrue(t *testing.T) {
	q := unavailableQuote(KindTPM, "test-vendor", []byte("hello"))
	if !q.Simulated {
		t.Fatal("unavailableQuote returned Simulated=false — real-looking quote leak")
	}
	if q.Blob != nil {
		t.Fatalf("stub quote must have nil Blob, got %d bytes", len(q.Blob))
	}
	if q.Kind != KindTPM {
		t.Fatalf("Kind = %q, want %q", q.Kind, KindTPM)
	}
	// ChallengeH must be sha256 of the challenge.
	want := sha256.Sum256([]byte("hello"))
	if !bytes.Equal(q.ChallengeH, want[:]) {
		t.Fatalf("ChallengeH = %x, want %x", q.ChallengeH, want[:])
	}
}

// hashChallenge is a small helper the real backends will reuse. Pin
// it here so a future refactor can't silently change the mapping.
func TestHashChallengeIsSHA256(t *testing.T) {
	for _, input := range []string{"", "x", "hello world", strings.Repeat("A", 1024)} {
		got := hashChallenge([]byte(input))
		want := sha256.Sum256([]byte(input))
		if !bytes.Equal(got, want[:]) {
			t.Fatalf("hashChallenge(%q) = %x, want %x", input, got, want[:])
		}
	}
}

// StubAll.Backends must enumerate every vendor exactly once and
// never leak a real backend into the list.
func TestStubAllBackendsAreAllStubs(t *testing.T) {
	sa := NewStubAll()
	backs := sa.Backends()
	if len(backs) != 4 {
		t.Fatalf("StubAll has %d backends, want 4", len(backs))
	}
	seen := map[Kind]bool{}
	for _, b := range backs {
		if b.Available() {
			t.Fatalf("StubAll backend %q Available()=true — audit this change", b.Kind())
		}
		if seen[b.Kind()] {
			t.Fatalf("StubAll duplicate backend %q", b.Kind())
		}
		seen[b.Kind()] = true
	}
	for _, want := range []Kind{KindTPM, KindSGX, KindSEVSNP, KindTrustZone} {
		if !seen[want] {
			t.Fatalf("StubAll missing backend %q", want)
		}
	}
}
