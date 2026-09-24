package aggregator

// Nova / folding-scheme aggregation harness.
//
// This file exposes the API surface a real Nova/SuperNova/HyperNova
// implementation would fill: a `Folder` interface with `Fold(step)` and a
// terminal `Compress()`, plus a public `Verify(instance, aggregate)` that
// re-checks the whole folded chain.
//
// The default implementation, `HashFolder`, is a hash-based folding
// stand-in: at each step it commits to
//
//     acc_{i+1} = H( acc_i || H(step.instance) || H(step.witness_digest) )
//
// where H is SHA-256. This is genuinely deterministic and genuinely
// verifiable (verifying the aggregate re-plays the same hash chain), but
// it is NOT a folding scheme in the cryptographic sense — it does not
// reduce k R1CS instances into one R1CS instance whose satisfiability
// implies satisfiability of the originals. It exists so callers can wire
// the API today; swapping in a real Nova (once the ecosystem's Go story
// matures) is a matter of implementing the same interface.
//
// The public envelope explicitly labels itself `"scheme":"hash-fold-v1"`
// so downstream consumers cannot mistake it for a cryptographic
// folding proof. See docs/roadmap/NOVA_HARNESS.md for the full honesty
// disclosure.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// FoldingScheme is the label reported inside AggregateProof.Scheme.
// Callers checking for a real folding scheme should match against the
// specific label they expect, not the presence of AggregateProof.
type FoldingScheme string

const (
	SchemeHashFoldV1 FoldingScheme = "hash-fold-v1"
	// SchemeNovaGoV1 is reserved for a future real Nova implementation.
	SchemeNovaGoV1 FoldingScheme = "nova-go-v1"
)

// FoldStep is one step submitted to a Folder. Instance and WitnessDigest
// are opaque byte-strings; the folder does not interpret them beyond
// hashing. The "witness_digest" is expected to be a public commitment to
// the private witness of the underlying computation (e.g. Poseidon hash
// of an R1CS assignment) — never the raw witness.
type FoldStep struct {
	Instance       []byte `json:"instance"`
	WitnessDigest  []byte `json:"witness_digest"`
}

// AggregateProof is what the folder emits after Compress(). It captures
// (a) the accumulator digest and (b) enough per-step metadata for a
// verifier to re-play the chain from public inputs.
type AggregateProof struct {
	Scheme    FoldingScheme `json:"scheme"`
	Steps     int           `json:"steps"`
	Root      string        `json:"root_hex"`      // final accumulator hex
	StepHashes []string     `json:"step_hashes_hex"`
}

// Folder is the pluggable interface. A real Nova would implement this on
// top of a curve-cycle (Pallas/Vesta) and expose the same signatures.
type Folder interface {
	Fold(step FoldStep) error
	Compress() (AggregateProof, error)
	Verify(steps []FoldStep, agg AggregateProof) (bool, error)
}

// HashFolder is the hash-chain stand-in. Thread-unsafe; caller owns
// serialization if concurrent.
type HashFolder struct {
	acc        [sha256.Size]byte
	stepHashes []string
	initted    bool
}

// NewHashFolder returns a fresh HashFolder with the accumulator set to
// SHA-256("hash-fold-v1"). Using a scheme-specific seed prevents
// same-content aggregates across future schemes from colliding.
func NewHashFolder() *HashFolder {
	seed := sha256.Sum256([]byte(SchemeHashFoldV1))
	return &HashFolder{acc: seed, initted: true}
}

// Fold advances the accumulator by one step.
func (f *HashFolder) Fold(step FoldStep) error {
	if !f.initted {
		return errors.New("nova/hashfold: folder not initialised (use NewHashFolder)")
	}
	if len(step.Instance) == 0 {
		return errors.New("nova/hashfold: empty instance")
	}
	stepH := sha256.Sum256(append(append([]byte{}, step.Instance...), step.WitnessDigest...))
	next := sha256.New()
	next.Write(f.acc[:])
	next.Write(stepH[:])
	copy(f.acc[:], next.Sum(nil))
	f.stepHashes = append(f.stepHashes, hex.EncodeToString(stepH[:]))
	return nil
}

// Compress finalises the aggregate. After Compress it is safe to reuse
// the folder for a new run (Fold would panic on the closed sequence);
// callers create a new HashFolder for a new aggregation batch.
func (f *HashFolder) Compress() (AggregateProof, error) {
	if !f.initted {
		return AggregateProof{}, errors.New("nova/hashfold: folder not initialised")
	}
	if len(f.stepHashes) == 0 {
		return AggregateProof{}, errors.New("nova/hashfold: no steps folded")
	}
	return AggregateProof{
		Scheme:     SchemeHashFoldV1,
		Steps:      len(f.stepHashes),
		Root:       hex.EncodeToString(f.acc[:]),
		StepHashes: append([]string{}, f.stepHashes...),
	}, nil
}

// Verify reconstructs the accumulator from `steps` and compares against agg.
// Returns (true, nil) iff every step was replayed and the final root matches.
// Any structural mismatch (step count, scheme, per-step hash) yields (false, error).
func (f *HashFolder) Verify(steps []FoldStep, agg AggregateProof) (bool, error) {
	if agg.Scheme != SchemeHashFoldV1 {
		return false, fmt.Errorf("nova/hashfold: unsupported scheme %q", agg.Scheme)
	}
	if agg.Steps != len(steps) {
		return false, fmt.Errorf("nova/hashfold: step count mismatch: got %d, agg says %d", len(steps), agg.Steps)
	}
	if len(agg.StepHashes) != len(steps) {
		return false, fmt.Errorf("nova/hashfold: step_hashes length mismatch: got %d, want %d", len(agg.StepHashes), len(steps))
	}
	seed := sha256.Sum256([]byte(SchemeHashFoldV1))
	acc := seed
	for i, step := range steps {
		if len(step.Instance) == 0 {
			return false, fmt.Errorf("nova/hashfold: step %d has empty instance", i)
		}
		stepH := sha256.Sum256(append(append([]byte{}, step.Instance...), step.WitnessDigest...))
		gotStep := hex.EncodeToString(stepH[:])
		if gotStep != agg.StepHashes[i] {
			return false, fmt.Errorf("nova/hashfold: step %d hash mismatch", i)
		}
		next := sha256.New()
		next.Write(acc[:])
		next.Write(stepH[:])
		copy(acc[:], next.Sum(nil))
	}
	got := hex.EncodeToString(acc[:])
	if got != agg.Root {
		return false, errors.New("nova/hashfold: root mismatch — sequence tampered")
	}
	return true, nil
}

// -----------------------------------------------------------------------------
// Convenience wrappers for the HTTP surface
// -----------------------------------------------------------------------------

// FoldAll builds a fresh HashFolder, folds every step in order, and
// returns the aggregate. Convenience for stateless HTTP handlers.
func FoldAll(steps []FoldStep) (AggregateProof, error) {
	f := NewHashFolder()
	for i, s := range steps {
		if err := f.Fold(s); err != nil {
			return AggregateProof{}, fmt.Errorf("fold step %d: %w", i, err)
		}
	}
	return f.Compress()
}

// VerifyAll is the stateless counterpart to FoldAll — reconstruct and check.
func VerifyAll(steps []FoldStep, agg AggregateProof) (bool, error) {
	f := NewHashFolder()
	return f.Verify(steps, agg)
}
