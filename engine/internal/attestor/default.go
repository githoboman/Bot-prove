package attestor

import "fmt"

// StubAll aggregates every vendor stub. It reports Available()==false
// on every backend and, on Attest, returns ErrAttestorUnavailable
// with a message that names every backend the caller could try.
//
// A production build would replace Default() with a real per-vendor
// backend selected via build tag or env var. See
// docs/HARDWARE_ATTESTORS.md § "Rollout sequence".
type StubAll struct {
	backends []Attestor
}

// NewStubAll returns the aggregate stub used by Default() today.
func NewStubAll() *StubAll {
	return &StubAll{
		backends: []Attestor{
			NewTPMStub(),
			NewSGXStub(),
			NewSEVSNPStub(),
			NewTrustZoneStub(),
		},
	}
}

func (s *StubAll) Kind() Kind      { return Kind("stub-all") }
func (s *StubAll) Available() bool { return false }

func (s *StubAll) Attest(challenge []byte) (*Quote, error) {
	names := ""
	for i, b := range s.backends {
		if i > 0 {
			names += ", "
		}
		names += string(b.Kind())
	}
	return nil, fmt.Errorf("no hardware attestor wired in (%s all stubs): %w", names, ErrAttestorUnavailable)
}

func (s *StubAll) Verify(q *Quote) error {
	return fmt.Errorf("stub-all: cannot verify without a real backend: %w", ErrAttestorUnavailable)
}

// Backends returns a snapshot of every registered backend, for
// debugging / status endpoints.
func (s *StubAll) Backends() []Attestor {
	out := make([]Attestor, len(s.backends))
	copy(out, s.backends)
	return out
}

// Default is what a handler should call when it wants "whatever
// attestor is configured today". In this build it is always
// StubAll — real backends are added post-invest.
func Default() Attestor {
	return NewStubAll()
}
