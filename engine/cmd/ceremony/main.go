// Command ceremony runs the Groth16 trusted-setup ceremony (Phase 1 +
// Phase 2) for CasperProver's PreimageCircuit, writes the resulting
// proving/verifying keys and Phase-1 SRS commons to disk, and emits a
// JSON attestation of the transcript.
//
// This is the same code path exercised by the ceremony package unit
// tests - just wrapped in a CLI so operators can reproduce it and
// verifiers can check the attestations.
//
// Usage:
//
//	go run ./cmd/ceremony --out zk/ceremony --n 1024 --p1 3 --p2 3 \
//	    --beacon "drand-<round>-<value>" > zk/ceremony/attestations.json
//
// Honesty label: SINGLE-COORDINATOR CEREMONY. See
// engine/internal/zkverifier/ceremony/ceremony.go for the multi-party
// upgrade path.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/zkverifier/ceremony"
)

func main() {
	outDir := flag.String("out", "", "output directory for pk/vk/commons binaries (empty = no files)")
	n := flag.Uint64("n", 1024, "Powers-of-Tau FFT domain size (power of two, >= 32)")
	p1 := flag.Int("p1", 3, "number of Phase-1 contributions")
	p2 := flag.Int("p2", 3, "number of Phase-2 contributions")
	beacon := flag.String("beacon", "", "beacon challenge (public randomness; required)")
	flag.Parse()

	if *beacon == "" {
		fmt.Fprintln(os.Stderr, "--beacon is required (drand round/value, block hash, etc.)")
		os.Exit(2)
	}

	cfg := ceremony.Config{
		N:                  *n,
		Phase1Contributors: *p1,
		Phase2Contributors: *p2,
		BeaconChallenge:    []byte(*beacon),
		OutDir:             *outDir,
	}

	t0 := time.Now()
	res, err := ceremony.Run(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ceremony:", err)
		os.Exit(1)
	}
	dur := time.Since(t0)

	out := struct {
		DurationSeconds float64             `json:"duration_seconds"`
		OutDir          string              `json:"out_dir"`
		Transcript      ceremony.Transcript `json:"transcript"`
	}{
		DurationSeconds: dur.Seconds(),
		OutDir:          *outDir,
		Transcript:      res.Transcript,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
}
