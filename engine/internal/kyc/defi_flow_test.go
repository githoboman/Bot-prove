package kyc

import (
	"testing"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
)

func TestNewDeFiFlow(t *testing.T) {
	eng := prover.New()
	f := NewDeFiFlow(eng)
	if f == nil {
		t.Fatal("nil flow")
	}
}

func TestDeFiFlowRunDemo(t *testing.T) {
	eng := prover.New()
	f := NewDeFiFlow(eng)
	err := f.RunDemo("test-agent")
	if err != nil {
		t.Fatalf("demo failed: %v", err)
	}
}

func TestDemoKYCGrantAccess(t *testing.T) {
	eng := prover.New()
	demo := NewDemo(eng)
	p := eng.Generate("agent", []byte("in"), []byte("out"), []byte("m"), "kyc")
	access, err := demo.GrantAccess("user1", p.ID)
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if !access.Whitelisted {
		t.Fatal("should be whitelisted")
	}
	if access.ProofID != p.ID {
		t.Fatalf("proof id mismatch: %s vs %s", access.ProofID, p.ID)
	}
}

func TestDemoKYCGrantAccessBadProof(t *testing.T) {
	eng := prover.New()
	demo := NewDemo(eng)
	_, err := demo.GrantAccess("user1", "P-999")
	if err == nil {
		t.Fatal("should fail with bad proof")
	}
}

func TestDemoIsWhitelisted(t *testing.T) {
	eng := prover.New()
	demo := NewDemo(eng)
	if demo.IsWhitelisted("nobody") {
		t.Fatal("unknown user should not be whitelisted")
	}

	p := eng.Generate("agent", []byte("i"), []byte("o"), []byte("m"), "kyc")
	_, _ = demo.GrantAccess("alice", p.ID)
	if !demo.IsWhitelisted("alice") {
		t.Fatal("alice should be whitelisted after grant")
	}
}

func TestKYCResultFields(t *testing.T) {
	r := KYCResult{
		User:     "alice",
		ProofID:  "P-1",
		Verified: true,
		TS:       1000,
	}
	if r.User != "alice" || !r.Verified {
		t.Fatal("bad fields")
	}
}

func TestDeFiAccessFields(t *testing.T) {
	a := DeFiAccess{
		User:        "bob",
		Whitelisted: true,
		ProofID:     "P-2",
	}
	if a.User != "bob" || !a.Whitelisted {
		t.Fatal("bad fields")
	}
}
