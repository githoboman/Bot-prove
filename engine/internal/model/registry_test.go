package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

// NOTE: the previous version of this file used fabricated SHA-256 test
// vectors (some were not even valid hex, e.g. containing a repeating
// "3f4a3f4d" pattern with no 65th char) and non-hex-64 placeholder hashes
// like "hash1"/"hash2" that fail this package's own (correct) 64-char-hex
// validation in Register(). It had never actually been run. Replaced with
// tests that use real SHA-256 output and the real Registry/ModelEntry API.

func hexHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestModelVersion_String(t *testing.T) {
	tests := []struct {
		name     string
		version  ModelVersion
		expected string
	}{
		{"empty version", ModelVersion{}, "0.0.0"},
		{"major only", ModelVersion{Major: 1}, "1.0.0"},
		{"minor only", ModelVersion{Minor: 2}, "0.2.0"},
		{"patch only", ModelVersion{Patch: 3}, "0.0.3"},
		{"full version", ModelVersion{Major: 1, Minor: 2, Patch: 3}, "1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.version.String(); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestComputeModelHash(t *testing.T) {
	// Verify against Go's own crypto/sha256, not a hand-copied literal.
	arch, weights, params := []byte{1, 2, 3}, []byte{4, 5, 6}, []byte{7, 8, 9}
	want := sha256.Sum256(append(append(append([]byte{}, arch...), weights...), params...))
	got := ComputeModelHash(arch, weights, params)
	if got != hex.EncodeToString(want[:]) {
		t.Errorf("expected %x, got %s", want, got)
	}
	if len(got) != 64 {
		t.Errorf("expected 64-char hex hash, got length %d", len(got))
	}
}

func TestComputeModelHash_Deterministic(t *testing.T) {
	h1 := ComputeModelHash([]byte("a"), []byte("b"), []byte("c"))
	h2 := ComputeModelHash([]byte("a"), []byte("b"), []byte("c"))
	if h1 != h2 {
		t.Error("expected identical inputs to produce identical hash")
	}
}

func TestRegistry_Register(t *testing.T) {
	r := New()

	m, err := r.Register("model1", "user1", hexHash("model1-weights"), "ipfs1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil model entry")
	}
	if m.Status != StatusActive {
		t.Errorf("expected new model to be active, got %s", m.Status)
	}

	m2, err := r.Register("model2", "user2", hexHash("model2-weights"), "ipfs2", "in-schema", "out-schema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m2.InputSchema != "in-schema" || m2.OutputSchema != "out-schema" {
		t.Errorf("expected schemas to be set, got %+v", m2)
	}
}

func TestRegistry_Register_Validation(t *testing.T) {
	r := New()
	if _, err := r.Register("", "user1", hexHash("x"), ""); err == nil {
		t.Error("expected error for empty name")
	}
	if _, err := r.Register("m", "", hexHash("x"), ""); err == nil {
		t.Error("expected error for empty owner")
	}
	if _, err := r.Register("m", "u", "not-a-valid-hash", ""); err == nil {
		t.Error("expected error for non-hex-64 hash")
	}

	hash := hexHash("dup")
	if _, err := r.Register("m1", "u1", hash, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := r.Register("m2", "u2", hash, ""); err == nil {
		t.Error("expected error for duplicate hash")
	}
}

func TestRegistry_GetByHash(t *testing.T) {
	r := New()
	hash1, hash2 := hexHash("model1"), hexHash("model2")
	if _, err := r.Register("model1", "user1", hash1, "ipfs1"); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, err := r.Register("model2", "user2", hash2, "ipfs2"); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	tests := []struct {
		name     string
		hash     string
		expected bool
	}{
		{"existing hash", hash1, true},
		{"another hash", hash2, true},
		{"non-existent hash", hexHash("nonexistent"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := r.GetByHash(tt.hash)
			if ok != tt.expected {
				t.Errorf("expected ok=%v, got %v", tt.expected, ok)
			}
			if ok && got == nil {
				t.Error("expected non-nil model when ok is true")
			}
		})
	}
}

func TestRegistry_Attest(t *testing.T) {
	r := New()
	m1, err := r.Register("model1", "user1", hexHash("model1"), "ipfs1")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	m3, err := r.Register("model3", "user3", hexHash("model3"), "ipfs3")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if err := r.Deprecate(m3.ID, "test deprecation"); err != nil {
		t.Fatalf("deprecate failed: %v", err)
	}

	a1, err := r.Attest(m1.ID, "attester1", "evidence1")
	if err != nil {
		t.Fatalf("attest failed: %v", err)
	}
	if !a1.Valid || a1.Hash != m1.Hash {
		t.Errorf("unexpected attestation: %+v", a1)
	}

	a2, err := r.Attest(m1.ID, "attester2", "evidence2")
	if err != nil {
		t.Fatalf("attest failed: %v", err)
	}
	if a2.AttesterID != "attester2" {
		t.Errorf("expected attester2, got %s", a2.AttesterID)
	}

	if _, err := r.Attest(m3.ID, "attester1", "evidence"); err == nil {
		t.Error("expected error attesting a deprecated model")
	}
	if _, err := r.Attest("nonexistent-id", "attester1", "evidence"); err == nil {
		t.Error("expected error attesting a nonexistent model")
	}

	updated, _ := r.Get(m1.ID)
	if updated.AttestationCount != 2 {
		t.Errorf("expected AttestationCount=2, got %d", updated.AttestationCount)
	}
}

func TestRegistry_MultipleAttestations(t *testing.T) {
	r := New()
	m, err := r.Register("model1", "user1", hexHash("model1"), "ipfs1")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, err := r.Attest(m.ID, fmt.Sprintf("attester%d", i), "evidence"); err != nil {
			t.Fatalf("attest %d failed: %v", i, err)
		}
	}

	atts := r.ListAttestations(m.ID)
	if len(atts) != 5 {
		t.Errorf("expected 5 attestations, got %d", len(atts))
	}
}

func TestRegistry_DeprecateAndRevoke(t *testing.T) {
	r := New()
	m, err := r.Register("model1", "user1", hexHash("model1"), "ipfs1")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := r.Deprecate(m.ID, "superseded"); err != nil {
		t.Fatalf("deprecate failed: %v", err)
	}
	got, _ := r.Get(m.ID)
	if got.Status != StatusDeprecated {
		t.Errorf("expected status deprecated, got %s", got.Status)
	}

	if err := r.Revoke(m.ID, "security issue"); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	got, _ = r.Get(m.ID)
	if got.Status != StatusRevoked {
		t.Errorf("expected status revoked, got %s", got.Status)
	}

	if err := r.Deprecate("nonexistent-id", "x"); err == nil {
		t.Error("expected error deprecating nonexistent model")
	}
}

func TestRegistry_VerifyIntegrity(t *testing.T) {
	r := New()
	hash := hexHash("model1")
	m, err := r.Register("model1", "user1", hash, "ipfs1")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if !r.VerifyIntegrity(m.ID, hash) {
		t.Error("expected integrity check to pass for the registered hash")
	}
	if r.VerifyIntegrity(m.ID, hexHash("tampered")) {
		t.Error("expected integrity check to fail for a different hash")
	}
	if r.VerifyIntegrity("nonexistent-id", hash) {
		t.Error("expected integrity check to fail for a nonexistent model")
	}
}

func TestRegistry_ListByOwnerAndSearch(t *testing.T) {
	r := New()
	if _, err := r.Register("alpha-model", "owner-a", hexHash("alpha"), ""); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, err := r.Register("beta-model", "owner-b", hexHash("beta"), ""); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, err := r.Register("alpha-model-2", "owner-a", hexHash("alpha2"), ""); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	byOwner := r.ListByOwner("owner-a")
	if len(byOwner) != 2 {
		t.Errorf("expected 2 models for owner-a, got %d", len(byOwner))
	}

	found := r.Search("alpha")
	if len(found) != 2 {
		t.Errorf("expected 2 models matching search 'alpha', got %d", len(found))
	}
}

func TestRegistry_Stats(t *testing.T) {
	r := New()
	m1, _ := r.Register("m1", "u1", hexHash("m1"), "")
	_, _ = r.Register("m2", "u2", hexHash("m2"), "")
	_ = r.Deprecate(m1.ID, "x")
	_, _ = r.Attest(m1.ID, "attester", "evidence") // will fail: deprecated, not counted

	stats := r.Stats()
	if stats["total_models"] != 2 {
		t.Errorf("expected total_models=2, got %v", stats["total_models"])
	}
}
