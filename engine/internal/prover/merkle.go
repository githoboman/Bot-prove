package prover

import (
	"crypto/sha256"
	"encoding/hex"
)

func BuildTree(leaves [][]byte) *MerkleNode {
	if len(leaves) == 0 {
		return nil
	}

	nodes := make([]*MerkleNode, len(leaves))
	for i, lf := range leaves {
		h := sha256.Sum256(lf)
		nodes[i] = &MerkleNode{Hash: h}
	}

	if len(nodes)%2 != 0 {
		nodes = append(nodes, nodes[len(nodes)-1])
	}

	for len(nodes) > 1 {
		var next []*MerkleNode
		for i := 0; i < len(nodes); i += 2 {
			combined := append(nodes[i].Hash[:], nodes[i+1].Hash[:]...)
			h := sha256.Sum256(combined)
			n := &MerkleNode{Hash: h, Left: nodes[i], Right: nodes[i+1]}
			next = append(next, n)
		}
		if len(next) > 1 && len(next)%2 != 0 {
			next = append(next, next[len(next)-1])
		}
		nodes = next
	}

	return nodes[0]
}

func Root(leaves [][]byte) string {
	tree := BuildTree(leaves)
	if tree == nil {
		return ""
	}
	return hex.EncodeToString(tree.Hash[:])
}

func GetPath(leaves [][]byte, idx int) []string {
	if idx < 0 || idx >= len(leaves) {
		return nil
	}

	padded := make([][]byte, len(leaves))
	copy(padded, leaves)
	if len(padded)%2 != 0 {
		padded = append(padded, padded[len(padded)-1])
	}

	hashes := make([][32]byte, len(padded))
	for i, lf := range padded {
		hashes[i] = sha256.Sum256(lf)
	}

	var path []string
	pos := idx

	for len(hashes) > 1 {
		var sibling int
		if pos%2 == 0 {
			sibling = pos + 1
		} else {
			sibling = pos - 1
		}
		if sibling >= 0 && sibling < len(hashes) {
			path = append(path, hex.EncodeToString(hashes[sibling][:]))
		}

		var next [][32]byte
		for i := 0; i < len(hashes); i += 2 {
			if i+1 >= len(hashes) {
				// Safety: should not happen after padding, but guard against OOB
				next = append(next, hashes[i])
				break
			}
			combined := append(hashes[i][:], hashes[i+1][:]...)
			next = append(next, sha256.Sum256(combined))
		}
		if len(next) > 1 && len(next)%2 != 0 {
			next = append(next, next[len(next)-1])
		}
		hashes = next
		pos /= 2
	}

	return path
}

func VerifyPath(leaf []byte, path []string, root string, idx int) bool {
	h := sha256.Sum256(leaf)
	pos := idx

	for _, sib := range path {
		sibBytes, err := hex.DecodeString(sib)
		if err != nil || len(sibBytes) != 32 {
			return false
		}
		var combined []byte
		if pos%2 == 0 {
			combined = append(h[:], sibBytes...)
		} else {
			combined = append(sibBytes, h[:]...)
		}
		h = sha256.Sum256(combined)
		pos /= 2
	}

	return hex.EncodeToString(h[:]) == root
}
