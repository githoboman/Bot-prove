package aggregator

import (
	"crypto/sha256"
	"testing"
)

func hashEach(items [][]byte) [][]byte {
	out := make([][]byte, len(items))
	for i, it := range items {
		h := sha256.Sum256(it)
		out[i] = h[:]
	}
	return out
}

// These are honest smoke tests against the actual exported API of this
// package. A prior test file here referenced functions/methods
// (NewStarkAggregator, Aggregate, Verify) that never existed in this
// package and had never been compiled or run; it was replaced with this
// file, which tests the real STARKAggregator/AggregateSTARKs/VerifyAggregate/
// CreateSTARKPack/UnpackAndVerify API.

func TestNewSTARKAggregator(t *testing.T) {
	sa := NewSTARKAggregator()
	if sa == nil {
		t.Fatal("expected non-nil aggregator")
	}
}

func TestAggregateSTARKs_EmptyInput(t *testing.T) {
	sa := NewSTARKAggregator()
	if _, err := sa.AggregateSTARKs(nil); err == nil {
		t.Error("expected error for empty proof list")
	}
}

func TestAggregateSTARKs_Deterministic(t *testing.T) {
	sa := NewSTARKAggregator()
	proofs := [][]byte{[]byte("proof-a"), []byte("proof-b"), []byte("proof-c")}

	agg1, err := sa.AggregateSTARKs(proofs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	agg2, err := sa.AggregateSTARKs(proofs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(agg1) != string(agg2) {
		t.Error("expected aggregation of the same proofs to be deterministic")
	}
}

func TestAggregateAndVerify_RoundTrip(t *testing.T) {
	sa := NewSTARKAggregator()
	proofs := [][]byte{[]byte("proof-1"), []byte("proof-2")}

	agg, err := sa.AggregateSTARKs(proofs)
	if err != nil {
		t.Fatalf("aggregate failed: %v", err)
	}

	ok, err := sa.VerifyAggregate(agg, hashEach(proofs))
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if !ok {
		t.Error("expected aggregate proof to verify against its own inputs")
	}

	// Tampered input hashes must not verify against the original aggregate.
	tampered := [][]byte{[]byte("proof-1"), []byte("proof-X")}
	ok, _ = sa.VerifyAggregate(agg, hashEach(tampered))
	if ok {
		t.Error("expected tampered proof set to fail verification")
	}
}

func TestSTARKPack_CreateAndUnpack(t *testing.T) {
	sa := NewSTARKAggregator()
	proofs := [][]byte{[]byte("p1"), []byte("p2"), []byte("p3")}

	pack, err := sa.CreateSTARKPack(proofs, map[string]string{"use_case": "test"})
	if err != nil {
		t.Fatalf("CreateSTARKPack failed: %v", err)
	}
	if pack.ProofCount != len(proofs) {
		t.Errorf("expected ProofCount=%d, got %d", len(proofs), pack.ProofCount)
	}

	ok, err := sa.UnpackAndVerify(pack)
	if err != nil {
		t.Fatalf("UnpackAndVerify failed: %v", err)
	}
	if !ok {
		t.Error("expected freshly created pack to verify")
	}
}
