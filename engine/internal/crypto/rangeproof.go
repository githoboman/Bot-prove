package crypto

// Package crypto - range proofs (backlog 2.14).
//
// Honest range proof that x in [0, 2^n) over the edwards25519 prime-order
// subgroup.
//
//   * Commitment:       Pedersen  C(x, r) = x*G + r*H  where H is a
//                       NUMS ("nothing up my sleeve") generator obtained
//                       by hashing a fixed domain string onto the curve.
//   * Bit commitments:  C_i = b_i*G + r_i*H, with
//                          x = sum(b_i * 2^i),
//                          r = sum(r_i * 2^i).
//   * Per-bit proof:    Chaum-Pedersen disjunctive Sigma protocol (Fiat-
//                       Shamir'd) proving C_i opens to 0 OR 1 - real
//                       zero-knowledge proof, not a placeholder.
//   * Homomorphic tie:  verifier recomputes sum(2^i * C_i) and checks
//                       equality with C, so the same x consistent with
//                       the bits must be committed.
//
// This is NOT Bulletproofs: proof size is O(n), not O(log n).  Same
// soundness for the range statement, worse succinctness.  A Bulletproofs
// upgrade is a follow-up (backlog: BP-succinct).
//
// Security (informal):
//   * Completeness: honest x in [0, 2^n) always verifies.
//   * Soundness:    forging accepts requires breaking Schnorr soundness
//                   of the OR-proof or the discrete log of H w.r.t. G.
//   * ZK:           per-bit blindings hide bit values; OR is HVZK Sigma.

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"

	"filippo.io/edwards25519"
)

const rangeProofSuite = "casperprover-rangeproof-v1"

var (
	rangeG *edwards25519.Point // basepoint in prime-order subgroup
	rangeH *edwards25519.Point // independent NUMS generator in prime-order subgroup
)

func init() {
	eight := scalarFromU64(8)
	rangeG = new(edwards25519.Point).ScalarMult(eight, edwards25519.NewGeneratorPoint())

	h, err := hashToCurveInSubgroup(rangeProofSuite + "|H-generator")
	if err != nil {
		panic(fmt.Sprintf("rangeproof: hash-to-curve for H failed: %v", err))
	}
	rangeH = h
}

// RangeProof carries the aggregate commitment C, per-bit commitments C_i,
// and per-bit disjunctive OR-proofs.
type RangeProof struct {
	N    int          // number of bits
	C    [32]byte     // aggregate commitment C = x*G + r*H
	Bits [][32]byte   // per-bit commitments C_i
	ORs  [][160]byte  // per-bit OR-proof (a0 || a1 || e0 || s0 || s1)
}

// CommitRange returns a Pedersen commitment C = x*G + r*H over a freshly
// sampled r.
func CommitRange(x uint64) (C [32]byte, r [32]byte, err error) {
	var rBytes [64]byte
	if _, err = rand.Read(rBytes[:]); err != nil {
		return
	}
	rr, err := new(edwards25519.Scalar).SetUniformBytes(rBytes[:])
	if err != nil {
		return
	}
	xs := scalarFromU64(x)
	xG := new(edwards25519.Point).ScalarMult(xs, rangeG)
	rH := new(edwards25519.Point).ScalarMult(rr, rangeH)
	P := new(edwards25519.Point).Add(xG, rH)
	copy(C[:], P.Bytes())
	copy(r[:], rr.Bytes())
	return C, r, nil
}

// ProveRange proves x in [0, 2^n) and returns a proof plus the aggregate
// commitment C (which the caller can publish separately if needed).
func ProveRange(x uint64, n int) (*RangeProof, error) {
	if n <= 0 || n > 64 {
		return nil, fmt.Errorf("rangeproof: n out of range (got %d, want 1..64)", n)
	}
	if n < 64 && x >= (uint64(1)<<uint(n)) {
		return nil, fmt.Errorf("rangeproof: x=%d out of range [0, 2^%d)", x, n)
	}

	// Sample per-bit blinding r_i; aggregate r = sum(2^i * r_i).
	rs := make([]*edwards25519.Scalar, n)
	for i := 0; i < n; i++ {
		var buf [64]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return nil, err
		}
		ri, err := new(edwards25519.Scalar).SetUniformBytes(buf[:])
		if err != nil {
			return nil, err
		}
		rs[i] = ri
	}
	rAgg := new(edwards25519.Scalar)
	for i := 0; i < n; i++ {
		coef := scalarFromU64(uint64(1) << uint(i))
		term := new(edwards25519.Scalar).Multiply(coef, rs[i])
		rAgg.Add(rAgg, term)
	}

	// Aggregate commitment C = x*G + r*H.
	xs := scalarFromU64(x)
	xG := new(edwards25519.Point).ScalarMult(xs, rangeG)
	rH := new(edwards25519.Point).ScalarMult(rAgg, rangeH)
	C := new(edwards25519.Point).Add(xG, rH)

	// Per-bit commitments and OR-proofs.
	bits := make([][32]byte, n)
	ors := make([][160]byte, n)
	for i := 0; i < n; i++ {
		bi := (x >> uint(i)) & 1
		Ci, orBytes, err := commitBitAndProve(bi, rs[i])
		if err != nil {
			return nil, err
		}
		bits[i] = Ci
		ors[i] = orBytes
	}

	rp := &RangeProof{N: n, Bits: bits, ORs: ors}
	copy(rp.C[:], C.Bytes())
	return rp, nil
}

// VerifyRange checks the proof without needing x or r.
func VerifyRange(rp *RangeProof) error {
	if rp == nil {
		return errors.New("rangeproof: nil proof")
	}
	if rp.N <= 0 || rp.N > 64 {
		return errors.New("rangeproof: n out of range")
	}
	if len(rp.Bits) != rp.N || len(rp.ORs) != rp.N {
		return errors.New("rangeproof: bit/OR length mismatch with n")
	}

	// 1. Every per-bit OR-proof must verify: C_i opens to 0 or 1.
	for i := 0; i < rp.N; i++ {
		if err := verifyBitOR(rp.Bits[i], rp.ORs[i]); err != nil {
			return fmt.Errorf("rangeproof: bit %d OR-proof failed: %w", i, err)
		}
	}

	// 2. Homomorphic check: sum(2^i * C_i) == C.
	acc := edwards25519.NewIdentityPoint()
	for i := 0; i < rp.N; i++ {
		Ci, err := new(edwards25519.Point).SetBytes(rp.Bits[i][:])
		if err != nil {
			return fmt.Errorf("rangeproof: bit %d commitment decode: %w", i, err)
		}
		coef := scalarFromU64(uint64(1) << uint(i))
		term := new(edwards25519.Point).ScalarMult(coef, Ci)
		acc.Add(acc, term)
	}
	C, err := new(edwards25519.Point).SetBytes(rp.C[:])
	if err != nil {
		return fmt.Errorf("rangeproof: aggregate commitment decode: %w", err)
	}
	if acc.Equal(C) != 1 {
		return errors.New("rangeproof: sum(2^i * C_i) != C (homomorphic check failed)")
	}
	return nil
}

// --- OR-proof machinery -----------------------------------------------------
//
// Statement (per bit i): C_i opens to 0 or 1, i.e.
//    exists w:  C_i     = w*H            (branch 0, b_i = 0)
//    OR
//    exists w:  C_i - G = w*H            (branch 1, b_i = 1)
//
// Chaum-Pedersen OR (Fiat-Shamir'd):
//   Prover for branch b picks fresh k, and (s_fake, e_fake) for the other.
//     a_true  = k * H
//     a_fake  = s_fake * H - e_fake * P_fake   where P_fake = statement point
//   Challenge e = H(suite || C_i || a0 || a1).
//   e_true  = e - e_fake
//   s_true  = k + e_true * r
//
//   Encoding: a0 || a1 || e0 || s0 || s1  (5 * 32 = 160 bytes).
//   Verifier: recompute e from (C_i, a0, a1); set e1 = e - e0.
//     check  s0 * H  ==  a0 + e0 * C_i
//     check  s1 * H  ==  a1 + e1 * (C_i - G)

func commitBitAndProve(b uint64, r *edwards25519.Scalar) (Ci [32]byte, or [160]byte, err error) {
	if b != 0 && b != 1 {
		return Ci, or, fmt.Errorf("commitBit: b must be 0 or 1, got %d", b)
	}

	bs := scalarFromU64(b)
	bG := new(edwards25519.Point).ScalarMult(bs, rangeG)
	rH := new(edwards25519.Point).ScalarMult(r, rangeH)
	C := new(edwards25519.Point).Add(bG, rH)
	copy(Ci[:], C.Bytes())

	kTrue, err := randomScalar()
	if err != nil {
		return Ci, or, err
	}
	sFake, err := randomScalar()
	if err != nil {
		return Ci, or, err
	}
	eFake, err := randomScalar()
	if err != nil {
		return Ci, or, err
	}

	var a0, a1 *edwards25519.Point
	if b == 0 {
		// Branch 0 true: a0 = kTrue * H
		a0 = new(edwards25519.Point).ScalarMult(kTrue, rangeH)
		// Branch 1 false: P1 = C - G ; a1 = sFake*H - eFake*P1
		P1 := new(edwards25519.Point).Subtract(C, rangeG)
		sfH := new(edwards25519.Point).ScalarMult(sFake, rangeH)
		efP := new(edwards25519.Point).ScalarMult(eFake, P1)
		a1 = new(edwards25519.Point).Subtract(sfH, efP)
	} else {
		// Branch 1 true: a1 = kTrue * H
		a1 = new(edwards25519.Point).ScalarMult(kTrue, rangeH)
		// Branch 0 false: P0 = C ; a0 = sFake*H - eFake*P0
		sfH := new(edwards25519.Point).ScalarMult(sFake, rangeH)
		efP := new(edwards25519.Point).ScalarMult(eFake, C)
		a0 = new(edwards25519.Point).Subtract(sfH, efP)
	}

	e := fsChallengeOR(C, a0, a1)

	var e0, s0, s1 *edwards25519.Scalar
	if b == 0 {
		e0 = new(edwards25519.Scalar).Subtract(e, eFake)
		e0r := new(edwards25519.Scalar).Multiply(e0, r)
		s0 = new(edwards25519.Scalar).Add(kTrue, e0r)
		s1 = sFake
	} else {
		e0 = eFake
		e1 := new(edwards25519.Scalar).Subtract(e, eFake)
		e1r := new(edwards25519.Scalar).Multiply(e1, r)
		s1 = new(edwards25519.Scalar).Add(kTrue, e1r)
		s0 = sFake
	}

	copy(or[0:32], a0.Bytes())
	copy(or[32:64], a1.Bytes())
	copy(or[64:96], e0.Bytes())
	copy(or[96:128], s0.Bytes())
	copy(or[128:160], s1.Bytes())
	return Ci, or, nil
}

func verifyBitOR(Ci [32]byte, or [160]byte) error {
	C, err := new(edwards25519.Point).SetBytes(Ci[:])
	if err != nil {
		return fmt.Errorf("decode C: %w", err)
	}
	a0, err := new(edwards25519.Point).SetBytes(or[0:32])
	if err != nil {
		return fmt.Errorf("decode a0: %w", err)
	}
	a1, err := new(edwards25519.Point).SetBytes(or[32:64])
	if err != nil {
		return fmt.Errorf("decode a1: %w", err)
	}
	e0, err := new(edwards25519.Scalar).SetCanonicalBytes(or[64:96])
	if err != nil {
		return fmt.Errorf("decode e0: %w", err)
	}
	s0, err := new(edwards25519.Scalar).SetCanonicalBytes(or[96:128])
	if err != nil {
		return fmt.Errorf("decode s0: %w", err)
	}
	s1, err := new(edwards25519.Scalar).SetCanonicalBytes(or[128:160])
	if err != nil {
		return fmt.Errorf("decode s1: %w", err)
	}

	e := fsChallengeOR(C, a0, a1)
	e1 := new(edwards25519.Scalar).Subtract(e, e0)

	// Check branch 0: s0 * H == a0 + e0 * C
	lhs0 := new(edwards25519.Point).ScalarMult(s0, rangeH)
	e0C := new(edwards25519.Point).ScalarMult(e0, C)
	rhs0 := new(edwards25519.Point).Add(a0, e0C)
	if lhs0.Equal(rhs0) != 1 {
		return errors.New("branch-0 Schnorr equation failed")
	}

	// Check branch 1: s1 * H == a1 + e1 * (C - G)
	P1 := new(edwards25519.Point).Subtract(C, rangeG)
	lhs1 := new(edwards25519.Point).ScalarMult(s1, rangeH)
	e1P := new(edwards25519.Point).ScalarMult(e1, P1)
	rhs1 := new(edwards25519.Point).Add(a1, e1P)
	if lhs1.Equal(rhs1) != 1 {
		return errors.New("branch-1 Schnorr equation failed")
	}
	return nil
}

// --- helpers ---------------------------------------------------------------

func hashToCurveInSubgroup(domain string) (*edwards25519.Point, error) {
	buf := make([]byte, 0, len(domain)+4)
	buf = append(buf, []byte(domain)...)
	buf = append(buf, 0, 0, 0, 0)
	pos := len(buf) - 4
	eight := scalarFromU64(8)
	for ctr := uint32(0); ctr < 1_000_000; ctr++ {
		binary.BigEndian.PutUint32(buf[pos:], ctr)
		h := sha512.Sum512(buf)
		P, err := new(edwards25519.Point).SetBytes(h[:32])
		if err == nil {
			Pc := new(edwards25519.Point).ScalarMult(eight, P)
			if Pc.Equal(edwards25519.NewIdentityPoint()) != 1 {
				return Pc, nil
			}
		}
	}
	return nil, errors.New("hashToCurveInSubgroup: exhausted counter")
}

func scalarFromU64(v uint64) *edwards25519.Scalar {
	var b [32]byte
	binary.LittleEndian.PutUint64(b[:8], v)
	s, err := new(edwards25519.Scalar).SetCanonicalBytes(b[:])
	if err != nil {
		panic(fmt.Sprintf("scalarFromU64: SetCanonicalBytes failed: %v", err))
	}
	return s
}

func randomScalar() (*edwards25519.Scalar, error) {
	var buf [64]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return nil, err
	}
	return new(edwards25519.Scalar).SetUniformBytes(buf[:])
}

func fsChallengeOR(C, a0, a1 *edwards25519.Point) *edwards25519.Scalar {
	buf := make([]byte, 0, len(rangeProofSuite)+2+32*3)
	buf = append(buf, []byte(rangeProofSuite)...)
	buf = append(buf, byte('|'), byte('e'))
	buf = append(buf, C.Bytes()...)
	buf = append(buf, a0.Bytes()...)
	buf = append(buf, a1.Bytes()...)
	h := sha512.Sum512(buf)
	s, err := new(edwards25519.Scalar).SetUniformBytes(h[:])
	if err != nil {
		panic("fsChallengeOR: SetUniformBytes failed")
	}
	return s
}
