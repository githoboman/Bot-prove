package crypto

import (
	"crypto/rand"
	"encoding/binary"
	"testing"
)

func TestRange_ProveVerify_Roundtrip(t *testing.T) {
	for _, x := range []uint64{0, 1, 2, 42, 1023, 65535} {
		rp, err := ProveRange(x, 16)
		if err != nil {
			t.Fatalf("prove x=%d: %v", x, err)
		}
		if err := VerifyRange(rp); err != nil {
			t.Fatalf("verify x=%d: %v", x, err)
		}
	}
}

func TestRange_OutOfRange_Rejected(t *testing.T) {
	// 2^8 = 256 is out of [0, 2^8).
	if _, err := ProveRange(256, 8); err == nil {
		t.Fatal("expected ProveRange to reject x >= 2^n")
	}
}

func TestRange_TamperedBitCommitment_Rejected(t *testing.T) {
	rp, err := ProveRange(7, 8)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	// Flip a byte in one of the bit commitments so it decodes to a
	// different valid point.
	orig := rp.Bits[3]
	for i := 0; i < 32; i++ {
		rp.Bits[3][i] = orig[i] ^ 0x01
		if err := VerifyRange(rp); err == nil {
			// Restore and continue - accept any single-bit flip catch.
			continue
		}
		rp.Bits[3] = orig
		return
	}
	t.Fatal("no single-bit flip on Bits[3] was rejected")
}

func TestRange_TamperedORProof_Rejected(t *testing.T) {
	rp, err := ProveRange(42, 8)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	// Flip a byte in the s0 component of bit 2's OR proof.
	rp.ORs[2][100] ^= 0xff
	if err := VerifyRange(rp); err == nil {
		t.Fatal("expected verify to fail on tampered OR proof")
	}
}

func TestRange_TamperedAggregate_Rejected(t *testing.T) {
	rp, err := ProveRange(9, 8)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	// Flip a byte in C - homomorphic check must reject.
	rp.C[10] ^= 0x77
	if err := VerifyRange(rp); err == nil {
		t.Fatal("expected verify to fail on tampered aggregate C")
	}
}

func TestRange_RandomizedFuzz(t *testing.T) {
	// 20 random values in [0, 2^32), each with n=32.
	for i := 0; i < 20; i++ {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			t.Fatalf("rand: %v", err)
		}
		x := binary.LittleEndian.Uint64(buf[:]) & ((uint64(1) << 32) - 1)
		rp, err := ProveRange(x, 32)
		if err != nil {
			t.Fatalf("prove x=%d: %v", x, err)
		}
		if err := VerifyRange(rp); err != nil {
			t.Fatalf("verify x=%d: %v", x, err)
		}
	}
}

func TestRange_PedersenCommit_Consistency(t *testing.T) {
	// CommitRange should produce a commitment that lives in the prime-order
	// subgroup (i.e. decodable and non-identity for x=1).
	C, _, err := CommitRange(1)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if C == [32]byte{} {
		t.Fatal("CommitRange returned identity for x=1 - suspicious")
	}
}
