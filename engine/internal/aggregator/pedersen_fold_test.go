package aggregator

import (
	"bytes"
	"testing"
)

func makeSteps(n int) []FoldStep {
	out := make([]FoldStep, n)
	for i := 0; i < n; i++ {
		out[i] = FoldStep{
			Instance:      []byte{byte(i), 0xa5, 0x5a},
			WitnessDigest: []byte{byte(i), 0x11, 0x22, 0x33},
		}
	}
	return out
}

func TestPedersenFoldEmptyRejected(t *testing.T) {
	f := NewPedersenFolder()
	if _, err := f.Compress(); err == nil {
		t.Fatal("Compress on empty folder should error")
	}
	if err := f.Fold(FoldStep{Instance: nil}); err == nil {
		t.Fatal("Fold with empty instance should error")
	}
}

func TestPedersenFoldAndVerifyHappyPath(t *testing.T) {
	steps := makeSteps(5)
	agg, err := FoldAllPedersen(steps)
	if err != nil {
		t.Fatalf("FoldAllPedersen: %v", err)
	}
	if agg.Scheme != SchemePedersenFoldV1 {
		t.Fatalf("scheme = %q, want %q", agg.Scheme, SchemePedersenFoldV1)
	}
	if agg.Steps != 5 {
		t.Fatalf("steps = %d, want 5", agg.Steps)
	}
	if agg.Root == "" {
		t.Fatal("empty root")
	}
	ok, err := VerifyAllPedersen(steps, agg)
	if err != nil || !ok {
		t.Fatalf("Verify: ok=%v err=%v", ok, err)
	}
}

func TestPedersenVerifyTamperDetected(t *testing.T) {
	steps := makeSteps(4)
	agg, err := FoldAllPedersen(steps)
	if err != nil {
		t.Fatalf("FoldAllPedersen: %v", err)
	}
	// Tamper: change one byte in the middle step's instance
	tampered := make([]FoldStep, len(steps))
	copy(tampered, steps)
	badInstance := bytes.Clone(steps[2].Instance)
	badInstance[0] ^= 0xff
	tampered[2] = FoldStep{Instance: badInstance, WitnessDigest: steps[2].WitnessDigest}

	ok, err := VerifyAllPedersen(tampered, agg)
	if ok {
		t.Fatal("tampered sequence verified as ok")
	}
	if err == nil {
		t.Fatal("tampered sequence returned nil error")
	}
}

func TestPedersenVerifyScheme(t *testing.T) {
	steps := makeSteps(2)
	agg, _ := FoldAllPedersen(steps)
	agg.Scheme = SchemeHashFoldV1 // wrong scheme
	if ok, err := VerifyAllPedersen(steps, agg); ok || err == nil {
		t.Fatalf("expected scheme mismatch, got ok=%v err=%v", ok, err)
	}
}

func TestPedersenVerifyStepCountMismatch(t *testing.T) {
	steps := makeSteps(3)
	agg, _ := FoldAllPedersen(steps)
	if ok, err := VerifyAllPedersen(steps[:2], agg); ok || err == nil {
		t.Fatalf("expected count mismatch, got ok=%v err=%v", ok, err)
	}
}

func TestPedersenDeterminism(t *testing.T) {
	// Fold same input twice — result must be identical.
	steps := makeSteps(6)
	a1, _ := FoldAllPedersen(steps)
	a2, _ := FoldAllPedersen(steps)
	if a1.Root != a2.Root {
		t.Fatalf("nondeterminism: %s vs %s", a1.Root, a2.Root)
	}
}

func TestPedersenHomomorphismCheck(t *testing.T) {
	// The whole point of Pedersen: Commit(a) + Commit(b) = Commit(a || b).
	// Check across several split points.
	steps := makeSteps(7)
	for _, split := range []int{0, 1, 3, 4, 6, 7} {
		ok, err := PedersenHomomorphismCheck(steps, split)
		if err != nil {
			t.Fatalf("split=%d err=%v", split, err)
		}
		if !ok {
			t.Fatalf("split=%d: homomorphism broken", split)
		}
	}
}

func TestPedersenAndHashFoldProduceDifferentRoots(t *testing.T) {
	// Sanity: the same steps under different schemes MUST yield
	// different roots (otherwise a scheme label swap would be silent).
	steps := makeSteps(4)
	hash, _ := FoldAll(steps)
	ped, _ := FoldAllPedersen(steps)
	if hash.Root == ped.Root {
		t.Fatal("hash-fold-v1 and pedersen-fold-v1 produced identical roots — impossible unless generators are trivial")
	}
}

func TestPedersenIntegrationWithFolderInterface(t *testing.T) {
	// A caller holding the Folder interface must be able to swap
	// backends without type assertions.
	var _ Folder = NewPedersenFolder()
	var _ Folder = NewHashFolder()
}
