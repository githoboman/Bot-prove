// Package attestor is the vendor-agnostic hardware-attestation surface
// (AT / backlog 6.1–6.4). Every backend that ever ships — TPM 2.0,
// Intel SGX (DCAP), AMD SEV-SNP, ARM TrustZone — implements the
// Attestor interface below.
//
// At the moment ALL BACKENDS ARE STUBS. Every stub reports
// Available() == false and returns ErrAttestorUnavailable from Attest
// and Verify. The engine does not call this package from any live
// handler; it sits on the shelf as a fixed target so future work can
// swap in a real backend without breaking call sites.
//
// See docs/HARDWARE_ATTESTORS.md for the full design + rollout plan.
package attestor

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"
)

// Kind identifies a hardware backend.
type Kind string

const (
	KindTPM       Kind = "tpm2"
	KindSGX       Kind = "sgx"
	KindSEVSNP    Kind = "sev-snp"
	KindTrustZone Kind = "trustzone"
)

// ErrAttestorUnavailable is the sentinel returned by every stub.
// Callers can test with errors.Is; the string embeds the literal
// "attestor: unavailable" so log-scrapers can pick it up too.
var ErrAttestorUnavailable = errors.New("attestor: unavailable (stub backend, no real hardware call)")

// Quote is the wire format for one attestation. Deliberately
// vendor-blob-in-envelope so a caller cannot mistake it for a
// generic signed message.
type Quote struct {
	Kind       Kind      `json:"kind"`
	Vendor     string    `json:"vendor"`
	Blob       []byte    `json:"blob,omitempty"`
	ChallengeH []byte    `json:"challenge_h,omitempty"`
	IssuedAt   time.Time `json:"issued_at"`
	// Simulated is true if the Quote was produced by a stub. Real
	// backends leave this false. A caller that gates on a real
	// attestation MUST reject Simulated=true.
	Simulated bool `json:"simulated"`
}

// Attestor is the vendor-agnostic surface every backend implements.
type Attestor interface {
	Kind() Kind
	// Available reports whether the backend can actually produce a
	// real attestation on this host. Stubs return false.
	Available() bool
	// Attest signs a challenge and returns a Quote. Stubs return
	// (nil, ErrAttestorUnavailable).
	Attest(challenge []byte) (*Quote, error)
	// Verify checks a quote. Stubs refuse.
	Verify(q *Quote) error
}

// hashChallenge returns sha256(challenge), the deterministic value a
// real backend would splice into its vendor blob. Exposed for tests
// and for the (future) real backend implementations.
func hashChallenge(challenge []byte) []byte {
	sum := sha256.Sum256(challenge)
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}

// unavailableQuote is a helper for stubs that WANT to return a
// diagnostic Quote (for tests / debugging) but must never emit a
// production-looking one. Simulated is always true; Blob is always
// nil. Callers that actually gate on a real attestation MUST reject
// this by checking q.Simulated.
func unavailableQuote(kind Kind, vendor string, challenge []byte) *Quote {
	return &Quote{
		Kind:       kind,
		Vendor:     vendor,
		Blob:       nil,
		ChallengeH: hashChallenge(challenge),
		IssuedAt:   time.Now().UTC(),
		Simulated:  true,
	}
}

// stub is the shared implementation body for every vendor stub.
// It composes into the vendor-typed structs below (TPMStub, SGXStub,
// …) so each vendor has a distinct type — future callers can pattern-
// match on it — but share their runtime behaviour today.
type stub struct {
	kind   Kind
	vendor string
}

func (s stub) Kind() Kind      { return s.kind }
func (s stub) Available() bool { return false }

func (s stub) Attest(_ []byte) (*Quote, error) {
	return nil, fmt.Errorf("%s: %w", s.kind, ErrAttestorUnavailable)
}

func (s stub) Verify(q *Quote) error {
	if q == nil {
		return fmt.Errorf("%s: nil quote: %w", s.kind, ErrAttestorUnavailable)
	}
	// Stubs refuse to verify anything: even a Simulated quote is
	// refused, so a caller cannot flip a stub into "yeah, that's a
	// verified attestation" mode.
	return fmt.Errorf("%s: refuse to verify (stub backend): %w", s.kind, ErrAttestorUnavailable)
}
