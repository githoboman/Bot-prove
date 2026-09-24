// Package gnarkzk — bundled circuit implementations.
//
// Two concrete Circuits ship in this package:
//
//   * MiMCPreimageCircuit — the historical /zk/groth16-real/prove circuit,
//     now expressed through the Circuit interface so it participates in
//     the Registry alongside other circuits (no behavioral change; the
//     existing legacy /zk/groth16-real/* handlers continue to work by
//     dispatching to it under id "mimc_preimage_v1").
//
//   * ModelInferenceCircuit — a small proof-of-inference stand-in that
//     verifies MiMC(model_weights_commitment || input) == committed
//     output_hash. Explicitly not a full layerwise-model circuit — see
//     docs/roadmap/CEREMONY.md — but it's the shape a real proof-of-
//     inference commitment would take, and it exercises multiple public
//     inputs (model_id + output_hash) so the registry / API surface
//     handles more than the trivial single-public-input case.
package gnarkzk

import (
	"fmt"
	"math/big"

	bn254mimc "github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/mimc"
)

// -- MiMC preimage ----------------------------------------------------------

// MiMCPreimageCircuit — the same PreimageCircuit as in circuit.go, wrapped
// in the Circuit interface. Kept here so the Registry owns the
// canonical implementation and the legacy Setup struct in circuit.go stays
// as a thin backwards-compatible wrapper (removed in a follow-up).
type MiMCPreimageCircuit struct{}

const MiMCPreimageID = "mimc_preimage_v1"

func (MiMCPreimageCircuit) Descriptor() Descriptor {
	return Descriptor{
		ID:          MiMCPreimageID,
		Version:     "1.0.0",
		Description: "Knowledge of preimage x such that MiMC(x) == public hash",
		Curve:       "BN254",
		Backend:     "groth16",
		PublicInputs: []PublicInput{{
			Name: "hash", Encoding: "big_int_decimal",
			Description: "MiMC commitment to preimage",
		}},
	}
}

func (MiMCPreimageCircuit) NewCircuit() frontend.Circuit {
	return &preimageCircuit{}
}

func (MiMCPreimageCircuit) AssignFull(inputs map[string]any) (frontend.Circuit, error) {
	pre, err := requireBigInt(inputs, "preimage")
	if err != nil {
		return nil, err
	}
	hash, err := optionalBigInt(inputs, "hash")
	if err != nil {
		return nil, err
	}
	if hash == nil {
		hash = ComputeMiMCHash(pre)
	}
	return &preimageCircuit{PreImage: pre, Hash: hash}, nil
}

func (MiMCPreimageCircuit) AssignPublic(inputs map[string]any) (frontend.Circuit, error) {
	hash, err := requireBigInt(inputs, "hash")
	if err != nil {
		return nil, err
	}
	return &preimageCircuit{Hash: hash}, nil
}

// preimageCircuit is the frontend.Circuit shape. Field names match the
// original circuit.go PreimageCircuit for backwards compatibility of the
// on-disk R1CS serialization.
type preimageCircuit struct {
	PreImage frontend.Variable
	Hash     frontend.Variable `gnark:",public"`
}

func (c *preimageCircuit) Define(api frontend.API) error {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		return fmt.Errorf("mimc init: %w", err)
	}
	h.Write(c.PreImage)
	api.AssertIsEqual(c.Hash, h.Sum())
	return nil
}

// -- Model inference commitment --------------------------------------------

// ModelInferenceCircuit proves that a prover knows an `input` value such
// that MiMC(model_commit, input) == output_hash, where model_commit and
// output_hash are public. This shape lets a caller assert "I ran the
// inference with this committed model on _some_ input I don't want to
// reveal, and the output was this" — the same commitment structure a
// full layerwise circuit would sit inside.
type ModelInferenceCircuit struct{}

const ModelInferenceID = "model_inference_commitment_v1"

func (ModelInferenceCircuit) Descriptor() Descriptor {
	return Descriptor{
		ID:          ModelInferenceID,
		Version:     "1.0.0",
		Description: "Knowledge of input x s.t. MiMC(model_commit, x) == output_hash (both public)",
		Curve:       "BN254",
		Backend:     "groth16",
		PublicInputs: []PublicInput{
			{Name: "model_commit", Encoding: "big_int_decimal", Description: "MiMC commitment to model weights"},
			{Name: "output_hash", Encoding: "big_int_decimal", Description: "MiMC(model_commit, input)"},
		},
	}
}

func (ModelInferenceCircuit) NewCircuit() frontend.Circuit {
	return &modelInferenceCircuit{}
}

func (ModelInferenceCircuit) AssignFull(inputs map[string]any) (frontend.Circuit, error) {
	inp, err := requireBigInt(inputs, "input")
	if err != nil {
		return nil, err
	}
	mc, err := requireBigInt(inputs, "model_commit")
	if err != nil {
		return nil, err
	}
	out, err := optionalBigInt(inputs, "output_hash")
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = ComputeModelOutputHash(mc, inp)
	}
	return &modelInferenceCircuit{Input: inp, ModelCommit: mc, OutputHash: out}, nil
}

func (ModelInferenceCircuit) AssignPublic(inputs map[string]any) (frontend.Circuit, error) {
	mc, err := requireBigInt(inputs, "model_commit")
	if err != nil {
		return nil, err
	}
	out, err := requireBigInt(inputs, "output_hash")
	if err != nil {
		return nil, err
	}
	return &modelInferenceCircuit{ModelCommit: mc, OutputHash: out}, nil
}

type modelInferenceCircuit struct {
	Input       frontend.Variable
	ModelCommit frontend.Variable `gnark:",public"`
	OutputHash  frontend.Variable `gnark:",public"`
}

func (c *modelInferenceCircuit) Define(api frontend.API) error {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		return fmt.Errorf("mimc init: %w", err)
	}
	h.Write(c.ModelCommit)
	h.Write(c.Input)
	api.AssertIsEqual(c.OutputHash, h.Sum())
	return nil
}

// ComputeModelOutputHash returns the out-of-circuit MiMC of model_commit
// then input, matching what modelInferenceCircuit checks. Callers use it
// to derive the public output_hash when preparing a proof.
func ComputeModelOutputHash(modelCommit, input *big.Int) *big.Int {
	h := bn254mimc.NewMiMC()
	// The gadget's MiMC absorbs one field element per Write call — mirror
	// that here by writing the 32-byte big-endian encoding of each.
	h.Write(padTo32(modelCommit.Bytes()))
	h.Write(padTo32(input.Bytes()))
	return new(big.Int).SetBytes(h.Sum(nil))
}

func padTo32(b []byte) []byte {
	if len(b) >= 32 {
		return b
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

// -- helpers ----------------------------------------------------------------

func requireBigInt(m map[string]any, name string) (*big.Int, error) {
	v, ok := m[name]
	if !ok {
		return nil, fmt.Errorf("missing required input %q", name)
	}
	return coerceBigInt(v, name)
}

func optionalBigInt(m map[string]any, name string) (*big.Int, error) {
	v, ok := m[name]
	if !ok {
		return nil, nil
	}
	return coerceBigInt(v, name)
}

func coerceBigInt(v any, name string) (*big.Int, error) {
	switch t := v.(type) {
	case *big.Int:
		if t == nil {
			return nil, fmt.Errorf("input %q is nil *big.Int", name)
		}
		return t, nil
	case big.Int:
		return &t, nil
	case string:
		n, ok := new(big.Int).SetString(t, 10)
		if !ok {
			return nil, fmt.Errorf("input %q is not a base-10 integer string", name)
		}
		return n, nil
	case int:
		return big.NewInt(int64(t)), nil
	case int64:
		return big.NewInt(t), nil
	case uint64:
		return new(big.Int).SetUint64(t), nil
	default:
		return nil, fmt.Errorf("input %q has unsupported type %T", name, v)
	}
}
