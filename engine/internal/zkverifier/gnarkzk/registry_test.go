package gnarkzk

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
)

// --- Registry basics -------------------------------------------------------

func TestRegistry_RegisterAndCompile(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register(MiMCPreimageCircuit{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	// duplicate rejected
	if err := r.Register(MiMCPreimageCircuit{}); err == nil {
		t.Fatal("expected duplicate register to error")
	}
	if r.DefaultID() != MiMCPreimageID {
		t.Fatalf("expected default %q, got %q", MiMCPreimageID, r.DefaultID())
	}
	if err := r.Compile(MiMCPreimageID); err != nil {
		t.Fatalf("compile: %v", err)
	}
	// second compile is a no-op
	if err := r.Compile(MiMCPreimageID); err != nil {
		t.Fatalf("re-compile: %v", err)
	}
	d, ok := r.Descriptor(MiMCPreimageID)
	if !ok {
		t.Fatal("descriptor not found after compile")
	}
	if d.Constraints == 0 {
		t.Fatal("expected non-zero constraints after compile")
	}
	if d.KeyDigest == "" {
		t.Fatal("expected non-empty key digest")
	}
}

func TestRegistry_IDsAndDescriptors_Sorted(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_ = r.Register(ModelInferenceCircuit{}) // registered first
	_ = r.Register(MiMCPreimageCircuit{})   // registered second
	ids := r.IDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %v", ids)
	}
	if ids[0] != MiMCPreimageID || ids[1] != ModelInferenceID {
		t.Fatalf("expected sorted [mimc..., model...] got %v", ids)
	}
	// SetDefault must reject unknown ids and accept known ones
	if err := r.SetDefault("does-not-exist"); err == nil {
		t.Fatal("expected SetDefault error for unknown id")
	}
	if err := r.SetDefault(ModelInferenceID); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	if r.DefaultID() != ModelInferenceID {
		t.Fatalf("expected default %q, got %q", ModelInferenceID, r.DefaultID())
	}
	descs := r.Descriptors()
	if len(descs) != 2 || descs[0].ID != MiMCPreimageID {
		t.Fatalf("descriptors not sorted: %+v", descs)
	}
}

// --- Prove + Verify (MiMC preimage, via registry) -------------------------

func TestRegistry_ProveVerify_MiMCPreimage(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register(MiMCPreimageCircuit{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Compile(MiMCPreimageID); err != nil {
		t.Fatalf("compile: %v", err)
	}
	preimage := big.NewInt(12345)
	proof, err := r.Prove(MiMCPreimageID, map[string]any{"preimage": preimage})
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	hash := ComputeMiMCHash(preimage)
	ok, err := r.Verify(MiMCPreimageID, proof, map[string]any{"hash": hash})
	if err != nil || !ok {
		t.Fatalf("verify: ok=%v err=%v", ok, err)
	}
	// wrong hash → false, no error
	wrong := new(big.Int).Add(hash, big.NewInt(1))
	ok, err = r.Verify(MiMCPreimageID, proof, map[string]any{"hash": wrong})
	if err != nil {
		t.Fatalf("verify tampered: unexpected err %v", err)
	}
	if ok {
		t.Fatal("expected tampered public input to fail verification")
	}
}

// --- Prove + Verify (model inference commitment) --------------------------

func TestRegistry_ProveVerify_ModelInference(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register(ModelInferenceCircuit{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Compile(ModelInferenceID); err != nil {
		t.Fatalf("compile: %v", err)
	}
	modelCommit := big.NewInt(111)
	input := big.NewInt(222)
	outHash := ComputeModelOutputHash(modelCommit, input)

	proof, err := r.Prove(ModelInferenceID, map[string]any{
		"model_commit": modelCommit,
		"input":        input,
	})
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	ok, err := r.Verify(ModelInferenceID, proof, map[string]any{
		"model_commit": modelCommit,
		"output_hash":  outHash,
	})
	if err != nil || !ok {
		t.Fatalf("verify: ok=%v err=%v", ok, err)
	}

	// tamper with output hash → verification fails
	ok, _ = r.Verify(ModelInferenceID, proof, map[string]any{
		"model_commit": modelCommit,
		"output_hash":  new(big.Int).Add(outHash, big.NewInt(1)),
	})
	if ok {
		t.Fatal("expected tampered output_hash to fail verification")
	}
	// tamper with model_commit → verification fails
	ok, _ = r.Verify(ModelInferenceID, proof, map[string]any{
		"model_commit": new(big.Int).Add(modelCommit, big.NewInt(1)),
		"output_hash":  outHash,
	})
	if ok {
		t.Fatal("expected tampered model_commit to fail verification")
	}
}

// --- Persistence -----------------------------------------------------------

func TestRegistry_LoadOrCreate_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// First bootstrap: cold cache → files generated.
	r1 := NewRegistry()
	if err := r1.Register(MiMCPreimageCircuit{}); err != nil {
		t.Fatalf("register r1: %v", err)
	}
	manifest, err := r1.LoadOrCreate(MiMCPreimageID, dir, false)
	if err != nil {
		t.Fatalf("first LoadOrCreate: %v", err)
	}
	if manifest.CircuitID != MiMCPreimageID || manifest.Backend != "groth16" || manifest.Curve != "BN254" {
		t.Fatalf("manifest fields wrong: %+v", manifest)
	}
	if manifest.CCSDigest == "" || manifest.PKDigest == "" || manifest.VKDigest == "" {
		t.Fatalf("manifest digests empty: %+v", manifest)
	}
	if !strings.Contains(manifest.Warning, "session-local") {
		t.Fatalf("expected session-local warning, got %q", manifest.Warning)
	}

	// prove/verify on the persisted setup
	preimage := big.NewInt(42)
	proof1, err := r1.Prove(MiMCPreimageID, map[string]any{"preimage": preimage})
	if err != nil {
		t.Fatalf("prove r1: %v", err)
	}
	hash := ComputeMiMCHash(preimage)

	// Second bootstrap: hot cache → same keys reloaded, digests match.
	r2 := NewRegistry()
	if err := r2.Register(MiMCPreimageCircuit{}); err != nil {
		t.Fatalf("register r2: %v", err)
	}
	manifest2, err := r2.LoadOrCreate(MiMCPreimageID, dir, false)
	if err != nil {
		t.Fatalf("second LoadOrCreate: %v", err)
	}
	if manifest2.VKDigest != manifest.VKDigest {
		t.Fatalf("vk digest changed across reloads: %s vs %s", manifest2.VKDigest, manifest.VKDigest)
	}
	// The key point: proof from r1 verifies under r2's reloaded vk.
	ok, err := r2.Verify(MiMCPreimageID, proof1, map[string]any{"hash": hash})
	if err != nil {
		t.Fatalf("verify r2: %v", err)
	}
	if !ok {
		t.Fatal("expected r1's proof to verify under r2's reloaded vk")
	}

	// LoadManifest returns the same disk record
	disk, err := LoadManifest(MiMCPreimageID, dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if disk.VKDigest != manifest.VKDigest {
		t.Fatalf("disk manifest digest mismatch: %s vs %s", disk.VKDigest, manifest.VKDigest)
	}

	// Force regenerate → digests change, previous proof no longer verifies.
	r3 := NewRegistry()
	if err := r3.Register(MiMCPreimageCircuit{}); err != nil {
		t.Fatalf("register r3: %v", err)
	}
	manifest3, err := r3.LoadOrCreate(MiMCPreimageID, dir, true)
	if err != nil {
		t.Fatalf("force LoadOrCreate: %v", err)
	}
	if manifest3.VKDigest == manifest.VKDigest {
		t.Fatal("expected different vk digest after force-regenerate")
	}
	ok, _ = r3.Verify(MiMCPreimageID, proof1, map[string]any{"hash": hash})
	if ok {
		t.Fatal("expected old proof to NOT verify under regenerated vk")
	}
}

// --- Manifest corruption handling -----------------------------------------

func TestRegistry_LoadOrCreate_RecoversFromMissingFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a manifest-only directory (no ccs/pk/vk) — must fall through
	// to regenerate rather than error out.
	if err := saveJSONAtomic(filepath.Join(dir, MiMCPreimageID, "manifest.json"),
		&KeyManifest{CircuitID: MiMCPreimageID, Backend: "groth16", Curve: "BN254", CCSDigest: "aa", PKDigest: "bb", VKDigest: "cc"}); err != nil {
		// mkdir dance: the helper doesn't create dirs
		if err := mkAll(filepath.Join(dir, MiMCPreimageID)); err != nil {
			t.Fatalf("mkAll: %v", err)
		}
		if err := saveJSONAtomic(filepath.Join(dir, MiMCPreimageID, "manifest.json"),
			&KeyManifest{CircuitID: MiMCPreimageID, Backend: "groth16", Curve: "BN254", CCSDigest: "aa", PKDigest: "bb", VKDigest: "cc"}); err != nil {
			t.Fatalf("seed manifest: %v", err)
		}
	}

	r := NewRegistry()
	if err := r.Register(MiMCPreimageCircuit{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	manifest, err := r.LoadOrCreate(MiMCPreimageID, dir, false)
	if err != nil {
		t.Fatalf("LoadOrCreate over partial state: %v", err)
	}
	// digests must be real (64-hex sha256), not the seeded "aa"/"bb"/"cc"
	if len(manifest.VKDigest) != 64 {
		t.Fatalf("expected regenerated 64-hex vk digest, got %q", manifest.VKDigest)
	}
	if _, err := hex.DecodeString(manifest.VKDigest); err != nil {
		t.Fatalf("vk digest not hex: %v", err)
	}
}

// --- Verifying key export --------------------------------------------------

func TestRegistry_VerifyingKey_Serializable(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_ = r.Register(MiMCPreimageCircuit{})
	if err := r.Compile(MiMCPreimageID); err != nil {
		t.Fatalf("compile: %v", err)
	}
	vk, err := r.VerifyingKey(MiMCPreimageID)
	if err != nil {
		t.Fatalf("vk: %v", err)
	}
	// Round-trip it via gnark's own reader to ensure it's a well-formed
	// BN254 verifying key.
	var buf bytes.Buffer
	if _, err := vk.WriteTo(&buf); err != nil {
		t.Fatalf("vk.WriteTo: %v", err)
	}
	vk2 := groth16.NewVerifyingKey(ecc.BN254)
	if _, err := vk2.ReadFrom(&buf); err != nil {
		t.Fatalf("vk round-trip: %v", err)
	}
}

// mkAll is a tiny helper so tests can seed intermediate dirs.
func mkAll(path string) error { return os.MkdirAll(path, 0o755) }
