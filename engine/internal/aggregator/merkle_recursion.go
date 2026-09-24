package aggregator

// Merkle recursion of proof commitments — intermediate scheme.
//
// This is an honest intermediate between "linear replay" (hash-fold-v1)
// and true STARK recursion (winterfell / arkworks-stark, Rust-only).
// A caller with k proofs — each summarised by its own commitment digest
// — builds a binary SHA-256 Merkle tree over the digests. The
// aggregate carries `(merkle_root, count, tree_height)` in 32+8+1
// bytes. A verifier who wants to check membership of a specific proof
// runs O(log n) SHA-256 hashes against its inclusion path — a genuine
// win over the O(n) linear replay of hash-fold-v1.
//
// This is NOT a STARK recursion in the cryptographic sense: it does
// not produce a single STARK proof whose validity implies the validity
// of every underlying proof. It reduces the verifier's *reference*
// work from O(n) to O(log n), but each membership check is against
// a commitment hash — not a re-execution of the underlying proof.
//
// The public envelope carries scheme = "merkle-recursion-v1" so
// downstream consumers cannot confuse it with a real STARK
// recursion. See docs/MERKLE_RECURSION.md for the honesty
// disclosure.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// SchemeMerkleRecursionV1 is the scheme label reported in
// MerkleRecursionProof.Scheme.
const SchemeMerkleRecursionV1 = "merkle-recursion-v1"

// merkleLeafTag / merkleNodeTag isolate the hash pre-image domain so
// leaf digests can never collide with interior nodes (second
// pre-image resistance in a Merkle tree design pattern).
var (
	merkleLeafTag = []byte{0x00}
	merkleNodeTag = []byte{0x01}
)

// MerkleRecursionProof is the aggregate over k proof commitments.
type MerkleRecursionProof struct {
	Scheme     string `json:"scheme"`
	Count      int    `json:"count"`
	TreeHeight int    `json:"tree_height"`
	RootHex    string `json:"merkle_root_hex"`
}

// MerkleInclusionProof authenticates a single leaf against the root
// carried in a MerkleRecursionProof. Path is bottom-up: each entry is
// the sibling hash, with `Positions` telling the verifier whether the
// current node is a left child (`false`) or a right child (`true`).
type MerkleInclusionProof struct {
	LeafIndex int      `json:"leaf_index"`
	LeafHex   string   `json:"leaf_hex"`  // hex of the leaf digest (32 bytes)
	Path      []string `json:"path_hex"`  // sibling digests bottom-up
	Positions []bool   `json:"positions"` // true = leaf/current is right child
}

// AggregateMerkleRecursion builds the tree over `leaves`. Each leaf is
// the caller-supplied commitment digest (canonically SHA-256 of a
// proof's public output; the aggregator does NOT interpret it beyond
// hashing).
//
// The tree is a padded binary tree: at every level with an odd node
// count, the last node is duplicated (Bitcoin-style). This is a
// pragmatic choice — it keeps the height computation predictable at
// the cost of a well-documented tradeoff (a same-value adjacent leaf
// can be inserted without changing the root; callers who care about
// uniqueness must pre-hash a positional tag into each leaf).
func AggregateMerkleRecursion(leaves [][]byte) (MerkleRecursionProof, error) {
	if len(leaves) == 0 {
		return MerkleRecursionProof{}, errors.New("aggregator/merkle: empty leaf list")
	}
	// Hash each leaf with the leaf tag.
	level := make([][sha256.Size]byte, len(leaves))
	for i, leaf := range leaves {
		if len(leaf) == 0 {
			return MerkleRecursionProof{}, fmt.Errorf("aggregator/merkle: leaf %d is empty", i)
		}
		h := sha256.New()
		h.Write(merkleLeafTag)
		h.Write(leaf)
		copy(level[i][:], h.Sum(nil))
	}
	height := 0
	for len(level) > 1 {
		if len(level)%2 == 1 {
			// Duplicate the last node.
			level = append(level, level[len(level)-1])
		}
		next := make([][sha256.Size]byte, len(level)/2)
		for i := 0; i < len(next); i++ {
			h := sha256.New()
			h.Write(merkleNodeTag)
			h.Write(level[2*i][:])
			h.Write(level[2*i+1][:])
			copy(next[i][:], h.Sum(nil))
		}
		level = next
		height++
	}
	return MerkleRecursionProof{
		Scheme:     SchemeMerkleRecursionV1,
		Count:      len(leaves),
		TreeHeight: height,
		RootHex:    hex.EncodeToString(level[0][:]),
	}, nil
}

// BuildInclusionProof returns the O(log n) authentication path for a
// specific leaf index against the same tree AggregateMerkleRecursion
// would have built. Returns ErrLeafOutOfRange if leafIndex is
// negative or >= len(leaves).
func BuildInclusionProof(leaves [][]byte, leafIndex int) (MerkleInclusionProof, error) {
	if leafIndex < 0 || leafIndex >= len(leaves) {
		return MerkleInclusionProof{}, fmt.Errorf("aggregator/merkle: leaf index %d out of range [0, %d)", leafIndex, len(leaves))
	}
	// Compute the same padded tree as AggregateMerkleRecursion, but
	// remember the path.
	level := make([][sha256.Size]byte, len(leaves))
	for i, leaf := range leaves {
		if len(leaf) == 0 {
			return MerkleInclusionProof{}, fmt.Errorf("aggregator/merkle: leaf %d is empty", i)
		}
		h := sha256.New()
		h.Write(merkleLeafTag)
		h.Write(leaf)
		copy(level[i][:], h.Sum(nil))
	}
	// Store leaf's own hash for the wire form.
	leafHash := level[leafIndex]

	cursor := leafIndex
	var path []string
	var positions []bool
	for len(level) > 1 {
		if len(level)%2 == 1 {
			level = append(level, level[len(level)-1])
		}
		// Sibling of cursor is the opposite parity index.
		isRight := cursor%2 == 1
		var siblingIdx int
		if isRight {
			siblingIdx = cursor - 1
		} else {
			siblingIdx = cursor + 1
		}
		path = append(path, hex.EncodeToString(level[siblingIdx][:]))
		positions = append(positions, isRight)
		// Compact to next level.
		next := make([][sha256.Size]byte, len(level)/2)
		for i := 0; i < len(next); i++ {
			h := sha256.New()
			h.Write(merkleNodeTag)
			h.Write(level[2*i][:])
			h.Write(level[2*i+1][:])
			copy(next[i][:], h.Sum(nil))
		}
		level = next
		cursor /= 2
	}
	return MerkleInclusionProof{
		LeafIndex: leafIndex,
		LeafHex:   hex.EncodeToString(leafHash[:]),
		Path:      path,
		Positions: positions,
	}, nil
}

// VerifyMerkleInclusion re-runs the O(log n) path and compares against
// the aggregate's root. Returns (true, nil) iff the leaf is proved to
// be at leafIndex in the tree whose root is agg.RootHex.
//
// The verifier accepts the caller-supplied leafHash (parsed from
// LeafHex) as the leaf-level hash — that value is what was fed to
// AggregateMerkleRecursion after the merkleLeafTag hash. A caller
// wanting to prove "this particular pre-image ends up at this
// index" must first check leafHash == SHA256(merkleLeafTag || preimage)
// separately; VerifyMerkleInclusion only proves inclusion of the
// hash itself, not of any specific pre-image. This split is
// intentional so consumers can decouple pre-image storage from
// verification.
func VerifyMerkleInclusion(agg MerkleRecursionProof, proof MerkleInclusionProof) (bool, error) {
	if agg.Scheme != SchemeMerkleRecursionV1 {
		return false, fmt.Errorf("aggregator/merkle: unsupported scheme %q", agg.Scheme)
	}
	if agg.TreeHeight != len(proof.Path) {
		return false, fmt.Errorf("aggregator/merkle: path length %d != tree height %d", len(proof.Path), agg.TreeHeight)
	}
	if len(proof.Path) != len(proof.Positions) {
		return false, fmt.Errorf("aggregator/merkle: path/positions length mismatch (%d vs %d)", len(proof.Path), len(proof.Positions))
	}
	if proof.LeafIndex < 0 || proof.LeafIndex >= agg.Count {
		return false, fmt.Errorf("aggregator/merkle: leaf index %d out of range [0, %d)", proof.LeafIndex, agg.Count)
	}
	leafBytes, err := hex.DecodeString(proof.LeafHex)
	if err != nil || len(leafBytes) != sha256.Size {
		return false, errors.New("aggregator/merkle: leaf_hex must be 32-byte hex")
	}
	var acc [sha256.Size]byte
	copy(acc[:], leafBytes)
	for i, sibHex := range proof.Path {
		sib, err := hex.DecodeString(sibHex)
		if err != nil || len(sib) != sha256.Size {
			return false, fmt.Errorf("aggregator/merkle: path[%d] must be 32-byte hex", i)
		}
		var next [sha256.Size]byte
		h := sha256.New()
		h.Write(merkleNodeTag)
		if proof.Positions[i] {
			// current is right child; sibling on left.
			h.Write(sib)
			h.Write(acc[:])
		} else {
			h.Write(acc[:])
			h.Write(sib)
		}
		copy(next[:], h.Sum(nil))
		acc = next
	}
	got := hex.EncodeToString(acc[:])
	if got != agg.RootHex {
		return false, errors.New("aggregator/merkle: computed root does not match aggregate — inclusion refused")
	}
	return true, nil
}
