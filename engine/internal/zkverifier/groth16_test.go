package zkverifier

import (
	"crypto/rand"
	"crypto/sha256"
	"io"
	"math/big"
	"testing"
	"time"
)

// NOTE: a prior version of this file referenced functions (HexEncode,
// HexDecode, GenerateRandomBytes, GenerateRandomString, NewBigInt, and a
// 2-return-value NewGroth16Verifier) that never existed in this package and
// had never been compiled. It's replaced with honest tests of the real
// exported API (NewGroth16Verifier/VerifyGroth16/BatchVerify/CreateChallenge/
// ResolveChallenge). See the package-level doc comment in groth16.go: this is
// an explicitly-labeled conceptual simulation, not real BN254 pairing math -
// tests reflect that, they don't pretend it's cryptographically sound.

func randBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = io.ReadFull(rand.Reader, b)
	return b
}

// findPassingComponents brute-forces a nonce so the conceptual VerifyGroth16
// hash check (last byte == 0x01) actually passes, instead of assuming the
// package's own generateDummyGroth16Components helper reliably does so (it
// doesn't: it only sets a byte in the input, not in the resulting hash).
func findPassingComponents(t *testing.T) (*Groth16VerificationKey, *Groth16Proof, []*big.Int) {
	t.Helper()
	gv := NewGroth16Verifier()
	for i := 0; i < 100000; i++ {
		vk, proof, inputs := generateDummyGroth16Components()
		ok, _ := gv.VerifyGroth16(vk, proof, inputs)
		if ok {
			return vk, proof, inputs
		}
	}
	t.Fatal("could not find a passing conceptual proof in 100000 tries")
	return nil, nil, nil
}

func TestNewGroth16Verifier(t *testing.T) {
	if NewGroth16Verifier() == nil {
		t.Fatal("expected non-nil verifier")
	}
}

func TestVerifyGroth16_RejectsNilInputs(t *testing.T) {
	gv := NewGroth16Verifier()
	if _, err := gv.VerifyGroth16(nil, nil, nil); err == nil {
		t.Error("expected error for nil vk/proof/inputs")
	}
}

func TestVerifyGroth16_PassingCaseVerifies(t *testing.T) {
	gv := NewGroth16Verifier()
	vk, proof, inputs := findPassingComponents(t)
	ok, err := gv.VerifyGroth16(vk, proof, inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected the found passing components to verify as valid")
	}
}

func TestVerifyGroth16_TamperedProofRejected(t *testing.T) {
	gv := NewGroth16Verifier()
	vk, proof, inputs := findPassingComponents(t)
	tampered := &Groth16Proof{A: append([]byte{}, proof.A...), B: proof.B, C: proof.C}
	tampered.A[0] ^= 0xFF
	ok, _ := gv.VerifyGroth16(vk, tampered, inputs)
	if ok {
		t.Error("expected tampered proof to fail verification")
	}
}

func TestBatchVerify_MismatchedLengthsRejected(t *testing.T) {
	gv := NewGroth16Verifier()
	vk, proof, inputs := findPassingComponents(t)
	_, err := gv.BatchVerify(vk, []*Groth16Proof{proof}, [][]*big.Int{inputs, inputs})
	if err == nil {
		t.Error("expected error for mismatched proof/input batch lengths")
	}
}

func TestBatchVerify_AllPassing(t *testing.T) {
	gv := NewGroth16Verifier()
	vk1, proof1, inputs1 := findPassingComponents(t)
	// Reuse the same vk for a second passing proof to keep the search cheap.
	var proof2 *Groth16Proof
	var inputs2 []*big.Int
	for i := 0; i < 100000; i++ {
		p := &Groth16Proof{A: randBytes(32), B: randBytes(64), C: randBytes(32)}
		in := []*big.Int{big.NewInt(1)}
		ok, _ := gv.VerifyGroth16(vk1, p, in)
		if ok {
			proof2, inputs2 = p, in
			break
		}
	}
	if proof2 == nil {
		t.Fatal("could not find a second passing proof")
	}

	ok, err := gv.BatchVerify(vk1, []*Groth16Proof{proof1, proof2}, [][]*big.Int{inputs1, inputs2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected batch of passing proofs to verify")
	}
}

func TestChallenge_CreateAndResolve(t *testing.T) {
	gv := NewGroth16Verifier()
	proofHash := randBytes(32)
	inputsHash := randBytes(32)

	ch, err := gv.CreateChallenge(proofHash, inputsHash)
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}
	if ch.Resolved {
		t.Error("expected a freshly created challenge to be unresolved")
	}

	// Build the exact response the implementation expects.
	h := append(append(append([]byte{}, ch.ChallengeValue...), proofHash...), inputsHash...)
	sum := sha256.Sum256(h)

	ok, err := gv.ResolveChallenge(ch, sum[:])
	if err != nil {
		t.Fatalf("ResolveChallenge failed: %v", err)
	}
	if !ok {
		t.Error("expected correctly derived response to resolve the challenge")
	}
	if !ch.Resolved {
		t.Error("expected challenge.Resolved to be true after successful resolution")
	}
}

func TestChallenge_ExpiredRejected(t *testing.T) {
	gv := NewGroth16Verifier()
	ch, err := gv.CreateChallenge(randBytes(32), randBytes(32))
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}
	ch.ExpiresAt = time.Now().Add(-time.Minute) // force expiry

	ok, err := gv.ResolveChallenge(ch, randBytes(32))
	if ok || err == nil {
		t.Error("expected expired challenge to be rejected with an error")
	}
}
