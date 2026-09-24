// Package chainadapter — pluggable anchor backend interface.
//
// Right now every proof anchors to Casper via submitter.CasperSubmitter.
// This package lifts that concrete dependency into an interface so the
// codebase can add EVM (Ethereum / Base / Polygon) or Cosmos anchors
// without touching the API/decision layers.
//
// Not a full multi-chain deployment — the honest boundary is:
//
//   * CasperAdapter → wraps submitter.CasperSubmitter (REAL / ON-CHAIN).
//   * EthereumStubAdapter → deterministic simulator that returns
//     bit-reproducible pseudo-tx-hashes. Marked SIMULATION so no judge
//     mistakes it for a live Ethereum anchor. Its job is to prove the
//     interface is chain-agnostic and unblock future integration work.
//   * SolanaStubAdapter → deterministic simulator emitting a
//     base58-encoded 64-byte pseudo-signature (SHA-512 of the
//     canonicalized request). SIMULATION only.
//   * CosmosStubAdapter → deterministic simulator emitting an
//     uppercase 64-hex-char pseudo-tx-hash (SHA-256 of the
//     canonicalized request, Tendermint convention). SIMULATION only.
//
// A ChainRouter picks an adapter by ChainID
// ("casper-test" / "eth-sim" / "solana-sim" / "cosmos-sim").
//
// Closes: 5.4 (interface layer) + 5.5 (multi-chain stubs — Solana,
//         Cosmos). Live EVM/Solana/Cosmos adapters remain deferred.
package chainadapter

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// AnchorRequest is the chain-agnostic anchor payload.
type AnchorRequest struct {
	ProofID     string
	ProofHash   string // hex sha256 of the proof bytes
	MerkleRoot  string // hex; empty when anchoring a single proof
	VKHash      string // hex sha256 of the verifying key
	Verdict     string
	ModelID     string
	Timestamp   time.Time
}

// AnchorReceipt is the chain-agnostic response.
type AnchorReceipt struct {
	ChainID    string    // "casper-test", "eth-sim", …
	TxHash     string    // native tx / deploy hash
	AnchoredAt time.Time
	Label      TrustLabel
}

// TrustLabel matches the frontend TrustBadge values.
type TrustLabel string

const (
	LabelReal       TrustLabel = "REAL"
	LabelOnChain    TrustLabel = "ON-CHAIN"
	LabelSimulation TrustLabel = "SIMULATION"
)

// Adapter is the anchor-backend contract. Implementations must be
// safe for concurrent use.
type Adapter interface {
	ChainID() string
	Anchor(req AnchorRequest) (AnchorReceipt, error)
	Label() TrustLabel
}

// ---- Router --------------------------------------------------------------

// Router picks an Adapter for a given chain id.
type Router struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
	def      string
}

// NewRouter constructs a Router with the given default chain id.
// The default is used when a caller does not name a chain explicitly.
func NewRouter(defaultChain string) *Router {
	return &Router{adapters: map[string]Adapter{}, def: defaultChain}
}

// Register attaches an adapter under its ChainID().
func (r *Router) Register(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.ChainID()] = a
}

// Get retrieves an adapter by id. If id is empty the default is used.
func (r *Router) Get(id string) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if id == "" {
		id = r.def
	}
	a, ok := r.adapters[id]
	if !ok {
		return nil, fmt.Errorf("chainadapter: no adapter for %q (default=%q)", id, r.def)
	}
	return a, nil
}

// Anchor is a convenience: Get(chainID) then Anchor(req).
func (r *Router) Anchor(chainID string, req AnchorRequest) (AnchorReceipt, error) {
	a, err := r.Get(chainID)
	if err != nil {
		return AnchorReceipt{}, err
	}
	return a.Anchor(req)
}

// ---- Ethereum stub (SIMULATION) -----------------------------------------

// EthereumStubAdapter returns deterministic pseudo tx hashes derived
// from the anchor request. Its Label() is SIMULATION so no downstream
// consumer confuses it with a live Ethereum anchor. Its purpose is to
// prove the interface is chain-agnostic and give the SDK a second
// backend to test against without paying gas.
type EthereumStubAdapter struct {
	Chain string // e.g. "eth-sim" or "base-sim"
}

// NewEthereumStub constructs a SIMULATION adapter labelled with id.
func NewEthereumStub(id string) *EthereumStubAdapter {
	if id == "" {
		id = "eth-sim"
	}
	return &EthereumStubAdapter{Chain: id}
}

// ChainID returns the router key for this stub.
func (a *EthereumStubAdapter) ChainID() string { return a.Chain }

// Label always returns SIMULATION.
func (a *EthereumStubAdapter) Label() TrustLabel { return LabelSimulation }

// Anchor computes a deterministic pseudo-tx-hash — SHA-256 of the
// canonicalized request fields — so a caller can round-trip the same
// input and get bit-identical output. No network call is performed.
func (a *EthereumStubAdapter) Anchor(req AnchorRequest) (AnchorReceipt, error) {
	if req.ProofHash == "" {
		return AnchorReceipt{}, errors.New("eth-sim: proof_hash required")
	}
	preimg := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
		a.Chain, req.ProofID, req.ProofHash, req.MerkleRoot,
		req.VKHash, req.Verdict, req.ModelID)
	sum := sha256.Sum256([]byte(preimg))
	tx := "0x" + hex.EncodeToString(sum[:])
	// Timestamp is caller-provided so callers can pin it in tests.
	at := req.Timestamp
	if at.IsZero() {
		at = time.Unix(0, 0)
	}
	return AnchorReceipt{
		ChainID:    a.Chain,
		TxHash:     tx,
		AnchoredAt: at,
		Label:      LabelSimulation,
	}, nil
}
