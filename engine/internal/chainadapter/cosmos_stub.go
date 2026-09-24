// Cosmos anchor stub — SIMULATION only.
//
// Mirrors EthereumStubAdapter and SolanaStubAdapter: deterministic
// pseudo-tx-hash computed from the canonicalized anchor request so
// callers can round-trip the same input and get bit-identical output.
// No network call is performed. Label() always returns SIMULATION.
//
// Format: 64-hex-char SHA-256 digest, uppercased, prefix-less — matches
// how Tendermint / CometBFT surfaces transaction hashes in explorers.
//
// Closes: 5.5 (multi-chain anchoring — Cosmos stub).
package chainadapter

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CosmosStubAdapter is a deterministic Cosmos/Tendermint anchor simulator.
type CosmosStubAdapter struct {
	Chain string // e.g. "cosmos-sim", "cosmoshub-4-sim"
}

// NewCosmosStub constructs a SIMULATION adapter for a Cosmos-like chain.
// Empty id defaults to "cosmos-sim".
func NewCosmosStub(id string) *CosmosStubAdapter {
	if id == "" {
		id = "cosmos-sim"
	}
	return &CosmosStubAdapter{Chain: id}
}

// ChainID returns the router key.
func (a *CosmosStubAdapter) ChainID() string { return a.Chain }

// Label always returns SIMULATION.
func (a *CosmosStubAdapter) Label() TrustLabel { return LabelSimulation }

// Anchor computes a deterministic pseudo-tx-hash so a caller can
// round-trip the same input and get bit-identical output.
func (a *CosmosStubAdapter) Anchor(req AnchorRequest) (AnchorReceipt, error) {
	if req.ProofHash == "" {
		return AnchorReceipt{}, errors.New("cosmos-sim: proof_hash required")
	}
	preimg := fmt.Sprintf("cosmos|%s|%s|%s|%s|%s|%s|%s",
		a.Chain, req.ProofID, req.ProofHash, req.MerkleRoot,
		req.VKHash, req.Verdict, req.ModelID)
	sum := sha256.Sum256([]byte(preimg))
	tx := strings.ToUpper(hex.EncodeToString(sum[:]))
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
