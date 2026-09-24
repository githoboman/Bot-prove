// Package quorum implements a real BLS12-381 threshold quorum for the
// evaluator committee referenced by docs/roadmap/BLS_QUORUM.md.
//
// This is NOT a stand-in like nova hash-fold. It is a real, cryptographic
// BLS aggregate-signature scheme built on top of gnark-crypto's
// BLS12-381 curve (RFC 9380 hash-to-curve, Sec. 8.8.1 for the "minimum
// signature size" variant):
//
//     sk ∈ Fr,        pk = sk * G2   (public key lives on G2)
//     sig = sk * H(m)                 (signature lives on G1)
//     verify: e(H(m), pk) == e(sig, G2_generator)
//
// Aggregate:
//     agg_pk  = sum_i pk_i          (in G2)
//     agg_sig = sum_i sig_i         (in G1)
//     verify:  e(H(m), agg_pk) == e(agg_sig, G2_generator)
//
// The "threshold" here is a plain k-of-n quorum on top of aggregate
// signatures: verifiers reconstruct agg_pk only from the signer bitset
// they trust (registered + not-slashed), then check the pairing. This
// matches path (1) in docs/roadmap/BLS_QUORUM.md — off-chain pairing
// check + on-chain commitment. It is honest about not being a threshold
// signature scheme in the (t,n) secret-sharing sense (that would be
// Shamir over Fr + Lagrange interpolation — future work).
//
// Domain-separation tag follows the "ciphersuite ID" convention from
// RFC 9380 §8.8.1: BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_NUL_ with a
// project-specific 6-byte tail so casperprover signatures can never be
// confused with, say, Ethereum's BLS12-381 signatures on the same key.
package quorum

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

// DomainSeparationTag is the RFC 9380 ciphersuite ID used for every
// signature this package produces. Tail "_CP_QUORUM_2026" is
// project-specific so BLS12-381 keys used elsewhere cannot cross-sign
// against a casperprover quorum evidence root by accident.
var DomainSeparationTag = []byte("BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_NUL_CP_QUORUM_2026")

// Errors surfaced to callers.
var (
	ErrEmptyMessage      = errors.New("quorum/bls: empty message")
	ErrEmptyBitset       = errors.New("quorum/bls: empty signer bitset")
	ErrEmptySignatures   = errors.New("quorum/bls: no signatures to aggregate")
	ErrThresholdNotMet   = errors.New("quorum/bls: signer count below threshold")
	ErrDuplicateSigner   = errors.New("quorum/bls: duplicate signer id in bitset")
	ErrUnknownSigner     = errors.New("quorum/bls: signer id not in registry")
	ErrPairingCheckFail  = errors.New("quorum/bls: pairing check failed — signature invalid")
	ErrInactiveSigner    = errors.New("quorum/bls: signer is slashed or not active")
	ErrInvalidPubKey     = errors.New("quorum/bls: invalid pubkey encoding")
	ErrInvalidSignature  = errors.New("quorum/bls: invalid signature encoding")
	ErrEmptyPubKey       = errors.New("quorum/bls: empty pubkey")
)

// SecretKey is a BLS12-381 secret scalar in Fr. It is opaque; callers
// never marshal it — the file-backed keystore is the store of record.
type SecretKey struct {
	scalar fr.Element
}

// PublicKey is a point on G2 (compressed on the wire).
type PublicKey struct {
	Point bls12381.G2Affine
}

// Signature is a point on G1 (compressed on the wire).
type Signature struct {
	Point bls12381.G1Affine
}

// GenerateSecretKey samples a fresh sk uniformly from Fr using
// crypto/rand. Return value: sk, pk, error.
func GenerateSecretKey() (*SecretKey, *PublicKey, error) {
	var sk fr.Element
	if _, err := sk.SetRandom(); err != nil {
		return nil, nil, fmt.Errorf("quorum/bls: SetRandom: %w", err)
	}
	// Rejection: sk == 0 breaks the scheme; SetRandom effectively never
	// returns zero (space size 2^-255) but guard anyway.
	if sk.IsZero() {
		// Retry once from crypto/rand deterministically.
		var buf [32]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return nil, nil, err
		}
		sk.SetBytes(buf[:])
		if sk.IsZero() {
			return nil, nil, errors.New("quorum/bls: sampled zero secret twice — abort")
		}
	}
	// pk = sk * G2_generator
	var pk bls12381.G2Affine
	pk.ScalarMultiplicationBase(sk.BigInt(new(big.Int)))
	return &SecretKey{scalar: sk}, &PublicKey{Point: pk}, nil
}

// PublicKey derives the pubkey from a secret. Convenient for tests and
// for reconstructing the pubkey after loading sk from the keystore.
func (sk *SecretKey) PublicKey() *PublicKey {
	var pk bls12381.G2Affine
	pk.ScalarMultiplicationBase(sk.scalar.BigInt(new(big.Int)))
	return &PublicKey{Point: pk}
}

// Sign hashes msg to G1 using the RFC 9380 SSWU map with our DST and
// multiplies by sk. Returns a signature on G1.
func (sk *SecretKey) Sign(msg []byte) (*Signature, error) {
	if len(msg) == 0 {
		return nil, ErrEmptyMessage
	}
	h, err := bls12381.HashToG1(msg, DomainSeparationTag)
	if err != nil {
		return nil, fmt.Errorf("quorum/bls: HashToG1: %w", err)
	}
	var sig bls12381.G1Affine
	sig.ScalarMultiplication(&h, sk.scalar.BigInt(new(big.Int)))
	return &Signature{Point: sig}, nil
}

// Verify checks a single-signer BLS signature over msg against pk.
// Returns nil on success, ErrPairingCheckFail on cryptographic failure.
func Verify(pk *PublicKey, msg []byte, sig *Signature) error {
	if pk == nil {
		return ErrEmptyPubKey
	}
	if len(msg) == 0 {
		return ErrEmptyMessage
	}
	h, err := bls12381.HashToG1(msg, DomainSeparationTag)
	if err != nil {
		return fmt.Errorf("quorum/bls: HashToG1: %w", err)
	}
	// PairingCheck computes prod e(P_i, Q_i) == 1_GT. We want
	//   e(H(m), pk) == e(sig, G2)
	// which is equivalent to
	//   e(H(m), pk) * e(-sig, G2) == 1_GT
	// Negate H(m) instead of sig (equivalent, saves one G1 op vs many).
	var negH bls12381.G1Affine
	negH.Neg(&h)
	_, _, _, g2Aff := bls12381.Generators()
	ok, err := bls12381.PairingCheck(
		[]bls12381.G1Affine{negH, sig.Point},
		[]bls12381.G2Affine{pk.Point, g2Aff},
	)
	if err != nil {
		return fmt.Errorf("quorum/bls: pairing: %w", err)
	}
	if !ok {
		return ErrPairingCheckFail
	}
	return nil
}

// Aggregate sums signatures on G1. Duplicate signatures are the
// caller's problem to prevent — the scheme does not detect them.
// Returns ErrEmptySignatures on empty input.
func Aggregate(sigs []*Signature) (*Signature, error) {
	if len(sigs) == 0 {
		return nil, ErrEmptySignatures
	}
	var accJac bls12381.G1Jac
	// Start from identity (implicit — G1Jac zero value is identity).
	for i, s := range sigs {
		if s == nil {
			return nil, fmt.Errorf("quorum/bls: nil signature at index %d", i)
		}
		accJac.AddMixed(&s.Point)
	}
	var out bls12381.G1Affine
	out.FromJacobian(&accJac)
	return &Signature{Point: out}, nil
}

// AggregatePubKeys sums pubkeys on G2. Used by VerifyQuorum to derive
// agg_pk from the signer bitset ∩ active registry.
func AggregatePubKeys(pks []*PublicKey) (*PublicKey, error) {
	if len(pks) == 0 {
		return nil, ErrEmptyPubKey
	}
	var accJac bls12381.G2Jac
	for i, p := range pks {
		if p == nil {
			return nil, fmt.Errorf("quorum/bls: nil pubkey at index %d", i)
		}
		accJac.AddMixed(&p.Point)
	}
	var out bls12381.G2Affine
	out.FromJacobian(&accJac)
	return &PublicKey{Point: out}, nil
}

// ---------------------------------------------------------------------------
// Wire encoding — compressed points.
// ---------------------------------------------------------------------------

// MarshalPubKey returns the 96-byte compressed G2 encoding.
func (pk *PublicKey) MarshalBinary() ([]byte, error) {
	b := pk.Point.Bytes() // [96]byte
	return append([]byte{}, b[:]...), nil
}

// UnmarshalPubKey parses a 96-byte compressed G2 encoding.
func UnmarshalPubKey(b []byte) (*PublicKey, error) {
	if len(b) == 0 {
		return nil, ErrEmptyPubKey
	}
	var p bls12381.G2Affine
	if _, err := p.SetBytes(b); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPubKey, err)
	}
	return &PublicKey{Point: p}, nil
}

// MarshalSignature returns the 48-byte compressed G1 encoding.
func (s *Signature) MarshalBinary() ([]byte, error) {
	b := s.Point.Bytes() // [48]byte
	return append([]byte{}, b[:]...), nil
}

// UnmarshalSignature parses a 48-byte compressed G1 encoding.
func UnmarshalSignature(b []byte) (*Signature, error) {
	if len(b) == 0 {
		return nil, ErrInvalidSignature
	}
	var p bls12381.G1Affine
	if _, err := p.SetBytes(b); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}
	return &Signature{Point: p}, nil
}

// Hex convenience — used by the HTTP surface.
func (pk *PublicKey) Hex() string { b, _ := pk.MarshalBinary(); return hex.EncodeToString(b) }
func (s *Signature) Hex() string  { b, _ := s.MarshalBinary(); return hex.EncodeToString(b) }

// PubKeyFromHex decodes a hex-encoded compressed G2 pubkey. Empty
// string or bad hex returns a wrapped ErrEmptyPubKey / ErrInvalidPubKey.
func PubKeyFromHex(s string) (*PublicKey, error) {
	if s == "" {
		return nil, ErrEmptyPubKey
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPubKey, err)
	}
	return UnmarshalPubKey(b)
}

// SigFromHex decodes a hex-encoded compressed G1 signature.
func SigFromHex(s string) (*Signature, error) {
	if s == "" {
		return nil, ErrInvalidSignature
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}
	return UnmarshalSignature(b)
}

// ---------------------------------------------------------------------------
// Threshold helper.
// ---------------------------------------------------------------------------

// ByzantineThreshold returns ⌊2n/3⌋ + 1 clamped to [1, n] — the minimum
// signer count for classical BFT quorum (tolerates f = ⌊(n-1)/3⌋
// malicious signers when n ≥ 3f+1). n=0 returns 0 (undefined).
// n=1..3 collapses to n (the tiny-committee edge, no BFT guarantee
// possible — documented in BLS_QUORUM.md).
//
// Table (n → t): 1→1, 2→2, 3→3, 4→3, 5→4, 6→5, 7→5, 10→7, 100→67.
func ByzantineThreshold(n int) int {
	if n <= 0 {
		return 0
	}
	t := 2*n/3 + 1
	if t > n {
		return n
	}
	return t
}

// SortSignerIDs sorts a signer bitset to canonical order — the wire
// order MUST be sorted-ascending so canonical hashing over quorum
// witnesses is stable across peers. Sort is stable + in-place.
func SortSignerIDs(ids []string) {
	sort.Strings(ids)
}
