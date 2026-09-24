package aggregator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

func mkLeaves(n int) [][]byte {
	out := make([][]byte, n)
	for i := 0; i < n; i++ {
		out[i] = []byte(fmt.Sprintf("proof-commitment-%d", i))
	}
	return out
}

func TestMerkleAggregateRejectsEmpty(t *testing.T) {
	if _, err := AggregateMerkleRecursion(nil); err == nil {
		t.Fatal("expected error on empty leaves")
	}
	if _, err := AggregateMerkleRecursion([][]byte{nil}); err == nil {
		t.Fatal("expected error on empty leaf")
	}
}

func TestMerkleAggregatePowerOfTwo(t *testing.T) {
	agg, err := AggregateMerkleRecursion(mkLeaves(8))
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if agg.Scheme != SchemeMerkleRecursionV1 {
		t.Fatalf("scheme = %q", agg.Scheme)
	}
	if agg.Count != 8 || agg.TreeHeight != 3 || len(agg.RootHex) != 64 {
		t.Fatalf("bad shape: %+v", agg)
	}
}

func TestMerkleAggregateOddCountPadsLast(t *testing.T) {
	// 5 leaves → padding at every odd level; height still 3.
	agg, err := AggregateMerkleRecursion(mkLeaves(5))
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if agg.Count != 5 || agg.TreeHeight != 3 {
		t.Fatalf("bad shape: %+v", agg)
	}
}

func TestMerkleAggregateSingleLeaf(t *testing.T) {
	agg, err := AggregateMerkleRecursion(mkLeaves(1))
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if agg.Count != 1 || agg.TreeHeight != 0 {
		t.Fatalf("bad shape: %+v", agg)
	}
	// With height 0 the root is the leaf-tagged hash of the only leaf.
	h := sha256.Sum256(append([]byte{0x00}, mkLeaves(1)[0]...))
	if agg.RootHex != hex.EncodeToString(h[:]) {
		t.Fatalf("root mismatch: got %s want %s", agg.RootHex, hex.EncodeToString(h[:]))
	}
}

func TestMerkleInclusionHappyPath(t *testing.T) {
	leaves := mkLeaves(7)
	agg, err := AggregateMerkleRecursion(leaves)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	for i := range leaves {
		proof, err := BuildInclusionProof(leaves, i)
		if err != nil {
			t.Fatalf("BuildInclusionProof(%d): %v", i, err)
		}
		ok, err := VerifyMerkleInclusion(agg, proof)
		if err != nil {
			t.Fatalf("VerifyMerkleInclusion(%d): %v", i, err)
		}
		if !ok {
			t.Fatalf("VerifyMerkleInclusion(%d) = false", i)
		}
	}
}

func TestMerkleInclusionTamperedLeafRejected(t *testing.T) {
	leaves := mkLeaves(4)
	agg, _ := AggregateMerkleRecursion(leaves)
	proof, _ := BuildInclusionProof(leaves, 2)

	// Tamper with the leaf hex.
	leafBytes, _ := hex.DecodeString(proof.LeafHex)
	leafBytes[0] ^= 0xff
	proof.LeafHex = hex.EncodeToString(leafBytes)

	ok, err := VerifyMerkleInclusion(agg, proof)
	if ok {
		t.Fatal("tampered leaf accepted")
	}
	if err == nil {
		t.Fatal("tampered leaf returned nil error")
	}
}

func TestMerkleInclusionTamperedPathRejected(t *testing.T) {
	leaves := mkLeaves(6)
	agg, _ := AggregateMerkleRecursion(leaves)
	proof, _ := BuildInclusionProof(leaves, 3)

	// Tamper with one path element.
	sib, _ := hex.DecodeString(proof.Path[0])
	sib[0] ^= 0xff
	proof.Path[0] = hex.EncodeToString(sib)

	ok, err := VerifyMerkleInclusion(agg, proof)
	if ok {
		t.Fatal("tampered path accepted")
	}
	if err == nil {
		t.Fatal("tampered path returned nil error")
	}
}

func TestMerkleInclusionTamperedPositionRejected(t *testing.T) {
	leaves := mkLeaves(4)
	agg, _ := AggregateMerkleRecursion(leaves)
	proof, _ := BuildInclusionProof(leaves, 1)
	// Flip the last position — root will not match.
	proof.Positions[len(proof.Positions)-1] = !proof.Positions[len(proof.Positions)-1]
	ok, _ := VerifyMerkleInclusion(agg, proof)
	if ok {
		t.Fatal("flipped position accepted")
	}
}

func TestMerkleInclusionSchemeMismatch(t *testing.T) {
	leaves := mkLeaves(3)
	agg, _ := AggregateMerkleRecursion(leaves)
	proof, _ := BuildInclusionProof(leaves, 0)
	agg.Scheme = string(SchemeHashFoldV1) // wrong scheme
	ok, err := VerifyMerkleInclusion(agg, proof)
	if ok || err == nil {
		t.Fatalf("expected scheme mismatch, got ok=%v err=%v", ok, err)
	}
}

func TestMerkleAggregateDeterminism(t *testing.T) {
	leaves := mkLeaves(11)
	a1, _ := AggregateMerkleRecursion(leaves)
	a2, _ := AggregateMerkleRecursion(leaves)
	if a1.RootHex != a2.RootHex {
		t.Fatalf("root non-deterministic: %s vs %s", a1.RootHex, a2.RootHex)
	}
}

func TestMerkleAggregateDetectsOrderChange(t *testing.T) {
	leaves := mkLeaves(8)
	a1, _ := AggregateMerkleRecursion(leaves)
	shuffled := make([][]byte, len(leaves))
	copy(shuffled, leaves)
	shuffled[0], shuffled[7] = shuffled[7], shuffled[0]
	a2, _ := AggregateMerkleRecursion(shuffled)
	if a1.RootHex == a2.RootHex {
		t.Fatal("reordering leaves did not change root — should have")
	}
	// bytes.Equal is exercised implicitly via hex compare above;
	// this keeps import list honest.
	_ = bytes.Equal
}

func TestMerkleInclusionOutOfRange(t *testing.T) {
	leaves := mkLeaves(4)
	if _, err := BuildInclusionProof(leaves, -1); err == nil {
		t.Fatal("negative index accepted")
	}
	if _, err := BuildInclusionProof(leaves, 4); err == nil {
		t.Fatal("index == n accepted")
	}
}
