package chainadapter

import (
	"testing"
	"time"
)

func TestEthereumStubDeterministic(t *testing.T) {
	a := NewEthereumStub("eth-sim")
	req := AnchorRequest{
		ProofID:    "proof-1",
		ProofHash:  "deadbeef",
		MerkleRoot: "cafebabe",
		VKHash:     "abcdabcd",
		Verdict:    "APPROVE",
		ModelID:    "model-42",
		Timestamp:  time.Unix(1_000_000, 0),
	}
	r1, err := a.Anchor(req)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := a.Anchor(req)
	if err != nil {
		t.Fatal(err)
	}
	if r1.TxHash != r2.TxHash {
		t.Fatalf("stub tx hash not deterministic: %s != %s", r1.TxHash, r2.TxHash)
	}
	if r1.Label != LabelSimulation {
		t.Fatalf("stub must be labelled SIMULATION; got %s", r1.Label)
	}
	if r1.ChainID != "eth-sim" {
		t.Fatalf("wrong chain id: %s", r1.ChainID)
	}
}

func TestEthereumStubDifferentInputDifferentHash(t *testing.T) {
	a := NewEthereumStub("eth-sim")
	r1, _ := a.Anchor(AnchorRequest{ProofID: "p1", ProofHash: "deadbeef"})
	r2, _ := a.Anchor(AnchorRequest{ProofID: "p2", ProofHash: "deadbeef"})
	if r1.TxHash == r2.TxHash {
		t.Fatal("different proof ids must yield different pseudo-tx-hashes")
	}
}

func TestEthereumStubRequiresProofHash(t *testing.T) {
	a := NewEthereumStub("eth-sim")
	_, err := a.Anchor(AnchorRequest{ProofID: "p1"})
	if err == nil {
		t.Fatal("empty ProofHash must fail")
	}
}

func TestRouterRegisterAndGet(t *testing.T) {
	r := NewRouter("eth-sim")
	r.Register(NewEthereumStub("eth-sim"))
	r.Register(NewEthereumStub("base-sim"))
	a, err := r.Get("base-sim")
	if err != nil {
		t.Fatal(err)
	}
	if a.ChainID() != "base-sim" {
		t.Fatalf("wrong adapter returned: %s", a.ChainID())
	}
}

func TestRouterDefault(t *testing.T) {
	r := NewRouter("eth-sim")
	r.Register(NewEthereumStub("eth-sim"))
	a, err := r.Get("")
	if err != nil {
		t.Fatal(err)
	}
	if a.ChainID() != "eth-sim" {
		t.Fatalf("default not honored: %s", a.ChainID())
	}
}

func TestRouterMissingAdapter(t *testing.T) {
	r := NewRouter("eth-sim")
	_, err := r.Get("solana-sim")
	if err == nil {
		t.Fatal("missing adapter must fail")
	}
}

func TestRouterAnchorHelper(t *testing.T) {
	r := NewRouter("eth-sim")
	r.Register(NewEthereumStub("eth-sim"))
	rec, err := r.Anchor("eth-sim", AnchorRequest{ProofID: "p", ProofHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.TxHash == "" || rec.Label != LabelSimulation {
		t.Fatalf("bad receipt: %+v", rec)
	}
}
