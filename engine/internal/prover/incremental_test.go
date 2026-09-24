package prover

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"
)

// Rooted invariant: an incremental accumulator that has appended the same
// sequence of leaves as BuildTree/Root must produce the exact same root.
// This locks the two APIs together so a downstream verifier that trusts
// merkle.go's Root() can also trust IncrementalMerkle.Root().
func TestIncremental_MatchesBuildTreeRoot(t *testing.T) {
	for n := 1; n <= 32; n++ {
		leaves := makeLeaves(n)
		wantRoot := Root(leaves)

		acc := NewIncrementalMerkle()
		for _, lf := range leaves {
			acc.Append(lf)
		}
		gotRoot := acc.Root()

		if gotRoot != wantRoot {
			t.Fatalf("n=%d root mismatch: incremental=%s buildtree=%s", n, gotRoot, wantRoot)
		}
	}
}

func TestIncremental_MatchesBuildTreeRoot_Random(t *testing.T) {
	r := rand.New(rand.NewSource(20260725))
	for i := 0; i < 500; i++ {
		n := 1 + r.Intn(63)
		leaves := makeLeavesSeeded(n, r)
		wantRoot := Root(leaves)

		acc := NewIncrementalMerkle()
		for _, lf := range leaves {
			acc.Append(lf)
		}
		if got := acc.Root(); got != wantRoot {
			t.Fatalf("iter=%d n=%d mismatch: incremental=%s buildtree=%s", i, n, got, wantRoot)
		}
	}
}

func TestBatchReceipt_HappyPath(t *testing.T) {
	acc := NewIncrementalMerkle()

	// First batch: 3 leaves from empty.
	batch1 := [][]byte{[]byte("a"), []byte("b"), []byte("c")}
	rc1, err := acc.AppendBatch(batch1)
	if err != nil {
		t.Fatalf("batch1: %v", err)
	}
	if rc1.PrevRoot != "" {
		t.Errorf("batch1: prev root expected empty, got %s", rc1.PrevRoot)
	}
	if rc1.NewRoot == "" {
		t.Errorf("batch1: new root empty")
	}
	if rc1.BatchSize != 3 {
		t.Errorf("batch1: size want 3 got %d", rc1.BatchSize)
	}

	// Second batch on top: 2 more leaves.
	batch2 := [][]byte{[]byte("d"), []byte("e")}
	rc2, err := acc.AppendBatch(batch2)
	if err != nil {
		t.Fatalf("batch2: %v", err)
	}
	if rc2.PrevRoot != rc1.NewRoot {
		t.Errorf("batch2: prev root should chain from batch1 new root; got %s vs %s",
			rc2.PrevRoot, rc1.NewRoot)
	}

	// Sanity: cumulative BuildTree root must match acc.Root().
	all := append(append([][]byte{}, batch1...), batch2...)
	if wantRoot := Root(all); wantRoot != rc2.NewRoot {
		t.Errorf("cumulative mismatch: buildtree=%s incremental=%s", wantRoot, rc2.NewRoot)
	}
}

func TestBatchReceipt_VerifyTransition(t *testing.T) {
	acc := NewIncrementalMerkle()

	// Seed the accumulator with 4 prior leaves the verifier already committed.
	for _, lf := range makeLeaves(4) {
		acc.Append(lf)
	}
	snap := acc.Clone()

	// Submit a new batch of 3 leaves.
	newLeaves := [][]byte{[]byte("x"), []byte("y"), []byte("z")}
	rc, err := acc.AppendBatch(newLeaves)
	if err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}

	// A verifier that only knows the pre-batch accumulator + the receipt must
	// accept the transition.
	ok, err := VerifyReceiptTransition(snap, rc)
	if err != nil {
		t.Fatalf("verify err: %v", err)
	}
	if !ok {
		t.Fatal("verifier rejected a well-formed batch receipt")
	}

	// Tamper: flip one bit of one leaf hash and confirm the verifier rejects.
	tampered := *rc
	tampered.Leaves = append([]string{}, rc.Leaves...)
	// Flip the last nibble of leaf 0.
	first := []byte(tampered.Leaves[0])
	first[len(first)-1] ^= 0x01
	tampered.Leaves[0] = string(first)
	ok, _ = VerifyReceiptTransition(snap, &tampered)
	if ok {
		t.Fatal("verifier accepted a tampered receipt")
	}

	// Tamper: swap NewRoot with a random hex string of correct length.
	tampered2 := *rc
	tampered2.NewRoot = hex.EncodeToString(sha256.New().Sum([]byte("evil-preimage")))[:64]
	ok, _ = VerifyReceiptTransition(snap, &tampered2)
	if ok {
		t.Fatal("verifier accepted a receipt with mutated NewRoot")
	}
}

func TestBatchReceipt_EmptyRejected(t *testing.T) {
	acc := NewIncrementalMerkle()
	_, err := acc.AppendBatch(nil)
	if err == nil {
		t.Fatal("empty batch accepted")
	}
}

func TestBatchReceipt_PrevRootMismatchRejected(t *testing.T) {
	acc := NewIncrementalMerkle()
	acc.Append([]byte("a"))
	acc.Append([]byte("b"))
	rc, _ := acc.AppendBatch([][]byte{[]byte("c")})

	other := NewIncrementalMerkle() // empty -> different PrevRoot
	ok, err := VerifyReceiptTransition(other, rc)
	if err == nil || ok {
		t.Fatalf("expected prev-root mismatch, got ok=%v err=%v", ok, err)
	}
}

// PBT: append the same 100 random leaves in K different batch splittings and
// verify that (a) every intermediate BatchReceipt validates against its
// starting snapshot, and (b) the FINAL root is identical to the monolithic
// BuildTree root for all splittings.
func TestPBT_BatchSplittingIsAssociative(t *testing.T) {
	const total = 100
	r := rand.New(rand.NewSource(20260726))
	leaves := makeLeavesSeeded(total, r)
	monolithic := Root(leaves)

	for trial := 0; trial < 30; trial++ {
		// Random partition of `total` into 1..8 batches.
		numBatches := 1 + r.Intn(8)
		cuts := make([]int, 0, numBatches-1)
		for i := 0; i < numBatches-1; i++ {
			cuts = append(cuts, 1+r.Intn(total-1))
		}
		cuts = uniqueSortedInts(cuts)
		cuts = append(cuts, total)

		acc := NewIncrementalMerkle()
		start := 0
		for _, end := range cuts {
			if end <= start {
				continue
			}
			snap := acc.Clone()
			rc, err := acc.AppendBatch(leaves[start:end])
			if err != nil {
				t.Fatalf("trial=%d batch %d..%d: %v", trial, start, end, err)
			}
			ok, err := VerifyReceiptTransition(snap, rc)
			if err != nil || !ok {
				t.Fatalf("trial=%d batch %d..%d verify failed: ok=%v err=%v",
					trial, start, end, ok, err)
			}
			start = end
		}
		if got := acc.Root(); got != monolithic {
			t.Fatalf("trial=%d final root mismatch: incremental=%s monolithic=%s",
				trial, got, monolithic)
		}
	}
}

// --- helpers ---

func makeLeaves(n int) [][]byte {
	out := make([][]byte, n)
	for i := 0; i < n; i++ {
		out[i] = []byte(fmt.Sprintf("leaf-%d", i))
	}
	return out
}

func makeLeavesSeeded(n int, r *rand.Rand) [][]byte {
	out := make([][]byte, n)
	for i := 0; i < n; i++ {
		out[i] = []byte(fmt.Sprintf("leaf-%d-%d", i, r.Int63()))
	}
	return out
}

func uniqueSortedInts(in []int) []int {
	seen := make(map[int]struct{}, len(in))
	out := make([]int, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	// insertion sort — tiny lists
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
