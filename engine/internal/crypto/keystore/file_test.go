package keystore

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
)

// makeFileKS constructs a fresh FileKeystore in a temp directory. Caller
// passes any custom passphrase; the resulting keystore is fully wired.
func makeFileKS(t *testing.T, pass string) (*FileKeystore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ks.bin")
	fk, err := NewFile(path, []byte(pass))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return fk, path
}

func TestFileKeystore_Persistence(t *testing.T) {
	ctx := context.Background()
	fk, path := makeFileKS(t, "correct-horse-battery-staple")

	meta, err := fk.CreateKey(ctx, pqcrypto.AlgoLamport)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sig, id, err := fk.Sign(ctx, pqcrypto.AlgoLamport, []byte("persisted"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Reopen the file with the same passphrase — signatures should still
	// verify and metadata should round-trip byte-for-byte.
	fk2, err := NewFile(path, []byte("correct-horse-battery-staple"))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := fk2.GetMeta(ctx, id)
	if err != nil {
		t.Fatalf("get meta reopened: %v", err)
	}
	if got.ID != meta.ID || got.PublicKey != meta.PublicKey {
		t.Fatalf("meta mismatch after reopen: got %+v want %+v", got, meta)
	}
	ok, err := fk2.Verify(ctx, id, []byte("persisted"), sig)
	if err != nil || !ok {
		t.Fatalf("verify after reopen: ok=%v err=%v", ok, err)
	}

	// Sign a fresh message via the reopened keystore — proves private
	// material also survived.
	sig2, _, err := fk2.Sign(ctx, pqcrypto.AlgoLamport, []byte("second"))
	if err != nil {
		t.Fatalf("sign reopened: %v", err)
	}
	ok, err = fk2.Verify(ctx, id, []byte("second"), sig2)
	if err != nil || !ok {
		t.Fatalf("verify reopened second sig: ok=%v err=%v", ok, err)
	}
}

func TestFileKeystore_WrongPassphraseRejected(t *testing.T) {
	ctx := context.Background()
	fk, path := makeFileKS(t, "right-pass")
	if _, err := fk.CreateKey(ctx, pqcrypto.AlgoEd25519); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := NewFile(path, []byte("wrong-pass")); err == nil {
		t.Fatal("wrong passphrase must not decrypt")
	}
}

func TestFileKeystore_TamperedFileRejected(t *testing.T) {
	ctx := context.Background()
	fk, path := makeFileKS(t, "pass")
	if _, err := fk.CreateKey(ctx, pqcrypto.AlgoEd25519); err != nil {
		t.Fatalf("create: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Flip a byte deep in the ciphertext body.
	if len(raw) < fileHeaderSize+10 {
		t.Fatalf("file too small: %d", len(raw))
	}
	raw[fileHeaderSize+5] ^= 0x80
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := NewFile(path, []byte("pass")); err == nil {
		t.Fatal("tampered ciphertext must fail decrypt (auth tag)")
	}
}

func TestFileKeystore_Rewrap(t *testing.T) {
	ctx := context.Background()
	fk, path := makeFileKS(t, "old-pass")
	if _, err := fk.CreateKey(ctx, pqcrypto.AlgoEd25519); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := fk.Rewrap([]byte("new-pass")); err != nil {
		t.Fatalf("rewrap: %v", err)
	}
	// Old passphrase must fail.
	if _, err := NewFile(path, []byte("old-pass")); err == nil {
		t.Fatal("old passphrase must no longer decrypt after rewrap")
	}
	// New passphrase must succeed.
	if _, err := NewFile(path, []byte("new-pass")); err != nil {
		t.Fatalf("reopen with new pass: %v", err)
	}
}

func TestFileKeystore_HybridPersists(t *testing.T) {
	// Hybrid keys carry TWO private key halves; make sure both survive
	// disk round-trip and produce a hybrid signature that verifies.
	ctx := context.Background()
	fk, path := makeFileKS(t, "h")
	if _, err := fk.CreateKey(ctx, pqcrypto.AlgoHybrid); err != nil {
		t.Fatalf("create hybrid: %v", err)
	}
	msg := []byte("hybrid")
	sig, id, err := fk.Sign(ctx, pqcrypto.AlgoHybrid, msg)
	if err != nil {
		t.Fatalf("sign hybrid: %v", err)
	}

	fk2, err := NewFile(path, []byte("h"))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	// Verify the sig produced pre-reopen with the rehydrated ring.
	ok, err := fk2.Verify(ctx, id, msg, sig)
	if err != nil || !ok {
		t.Fatalf("hybrid verify after reopen: ok=%v err=%v", ok, err)
	}
	// And produce a fresh hybrid sig using the rehydrated private keys.
	sig2, _, err := fk2.Sign(ctx, pqcrypto.AlgoHybrid, msg)
	if err != nil {
		t.Fatalf("sign hybrid reopened: %v", err)
	}
	if hex.EncodeToString(sig2) == hex.EncodeToString(sig) {
		// Hybrid signatures include randomness from ML-DSA; identical
		// output on the same input would be surprising and worth a
		// note in the diff.
		t.Log("note: hybrid sig deterministic on reopen — check ML-DSA randomness path")
	}
}

func TestFileKeystore_Info(t *testing.T) {
	ctx := context.Background()
	fk, path := makeFileKS(t, "p")
	if _, err := fk.CreateKey(ctx, pqcrypto.AlgoEd25519); err != nil {
		t.Fatalf("create: %v", err)
	}
	info := fk.Info(ctx)
	if info.Kind != KindFile || !info.Persistent {
		t.Fatalf("bad info: %+v", info)
	}
	if info.HardwareBacked {
		t.Fatal("file keystore must not claim hardware backing")
	}
	if info.KeyCount != 1 {
		t.Fatalf("count: want 1, got %d", info.KeyCount)
	}
	if info.Backing != "encrypted file at "+path {
		t.Fatalf("backing: %q", info.Backing)
	}
}

func TestFileKeystore_FactoryFromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.bin")
	t.Setenv("CP_KEYSTORE_KIND", "file")
	t.Setenv("CP_KEYSTORE_PATH", path)
	t.Setenv("CP_KEYSTORE_PASSPHRASE", "env-pass")

	ks, backing, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if backing != "file at "+path {
		t.Fatalf("backing: %q", backing)
	}
	if _, err := ks.CreateKey(context.Background(), pqcrypto.AlgoEd25519); err != nil {
		t.Fatalf("create via env-configured keystore: %v", err)
	}
}

func TestFileKeystore_MissingPassphraseRejected(t *testing.T) {
	t.Setenv("CP_KEYSTORE_KIND", "file")
	t.Setenv("CP_KEYSTORE_PATH", "/tmp/should-not-be-touched")
	t.Setenv("CP_KEYSTORE_PASSPHRASE", "")
	if _, _, err := FromEnv(); err == nil {
		t.Fatal("factory must reject empty CP_KEYSTORE_PASSPHRASE")
	}
}
