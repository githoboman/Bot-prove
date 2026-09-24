// Package gnarkzk provides a real Groth16 zk-SNARK (BN254 pairing-based,
// via github.com/consensys/gnark) proving knowledge of a preimage x such
// that MiMC(x) == a public hash commitment, without revealing x.
//
// This is deliberately a representative, well-scoped circuit rather than a
// full proof-of-inference circuit for an arbitrary AI model (that's a much
// larger undertaking - see docs/ROADMAP.md Phase 3 "Layerwise ZK"). What it
// demonstrates for real, with actual BN254 pairing checks (not the
// hash-based simulation in ../groth16.go), is: (1) a real circuit
// definition, (2) a real (non-production, session-local) trusted setup,
// (3) real proof generation, (4) real cryptographic verification that
// rejects tampered proofs/public inputs. It's the building block a full
// proof-of-inference circuit (e.g. "MiMC(model_weights || input) ==
// committed_hash") would be built on top of.
package gnarkzk

import (
	"fmt"
	"math/big"

	bn254mimc "github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/std/hash/mimc"
)

// PreimageCircuit proves knowledge of PreImage such that MiMC(PreImage)
// (BN254-scalar-field MiMC, gnark's own std/hash/mimc gadget) equals the
// public Hash commitment.
type PreimageCircuit struct {
	PreImage frontend.Variable
	Hash     frontend.Variable `gnark:",public"`
}

func (c *PreimageCircuit) Define(api frontend.API) error {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		return fmt.Errorf("mimc init: %w", err)
	}
	h.Write(c.PreImage)
	api.AssertIsEqual(c.Hash, h.Sum())
	return nil
}

// ComputeMiMCHash returns MiMC(preimage) over the BN254 scalar field using
// gnark-crypto's native (out-of-circuit) MiMC implementation - the same
// hash the circuit checks, so callers can compute the public Hash input.
func ComputeMiMCHash(preimage *big.Int) *big.Int {
	h := bn254mimc.NewMiMC()
	h.Write(preimage.Bytes())
	return new(big.Int).SetBytes(h.Sum(nil))
}

// Setup holds the artifacts of a one-time (per-process, in-memory) trusted
// setup for PreimageCircuit: compiled R1CS, proving key, verifying key.
//
// NOT for production use: a real deployment needs a proper multi-party
// trusted setup ceremony and persisted, audited keys - this regenerates a
// fresh one every process start, which is fine for demonstrating the real
// cryptography but explicitly not a security-hardened setup. See
// docs/KNOWN_LIMITATIONS.md.
type Setup struct {
	pk groth16.ProvingKey
	vk groth16.VerifyingKey
}

// NewSetup compiles PreimageCircuit and runs Groth16's Setup phase.
func NewSetup() (*Setup, error) {
	var circuit PreimageCircuit
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		return nil, fmt.Errorf("compile circuit: %w", err)
	}
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		return nil, fmt.Errorf("groth16 setup: %w", err)
	}
	return &Setup{pk: pk, vk: vk}, nil
}

// Prove generates a real Groth16 proof that the caller knows preimage such
// that MiMC(preimage) == expectedHash, without revealing preimage. Returns
// an error (proof generation itself fails, mirroring a circuit constraint
// violation) if preimage doesn't actually hash to expectedHash.
func (s *Setup) Prove(preimage, expectedHash *big.Int) (groth16.Proof, error) {
	var circuit PreimageCircuit
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		return nil, fmt.Errorf("compile circuit: %w", err)
	}

	assignment := PreimageCircuit{PreImage: preimage, Hash: expectedHash}
	witness, err := frontend.NewWitness(&assignment, ecc.BN254.ScalarField())
	if err != nil {
		return nil, fmt.Errorf("build witness: %w", err)
	}

	proof, err := groth16.Prove(ccs, s.pk, witness)
	if err != nil {
		return nil, fmt.Errorf("groth16 prove: %w", err)
	}
	return proof, nil
}

// Verify runs the real BN254 pairing-based Groth16 verification of proof
// against the public hash commitment.
func (s *Setup) Verify(proof groth16.Proof, expectedHash *big.Int) (bool, error) {
	publicAssignment := PreimageCircuit{Hash: expectedHash}
	publicWitness, err := frontend.NewWitness(&publicAssignment, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err != nil {
		return false, fmt.Errorf("build public witness: %w", err)
	}
	if err := groth16.Verify(proof, s.vk, publicWitness); err != nil {
		return false, nil //nolint:nilerr // a verification failure is a normal false, not a caller error
	}
	return true, nil
}
