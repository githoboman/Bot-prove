package prover

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// leavesGen generates a non-empty slice of random byte-slice leaves.
func leavesGen() gopter.Gen {
	return gen.SliceOfN(8, gen.SliceOf(gen.UInt8Range(0, 255))).
		Map(func(vals [][]uint8) [][]byte {
			out := make([][]byte, len(vals))
			for i, v := range vals {
				b := make([]byte, len(v))
				for j, x := range v {
					b[j] = byte(x)
				}
				// ensure non-empty leaf content so hashing is meaningful
				if len(b) == 0 {
					b = []byte{byte(i)}
				}
				out[i] = b
			}
			return out
		})
}

// TestMerkleInclusionProperty checks that for any randomly generated set of
// leaves and any valid index into it, the inclusion path produced by
// GetPath always verifies successfully against the root produced by Root.
func TestMerkleInclusionProperty(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 200
	properties := gopter.NewProperties(parameters)

	properties.Property("every leaf's inclusion path verifies against the root", prop.ForAll(
		func(leaves [][]byte, idx int) bool {
			if len(leaves) == 0 {
				return true
			}
			idx = idx % len(leaves)
			if idx < 0 {
				idx += len(leaves)
			}

			root := Root(leaves)
			path := GetPath(leaves, idx)

			return VerifyPath(leaves[idx], path, root, idx)
		},
		leavesGen(),
		gen.IntRange(0, 1<<20),
	))

	properties.Property("tampering with the leaf content breaks verification", prop.ForAll(
		func(leaves [][]byte, idx int) bool {
			if len(leaves) == 0 {
				return true
			}
			idx = idx % len(leaves)
			if idx < 0 {
				idx += len(leaves)
			}

			root := Root(leaves)
			path := GetPath(leaves, idx)

			tampered := append([]byte{}, leaves[idx]...)
			tampered = append(tampered, 0xFF)

			return !VerifyPath(tampered, path, root, idx)
		},
		leavesGen(),
		gen.IntRange(0, 1<<20),
	))

	properties.Property("root is deterministic and order-sensitive", prop.ForAll(
		func(leaves [][]byte) bool {
			if len(leaves) < 2 {
				return true
			}
			r1 := Root(leaves)
			r2 := Root(leaves)
			if r1 != r2 {
				return false
			}

			swapped := make([][]byte, len(leaves))
			copy(swapped, leaves)
			swapped[0], swapped[len(swapped)-1] = swapped[len(swapped)-1], swapped[0]

			// If the first and last leaves happen to be identical, a swap
			// produces no change, so skip in that case.
			if string(leaves[0]) == string(leaves[len(leaves)-1]) {
				return true
			}
			return Root(swapped) != r1
		},
		leavesGen(),
	))

	properties.TestingRun(t)
}
