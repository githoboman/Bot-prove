package keystore

import (
	"context"
	"testing"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
)

func TestMemoryKeystore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	ks := NewMemory(nil)

	info := ks.Info(ctx)
	if info.Kind != KindMemory {
		t.Fatalf("kind: want %q, got %q", KindMemory, info.Kind)
	}
	if info.Persistent || info.HardwareBacked {
		t.Fatal("memory keystore must never claim persistence or hardware backing")
	}
	if info.KeyCount != 0 {
		t.Fatalf("fresh ring should have 0 keys, got %d", info.KeyCount)
	}

	meta, err := ks.CreateKey(ctx, pqcrypto.AlgoEd25519)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if meta.Algo != pqcrypto.AlgoEd25519 || !meta.Active {
		t.Fatalf("bad meta: %+v", meta)
	}

	msg := []byte("hello")
	sig, id, err := ks.Sign(ctx, pqcrypto.AlgoEd25519, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if id != meta.ID {
		t.Fatalf("sign returned id %q, want %q", id, meta.ID)
	}
	ok, err := ks.Verify(ctx, id, msg, sig)
	if err != nil || !ok {
		t.Fatalf("verify: ok=%v err=%v", ok, err)
	}
}

func TestMemoryKeystore_Migrate(t *testing.T) {
	ctx := context.Background()
	ks := NewMemory(nil)

	// Old key: ed25519
	oldMeta, err := ks.CreateKey(ctx, pqcrypto.AlgoEd25519)
	if err != nil {
		t.Fatalf("create old: %v", err)
	}
	msg := []byte("upgrade me")
	oldSig, _, err := ks.Sign(ctx, pqcrypto.AlgoEd25519, msg)
	if err != nil {
		t.Fatalf("sign old: %v", err)
	}

	// New key: hybrid
	if _, err := ks.CreateKey(ctx, pqcrypto.AlgoHybrid); err != nil {
		t.Fatalf("create new: %v", err)
	}

	newSig, newID, err := ks.MigrateSignature(ctx, oldMeta.ID, msg, oldSig, pqcrypto.AlgoHybrid)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ok, err := ks.Verify(ctx, newID, msg, newSig)
	if err != nil || !ok {
		t.Fatalf("migrated sig verify: ok=%v err=%v", ok, err)
	}
}

func TestMemoryKeystore_TamperFails(t *testing.T) {
	ctx := context.Background()
	ks := NewMemory(nil)
	if _, err := ks.CreateKey(ctx, pqcrypto.AlgoEd25519); err != nil {
		t.Fatalf("create: %v", err)
	}
	sig, id, err := ks.Sign(ctx, pqcrypto.AlgoEd25519, []byte("msg"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig[0] ^= 0xff
	ok, _ := ks.Verify(ctx, id, []byte("msg"), sig)
	if ok {
		t.Fatal("tampered signature must not verify")
	}
}
