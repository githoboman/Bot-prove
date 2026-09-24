package aggregator

// Pedersen commitment folding on BLS12-381 G1.
//
// This is an intermediate cryptographic upgrade of the earlier
// `hash-fold-v1` accumulator. It is genuinely cryptographic — under
// the Discrete Logarithm assumption on BLS12-381 G1 the accumulator is
// hiding (given r) and computationally binding — but it is NOT a
// folding scheme in the Nova sense: it does not reduce k R1CS
// instances into one R1CS instance whose satisfiability implies the
// originals. It exists to give callers a real elliptic-curve
// commitment today, while the ecosystem's Go Nova (Pallas/Vesta
// cycle) matures.
//
// Scheme label: "pedersen-fold-v1".
// See docs/PEDERSEN_FOLD.md for the honest contract.
//
// Construction
// ------------
// Two independent generators G, H in G1 are derived at package init
// from hash-to-curve using disjoint domain-separation tags. G is the
// canonical G1 generator; H is drawn by hashing the constant
// "CP_PED_H_V1" to a curve point (SSWU + isogeny), so the discrete
// log of H w.r.t. G is unknown to anyone.
//
// For each step i, we deterministically derive two scalars from the
// caller's opaque bytes:
//
//     m_i = H2Fr("CP_PED_M_V1" || instance_i)
//     r_i = H2Fr("CP_PED_R_V1" || witness_digest_i)
//
// and add `m_i·G + r_i·H` to the running accumulator C. The final
// accumulator C is emitted as compressed hex.
//
// Verifying re-derives every scalar from the same (public) bytes and
// re-runs the sum. The equality check on curve points is a real
// cryptographic check under DLP.
//
// Homomorphism
// ------------
// This is Pedersen's commitment scheme with two generators, so it is
// homomorphic:
//
//     Commit(m_1 + m_2, r_1 + r_2) = Commit(m_1, r_1) + Commit(m_2, r_2)
//
// An aggregator can therefore compose partial folds without opening
// each step — which is the practical advantage over hash-fold-v1
// (where the whole sequence must be re-played linearly by any
// verifier).

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

// SchemePedersenFoldV1 is the scheme label reported in AggregateProof.
const SchemePedersenFoldV1 FoldingScheme = "pedersen-fold-v1"

// pedersen_G and pedersen_H are the two independent generators for the
// Pedersen commitment. pedersen_G is the canonical G1 generator;
// pedersen_H is hash-to-curve of "CP_PED_H_V1" so nobody knows its
// discrete log wrt G.
var (
	pedersenG bls.G1Affine
	pedersenH bls.G1Affine
)

func init() {
	_, _, g, _ := bls.Generators()
	pedersenG = g
	pt, err := bls.HashToG1([]byte("CP_PED_H_V1_generator"), []byte("CP_PED_H_V1"))
	if err != nil {
		panic(fmt.Sprintf("aggregator/pedersen: hash-to-curve for H failed: %v", err))
	}
	pedersenH = pt
}

// hashToFr derives a canonical Fr scalar from arbitrary bytes with a
// domain-separation tag. SHA-256 → reduce mod r. The reduction bias is
// negligible for a 256-bit hash into a ~381-bit field.
func hashToFr(dst string, data []byte) fr.Element {
	h := sha256.New()
	h.Write([]byte(dst))
	h.Write([]byte{0x1e}) // record separator
	h.Write(data)
	var out fr.Element
	out.SetBytes(h.Sum(nil))
	return out
}

// PedersenFolder builds a Pedersen commitment sum over BLS12-381 G1.
// Thread-unsafe; caller serialises concurrent Fold calls.
type PedersenFolder struct {
	acc        bls.G1Jac // running accumulator
	stepHashes []string  // one SHA-256 tag per step for the wire form
	initted    bool
}

// NewPedersenFolder returns a fresh folder with an identity
// accumulator (zero-value G1Jac is the point at infinity).
func NewPedersenFolder() *PedersenFolder {
	return &PedersenFolder{initted: true}
}

// Fold adds `m_i·G + r_i·H` to the running accumulator.
func (f *PedersenFolder) Fold(step FoldStep) error {
	if !f.initted {
		return errors.New("aggregator/pedersen: folder not initialised (use NewPedersenFolder)")
	}
	if len(step.Instance) == 0 {
		return errors.New("aggregator/pedersen: empty instance")
	}
	m := hashToFr("CP_PED_M_V1", step.Instance)
	r := hashToFr("CP_PED_R_V1", step.WitnessDigest)
	var mBig, rBig big.Int
	m.BigInt(&mBig)
	r.BigInt(&rBig)

	var mG, rH, contrib bls.G1Jac
	mG.FromAffine(&pedersenG)
	rH.FromAffine(&pedersenH)
	mG.ScalarMultiplication(&mG, &mBig)
	rH.ScalarMultiplication(&rH, &rBig)
	contrib.Set(&mG).AddAssign(&rH)
	f.acc.AddAssign(&contrib)

	stepH := sha256.Sum256(append(append([]byte{}, step.Instance...), step.WitnessDigest...))
	f.stepHashes = append(f.stepHashes, hex.EncodeToString(stepH[:]))
	return nil
}

// Compress finalises. Emits the compressed 48-byte G1 point as hex.
func (f *PedersenFolder) Compress() (AggregateProof, error) {
	if !f.initted {
		return AggregateProof{}, errors.New("aggregator/pedersen: folder not initialised")
	}
	if len(f.stepHashes) == 0 {
		return AggregateProof{}, errors.New("aggregator/pedersen: no steps folded")
	}
	var affine bls.G1Affine
	affine.FromJacobian(&f.acc)
	pt := affine.Bytes() // compressed
	return AggregateProof{
		Scheme:     SchemePedersenFoldV1,
		Steps:      len(f.stepHashes),
		Root:       hex.EncodeToString(pt[:]),
		StepHashes: append([]string{}, f.stepHashes...),
	}, nil
}

// Verify reconstructs the Pedersen sum from `steps` and checks the
// resulting curve point matches agg.Root. Structural mismatches
// (scheme, step count, per-step hash) return (false, err).
func (f *PedersenFolder) Verify(steps []FoldStep, agg AggregateProof) (bool, error) {
	if agg.Scheme != SchemePedersenFoldV1 {
		return false, fmt.Errorf("aggregator/pedersen: unsupported scheme %q", agg.Scheme)
	}
	if agg.Steps != len(steps) {
		return false, fmt.Errorf("aggregator/pedersen: step count mismatch: got %d, agg says %d", len(steps), agg.Steps)
	}
	if len(agg.StepHashes) != len(steps) {
		return false, fmt.Errorf("aggregator/pedersen: step_hashes length mismatch: got %d, want %d", len(agg.StepHashes), len(steps))
	}
	var acc bls.G1Jac // zero = infinity
	for i, step := range steps {
		if len(step.Instance) == 0 {
			return false, fmt.Errorf("aggregator/pedersen: step %d has empty instance", i)
		}
		stepH := sha256.Sum256(append(append([]byte{}, step.Instance...), step.WitnessDigest...))
		if hex.EncodeToString(stepH[:]) != agg.StepHashes[i] {
			return false, fmt.Errorf("aggregator/pedersen: step %d hash mismatch", i)
		}
		m := hashToFr("CP_PED_M_V1", step.Instance)
		r := hashToFr("CP_PED_R_V1", step.WitnessDigest)
		var mBig, rBig big.Int
		m.BigInt(&mBig)
		r.BigInt(&rBig)
		var mG, rH, contrib bls.G1Jac
		mG.FromAffine(&pedersenG)
		rH.FromAffine(&pedersenH)
		mG.ScalarMultiplication(&mG, &mBig)
		rH.ScalarMultiplication(&rH, &rBig)
		contrib.Set(&mG).AddAssign(&rH)
		acc.AddAssign(&contrib)
	}
	var affine bls.G1Affine
	affine.FromJacobian(&acc)
	pt := affine.Bytes()
	if hex.EncodeToString(pt[:]) != agg.Root {
		return false, errors.New("aggregator/pedersen: commitment mismatch — sequence tampered or scheme mislabelled")
	}
	return true, nil
}

// FoldAllPedersen is the stateless helper mirroring FoldAll.
func FoldAllPedersen(steps []FoldStep) (AggregateProof, error) {
	f := NewPedersenFolder()
	for i, s := range steps {
		if err := f.Fold(s); err != nil {
			return AggregateProof{}, fmt.Errorf("fold step %d: %w", i, err)
		}
	}
	return f.Compress()
}

// VerifyAllPedersen is the stateless counterpart.
func VerifyAllPedersen(steps []FoldStep, agg AggregateProof) (bool, error) {
	f := NewPedersenFolder()
	return f.Verify(steps, agg)
}

// PedersenHomomorphismCheck is a diagnostic that recomputes the sum
// starting from a mid-point split — it returns true iff
//
//     Commit(steps[:k]) + Commit(steps[k:]) == Commit(steps)
//
// as curve points. Callers use this to sanity-check the homomorphic
// property of the scheme against their own aggregation topology. It is
// NOT called in the main Verify path.
func PedersenHomomorphismCheck(steps []FoldStep, split int) (bool, error) {
	if split < 0 || split > len(steps) {
		return false, fmt.Errorf("aggregator/pedersen: split %d out of range [0, %d]", split, len(steps))
	}
	whole, err := FoldAllPedersen(steps)
	if err != nil {
		return false, err
	}
	if split == 0 || split == len(steps) {
		// One side is empty — homomorphism vacuously holds; nothing to check.
		return true, nil
	}
	left, err := FoldAllPedersen(steps[:split])
	if err != nil {
		return false, err
	}
	right, err := FoldAllPedersen(steps[split:])
	if err != nil {
		return false, err
	}
	// Sum left.Root and right.Root as curve points; compare against whole.Root.
	lBytes, err := hex.DecodeString(left.Root)
	if err != nil {
		return false, err
	}
	rBytes, err := hex.DecodeString(right.Root)
	if err != nil {
		return false, err
	}
	var la, ra bls.G1Affine
	if _, err := la.SetBytes(lBytes); err != nil {
		return false, err
	}
	if _, err := ra.SetBytes(rBytes); err != nil {
		return false, err
	}
	var sum bls.G1Jac
	sum.FromAffine(&la)
	var raJ bls.G1Jac
	raJ.FromAffine(&ra)
	sum.AddAssign(&raJ)
	var sumA bls.G1Affine
	sumA.FromJacobian(&sum)
	pt := sumA.Bytes()
	return hex.EncodeToString(pt[:]) == whole.Root, nil
}
