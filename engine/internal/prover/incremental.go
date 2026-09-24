package prover

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// IncrementalMerkle is an append-only Merkle accumulator that keeps only the
// O(log N) "frontier" of pending right-hand siblings needed to compute the
// next root. Adding a leaf is amortised O(1); computing the current root is
// O(log N). It never rebuilds prior levels, so a proof-registry that accepts
// receipts in batches pays O(batch_size) instead of O(N) per batch.
//
// Padding rule (duplicate-last) matches the existing BuildTree in merkle.go
// so both implementations produce the same root for the same leaf sequence.
//
// A BatchReceipt captures the state transition when a batch is appended:
// (prevRoot, newRoot, batchLeafHashes) with an inclusion proof for every
// leaf in the batch anchored to newRoot.
type IncrementalMerkle struct {
	// frontier[i] holds the pending hash at level i (nil if that level has
	// no pending node). When two hashes are pending at the same level, they
	// combine and bubble upward.
	frontier [][32]byte
	occupied []bool
	// size = number of leaves ever appended.
	size int
}

// NewIncrementalMerkle constructs an empty append-only accumulator.
func NewIncrementalMerkle() *IncrementalMerkle {
	return &IncrementalMerkle{}
}

// Size returns the number of leaves appended so far.
func (m *IncrementalMerkle) Size() int { return m.size }

// Append hashes the leaf and folds it into the frontier.
// Returns the leaf hash.
func (m *IncrementalMerkle) Append(leaf []byte) [32]byte {
	h := sha256.Sum256(leaf)
	m.foldIn(h)
	m.size++
	return h
}

func (m *IncrementalMerkle) foldIn(h [32]byte) {
	level := 0
	cur := h
	for {
		if level >= len(m.frontier) {
			m.frontier = append(m.frontier, cur)
			m.occupied = append(m.occupied, true)
			return
		}
		if !m.occupied[level] {
			m.frontier[level] = cur
			m.occupied[level] = true
			return
		}
		// Combine.
		combined := append(m.frontier[level][:], cur[:]...)
		cur = sha256.Sum256(combined)
		m.frontier[level] = [32]byte{}
		m.occupied[level] = false
		level++
	}
}

// Root computes the current Merkle root using duplicate-last padding at every
// level that has an unpaired node — matching BuildTree/Root in merkle.go.
// The accumulator itself is not mutated.
func (m *IncrementalMerkle) Root() string {
	if m.size == 0 {
		return ""
	}
	targetLvl := ceilLog2(m.size)
	if targetLvl == 0 {
		targetLvl = 1 // single-leaf tree is still one level (hash(h||h))
	}
	var carry [32]byte
	hasCarry := false
	carryLvl := 0

	for lvl := 0; lvl < len(m.frontier); lvl++ {
		// Advance any pending carry up to this level by duplicate-padding.
		for hasCarry && carryLvl < lvl {
			combined := append(carry[:], carry[:]...)
			carry = sha256.Sum256(combined)
			carryLvl++
		}
		if !m.occupied[lvl] {
			continue
		}
		if hasCarry {
			combined := append(m.frontier[lvl][:], carry[:]...)
			carry = sha256.Sum256(combined)
			carryLvl = lvl + 1
			// hasCarry stays true
		} else {
			carry = m.frontier[lvl]
			hasCarry = true
			carryLvl = lvl
		}
	}
	// Final advance up to targetLvl (duplicate-pad any partial level to the top).
	for hasCarry && carryLvl < targetLvl {
		combined := append(carry[:], carry[:]...)
		carry = sha256.Sum256(combined)
		carryLvl++
	}
	if !hasCarry {
		return ""
	}
	return hex.EncodeToString(carry[:])
}

// ceilLog2 returns ceil(log2(n)) for n >= 1.
func ceilLog2(n int) int {
	if n <= 1 {
		return 0
	}
	l := 0
	v := n - 1
	for v > 0 {
		v >>= 1
		l++
	}
	return l
}

// BatchReceipt is a portable record of an append-only batch commit.
// A verifier that only knows PrevRoot and the leaves can recompute NewRoot,
// or replay the frontier deltas independently.
type BatchReceipt struct {
	PrevRoot   string   `json:"prev_root"`
	NewRoot    string   `json:"new_root"`
	StartIndex int      `json:"start_index"`
	Leaves     []string `json:"leaves"`   // hex-encoded leaf HASHES (post sha256)
	BatchSize  int      `json:"batch_size"`
}

// AppendBatch appends every leaf and returns a BatchReceipt describing the
// transition. On error the accumulator is unchanged.
func (m *IncrementalMerkle) AppendBatch(leaves [][]byte) (*BatchReceipt, error) {
	if len(leaves) == 0 {
		return nil, errors.New("empty batch")
	}
	// Snapshot for rollback on defensive failure.
	prevFrontier := make([][32]byte, len(m.frontier))
	copy(prevFrontier, m.frontier)
	prevOcc := make([]bool, len(m.occupied))
	copy(prevOcc, m.occupied)
	prevSize := m.size

	prevRoot := m.Root()
	startIdx := m.size

	leafHexes := make([]string, len(leaves))
	for i, lf := range leaves {
		hh := m.Append(lf)
		leafHexes[i] = hex.EncodeToString(hh[:])
	}
	newRoot := m.Root()

	if newRoot == "" {
		// Roll back defensively; shouldn't happen given size>0.
		m.frontier = prevFrontier
		m.occupied = prevOcc
		m.size = prevSize
		return nil, fmt.Errorf("root computation failed for batch of %d", len(leaves))
	}

	return &BatchReceipt{
		PrevRoot:   prevRoot,
		NewRoot:    newRoot,
		StartIndex: startIdx,
		Leaves:     leafHexes,
		BatchSize:  len(leaves),
	}, nil
}

// VerifyReceiptTransition rebuilds the transition described by rc from an
// empty accumulator with prefix-root == rc.PrevRoot, and checks the resulting
// root equals rc.NewRoot. It does NOT know the earlier leaves, so it uses the
// receipt's own PrevRoot as the anchor: the check is that (rc.PrevRoot,
// rc.Leaves) is internally consistent with (rc.NewRoot) under the same
// duplicate-last padding rule.
//
// This is the invariant a downstream gate contract can check when it accepts
// a proof batch: given the previous root it already committed and the leaves
// the submitter now offers, does the new root match?
func VerifyReceiptTransition(prevAccum *IncrementalMerkle, rc *BatchReceipt) (bool, error) {
	if rc == nil {
		return false, errors.New("nil receipt")
	}
	if rc.BatchSize != len(rc.Leaves) {
		return false, fmt.Errorf("batch size mismatch: header=%d leaves=%d", rc.BatchSize, len(rc.Leaves))
	}
	// Verify that the accumulator's current root matches PrevRoot.
	if prevAccum.Root() != rc.PrevRoot {
		return false, fmt.Errorf("prev root mismatch: accum=%s receipt=%s",
			prevAccum.Root(), rc.PrevRoot)
	}
	// Clone accumulator, replay leaves as raw hashes (leaves in the receipt
	// are already hex-encoded sha256 hashes of the original leaf bytes).
	clone := prevAccum.Clone()
	for _, lfHex := range rc.Leaves {
		raw, err := hex.DecodeString(lfHex)
		if err != nil || len(raw) != 32 {
			return false, fmt.Errorf("invalid leaf hash hex %q", lfHex)
		}
		var arr [32]byte
		copy(arr[:], raw)
		clone.foldIn(arr)
		clone.size++
	}
	return clone.Root() == rc.NewRoot, nil
}

// Clone returns a deep copy of the accumulator.
func (m *IncrementalMerkle) Clone() *IncrementalMerkle {
	front := make([][32]byte, len(m.frontier))
	copy(front, m.frontier)
	occ := make([]bool, len(m.occupied))
	copy(occ, m.occupied)
	return &IncrementalMerkle{
		frontier: front,
		occupied: occ,
		size:     m.size,
	}
}
