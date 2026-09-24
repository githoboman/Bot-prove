package chainadapter

import (
	"strings"
	"testing"
	"time"
)

// ---- Solana stub --------------------------------------------------------

func TestSolanaStubDeterministic(t *testing.T) {
	a := NewSolanaStub("solana-sim")
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
		t.Fatalf("solana stub not deterministic: %s != %s", r1.TxHash, r2.TxHash)
	}
	if r1.Label != LabelSimulation {
		t.Fatalf("solana stub must be labelled SIMULATION; got %s", r1.Label)
	}
	if r1.ChainID != "solana-sim" {
		t.Fatalf("wrong chain id: %s", r1.ChainID)
	}
}

func TestSolanaStubDifferentInputDifferentHash(t *testing.T) {
	a := NewSolanaStub("solana-sim")
	r1, _ := a.Anchor(AnchorRequest{ProofID: "p1", ProofHash: "deadbeef"})
	r2, _ := a.Anchor(AnchorRequest{ProofID: "p2", ProofHash: "deadbeef"})
	if r1.TxHash == r2.TxHash {
		t.Fatal("different proof ids must yield different pseudo signatures")
	}
}

func TestSolanaStubRequiresProofHash(t *testing.T) {
	a := NewSolanaStub("solana-sim")
	_, err := a.Anchor(AnchorRequest{ProofID: "p1"})
	if err == nil {
		t.Fatal("empty ProofHash must fail")
	}
}

func TestSolanaStubBase58AlphabetOnly(t *testing.T) {
	a := NewSolanaStub("solana-sim")
	r, _ := a.Anchor(AnchorRequest{ProofID: "p", ProofHash: "abcd"})
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	for _, c := range r.TxHash {
		if !strings.ContainsRune(alphabet, c) {
			t.Fatalf("solana stub signature contains non-base58 character %q in %s", c, r.TxHash)
		}
	}
	// Solana signatures are typically 86-88 base58 chars for 64 bytes; sanity-check length.
	if len(r.TxHash) < 80 || len(r.TxHash) > 90 {
		t.Fatalf("solana stub signature has unexpected length %d: %s", len(r.TxHash), r.TxHash)
	}
}

func TestSolanaStubDefaultChainID(t *testing.T) {
	a := NewSolanaStub("")
	if a.ChainID() != "solana-sim" {
		t.Fatalf("empty id should default to solana-sim, got %s", a.ChainID())
	}
}

// ---- Cosmos stub --------------------------------------------------------

func TestCosmosStubDeterministic(t *testing.T) {
	a := NewCosmosStub("cosmos-sim")
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
		t.Fatalf("cosmos stub not deterministic: %s != %s", r1.TxHash, r2.TxHash)
	}
	if r1.Label != LabelSimulation {
		t.Fatalf("cosmos stub must be labelled SIMULATION; got %s", r1.Label)
	}
	if r1.ChainID != "cosmos-sim" {
		t.Fatalf("wrong chain id: %s", r1.ChainID)
	}
}

func TestCosmosStubUppercaseHex(t *testing.T) {
	a := NewCosmosStub("cosmos-sim")
	r, _ := a.Anchor(AnchorRequest{ProofID: "p", ProofHash: "abcd"})
	if len(r.TxHash) != 64 {
		t.Fatalf("cosmos tx hash must be 64 hex chars (SHA-256), got %d: %s", len(r.TxHash), r.TxHash)
	}
	if r.TxHash != strings.ToUpper(r.TxHash) {
		t.Fatalf("cosmos tx hash must be uppercase (Tendermint convention): %s", r.TxHash)
	}
	for _, c := range r.TxHash {
		if (c < '0' || c > '9') && (c < 'A' || c > 'F') {
			t.Fatalf("cosmos tx hash contains non-hex uppercase char %q: %s", c, r.TxHash)
		}
	}
}

func TestCosmosStubRequiresProofHash(t *testing.T) {
	a := NewCosmosStub("cosmos-sim")
	_, err := a.Anchor(AnchorRequest{ProofID: "p1"})
	if err == nil {
		t.Fatal("empty ProofHash must fail")
	}
}

// ---- Cross-adapter isolation --------------------------------------------

func TestStubsProduceDifferentHashesForSameInput(t *testing.T) {
	req := AnchorRequest{ProofID: "p1", ProofHash: "deadbeef", Verdict: "APPROVE"}
	eth, _ := NewEthereumStub("eth-sim").Anchor(req)
	sol, _ := NewSolanaStub("solana-sim").Anchor(req)
	cos, _ := NewCosmosStub("cosmos-sim").Anchor(req)
	// Same input → different tx-hashes because the chain-prefix in the
	// preimage disambiguates. Guards against a caller confusing chains.
	if eth.TxHash == sol.TxHash || eth.TxHash == cos.TxHash || sol.TxHash == cos.TxHash {
		t.Fatalf("stub hashes collide across chains: eth=%s sol=%s cos=%s",
			eth.TxHash, sol.TxHash, cos.TxHash)
	}
	if eth.Label != LabelSimulation || sol.Label != LabelSimulation || cos.Label != LabelSimulation {
		t.Fatal("all three stubs must be labelled SIMULATION")
	}
}

// ---- Router extension ---------------------------------------------------

func TestRouterMultichain(t *testing.T) {
	r := NewRouter("eth-sim")
	r.Register(NewEthereumStub("eth-sim"))
	r.Register(NewSolanaStub("solana-sim"))
	r.Register(NewCosmosStub("cosmos-sim"))

	cases := []struct {
		id    string
		label TrustLabel
	}{
		{"eth-sim", LabelSimulation},
		{"solana-sim", LabelSimulation},
		{"cosmos-sim", LabelSimulation},
	}
	for _, c := range cases {
		a, err := r.Get(c.id)
		if err != nil {
			t.Fatalf("router lost adapter %s: %v", c.id, err)
		}
		if a.Label() != c.label {
			t.Fatalf("%s wrong label: %s", c.id, a.Label())
		}
	}
}

func TestRouterAnchorAcrossChains(t *testing.T) {
	r := NewRouter("eth-sim")
	r.Register(NewEthereumStub("eth-sim"))
	r.Register(NewSolanaStub("solana-sim"))
	r.Register(NewCosmosStub("cosmos-sim"))

	req := AnchorRequest{ProofID: "p", ProofHash: "abcd", Verdict: "APPROVE"}

	for _, id := range []string{"eth-sim", "solana-sim", "cosmos-sim"} {
		rec, err := r.Anchor(id, req)
		if err != nil {
			t.Fatalf("anchor via router %s failed: %v", id, err)
		}
		if rec.ChainID != id {
			t.Fatalf("receipt chain id mismatch: want %s got %s", id, rec.ChainID)
		}
		if rec.TxHash == "" {
			t.Fatalf("empty tx hash for %s", id)
		}
		if rec.Label != LabelSimulation {
			t.Fatalf("non-simulation label for %s: %s", id, rec.Label)
		}
	}
}
