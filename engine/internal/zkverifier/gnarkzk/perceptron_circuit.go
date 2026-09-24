package gnarkzk

// PerceptronCircuit proves that a prover ran a small, real linear
// classifier on a private input and got the claimed public output.
//
// This is a stepping-stone from the earlier ModelInferenceCircuit
// (which was a MiMC commitment scaffold, not a real inference in
// R1CS): we now encode the actual dot-product + threshold as gnark
// constraints, so the proof genuinely certifies that the arithmetic
// happened — not merely that the prover knew an input matching a
// commitment.
//
// Model shape:
//
//     output = 1  if <weights, input> + bias >= 0
//              0  otherwise
//
// A single perceptron unit. Deliberately small — the point of this
// pack is to encode a real network in R1CS end-to-end, not to serve
// production-scale inference.
//
// Fixed-point arithmetic:
//
//   All scalars are field elements (BN254 scalar field, ~254 bits).
//   Real-valued weights and inputs are quantised into signed 16-bit
//   integers in [-32768, 32767] and lifted into the field. Internally
//   the field's characteristic is prime, so negative values are
//   represented as `p - |x|`. The circuit's IsLess constraint uses
//   gnark's `cmp.IsLess` gadget, which works on canonical
//   representatives up to the field size — safe for our 16-bit range.
//
// Circuit inputs:
//
//   private:  Input[N]        — the N-dimensional input vector
//             Bias            — model bias (also private — a caller
//                               who publishes it can also make it
//                               public in the assignment via a
//                               separate constant, but we keep the
//                               canonical layout private so the same
//                               R1CS serves both settings)
//
//   public:   WeightsCommit   — MiMC(weights || bias), so a caller
//                               who has the model can check the
//                               proof was made under the model they
//                               believe. The circuit RECOMPUTES this
//                               commitment on the assigned weights,
//                               so no separate "model registration"
//                               is needed — the commitment IS the
//                               model identity.
//             Weights[N]      — public (the whole point of a
//                               classifier receipt is you can see
//                               the weights). See the honesty note
//                               below for why weights are public.
//             Output          — 0 or 1, the classifier decision
//
// Honesty notes:
//
//   * Weights are PUBLIC. This is a modelling choice: proving
//     "I ran the classifier with the *committed* weights and got
//     output Y" without revealing weights would require the caller
//     to trust our commitment scheme end-to-end AND would double the
//     R1CS constraint count (the whole dot product would need to be
//     done under commitments). We chose the transparent-model path
//     for this circuit; a later commit can add a
//     `PerceptronPrivateWeightsCircuit` variant.
//
//   * This is a LINEAR model. It cannot express non-linear behaviours
//     a real neural net has (ReLU, softmax). Adding those inside the
//     circuit is a matter of bookkeeping (Range-check + Booleanise
//     gadgets exist in gnark's std/rangecheck), but they multiply the
//     constraint count. Out of scope for this pack. See docs/
//     ZK_PERCEPTRON.md § "Roadmap" for the next step.
//
//   * The proof is Groth16 over BN254 — same backend as the rest of
//     the /v1/zk/ pipeline, so ceremony reuse is trivial.

import (
	"fmt"
	"math/big"

	bn254mimc "github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/mimc"
	"github.com/consensys/gnark/std/math/cmp"
)

// PerceptronInputDim is the fixed dimensionality of the input vector.
// Small enough to keep constraint count under 500 (fast prove/verify
// on modest hardware), large enough to be a non-toy classifier.
const PerceptronInputDim = 8

// PerceptronCircuit is the Registry entry (empty tag; Descriptor
// carries metadata).
type PerceptronCircuit struct{}

// PerceptronCircuitID uniquely identifies this circuit variant.
const PerceptronCircuitID = "perceptron_linear_v1"

// Descriptor returns the public advertisement.
func (PerceptronCircuit) Descriptor() Descriptor {
	pubIns := []PublicInput{
		{Name: "weights_commit", Encoding: "big_int_decimal", Description: "MiMC(weights || bias) — model identity"},
	}
	for i := 0; i < PerceptronInputDim; i++ {
		pubIns = append(pubIns, PublicInput{
			Name:        fmt.Sprintf("weight_%d", i),
			Encoding:    "big_int_decimal",
			Description: fmt.Sprintf("Signed 16-bit weight for dimension %d (negative values as p-|x|)", i),
		})
	}
	pubIns = append(pubIns, PublicInput{
		Name:        "output",
		Encoding:    "big_int_decimal",
		Description: "Classifier output — 0 (negative class) or 1 (positive class)",
	})
	return Descriptor{
		ID:           PerceptronCircuitID,
		Version:      "1.0.0",
		Description:  fmt.Sprintf("Groth16 proof that <w,x>+b ⋛ 0 for a %d-dim single-perceptron classifier", PerceptronInputDim),
		Curve:        "BN254",
		Backend:      "groth16",
		PublicInputs: pubIns,
	}
}

// NewCircuit returns an empty gnark circuit for compilation.
func (PerceptronCircuit) NewCircuit() frontend.Circuit {
	return &perceptronCircuit{}
}

// AssignFull assigns every wire (public + private) from a full input
// map. Callers supply:
//
//   inputs["input"]    []int  or []*big.Int   — length must equal PerceptronInputDim
//   inputs["weights"]  []int  or []*big.Int   — length must equal PerceptronInputDim
//   inputs["bias"]     int    or *big.Int
//
// The circuit deterministically derives `output` and `weights_commit`
// so the caller does not have to pre-compute them. The values are
// returned inside the *perceptronCircuit assignment; the caller reads
// them off via AssignPublic (or the wrapping Prove() helper).
func (PerceptronCircuit) AssignFull(inputs map[string]any) (frontend.Circuit, error) {
	inp, err := requireIntSlice(inputs, "input", PerceptronInputDim)
	if err != nil {
		return nil, err
	}
	wts, err := requireIntSlice(inputs, "weights", PerceptronInputDim)
	if err != nil {
		return nil, err
	}
	biasVal, err := requireBigInt(inputs, "bias")
	if err != nil {
		return nil, err
	}
	commit := ComputePerceptronCommit(wts, biasVal)
	// Compute expected output out-of-circuit: 1 iff <w,x>+b >= 0.
	dot := new(big.Int).Set(biasVal)
	tmp := new(big.Int)
	for i := 0; i < PerceptronInputDim; i++ {
		tmp.Mul(wts[i], inp[i])
		dot.Add(dot, tmp)
	}
	output := big.NewInt(0)
	if dot.Sign() >= 0 {
		output = big.NewInt(1)
	}

	c := &perceptronCircuit{Bias: biasVal, WeightsCommit: commit, Output: output}
	for i := 0; i < PerceptronInputDim; i++ {
		c.Input[i] = inp[i]
		c.Weights[i] = wts[i]
	}
	return c, nil
}

// AssignPublic keeps only public wires assigned. Used by the verifier.
func (PerceptronCircuit) AssignPublic(inputs map[string]any) (frontend.Circuit, error) {
	wts, err := requireIntSlice(inputs, "weights", PerceptronInputDim)
	if err != nil {
		return nil, err
	}
	commit, err := requireBigInt(inputs, "weights_commit")
	if err != nil {
		return nil, err
	}
	output, err := requireBigInt(inputs, "output")
	if err != nil {
		return nil, err
	}
	c := &perceptronCircuit{WeightsCommit: commit, Output: output}
	for i := 0; i < PerceptronInputDim; i++ {
		c.Weights[i] = wts[i]
	}
	return c, nil
}

// perceptronCircuit is the gnark circuit definition itself.
type perceptronCircuit struct {
	Input         [PerceptronInputDim]frontend.Variable
	Bias          frontend.Variable
	Weights       [PerceptronInputDim]frontend.Variable `gnark:",public"`
	WeightsCommit frontend.Variable                     `gnark:",public"`
	Output        frontend.Variable                     `gnark:",public"`
}

// Define emits the R1CS constraints. Three layers:
//   1. Recompute MiMC(weights || bias) and assert it equals the public
//      WeightsCommit. Any tampering with the private Bias here would
//      change the commit and fail — this is the mechanism that binds
//      the claimed model identity to the actual bias used in the dot
//      product below.
//   2. Compute dot = <weights, input> + bias in-circuit.
//   3. Assert Output == 1 iff dot >= 0; else 0. Uses gnark's cmp gadget
//      over a shifted, non-negative representative — see comment below.
func (c *perceptronCircuit) Define(api frontend.API) error {
	// (1) Commitment binding.
	h, err := mimc.NewMiMC(api)
	if err != nil {
		return fmt.Errorf("mimc init: %w", err)
	}
	for i := 0; i < PerceptronInputDim; i++ {
		h.Write(c.Weights[i])
	}
	h.Write(c.Bias)
	api.AssertIsEqual(c.WeightsCommit, h.Sum())

	// (2) Dot product + bias.
	dot := frontend.Variable(c.Bias)
	for i := 0; i < PerceptronInputDim; i++ {
		dot = api.Add(dot, api.Mul(c.Weights[i], c.Input[i]))
	}

	// (3) Compare `dot` against 0 in the signed sense. We can't do that
	// directly in the field (there is no sign bit), so we shift by a
	// large positive offset that guarantees the shifted value fits in
	// 32 bits AND is non-negative for every legal input. With 8 dims of
	// 16-bit signed weights x inputs, the absolute value of the dot
	// product is bounded by 8 * 32768 * 32768 ≈ 2^33. Adding a bias of
	// the same magnitude keeps us under 2^34. Shift by 2^34 to make
	// every legal dot non-negative and comfortably fit into 40 bits.
	const shiftBits = 40
	shift := new(big.Int).Lsh(big.NewInt(1), shiftBits-1) // 2^39
	shifted := api.Add(dot, shift)

	// The classifier decision: output = 1 iff dot >= 0 iff shifted >= shift.
	// cmp.IsLess(shifted, shift) is 1 when shifted < shift (i.e. dot < 0),
	// so `output` is its logical inverse.
	comparator := cmp.NewBoundedComparator(api, new(big.Int).Lsh(big.NewInt(1), shiftBits), false)
	isNegative := comparator.IsLess(shifted, shift)
	// Assert output is boolean.
	api.AssertIsBoolean(c.Output)
	// Assert output = 1 - isNegative.
	api.AssertIsEqual(c.Output, api.Sub(1, isNegative))
	return nil
}

// ComputePerceptronCommit recomputes the MiMC commitment used inside
// the circuit — MiMC absorbs weights[0..N-1] then bias, in that order.
// Callers derive `weights_commit` off-circuit using this helper.
func ComputePerceptronCommit(weights []*big.Int, bias *big.Int) *big.Int {
	h := bn254mimc.NewMiMC()
	for _, w := range weights {
		h.Write(padTo32(canonicalScalarBytes(w)))
	}
	h.Write(padTo32(canonicalScalarBytes(bias)))
	return new(big.Int).SetBytes(h.Sum(nil))
}

// canonicalScalarBytes returns the big-endian bytes of x reduced
// modulo the BN254 scalar field order — matching what gnark's frontend
// sees when the circuit is assigned. For negative x we lift into the
// field by returning (r - |x|) mod r; for non-negative x we return x
// (already in canonical form).
func canonicalScalarBytes(x *big.Int) []byte {
	// gnark-crypto exposes the scalar order via fr.Modulus() in the
	// bn254/fr package. We use a small helper to avoid importing that
	// path here explicitly — the modulus is copied inline. BN254 scalar
	// order (from gnark-crypto/ecc/bn254/fr): 21888242871839275222246405745257275088548364400416034343698204186575808495617.
	r, _ := new(big.Int).SetString(
		"21888242871839275222246405745257275088548364400416034343698204186575808495617", 10)
	xx := new(big.Int).Mod(x, r)
	if xx.Sign() < 0 {
		xx.Add(xx, r)
	}
	return xx.Bytes()
}

// requireIntSlice extracts an int/big.Int slice of exactly `length`
// entries from the assignment map. Accepts []int, []int64 or []*big.Int.
func requireIntSlice(m map[string]any, name string, length int) ([]*big.Int, error) {
	v, ok := m[name]
	if !ok {
		return nil, fmt.Errorf("missing required input %q", name)
	}
	out := make([]*big.Int, length)
	switch xs := v.(type) {
	case []*big.Int:
		if len(xs) != length {
			return nil, fmt.Errorf("input %q: expected length %d, got %d", name, length, len(xs))
		}
		for i, x := range xs {
			out[i] = new(big.Int).Set(x)
		}
	case []int:
		if len(xs) != length {
			return nil, fmt.Errorf("input %q: expected length %d, got %d", name, length, len(xs))
		}
		for i, x := range xs {
			out[i] = big.NewInt(int64(x))
		}
	case []int64:
		if len(xs) != length {
			return nil, fmt.Errorf("input %q: expected length %d, got %d", name, length, len(xs))
		}
		for i, x := range xs {
			out[i] = big.NewInt(x)
		}
	case []any:
		if len(xs) != length {
			return nil, fmt.Errorf("input %q: expected length %d, got %d", name, length, len(xs))
		}
		for i, x := range xs {
			bi, err := coerceBigInt(x, fmt.Sprintf("%s[%d]", name, i))
			if err != nil {
				return nil, err
			}
			out[i] = bi
		}
	default:
		return nil, fmt.Errorf("input %q: expected []int/[]*big.Int/[]int64/[]any, got %T", name, v)
	}
	return out, nil
}
