package gnarkzk

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

// TestPerceptronCircuit_ClassifyPositive exercises the full compile ->
// setup -> prove -> verify pipeline for an assignment the circuit
// should ACCEPT (dot product ends up >= 0, output=1).
func TestPerceptronCircuit_ClassifyPositive(t *testing.T) {
	// weights = [1,1,1,1,1,1,1,1], bias = 0, input = [1,...,1]
	// dot = 8 >= 0, so output = 1.
	weights := make([]*big.Int, PerceptronInputDim)
	input := make([]*big.Int, PerceptronInputDim)
	for i := range weights {
		weights[i] = big.NewInt(1)
		input[i] = big.NewInt(1)
	}
	bias := big.NewInt(0)
	proveAndVerify(t, weights, input, bias, big.NewInt(1))
}

// TestPerceptronCircuit_ClassifyNegative exercises an assignment
// that should produce output=0.
func TestPerceptronCircuit_ClassifyNegative(t *testing.T) {
	// weights = [-1,-1,-1,-1,-1,-1,-1,-1], bias = -1, input = [1,...,1]
	// dot = -9 < 0, output = 0.
	weights := make([]*big.Int, PerceptronInputDim)
	input := make([]*big.Int, PerceptronInputDim)
	for i := range weights {
		weights[i] = big.NewInt(-1)
		input[i] = big.NewInt(1)
	}
	bias := big.NewInt(-1)
	proveAndVerify(t, weights, input, bias, big.NewInt(0))
}

// TestPerceptronCircuit_ClassifyBoundary tests the boundary case
// where dot product equals 0 (should be classified as positive =>
// output=1 because we use >= 0).
func TestPerceptronCircuit_ClassifyBoundary(t *testing.T) {
	weights := make([]*big.Int, PerceptronInputDim)
	input := make([]*big.Int, PerceptronInputDim)
	for i := range weights {
		weights[i] = big.NewInt(0)
		input[i] = big.NewInt(1)
	}
	bias := big.NewInt(0)
	// dot = 0, should be output=1 (>= 0 threshold).
	proveAndVerify(t, weights, input, bias, big.NewInt(1))
}

// TestPerceptronCircuit_RejectsFraudulentOutput ensures the circuit
// rejects a proof where the caller lies about the output.
func TestPerceptronCircuit_RejectsFraudulentOutput(t *testing.T) {
	// dot = 8 >= 0 => true output = 1. We try to claim output = 0.
	weights := make([]*big.Int, PerceptronInputDim)
	input := make([]*big.Int, PerceptronInputDim)
	for i := range weights {
		weights[i] = big.NewInt(1)
		input[i] = big.NewInt(1)
	}
	bias := big.NewInt(0)

	pc := PerceptronCircuit{}
	// Build a full assignment but hand-tampered to claim wrong output.
	commit := ComputePerceptronCommit(weights, bias)
	c := &perceptronCircuit{
		Bias:          bias,
		WeightsCommit: commit,
		Output:        big.NewInt(0), // LIE
	}
	for i := 0; i < PerceptronInputDim; i++ {
		c.Input[i] = input[i]
		c.Weights[i] = weights[i]
	}

	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, pc.NewCircuit())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	pk, _, err := groth16.Setup(ccs)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	witness, err := frontend.NewWitness(c, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatalf("witness: %v", err)
	}
	if _, err := groth16.Prove(ccs, pk, witness); err == nil {
		t.Errorf("Prove must fail when caller lies about output")
	}
}

// TestPerceptronCircuit_RejectsTamperedCommitment ensures a mismatch
// between claimed weights_commit and the actual weights|bias also fails.
func TestPerceptronCircuit_RejectsTamperedCommitment(t *testing.T) {
	weights := make([]*big.Int, PerceptronInputDim)
	input := make([]*big.Int, PerceptronInputDim)
	for i := range weights {
		weights[i] = big.NewInt(1)
		input[i] = big.NewInt(1)
	}
	bias := big.NewInt(0)

	pc := PerceptronCircuit{}
	c := &perceptronCircuit{
		Bias:          bias,
		WeightsCommit: big.NewInt(12345), // WRONG
		Output:        big.NewInt(1),
	}
	for i := 0; i < PerceptronInputDim; i++ {
		c.Input[i] = input[i]
		c.Weights[i] = weights[i]
	}
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, pc.NewCircuit())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	pk, _, err := groth16.Setup(ccs)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	witness, err := frontend.NewWitness(c, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatalf("witness: %v", err)
	}
	if _, err := groth16.Prove(ccs, pk, witness); err == nil {
		t.Errorf("Prove must fail when commit is wrong")
	}
}

// TestPerceptronCircuit_AssignFullDerivesCommitAndOutput checks that
// the assignment helper produces a valid witness *without* the caller
// having to compute weights_commit or output out-of-band.
func TestPerceptronCircuit_AssignFullDerivesCommitAndOutput(t *testing.T) {
	weights := make([]*big.Int, PerceptronInputDim)
	input := make([]*big.Int, PerceptronInputDim)
	for i := range weights {
		weights[i] = big.NewInt(2)
		input[i] = big.NewInt(3)
	}
	bias := big.NewInt(-1)

	pc := PerceptronCircuit{}
	c, err := pc.AssignFull(map[string]any{
		"input":   input,
		"weights": weights,
		"bias":    bias,
	})
	if err != nil {
		t.Fatalf("AssignFull: %v", err)
	}
	pcc, ok := c.(*perceptronCircuit)
	if !ok {
		t.Fatalf("AssignFull: expected *perceptronCircuit, got %T", c)
	}
	expectedCommit := ComputePerceptronCommit(weights, bias)
	claimedCommit, ok := pcc.WeightsCommit.(*big.Int)
	if !ok {
		t.Fatalf("WeightsCommit: expected *big.Int, got %T", pcc.WeightsCommit)
	}
	if expectedCommit.Cmp(claimedCommit) != 0 {
		t.Errorf("AssignFull did not derive commit: got %s want %s", claimedCommit, expectedCommit)
	}
	// dot = 8*2*3 + (-1) = 47 >= 0 => output = 1.
	output, ok := pcc.Output.(*big.Int)
	if !ok {
		t.Fatalf("Output: expected *big.Int, got %T", pcc.Output)
	}
	if output.Sign() == 0 {
		t.Errorf("AssignFull derived wrong output: expected 1 (dot >= 0)")
	}
}

// TestPerceptronCircuit_Descriptor is a sanity check that the metadata
// exposed to /v1/circuits stays honest.
func TestPerceptronCircuit_Descriptor(t *testing.T) {
	d := PerceptronCircuit{}.Descriptor()
	if d.ID != PerceptronCircuitID {
		t.Errorf("bad ID: %s", d.ID)
	}
	if d.Curve != "BN254" || d.Backend != "groth16" {
		t.Errorf("unexpected curve/backend: %+v", d)
	}
	// weights_commit + 8 weights + output = 10 public inputs.
	if len(d.PublicInputs) != PerceptronInputDim+2 {
		t.Errorf("bad number of public inputs: %d", len(d.PublicInputs))
	}
}

// TestPerceptronCircuit_CanonicalScalarBytes_NegativeLifted checks the
// out-of-circuit negative-to-field lift matches what gnark's frontend
// does internally.
func TestPerceptronCircuit_CanonicalScalarBytes_NegativeLifted(t *testing.T) {
	r, _ := new(big.Int).SetString(
		"21888242871839275222246405745257275088548364400416034343698204186575808495617", 10)
	x := big.NewInt(-5)
	got := canonicalScalarBytes(x)
	expected := new(big.Int).Sub(r, big.NewInt(5))
	if !bytes.Equal(got, expected.Bytes()) {
		t.Errorf("canonicalScalarBytes(-5) = %x, want %x", got, expected.Bytes())
	}
}

// --- helper: full compile -> setup -> prove -> verify roundtrip ---

func proveAndVerify(t *testing.T, weights, input []*big.Int, bias, expectedOutput *big.Int) {
	t.Helper()
	pc := PerceptronCircuit{}
	full, err := pc.AssignFull(map[string]any{
		"input":   input,
		"weights": weights,
		"bias":    bias,
	})
	if err != nil {
		t.Fatalf("AssignFull: %v", err)
	}
	// Sanity: the derived output must match what the test expects.
	fullCircuit, ok := full.(*perceptronCircuit)
	if !ok {
		t.Fatalf("AssignFull: expected *perceptronCircuit, got %T", full)
	}
	got, ok := fullCircuit.Output.(*big.Int)
	if !ok {
		t.Fatalf("Output: expected *big.Int, got %T", fullCircuit.Output)
	}
	if got.Cmp(expectedOutput) != 0 {
		t.Fatalf("assignment derived output=%s, expected %s", got, expectedOutput)
	}
	commit, ok := fullCircuit.WeightsCommit.(*big.Int)
	if !ok {
		t.Fatalf("WeightsCommit: expected *big.Int, got %T", fullCircuit.WeightsCommit)
	}

	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, pc.NewCircuit())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	witness, err := frontend.NewWitness(full, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatalf("witness: %v", err)
	}
	proof, err := groth16.Prove(ccs, pk, witness)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	// Public witness for verification.
	pubAssign, err := pc.AssignPublic(map[string]any{
		"weights":         weights,
		"weights_commit":  commit,
		"output":          expectedOutput,
	})
	if err != nil {
		t.Fatalf("AssignPublic: %v", err)
	}
	pubWitness, err := frontend.NewWitness(pubAssign, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err != nil {
		t.Fatalf("public witness: %v", err)
	}
	if err := groth16.Verify(proof, vk, pubWitness); err != nil {
		t.Errorf("verify: %v", err)
	}
}
