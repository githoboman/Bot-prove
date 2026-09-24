package prover

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"testing"
)

// leafSizes covers small/medium/large proof shapes so the benchmark suite
// exercises realistic complexity classes (see ComplexityClass in classifier.go).
var leafCounts = []int{2, 8, 32, 128, 512}

func randomLeaves(n int, seed int64) [][]byte {
	r := rand.New(rand.NewSource(seed))
	leaves := make([][]byte, n)
	for i := 0; i < n; i++ {
		buf := make([]byte, 256)
		_, _ = r.Read(buf)
		leaves[i] = buf
	}
	return leaves
}

// BenchmarkBuildTree measures Merkle tree construction cost across leaf counts.
func BenchmarkBuildTree(b *testing.B) {
	for _, n := range leafCounts {
		leaves := randomLeaves(n, int64(n))
		b.Run(fmt.Sprintf("leaves=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = BuildTree(leaves)
			}
		})
	}
}

// BenchmarkRoot measures end-to-end root computation across leaf counts.
func BenchmarkRoot(b *testing.B) {
	for _, n := range leafCounts {
		leaves := randomLeaves(n, int64(n)+1)
		b.Run(fmt.Sprintf("leaves=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = Root(leaves)
			}
		})
	}
}

// BenchmarkGetPath measures Merkle inclusion-path derivation across leaf counts.
func BenchmarkGetPath(b *testing.B) {
	for _, n := range leafCounts {
		leaves := randomLeaves(n, int64(n)+2)
		b.Run(fmt.Sprintf("leaves=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = GetPath(leaves, 0)
			}
		})
	}
}

// BenchmarkSHA256 is a control benchmark: raw hashing cost with no tree
// overhead, so regressions in BuildTree/Root can be attributed to tree logic
// rather than the underlying hash primitive.
func BenchmarkSHA256(b *testing.B) {
	data := randomLeaves(1, 99)[0]
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = sha256.Sum256(data)
	}
}
