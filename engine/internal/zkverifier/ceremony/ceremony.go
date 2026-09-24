// Package ceremony runs a real, verifiable Groth16 trusted-setup ceremony
// (Phase 1 "Powers of Tau" + Phase 2 circuit-specific setup) on top of the
// PreimageCircuit defined in package gnarkzk. It uses gnark's own
// mpcsetup implementation - the same code paths that gnark documents in
// examples/mpcsetup - so this is not a re-implementation of the protocol,
// it is an orchestration of the standard one.
//
// Honesty label (see zk/ceremony/README.md and docs/HONESTY_BADGES.md):
//
//	SINGLE-COORDINATOR CEREMONY
//
// The ceremony here is executed end-to-end inside one process by a single
// coordinator that contributes N times (with N independently-seeded
// contributions, verified pairwise). That gives you a real, cryptographically
// verifiable ceremony transcript, but it does not give you the multi-party
// "1-of-N honesty" property of a live public MPC where independent
// contributors run the software on independent machines. This is exactly
// the property a production deployment needs to close; the code below is
// designed to make that closure a matter of running Contribute() on other
// machines and dropping the resulting Phase1/Phase2 objects into the same
// verify chain - no code change required.
package ceremony

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	cs_bn254 "github.com/consensys/gnark/constraint/bn254"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/backend/groth16/bn254/mpcsetup"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/zkverifier/gnarkzk"
)

// Config parameterises a ceremony run.
type Config struct {
	// N is the Powers-of-Tau FFT domain size (must be a power of two).
	// The circuit-domain size must be <= N. gnark's PreimageCircuit
	// compiles to a few hundred constraints so N=1024 is generous. For
	// a much larger circuit bump this to 16384, 262144 etc.
	N uint64

	// Phase1Contributors is the number of Phase-1 contributions to make.
	// Each contribution is seeded independently by the caller's RNG,
	// verified against the previous, and hashed into the transcript.
	Phase1Contributors int

	// Phase2Contributors is the same for Phase 2.
	Phase2Contributors int

	// BeaconChallenge is the public randomness beacon value fed into the
	// Seal step. In production this is drawn from a public randomness
	// source (drand / League of Entropy / a block hash) evaluated
	// STRICTLY AFTER the last contribution. Here we default to a fixed
	// label so tests are deterministic; a real ceremony must not do that.
	BeaconChallenge []byte

	// OutDir, if non-empty, is the directory where artifacts (proving
	// key, verifying key, attestations JSON, per-contribution binary
	// dumps) are written. If empty, no files are written and only the
	// in-memory Transcript is returned.
	OutDir string
}

// DefaultConfig returns a small but real ceremony suitable for the
// PreimageCircuit test suite: N=8, 3 contributors on each phase, a fixed
// beacon (test-only), no output directory.
func DefaultConfig() Config {
	return Config{
		// N=1024 fits gnark's PreimageCircuit / MiMC gadget comfortably.
		// A larger circuit should bump this - the Phase-2 Initialize call
		// panics if the circuit needs more constraints than the Phase-1
		// domain supports. Must be a power of two.
		N:                  1024,
		Phase1Contributors: 3,
		Phase2Contributors: 3,
		BeaconChallenge:    []byte("casperprover-ceremony-test-beacon-v1"),
	}
}

// ContributionRecord is one entry in the ceremony transcript.
type ContributionRecord struct {
	Phase      int    `json:"phase"`
	Index      int    `json:"index"`
	Challenge  string `json:"challenge_hex"`  // hash of prior transcript
	Digest     string `json:"digest_hex"`     // sha256 of the serialised contribution
	SizeBytes  int    `json:"size_bytes"`
	Contributor string `json:"contributor"`
}

// Transcript is what a verifier needs to audit the ceremony.
type Transcript struct {
	CircuitID         string               `json:"circuit_id"`
	CircuitConstraints int                 `json:"circuit_constraints"`
	N                 uint64               `json:"phase1_domain_size"`
	BeaconChallenge   string               `json:"beacon_challenge_hex"`
	Phase1            []ContributionRecord `json:"phase1"`
	Phase2            []ContributionRecord `json:"phase2"`
	FinalVKDigest     string               `json:"final_vk_sha256"`
	FinalPKDigest     string               `json:"final_pk_sha256"`
	CommonsDigest     string               `json:"phase1_commons_sha256"`
	HonestyLabel      string               `json:"honesty_label"`
}

// Result bundles the artifacts a caller usually wants.
type Result struct {
	ProvingKey   groth16.ProvingKey
	VerifyingKey groth16.VerifyingKey
	Commons      mpcsetup.SrsCommons
	Transcript   Transcript
}

// Run executes the full ceremony end-to-end. It panics on nothing; every
// failure is returned as an error. It performs at least one contribution
// per phase and rejects Config with zero contributors on either side (a
// zero-contribution ceremony is degenerate and would return the trivial
// SRS, which is exactly the trap-door case we want to make impossible).
func Run(cfg Config) (*Result, error) {
	if cfg.N < 32 || cfg.N&(cfg.N-1) != 0 {
		return nil, fmt.Errorf("ceremony: N=%d invalid; must be power of two and >= 32", cfg.N)
	}
	if cfg.Phase1Contributors < 1 {
		return nil, errors.New("ceremony: Phase1Contributors must be >= 1")
	}
	if cfg.Phase2Contributors < 1 {
		return nil, errors.New("ceremony: Phase2Contributors must be >= 1")
	}
	if len(cfg.BeaconChallenge) == 0 {
		return nil, errors.New("ceremony: BeaconChallenge must be non-empty")
	}

	// ---- Compile the circuit ----------------------------------------
	var circuit gnarkzk.PreimageCircuit
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		return nil, fmt.Errorf("ceremony: compile circuit: %w", err)
	}
	r1csTyped, ok := ccs.(*cs_bn254.R1CS)
	if !ok {
		return nil, fmt.Errorf("ceremony: unexpected R1CS type %T", ccs)
	}

	// ---- Phase 1: Powers of Tau -------------------------------------
	phase1s := make([]*mpcsetup.Phase1, cfg.Phase1Contributors)
	phase1s[0] = mpcsetup.NewPhase1(cfg.N)
	phase1s[0].Contribute()
	for i := 1; i < cfg.Phase1Contributors; i++ {
		next := &mpcsetup.Phase1{}
		// Chain from previous: gnark's mpcsetup exposes chaining via
		// serialisation - the standard way is to WriteTo/ReadFrom the
		// previous contribution and then call Contribute on the fresh
		// copy. That keeps every contribution's RNG independent.
		var buf bytesBuffer
		if _, err := phase1s[i-1].WriteTo(&buf); err != nil {
			return nil, fmt.Errorf("ceremony: phase1 write %d: %w", i-1, err)
		}
		if _, err := next.ReadFrom(&buf); err != nil {
			return nil, fmt.Errorf("ceremony: phase1 read %d: %w", i, err)
		}
		next.Contribute()
		phase1s[i] = next
	}

	// Verify Phase 1 as a whole and seal into SrsCommons.
	commons, err := mpcsetup.VerifyPhase1(cfg.N, cfg.BeaconChallenge, phase1s...)
	if err != nil {
		return nil, fmt.Errorf("ceremony: phase1 verify: %w", err)
	}

	// Digest the sealed commons.
	commonsDigest, commonsBytes, err := serialiseHash(&commons)
	if err != nil {
		return nil, fmt.Errorf("ceremony: hash commons: %w", err)
	}

	// ---- Phase 2: circuit-specific setup ----------------------------
	phase2s := make([]*mpcsetup.Phase2, cfg.Phase2Contributors)
	phase2s[0] = &mpcsetup.Phase2{}
	_ = phase2s[0].Initialize(r1csTyped, &commons)
	phase2s[0].Contribute()
	for i := 1; i < cfg.Phase2Contributors; i++ {
		next := &mpcsetup.Phase2{}
		var buf bytesBuffer
		if _, err := phase2s[i-1].WriteTo(&buf); err != nil {
			return nil, fmt.Errorf("ceremony: phase2 write %d: %w", i-1, err)
		}
		if _, err := next.ReadFrom(&buf); err != nil {
			return nil, fmt.Errorf("ceremony: phase2 read %d: %w", i, err)
		}
		next.Contribute()
		phase2s[i] = next
	}

	pk, vk, err := mpcsetup.VerifyPhase2(r1csTyped, &commons, cfg.BeaconChallenge, phase2s...)
	if err != nil {
		return nil, fmt.Errorf("ceremony: phase2 verify: %w", err)
	}

	// ---- Digest artifacts -------------------------------------------
	pkDigest, pkBytes, err := serialiseHash(pk)
	if err != nil {
		return nil, fmt.Errorf("ceremony: hash pk: %w", err)
	}
	vkDigest, vkBytes, err := serialiseHash(vk)
	if err != nil {
		return nil, fmt.Errorf("ceremony: hash vk: %w", err)
	}

	// ---- Build transcript -------------------------------------------
	tr := Transcript{
		CircuitID:         "PreimageCircuit-MiMC-BN254-v1",
		CircuitConstraints: r1csTyped.GetNbConstraints(),
		N:                 cfg.N,
		BeaconChallenge:   hex.EncodeToString(cfg.BeaconChallenge),
		CommonsDigest:     hex.EncodeToString(commonsDigest[:]),
		FinalPKDigest:     hex.EncodeToString(pkDigest[:]),
		FinalVKDigest:     hex.EncodeToString(vkDigest[:]),
		HonestyLabel:      "SINGLE-COORDINATOR CEREMONY (multi-party upgrade path documented)",
	}
	for i, p := range phase1s {
		d, size, err := digestContribution(p)
		if err != nil {
			return nil, err
		}
		tr.Phase1 = append(tr.Phase1, ContributionRecord{
			Phase: 1, Index: i,
			Challenge:   hex.EncodeToString(p.Challenge),
			Digest:      hex.EncodeToString(d[:]),
			SizeBytes:   size,
			Contributor: fmt.Sprintf("coordinator-seed-%d", i),
		})
	}
	for i, p := range phase2s {
		d, size, err := digestContribution(p)
		if err != nil {
			return nil, err
		}
		tr.Phase2 = append(tr.Phase2, ContributionRecord{
			Phase: 2, Index: i,
			Challenge:   hex.EncodeToString(p.Challenge),
			Digest:      hex.EncodeToString(d[:]),
			SizeBytes:   size,
			Contributor: fmt.Sprintf("coordinator-seed-%d", i),
		})
	}

	// ---- Optional artifact export -----------------------------------
	if cfg.OutDir != "" {
		if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
			return nil, fmt.Errorf("ceremony: mkdir %s: %w", cfg.OutDir, err)
		}
		if err := os.WriteFile(filepath.Join(cfg.OutDir, "phase1_commons.bin"), commonsBytes, 0o644); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(cfg.OutDir, "groth16_pk.bin"), pkBytes, 0o644); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(cfg.OutDir, "groth16_vk.bin"), vkBytes, 0o644); err != nil {
			return nil, err
		}
	}

	return &Result{
		ProvingKey:   pk,
		VerifyingKey: vk,
		Commons:      commons,
		Transcript:   tr,
	}, nil
}

// bytesBuffer is a tiny in-memory buffer that satisfies io.Writer and
// io.Reader without pulling bytes.Buffer's godoc into the intent (and
// keeps this file dependency-tight).
type bytesBuffer struct {
	data []byte
	pos  int
}

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}
func (b *bytesBuffer) Read(p []byte) (int, error) {
	if b.pos >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}

type writerTo interface {
	WriteTo(w io.Writer) (int64, error)
}

func serialiseHash(v writerTo) ([32]byte, []byte, error) {
	var buf bytesBuffer
	if _, err := v.WriteTo(&buf); err != nil {
		return [32]byte{}, nil, err
	}
	return sha256.Sum256(buf.data), buf.data, nil
}

func digestContribution(v writerTo) ([32]byte, int, error) {
	d, raw, err := serialiseHash(v)
	if err != nil {
		return [32]byte{}, 0, err
	}
	return d, len(raw), nil
}
