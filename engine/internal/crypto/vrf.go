package crypto

// Package crypto - VRF (Verifiable Random Function) implementation.
//
// This file implements a real ECVRF construction on top of the Ed25519 /
// edwards25519 group (RFC 9381 style, "TAI" try-and-increment variant).
// It is REAL cryptography: sign-then-verify actually works, tampering with
// message, proof, or key breaks Verify. It is NOT a bit-identical RFC 9381
// implementation - the suite string, hash-to-curve style, and cofactor
// clearing steps are simplified for compactness and pod-side auditability.
// Do NOT use this VRF to interop with an external ECVRF implementation
// (Algorand, Chainlink, IETF test vectors); use it for our internal
// sortition and challenge-selection where both sides run this same code.
//
// Scheme (informal):
//   Keygen: sk in Z_q, pk = sk * B (B = ed25519 basepoint)
//   Prove(sk, alpha):
//     H = HashToCurve(pk, alpha)     # RFC 9381 TAI
//     Gamma = sk * H
//     k = HashToScalar(sk_seed || H) # deterministic nonce a la RFC 6979
//     c = HashPoints(H, Gamma, k*B, k*H)
//     s = k + c*sk mod q
//     proof = (Gamma || c || s)
//     beta  = Hash("beta" || Gamma)  # VRF output
//   Verify(pk, alpha, proof):
//     H  = HashToCurve(pk, alpha)
//     U  = s*B - c*pk
//     V  = s*H - c*Gamma
//     c' = HashPoints(H, Gamma, U, V)
//     accept iff c == c'; recompute beta from Gamma
//
// The construction is a straight Schnorr-in-the-exponent proof over the
// edwards25519 group with a hash-to-curve step - i.e. the same idea as
// RFC 9381 §5 but not byte-identical to it.
//
// Sizes:
//   VRF sk seed:       32 bytes (ed25519 seed)
//   VRF pk:            32 bytes (compressed edwards25519 point)
//   VRF proof:         32 + 32 + 32 = 96 bytes (Gamma || c || s)
//   VRF output beta:   64 bytes (SHA-512 output)

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"errors"

	"filippo.io/edwards25519"
)

const (
	VRFPKSize      = 32
	VRFSKSeedSize  = ed25519.SeedSize // 32
	VRFProofSize   = 96
	VRFOutputSize  = 64
	vrfSuiteString = "casperprover-vrf-v1"
)

var (
	ErrVRFInvalidPK    = errors.New("vrf: invalid public key")
	ErrVRFInvalidProof = errors.New("vrf: invalid proof")
	ErrVRFVerifyFail   = errors.New("vrf: proof does not verify against pk and alpha")
)

// VRFKeypair generates a fresh keypair. The seed is 32 bytes (as in Ed25519)
// and the public key is a compressed edwards25519 point.
func VRFKeypair() (seed [VRFSKSeedSize]byte, pk [VRFPKSize]byte, err error) {
	if _, err = rand.Read(seed[:]); err != nil {
		return
	}
	pk, err = vrfDerivePK(seed)
	return
}

// VRFDerivePK derives the public key from a 32-byte seed. Exposed so callers
// can persist only the seed and rederive the pk on demand.
func VRFDerivePK(seed [VRFSKSeedSize]byte) ([VRFPKSize]byte, error) {
	return vrfDerivePK(seed)
}

func vrfDerivePK(seed [VRFSKSeedSize]byte) ([VRFPKSize]byte, error) {
	scalar := vrfSeedToScalar(seed)
	B := edwards25519.NewGeneratorPoint()
	P := new(edwards25519.Point).ScalarMult(scalar, B)
	var pk [VRFPKSize]byte
	copy(pk[:], P.Bytes())
	return pk, nil
}

// vrfSeedToScalar clamps the SHA-512 of the seed into a valid scalar,
// the same key-derivation Ed25519 uses.
func vrfSeedToScalar(seed [VRFSKSeedSize]byte) *edwards25519.Scalar {
	h := sha512.Sum512(seed[:])
	// Standard Ed25519 clamping.
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64
	s, err := edwards25519.NewScalar().SetBytesWithClamping(h[:32])
	if err != nil {
		// SetBytesWithClamping accepts any 32 bytes, so this cannot fail.
		panic("vrf: unreachable scalar decode")
	}
	return s
}

// VRFProve returns (proof, beta) for message alpha under seed.
func VRFProve(seed [VRFSKSeedSize]byte, alpha []byte) (proof [VRFProofSize]byte, beta [VRFOutputSize]byte, err error) {
	sk := vrfSeedToScalar(seed)
	pk, err := vrfDerivePK(seed)
	if err != nil {
		return proof, beta, err
	}

	// H = HashToCurve(pk, alpha)
	H, err := vrfHashToCurve(pk, alpha)
	if err != nil {
		return proof, beta, err
	}

	// Gamma = sk * H
	Gamma := new(edwards25519.Point).ScalarMult(sk, H)

	// k = HashToScalar(seed || H || alpha) - deterministic nonce
	kSeedBuf := make([]byte, 0, 32+32+len(alpha)+len(vrfSuiteString))
	kSeedBuf = append(kSeedBuf, []byte(vrfSuiteString)...)
	kSeedBuf = append(kSeedBuf, seed[:]...)
	kSeedBuf = append(kSeedBuf, H.Bytes()...)
	kSeedBuf = append(kSeedBuf, alpha...)
	k := vrfHashToScalar(kSeedBuf)

	B := edwards25519.NewGeneratorPoint()
	kB := new(edwards25519.Point).ScalarMult(k, B)
	kH := new(edwards25519.Point).ScalarMult(k, H)

	// c = HashPoints(H, Gamma, k*B, k*H) reduced to a scalar
	c := vrfHashPoints(H, Gamma, kB, kH)

	// s = k + c*sk mod q
	cSk := new(edwards25519.Scalar).Multiply(c, sk)
	s := new(edwards25519.Scalar).Add(k, cSk)

	// proof = Gamma || c || s   (each 32 bytes)
	copy(proof[0:32], Gamma.Bytes())
	copy(proof[32:64], c.Bytes())
	copy(proof[64:96], s.Bytes())

	// beta = SHA-512("beta" || suite || Gamma)
	beta = vrfProofToOutput(Gamma)
	return proof, beta, nil
}

// VRFVerify checks that proof was produced by the holder of the secret key
// matching pk, against alpha, and returns the derived beta.
func VRFVerify(pk [VRFPKSize]byte, alpha []byte, proof [VRFProofSize]byte) (beta [VRFOutputSize]byte, err error) {
	// Decode components.
	Gamma, err := new(edwards25519.Point).SetBytes(proof[0:32])
	if err != nil {
		return beta, ErrVRFInvalidProof
	}
	c, err := new(edwards25519.Scalar).SetCanonicalBytes(proof[32:64])
	if err != nil {
		return beta, ErrVRFInvalidProof
	}
	s, err := new(edwards25519.Scalar).SetCanonicalBytes(proof[64:96])
	if err != nil {
		return beta, ErrVRFInvalidProof
	}
	PK, err := new(edwards25519.Point).SetBytes(pk[:])
	if err != nil {
		return beta, ErrVRFInvalidPK
	}

	H, err := vrfHashToCurve(pk, alpha)
	if err != nil {
		return beta, err
	}

	B := edwards25519.NewGeneratorPoint()
	// U = s*B - c*pk
	sB := new(edwards25519.Point).ScalarMult(s, B)
	cPK := new(edwards25519.Point).ScalarMult(c, PK)
	U := new(edwards25519.Point).Subtract(sB, cPK)

	// V = s*H - c*Gamma
	sH := new(edwards25519.Point).ScalarMult(s, H)
	cG := new(edwards25519.Point).ScalarMult(c, Gamma)
	V := new(edwards25519.Point).Subtract(sH, cG)

	cCheck := vrfHashPoints(H, Gamma, U, V)
	if cCheck.Equal(c) != 1 {
		return beta, ErrVRFVerifyFail
	}

	beta = vrfProofToOutput(Gamma)
	return beta, nil
}

// vrfHashToCurve implements a RFC 9381-style try-and-increment hash-to-curve
// over edwards25519. We hash (suite || pk || alpha || ctr) with SHA-512,
// take the first 32 bytes as a candidate compressed point, and increment
// ctr until we hit a valid on-curve point. Expected iterations: <2.
func vrfHashToCurve(pk [VRFPKSize]byte, alpha []byte) (*edwards25519.Point, error) {
	buf := make([]byte, 0, len(vrfSuiteString)+VRFPKSize+len(alpha)+4)
	buf = append(buf, []byte(vrfSuiteString)...)
	buf = append(buf, pk[:]...)
	buf = append(buf, alpha...)
	ctrPos := len(buf)
	buf = append(buf, 0, 0, 0, 0) // 4-byte counter suffix
	for ctr := uint32(0); ctr < 1_000_000; ctr++ {
		binary.BigEndian.PutUint32(buf[ctrPos:], ctr)
		h := sha512.Sum512(buf)
		P, err := new(edwards25519.Point).SetBytes(h[:32])
		if err == nil {
			// Multiply by 8 to clear cofactor.
			eight, _ := new(edwards25519.Scalar).SetCanonicalBytes([]byte{
				8, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
			})
			Pc := new(edwards25519.Point).ScalarMult(eight, P)
			// Guard against the identity - shouldn't happen for random hash,
			// but reject it explicitly.
			if Pc.Equal(edwards25519.NewIdentityPoint()) == 1 {
				continue
			}
			return Pc, nil
		}
	}
	return nil, errors.New("vrf: hash-to-curve failed (should be impossible)")
}

// vrfHashToScalar hashes arbitrary bytes into a scalar via SHA-512 mod q.
func vrfHashToScalar(data []byte) *edwards25519.Scalar {
	h := sha512.Sum512(data)
	s, err := new(edwards25519.Scalar).SetUniformBytes(h[:])
	if err != nil {
		panic("vrf: SetUniformBytes cannot fail on 64 bytes")
	}
	return s
}

// vrfHashPoints hashes four points into a scalar (the Schnorr challenge).
func vrfHashPoints(a, b, c, d *edwards25519.Point) *edwards25519.Scalar {
	buf := make([]byte, 0, 4+32*4+len(vrfSuiteString))
	buf = append(buf, []byte(vrfSuiteString)...)
	buf = append(buf, byte('c'))
	buf = append(buf, a.Bytes()...)
	buf = append(buf, b.Bytes()...)
	buf = append(buf, c.Bytes()...)
	buf = append(buf, d.Bytes()...)
	return vrfHashToScalar(buf)
}

// vrfProofToOutput derives the pseudorandom VRF output beta from Gamma.
func vrfProofToOutput(Gamma *edwards25519.Point) [VRFOutputSize]byte {
	buf := make([]byte, 0, 8+32+len(vrfSuiteString))
	buf = append(buf, []byte(vrfSuiteString)...)
	buf = append(buf, byte('b'), byte('e'), byte('t'), byte('a'))
	// Clear cofactor of Gamma before hashing (RFC 9381).
	eight, _ := new(edwards25519.Scalar).SetCanonicalBytes([]byte{
		8, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
	})
	cleared := new(edwards25519.Point).ScalarMult(eight, Gamma)
	buf = append(buf, cleared.Bytes()...)
	return sha512.Sum512(buf)
}
