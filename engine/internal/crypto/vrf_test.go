package crypto

import (
	"bytes"
	"testing"
)

func TestVRF_ProveVerify_Roundtrip(t *testing.T) {
	seed, pk, err := VRFKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	alpha := []byte("cp:sortition:round-42")

	proof, beta, err := VRFProve(seed, alpha)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	beta2, err := VRFVerify(pk, alpha, proof)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !bytes.Equal(beta[:], beta2[:]) {
		t.Fatalf("beta mismatch: prover=%x verifier=%x", beta, beta2)
	}
}

func TestVRF_Deterministic(t *testing.T) {
	// Same seed + alpha -> same proof, same beta. (Nonce is deterministic.)
	seed, _, err := VRFKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	alpha := []byte("determinism-probe")

	p1, b1, err := VRFProve(seed, alpha)
	if err != nil {
		t.Fatalf("prove1: %v", err)
	}
	p2, b2, err := VRFProve(seed, alpha)
	if err != nil {
		t.Fatalf("prove2: %v", err)
	}
	if p1 != p2 {
		t.Fatal("VRF proof should be deterministic for the same (seed, alpha)")
	}
	if b1 != b2 {
		t.Fatal("VRF beta should be deterministic for the same (seed, alpha)")
	}
}

func TestVRF_DifferentAlphaDifferentBeta(t *testing.T) {
	seed, _, err := VRFKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	_, b1, err := VRFProve(seed, []byte("alpha-A"))
	if err != nil {
		t.Fatalf("prove A: %v", err)
	}
	_, b2, err := VRFProve(seed, []byte("alpha-B"))
	if err != nil {
		t.Fatalf("prove B: %v", err)
	}
	if b1 == b2 {
		t.Fatal("VRF beta must differ for different alpha (collision risk 2^-512)")
	}
}

func TestVRF_TamperedProof_Rejected(t *testing.T) {
	seed, pk, err := VRFKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	alpha := []byte("tamper-probe")
	proof, _, err := VRFProve(seed, alpha)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}

	// Flip a bit in the c component (middle 32 bytes).
	tampered := proof
	tampered[40] ^= 0x01
	if _, err := VRFVerify(pk, alpha, tampered); err == nil {
		t.Fatal("expected verify to fail on tampered proof")
	}
}

func TestVRF_TamperedMessage_Rejected(t *testing.T) {
	seed, pk, err := VRFKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	proof, _, err := VRFProve(seed, []byte("msg-original"))
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	if _, err := VRFVerify(pk, []byte("msg-tampered"), proof); err == nil {
		t.Fatal("expected verify to fail on different message")
	}
}

func TestVRF_WrongPK_Rejected(t *testing.T) {
	seedA, _, err := VRFKeypair()
	if err != nil {
		t.Fatalf("keygen A: %v", err)
	}
	_, pkB, err := VRFKeypair()
	if err != nil {
		t.Fatalf("keygen B: %v", err)
	}
	proof, _, err := VRFProve(seedA, []byte("msg"))
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	if _, err := VRFVerify(pkB, []byte("msg"), proof); err == nil {
		t.Fatal("expected verify to fail with unrelated pk")
	}
}

func TestVRF_DerivePK_Matches(t *testing.T) {
	seed, pk1, err := VRFKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	pk2, err := VRFDerivePK(seed)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if pk1 != pk2 {
		t.Fatal("VRFDerivePK must reproduce the public key from the seed")
	}
}
