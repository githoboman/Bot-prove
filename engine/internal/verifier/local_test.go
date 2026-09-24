package verifier

import (
	"testing"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
)

func setup() (*LocalVerifier, *prover.ProofEngine) {
	return New(), prover.New()
}

func TestVerifyValidProof(t *testing.T) {
	v, eng := setup()
	in, out, m := []byte("input"), []byte("output"), []byte("model")
	p := eng.Generate("agent", in, out, m, "test")
	err := v.VerifyProof(p, in, out, m)
	if err != nil {
		t.Fatalf("valid proof rejected: %v", err)
	}
}

func TestVerifyNilProof(t *testing.T) {
	v := New()
	err := v.VerifyProof(nil, nil, nil, nil)
	if err == nil {
		t.Fatal("nil proof should error")
	}
}

func TestVerifyRevokedProof(t *testing.T) {
	v, eng := setup()
	in, out, m := []byte("i"), []byte("o"), []byte("m")
	p := eng.Generate("a", in, out, m, "uc")
	_ = eng.Revoke(p.ID, "bad")
	rp, _ := eng.Get(p.ID)
	err := v.VerifyProof(rp, in, out, m)
	if err == nil {
		t.Fatal("revoked proof should fail")
	}
}

func TestVerifyTamperedInput(t *testing.T) {
	v, eng := setup()
	in, out, m := []byte("real-in"), []byte("out"), []byte("m")
	p := eng.Generate("a", in, out, m, "uc")
	err := v.VerifyProof(p, []byte("fake-in"), out, m)
	if err == nil {
		t.Fatal("tampered input should fail")
	}
}

func TestVerifyTamperedOutput(t *testing.T) {
	v, eng := setup()
	in, out, m := []byte("in"), []byte("real-out"), []byte("m")
	p := eng.Generate("a", in, out, m, "uc")
	err := v.VerifyProof(p, in, []byte("fake-out"), m)
	if err == nil {
		t.Fatal("tampered output should fail")
	}
}

func TestVerifyTamperedModel(t *testing.T) {
	v, eng := setup()
	in, out, m := []byte("in"), []byte("out"), []byte("real-model")
	p := eng.Generate("a", in, out, m, "uc")
	err := v.VerifyProof(p, in, out, []byte("fake-model"))
	if err == nil {
		t.Fatal("tampered model should fail")
	}
}

func TestQuickCheckValid(t *testing.T) {
	v := New()
	in, out, m := []byte("i"), []byte("o"), []byte("m")
	eng := prover.New()
	p := eng.Generate("a", in, out, m, "uc")
	if !v.QuickCheck(p.PH, in, out, m) {
		t.Fatal("quick check failed for valid data")
	}
}

func TestQuickCheckInvalid(t *testing.T) {
	v := New()
	if v.QuickCheck("badhash", []byte("i"), []byte("o"), []byte("m")) {
		t.Fatal("quick check should fail for bad hash")
	}
}

func TestValidateHashGood(t *testing.T) {
	v := New()
	h := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	if !v.ValidateHash(h) {
		t.Fatal("valid 64-char hex should pass")
	}
}

func TestValidateHashBad(t *testing.T) {
	v := New()
	cases := []string{"", "short", "zzzz", "a1b2c3d4"}
	for _, c := range cases {
		if v.ValidateHash(c) {
			t.Fatalf("invalid hash %q should fail", c)
		}
	}
}
