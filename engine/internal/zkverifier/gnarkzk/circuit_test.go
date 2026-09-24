package gnarkzk

import (
	"math/big"
	"testing"
)

func TestRealGroth16_ProveAndVerify(t *testing.T) {
	setup, err := NewSetup()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	preimage := big.NewInt(424242)
	hash := ComputeMiMCHash(preimage)

	proof, err := setup.Prove(preimage, hash)
	if err != nil {
		t.Fatalf("prove failed: %v", err)
	}

	ok, err := setup.Verify(proof, hash)
	if err != nil {
		t.Fatalf("verify errored: %v", err)
	}
	if !ok {
		t.Error("expected a genuine proof to verify successfully")
	}
}

func TestRealGroth16_WrongPreimageFailsToProve(t *testing.T) {
	setup, err := NewSetup()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	preimage := big.NewInt(1)
	wrongHash := ComputeMiMCHash(big.NewInt(2)) // doesn't match preimage=1

	if _, err := setup.Prove(preimage, wrongHash); err == nil {
		t.Error("expected proof generation to fail for a preimage/hash mismatch (circuit constraint violated)")
	}
}

func TestRealGroth16_TamperedPublicHashRejected(t *testing.T) {
	setup, err := NewSetup()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	preimage := big.NewInt(999)
	hash := ComputeMiMCHash(preimage)
	proof, err := setup.Prove(preimage, hash)
	if err != nil {
		t.Fatalf("prove failed: %v", err)
	}

	tamperedHash := new(big.Int).Add(hash, big.NewInt(1))
	ok, err := setup.Verify(proof, tamperedHash)
	if err != nil {
		t.Fatalf("verify errored: %v", err)
	}
	if ok {
		t.Error("expected verification against a tampered public hash to fail")
	}
}

func TestComputeMiMCHash_Deterministic(t *testing.T) {
	a := ComputeMiMCHash(big.NewInt(7))
	b := ComputeMiMCHash(big.NewInt(7))
	if a.Cmp(b) != 0 {
		t.Error("expected identical preimages to produce identical MiMC hashes")
	}
	c := ComputeMiMCHash(big.NewInt(8))
	if a.Cmp(c) == 0 {
		t.Error("expected different preimages to produce different MiMC hashes")
	}
}
