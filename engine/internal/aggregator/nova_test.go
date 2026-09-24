package aggregator

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex fixture %q: %v", s, err)
	}
	return b
}

func TestHashFolder_RoundTrip(t *testing.T) {
	steps := []FoldStep{
		{Instance: []byte("layer1:matmul"), WitnessDigest: mustHex(t, "aa"+"bb"+"cc"+"dd")},
		{Instance: []byte("layer2:relu"), WitnessDigest: mustHex(t, "11223344")},
		{Instance: []byte("layer3:softmax"), WitnessDigest: mustHex(t, "deadbeefdeadbeef")},
	}
	agg, err := FoldAll(steps)
	if err != nil {
		t.Fatalf("FoldAll: %v", err)
	}
	if agg.Scheme != SchemeHashFoldV1 {
		t.Fatalf("wrong scheme label: %s", agg.Scheme)
	}
	if agg.Steps != 3 {
		t.Fatalf("expected 3 steps, got %d", agg.Steps)
	}
	if agg.Root == "" || len(agg.StepHashes) != 3 {
		t.Fatalf("bad aggregate shape: %+v", agg)
	}

	ok, err := VerifyAll(steps, agg)
	if err != nil {
		t.Fatalf("VerifyAll: %v", err)
	}
	if !ok {
		t.Fatalf("aggregate must verify")
	}
}

func TestHashFolder_TamperedStep(t *testing.T) {
	steps := []FoldStep{
		{Instance: []byte("step-a"), WitnessDigest: mustHex(t, "01020304")},
		{Instance: []byte("step-b"), WitnessDigest: mustHex(t, "05060708")},
	}
	agg, err := FoldAll(steps)
	if err != nil {
		t.Fatalf("FoldAll: %v", err)
	}
	// Tamper: replace the second step's witness digest.
	tampered := []FoldStep{
		steps[0],
		{Instance: steps[1].Instance, WitnessDigest: mustHex(t, "ffffffff")},
	}
	ok, err := VerifyAll(tampered, agg)
	if err == nil {
		t.Fatalf("verify must reject tampered steps with an error")
	}
	if ok {
		t.Fatalf("verify must return false on tamper")
	}
}

func TestHashFolder_ReorderedSteps(t *testing.T) {
	steps := []FoldStep{
		{Instance: []byte("A"), WitnessDigest: mustHex(t, "11")},
		{Instance: []byte("B"), WitnessDigest: mustHex(t, "22")},
	}
	agg, _ := FoldAll(steps)
	reordered := []FoldStep{steps[1], steps[0]}
	ok, err := VerifyAll(reordered, agg)
	if ok || err == nil {
		t.Fatalf("reordered sequence must not verify (got ok=%v err=%v)", ok, err)
	}
}

func TestHashFolder_DeterministicAcrossRuns(t *testing.T) {
	steps := []FoldStep{
		{Instance: []byte("deterministic"), WitnessDigest: mustHex(t, "cafe")},
		{Instance: []byte("deterministic"), WitnessDigest: mustHex(t, "babe")},
	}
	agg1, _ := FoldAll(steps)
	agg2, _ := FoldAll(steps)
	if agg1.Root != agg2.Root {
		t.Fatalf("hash-fold must be deterministic: %s vs %s", agg1.Root, agg2.Root)
	}
	if !equalStrSlice(agg1.StepHashes, agg2.StepHashes) {
		t.Fatalf("step hashes must be deterministic")
	}
}

func TestHashFolder_RejectsEmptyStep(t *testing.T) {
	f := NewHashFolder()
	err := f.Fold(FoldStep{Instance: nil, WitnessDigest: []byte{0}})
	if err == nil {
		t.Fatalf("empty Instance must be rejected")
	}
}

func TestHashFolder_RejectsUnknownScheme(t *testing.T) {
	steps := []FoldStep{{Instance: []byte("x"), WitnessDigest: []byte{1}}}
	agg, _ := FoldAll(steps)
	agg.Scheme = SchemeNovaGoV1 // pretend it's real Nova
	ok, err := VerifyAll(steps, agg)
	if ok || err == nil {
		t.Fatalf("verify must refuse an aggregate labelled with a scheme it does not implement")
	}
}

func TestHashFolder_UsedIncrementally(t *testing.T) {
	// FoldAll is a convenience; ensure the interface is usable step-by-step.
	f := NewHashFolder()
	steps := []FoldStep{
		{Instance: []byte("s1"), WitnessDigest: []byte{0x01}},
		{Instance: []byte("s2"), WitnessDigest: []byte{0x02}},
		{Instance: []byte("s3"), WitnessDigest: []byte{0x03}},
	}
	for _, s := range steps {
		if err := f.Fold(s); err != nil {
			t.Fatalf("Fold: %v", err)
		}
	}
	agg, err := f.Compress()
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	// Independently compute expected root through FoldAll and compare.
	agg2, _ := FoldAll(steps)
	if agg.Root != agg2.Root {
		t.Fatalf("incremental root must match FoldAll: %s vs %s", agg.Root, agg2.Root)
	}
	// Guard: root is 32 bytes hex.
	if got, _ := hex.DecodeString(agg.Root); len(got) != 32 {
		t.Fatalf("root must be sha256 (32B), got %d bytes", len(got))
	}
	_ = bytes.Compare // reserved for future assertions
}

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
