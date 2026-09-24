package quorum

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// QuorumWitness is the on-the-wire artifact produced by VerifyQuorum. It
// is what the off-chain verifier commits to the chain (path 1 in
// docs/roadmap/BLS_QUORUM.md): the evidence root, the bitset of signers
// that contributed to the aggregate, the aggregate-pubkey hex, and the
// verdict. `Scheme` labels the exact ciphersuite so consumers can refuse
// unknown schemes rather than silently accept new ones.
type QuorumWitness struct {
	Scheme            string    `json:"scheme"`
	EvidenceRootHex   string    `json:"evidence_root_hex"`
	SignerBitset      []string  `json:"signer_bitset"`
	AggregatePubKeyHex string   `json:"aggregate_pubkey_hex"`
	AggregateSigHex   string    `json:"aggregate_sig_hex"`
	Threshold         int       `json:"threshold"`
	ActiveSigners     int       `json:"active_signers"`
	Verdict           string    `json:"verdict"` // "APPROVE" | "REJECT" (constant here: APPROVE on ok)
	VerifiedAt        time.Time `json:"verified_at"`
	WitnessHashHex    string    `json:"witness_hash_hex"`
}

// SchemeBLS12381G1V1 identifies the scheme this package implements.
// A future threshold scheme (BLS-TSS with Shamir over Fr + Lagrange
// interpolation) would ship as SchemeBLS12381TSSv1 and be reserved as a
// separate label so verifiers refuse a mislabelled aggregate.
const (
	SchemeBLS12381G1V1  = "bls12-381-g1-agg-v1"
	SchemeBLS12381TSSv1 = "bls12-381-tss-v1" // reserved, not implemented
)

// VerifyQuorum runs the full quorum check against `evidenceRoot`:
//
//  1. Every id in `signerBitset` must be REGISTERED and ACTIVE.
//  2. Bitset must be duplicate-free.
//  3. len(bitset) >= threshold (⌈2n/3⌉ + 1 by convention, but callers
//     can pass a tighter one for high-stake evidence).
//  4. e(H(evidenceRoot), agg_pk) == e(agg_sig, G2) — the cryptographic
//     check that binds THIS agg_sig to the aggregation of exactly the
//     pubkeys in the bitset.
//
// The witness returned carries a SHA-256 commitment over its own fields
// (`witness_hash_hex`) so the on-chain writer commits a fixed-length
// digest rather than variable-length JSON.
func VerifyQuorum(
	reg *Registry,
	evidenceRoot []byte,
	signerBitset []string,
	aggSig *Signature,
	threshold int,
) (*QuorumWitness, error) {
	if reg == nil {
		return nil, errors.New("quorum/verify: nil registry")
	}
	if len(evidenceRoot) == 0 {
		return nil, ErrEmptyMessage
	}
	if len(signerBitset) == 0 {
		return nil, ErrEmptyBitset
	}
	if aggSig == nil {
		return nil, ErrInvalidSignature
	}
	active := reg.ActiveCount()
	minT := ByzantineThreshold(active)
	if threshold <= 0 {
		threshold = minT
	}
	if threshold < 1 {
		threshold = 1
	}
	if len(signerBitset) < threshold {
		return nil, fmt.Errorf("%w: got %d signers, need %d (active=%d)",
			ErrThresholdNotMet, len(signerBitset), threshold, active)
	}
	// Canonical sort. This makes agg_pk deterministic regardless of the
	// caller's bitset ordering; the returned witness carries the sorted
	// order.
	sorted := make([]string, len(signerBitset))
	copy(sorted, signerBitset)
	SortSignerIDs(sorted)

	pks, err := reg.activePubKeys(sorted)
	if err != nil {
		return nil, err
	}
	aggPK, err := AggregatePubKeys(pks)
	if err != nil {
		return nil, err
	}
	if err := Verify(aggPK, evidenceRoot, aggSig); err != nil {
		return nil, err
	}
	w := &QuorumWitness{
		Scheme:             SchemeBLS12381G1V1,
		EvidenceRootHex:    hex.EncodeToString(evidenceRoot),
		SignerBitset:       sorted,
		AggregatePubKeyHex: aggPK.Hex(),
		AggregateSigHex:    aggSig.Hex(),
		Threshold:          threshold,
		ActiveSigners:      active,
		Verdict:            "APPROVE",
		VerifiedAt:         time.Now().UTC(),
	}
	w.WitnessHashHex = commitmentHash(w)
	return w, nil
}

// commitmentHash returns SHA-256 over a deterministic serialisation of
// the witness fields (excluding WitnessHashHex itself). The on-chain
// writer commits exactly this hex so replaying the same quorum on-chain
// produces the same commitment.
func commitmentHash(w *QuorumWitness) string {
	h := sha256.New()
	h.Write([]byte(w.Scheme))
	h.Write([]byte{0x00})
	h.Write([]byte(w.EvidenceRootHex))
	h.Write([]byte{0x00})
	for _, id := range w.SignerBitset {
		h.Write([]byte(id))
		h.Write([]byte{0x1e}) // record separator
	}
	h.Write([]byte{0x00})
	h.Write([]byte(w.AggregatePubKeyHex))
	h.Write([]byte{0x00})
	h.Write([]byte(w.AggregateSigHex))
	h.Write([]byte{0x00})
	// Threshold + verdict + active-count folded in.
	_, _ = fmt.Fprintf(h, "t=%d;n=%d;v=%s", w.Threshold, w.ActiveSigners, w.Verdict)
	return hex.EncodeToString(h.Sum(nil))
}
