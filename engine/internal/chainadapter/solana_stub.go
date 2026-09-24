// Solana anchor stub — SIMULATION only.
//
// Mirrors EthereumStubAdapter: deterministic pseudo-signature computed
// from the canonicalized anchor request so callers can round-trip the
// same input and get bit-identical output. No network call is performed.
//
// The signature format apes Solana's 64-byte base58 tx signatures. It
// is prefixed nowhere — Label() always returns SIMULATION so downstream
// consumers can't mistake this for a live Solana anchor.
//
// Closes: 5.5 (multi-chain anchoring — Solana stub).
package chainadapter

import (
	"crypto/sha512"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// SolanaStubAdapter is a deterministic Solana anchor simulator.
type SolanaStubAdapter struct {
	Chain string // e.g. "solana-sim" or "solana-devnet-sim"
}

// NewSolanaStub constructs a SIMULATION adapter for a Solana-like chain.
// Empty id defaults to "solana-sim".
func NewSolanaStub(id string) *SolanaStubAdapter {
	if id == "" {
		id = "solana-sim"
	}
	return &SolanaStubAdapter{Chain: id}
}

// ChainID returns the router key.
func (a *SolanaStubAdapter) ChainID() string { return a.Chain }

// Label always returns SIMULATION.
func (a *SolanaStubAdapter) Label() TrustLabel { return LabelSimulation }

// Anchor computes a deterministic pseudo signature — SHA-512 of the
// canonicalized request fields, base58-encoded — so a caller can
// round-trip the same input and get bit-identical output.
func (a *SolanaStubAdapter) Anchor(req AnchorRequest) (AnchorReceipt, error) {
	if req.ProofHash == "" {
		return AnchorReceipt{}, errors.New("solana-sim: proof_hash required")
	}
	preimg := fmt.Sprintf("solana|%s|%s|%s|%s|%s|%s|%s",
		a.Chain, req.ProofID, req.ProofHash, req.MerkleRoot,
		req.VKHash, req.Verdict, req.ModelID)
	// SHA-512 → 64 bytes matches Solana's Ed25519 signature width.
	sum := sha512.Sum512([]byte(preimg))
	sig := base58EncodeBytes(sum[:])
	at := req.Timestamp
	if at.IsZero() {
		at = time.Unix(0, 0)
	}
	return AnchorReceipt{
		ChainID:    a.Chain,
		TxHash:     sig,
		AnchoredAt: at,
		Label:      LabelSimulation,
	}, nil
}

// base58EncodeBytes is a minimal RFC-agnostic base58 (Bitcoin alphabet)
// implementation. Kept package-private so this stub owns its encoding
// and does not pull in a full base58 dependency for a simulator.
func base58EncodeBytes(b []byte) string {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	// Convert byte slice to big.Int, encode in base 58, then re-attach
	// leading-zero-byte counts as '1' prefix per Bitcoin base58 convention.
	x := new(big.Int).SetBytes(b)
	base := big.NewInt(58)
	mod := new(big.Int)
	var out []byte
	for x.Sign() > 0 {
		x.DivMod(x, base, mod)
		out = append(out, alphabet[mod.Int64()])
	}
	// Leading zero bytes → '1' characters.
	for i := 0; i < len(b) && b[i] == 0; i++ {
		out = append(out, alphabet[0])
	}
	// Reverse in place.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

// hexOfSHA512 was intended for callers with strict hex expectations (e.g.
// contract-tests round-tripping via Casper hex) to compare the underlying
// digest without depending on the base58 encoding -- but it's unexported
// and nothing ever called it, so it was always dead code. Removed; re-add
// if a real caller needs it.
