package hasher

import "testing"

func TestHash(t *testing.T) {
	h := Hash([]byte("hello"))
	if h == [32]byte{} {
		t.Fatal("empty hash")
	}
}

func TestHexHash(t *testing.T) {
	h := HexHash([]byte("hello"))
	if len(h) != 64 {
		t.Fatalf("want 64 hex chars, got %d", len(h))
	}
}

func TestCommitHash(t *testing.T) {
	h1 := CommitHash([]byte("in"), []byte("out"), []byte("model"))
	h2 := CommitHash([]byte("in"), []byte("out"), []byte("model"))
	if h1 != h2 {
		t.Fatal("commit hash not deterministic")
	}
}

func TestVerifyCommit(t *testing.T) {
	in, out, m := []byte("a"), []byte("b"), []byte("c")
	c := CommitHash(in, out, m)
	if !VerifyCommit(c, in, out, m) {
		t.Fatal("valid commit failed")
	}
	if VerifyCommit(c, []byte("x"), out, m) {
		t.Fatal("tampered input should fail")
	}
}

func TestDifferentInputs(t *testing.T) {
	h1 := CommitHash([]byte("a"), []byte("b"), []byte("c"))
	h2 := CommitHash([]byte("x"), []byte("b"), []byte("c"))
	if h1 == h2 {
		t.Fatal("different inputs produced same hash")
	}
}

func TestHashEmpty(t *testing.T) {
	h := Hash([]byte{})
	if h == [32]byte{} {
		t.Fatal("empty input should still produce a hash")
	}
}

func TestHexHashDeterministic(t *testing.T) {
	a := HexHash([]byte("test"))
	b := HexHash([]byte("test"))
	if a != b {
		t.Fatal("same input, different hash")
	}
}

func TestCommitHashOutputChanged(t *testing.T) {
	h1 := CommitHash([]byte("in"), []byte("out1"), []byte("m"))
	h2 := CommitHash([]byte("in"), []byte("out2"), []byte("m"))
	if h1 == h2 {
		t.Fatal("different outputs should give different commits")
	}
}

func TestCommitHashModelChanged(t *testing.T) {
	h1 := CommitHash([]byte("in"), []byte("out"), []byte("model-a"))
	h2 := CommitHash([]byte("in"), []byte("out"), []byte("model-b"))
	if h1 == h2 {
		t.Fatal("different models should give different commits")
	}
}

func TestVerifyCommitAllTampered(t *testing.T) {
	in, out, m := []byte("input"), []byte("output"), []byte("model")
	commit := CommitHash(in, out, m)

	cases := []struct {
		name string
		in2  []byte
		out2 []byte
		m2   []byte
		want bool
	}{
		{"valid", in, out, m, true},
		{"tampered_input", []byte("X"), out, m, false},
		{"tampered_output", in, []byte("X"), m, false},
		{"tampered_model", in, out, []byte("X"), false},
		{"all_tampered", []byte("X"), []byte("Y"), []byte("Z"), false},
	}

	for _, tc := range cases {
		got := VerifyCommit(commit, tc.in2, tc.out2, tc.m2)
		if got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestHexHashLength(t *testing.T) {
	inputs := []string{"", "a", "hello world", "0123456789abcdef"}
	for _, s := range inputs {
		h := HexHash([]byte(s))
		if len(h) != 64 {
			t.Errorf("input %q: got len %d, want 64", s, len(h))
		}
	}
}
