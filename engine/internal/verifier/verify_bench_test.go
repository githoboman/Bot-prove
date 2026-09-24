package verifier

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
)

func randBytes(n int, seed int64) []byte {
	r := rand.New(rand.NewSource(seed))
	buf := make([]byte, n)
	_, _ = r.Read(buf)
	return buf
}

// payloadSizes covers the same complexity-class boundaries as
// prover.ClassifierConfig defaults (trivial/low/medium) so the benchmark
// tracks verification cost across realistic proof sizes.
var payloadSizes = []int{64, 1024, 65536}

// BenchmarkVerifyProof measures LocalVerifier.VerifyProof end-to-end
// (generate a proof, then verify it) across payload sizes.
func BenchmarkVerifyProof(b *testing.B) {
	v := New()
	for _, sz := range payloadSizes {
		input := randBytes(sz, int64(sz))
		output := randBytes(sz, int64(sz)+1)
		model := randBytes(sz, int64(sz)+2)

		engine := prover.New()
		p := engine.Generate("bench-agent", input, output, model, "bench-uc")

		b.Run(fmt.Sprintf("payload=%dB", sz), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := v.VerifyProof(p, input, output, model); err != nil {
					b.Fatalf("unexpected verify error: %v", err)
				}
			}
		})
	}
}

// BenchmarkGenerateProof measures ProofEngine.Generate cost across payload
// sizes — the write-path counterpart to BenchmarkVerifyProof.
//
// Steady-state hygiene: ProofEngine.Generate calls evictIfNeeded on every
// invocation, which becomes an O(n) full-map scan once the in-memory
// proof map exceeds prover.MaxProofs (100_000). Because the benchmark
// generates new proofs without revoking any, an unbounded run will
// eventually cross that threshold mid-loop and start double-counting the
// eviction scan into the per-op cost — turning the reported ns/op into
// an artifact of bookkeeping growth rather than steady-state Generate
// cost. This is exactly what happens for the smallest payload class,
// which iterates the most inside a fixed benchtime budget.
//
// Guard: pause the timer periodically (every proofRefreshInterval
// iterations) and swap in a fresh engine so the in-memory map never
// crosses prover.MaxProofs mid-run. The refresh itself is not counted
// against per-op cost. See R11 review notes.
const proofRefreshInterval = 50_000

func BenchmarkGenerateProof(b *testing.B) {
	if proofRefreshInterval >= prover.MaxProofs {
		b.Fatalf("proofRefreshInterval (%d) must be < prover.MaxProofs (%d) to keep the map below the eviction threshold",
			proofRefreshInterval, prover.MaxProofs)
	}
	for _, sz := range payloadSizes {
		input := randBytes(sz, int64(sz)+10)
		output := randBytes(sz, int64(sz)+11)
		model := randBytes(sz, int64(sz)+12)

		b.Run(fmt.Sprintf("payload=%dB", sz), func(b *testing.B) {
			engine := prover.New()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if i > 0 && i%proofRefreshInterval == 0 {
					b.StopTimer()
					engine = prover.New()
					b.StartTimer()
				}
				_ = engine.Generate("bench-agent", input, output, model, "bench-uc")
			}
		})
	}
}
