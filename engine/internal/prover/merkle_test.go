package prover

import "testing"

func TestBuildTree(t *testing.T) {
	leaves := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d")}
	tree := BuildTree(leaves)
	if tree == nil {
		t.Fatal("nil tree")
	}
	if tree.Left == nil || tree.Right == nil {
		t.Fatal("missing children")
	}
}

func TestBuildTreeEmpty(t *testing.T) {
	tree := BuildTree(nil)
	if tree != nil {
		t.Fatal("expected nil for empty")
	}
}

func TestRoot(t *testing.T) {
	leaves := [][]byte{[]byte("a"), []byte("b")}
	r := Root(leaves)
	if len(r) != 64 {
		t.Fatalf("want 64 hex chars, got %d", len(r))
	}
}

func TestRootDeterministic(t *testing.T) {
	leaves := [][]byte{[]byte("x"), []byte("y"), []byte("z")}
	r1 := Root(leaves)
	r2 := Root(leaves)
	if r1 != r2 {
		t.Fatal("root not deterministic")
	}
}

func TestGetPath(t *testing.T) {
	leaves := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d")}
	path := GetPath(leaves, 0)
	if len(path) == 0 {
		t.Fatal("empty path")
	}
}

func TestVerifyPath(t *testing.T) {
	leaves := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d")}
	root := Root(leaves)
	path := GetPath(leaves, 0)
	if !VerifyPath([]byte("a"), path, root, 0) {
		t.Fatal("valid path failed")
	}
}

func TestVerifyPathTampered(t *testing.T) {
	leaves := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d")}
	root := Root(leaves)
	path := GetPath(leaves, 0)
	if VerifyPath([]byte("tampered"), path, root, 0) {
		t.Fatal("tampered leaf should fail")
	}
}

func TestOddLeaves(t *testing.T) {
	leaves := [][]byte{[]byte("a"), []byte("b"), []byte("c")}
	r := Root(leaves)
	if len(r) != 64 {
		t.Fatal("odd leaves failed")
	}
}

func TestSingleLeaf(t *testing.T) {
	leaves := [][]byte{[]byte("only")}
	r := Root(leaves)
	if len(r) != 64 {
		t.Fatalf("single leaf root: got len %d", len(r))
	}
}

func TestVerifyAllLeaves(t *testing.T) {
	leaves := [][]byte{[]byte("w"), []byte("x"), []byte("y"), []byte("z")}
	root := Root(leaves)
	for i, leaf := range leaves {
		path := GetPath(leaves, i)
		if !VerifyPath(leaf, path, root, i) {
			t.Fatalf("leaf %d failed verification", i)
		}
	}
}

func TestDifferentLeavesDifferentRoots(t *testing.T) {
	r1 := Root([][]byte{[]byte("a"), []byte("b")})
	r2 := Root([][]byte{[]byte("c"), []byte("d")})
	if r1 == r2 {
		t.Fatal("different leaves should give different roots")
	}
}

func TestPathLengthPowerOfTwo(t *testing.T) {
	leaves := [][]byte{[]byte("1"), []byte("2"), []byte("3"), []byte("4")}
	path := GetPath(leaves, 0)
	// For 4 leaves (depth 2), path should have 2 siblings
	if len(path) != 2 {
		t.Fatalf("expected path length 2 for 4 leaves, got %d", len(path))
	}
}

func TestLargerTree(t *testing.T) {
	leaves := make([][]byte, 16)
	for i := range leaves {
		leaves[i] = []byte{byte(i)}
	}
	root := Root(leaves)
	if len(root) != 64 {
		t.Fatal("bad root")
	}
	for i, leaf := range leaves {
		path := GetPath(leaves, i)
		if !VerifyPath(leaf, path, root, i) {
			t.Fatalf("leaf %d failed", i)
		}
	}
}
