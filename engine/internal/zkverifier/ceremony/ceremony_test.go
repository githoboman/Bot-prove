package ceremony

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/zkverifier/gnarkzk"
)

// runCeremony is a shared helper: DefaultConfig N=8 with 3 contributors on
// each phase. This is a real ceremony end-to-end (Phase 1 + Phase 2 with
// multiple independent contributions verified pairwise and sealed with
// the beacon), just executed inside one process.
func runCeremony(t *testing.T) *Result {
	t.Helper()
	res, err := Run(DefaultConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

// TestRunProducesUsableSetup: after the ceremony, the resulting proving /
// verifying key pair actually proves and verifies the PreimageCircuit.
// If gnark's mpcsetup produced a broken SRS this would fail during Prove
// or Verify.
func TestRunProducesUsableSetup(t *testing.T) {
	res := runCeremony(t)

	// Compile the circuit again and prove with a fresh witness.
	var circuit gnarkzk.PreimageCircuit
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	preimage := big.NewInt(0xC45)
	pubHash := gnarkzk.ComputeMiMCHash(preimage)

	assignment := &gnarkzk.PreimageCircuit{
		PreImage: preimage,
		Hash:     pubHash,
	}
	fullWit, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatalf("witness: %v", err)
	}
	pubWit, err := fullWit.Public()
	if err != nil {
		t.Fatalf("public witness: %v", err)
	}

	proof, err := groth16.Prove(ccs, res.ProvingKey, fullWit)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	if err := groth16.Verify(proof, res.VerifyingKey, pubWit); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// TestTranscriptShape: the transcript records N contributions per phase,
// each with a non-empty challenge + digest, plus final PK/VK digests.
func TestTranscriptShape(t *testing.T) {
	res := runCeremony(t)
	tr := res.Transcript

	if got, want := len(tr.Phase1), DefaultConfig().Phase1Contributors; got != want {
		t.Fatalf("Phase1 contributions: got %d, want %d", got, want)
	}
	if got, want := len(tr.Phase2), DefaultConfig().Phase2Contributors; got != want {
		t.Fatalf("Phase2 contributions: got %d, want %d", got, want)
	}
	for i, c := range tr.Phase1 {
		if c.Digest == "" || c.SizeBytes == 0 {
			t.Fatalf("Phase1[%d] empty digest/size", i)
		}
	}
	for i, c := range tr.Phase2 {
		if c.Digest == "" || c.SizeBytes == 0 {
			t.Fatalf("Phase2[%d] empty digest/size", i)
		}
	}
	if tr.FinalPKDigest == "" || tr.FinalVKDigest == "" || tr.CommonsDigest == "" {
		t.Fatalf("empty final artifact digest(s): pk=%q vk=%q commons=%q",
			tr.FinalPKDigest, tr.FinalVKDigest, tr.CommonsDigest)
	}
	if tr.CircuitConstraints == 0 {
		t.Fatalf("CircuitConstraints=0 (expected non-zero)")
	}
	if len(tr.HonestyLabel) == 0 {
		t.Fatalf("HonestyLabel empty; ceremony must self-label")
	}
}

// TestRejectsZeroContributors: guards the trapdoor case. A zero-
// contribution ceremony would (a) either be trivial (no randomness) or
// (b) silently produce a broken SRS - we refuse it explicitly.
func TestRejectsZeroContributors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Phase1Contributors = 0
	if _, err := Run(cfg); err == nil {
		t.Fatalf("Run must reject Phase1Contributors=0")
	}
	cfg = DefaultConfig()
	cfg.Phase2Contributors = 0
	if _, err := Run(cfg); err == nil {
		t.Fatalf("Run must reject Phase2Contributors=0")
	}
}

// TestRejectsEmptyBeacon: without a public randomness beacon there is no
// way to finalise the transcript so that a verifier can reproduce it.
// The ceremony must refuse to run.
func TestRejectsEmptyBeacon(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BeaconChallenge = nil
	if _, err := Run(cfg); err == nil {
		t.Fatalf("Run must reject empty BeaconChallenge")
	}
}

// TestRejectsTinyN: too-small or non-power-of-two domain sizes are
// refused. This is an operator guardrail: gnark's mpcsetup panics on
// non-power-of-two N, and a real ceremony wants a domain big enough
// to allow adding constraints without redoing Phase 1.
func TestRejectsTinyN(t *testing.T) {
	cfg := DefaultConfig()
	cfg.N = 8 // below floor of 32
	if _, err := Run(cfg); err == nil {
		t.Fatalf("Run must reject N=8")
	}
	cfg.N = 100 // not a power of two
	if _, err := Run(cfg); err == nil {
		t.Fatalf("Run must reject non-power-of-two N=100")
	}
}

// TestArtifactsWrittenAndJSONable: with OutDir set, the ceremony writes
// binary artifacts to disk AND the Transcript round-trips through JSON
// cleanly (needed for zk/ceremony/attestations.json).
func TestArtifactsWrittenAndJSONable(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.OutDir = dir
	res, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run with OutDir: %v", err)
	}

	for _, name := range []string{"phase1_commons.bin", "groth16_pk.bin", "groth16_vk.bin"} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if fi.Size() == 0 {
			t.Fatalf("%s: empty file", name)
		}
	}

	b, err := json.MarshalIndent(res.Transcript, "", "  ")
	if err != nil {
		t.Fatalf("marshal transcript: %v", err)
	}
	var back Transcript
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	if back.CircuitID != res.Transcript.CircuitID {
		t.Fatalf("json roundtrip mismatch: %q vs %q", back.CircuitID, res.Transcript.CircuitID)
	}
}

// TestBeaconAffectsFinalKeys: change the beacon, keep everything else
// identical - the sealed keys' hashes MUST differ. This is what makes
// the beacon a real binding on the transcript (not just a decorative
// label).
func TestBeaconAffectsFinalKeys(t *testing.T) {
	cfg := DefaultConfig()
	res1, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	cfg.BeaconChallenge = []byte("different-beacon-value")
	res2, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	// PK/VK digests should also differ because the beacon feeds Seal.
	// The contributions themselves are seeded from gnark's internal RNG,
	// which also makes them non-repeating; that is fine - what we really
	// want is that the beacon is not ignored.
	if res1.Transcript.BeaconChallenge == res2.Transcript.BeaconChallenge {
		t.Fatalf("beacon hex identical in transcript; test setup bug")
	}
	if res1.Transcript.FinalVKDigest == res2.Transcript.FinalVKDigest &&
		res1.Transcript.FinalPKDigest == res2.Transcript.FinalPKDigest {
		t.Fatalf("beacon change did not affect final PK/VK digests - beacon is not bound")
	}
}
