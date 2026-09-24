package crypto

import (
	"bytes"
	"testing"
	"time"
)

func TestKeyRing_CreateAndSignPerAlgo(t *testing.T) {
	for _, algo := range SupportedAlgos() {
		algo := algo
		t.Run(string(algo), func(t *testing.T) {
			r := NewKeyRing()
			meta, err := r.CreateKey(algo)
			if err != nil {
				t.Fatalf("CreateKey(%s): %v", algo, err)
			}
			if !meta.Active {
				t.Fatalf("new key must be active, got meta=%+v", meta)
			}
			if meta.Version != 1 {
				t.Fatalf("first key must be v1, got v%d", meta.Version)
			}

			sig, id, err := r.Sign(algo, []byte("hello world"))
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if id != meta.ID {
				t.Fatalf("Sign used unexpected key: got %s want %s", id, meta.ID)
			}

			ok, err := r.Verify(id, []byte("hello world"), sig)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if !ok {
				t.Fatalf("signature must verify")
			}

			// Tamper.
			bad, err := r.Verify(id, []byte("hello WORLD"), sig)
			if err != nil {
				t.Fatalf("Verify tampered: %v", err)
			}
			if bad {
				t.Fatalf("tampered signature must NOT verify")
			}
		})
	}
}

func TestKeyRing_RotationRetiresPrevious(t *testing.T) {
	r := NewKeyRing()
	v1, err := r.CreateKey(AlgoEd25519)
	if err != nil {
		t.Fatalf("v1: %v", err)
	}
	sig1, id1, err := r.Sign(AlgoEd25519, []byte("payload"))
	if err != nil {
		t.Fatalf("sign v1: %v", err)
	}
	if id1 != v1.ID {
		t.Fatalf("id1 mismatch")
	}

	v2, err := r.RotateKey(AlgoEd25519)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("expected v2, got v%d", v2.Version)
	}
	if v2.ID == v1.ID {
		t.Fatalf("rotate must produce a different key ID")
	}

	// v1 metadata must now be retired.
	metaV1, err := r.GetMeta(v1.ID)
	if err != nil {
		t.Fatalf("get v1 meta: %v", err)
	}
	if metaV1.Active {
		t.Fatalf("v1 must be inactive after rotation")
	}
	if metaV1.RetiredAt == nil {
		t.Fatalf("v1 must carry a RetiredAt timestamp")
	}

	// Active signing now uses v2.
	_, id2, err := r.Sign(AlgoEd25519, []byte("payload"))
	if err != nil {
		t.Fatalf("sign v2: %v", err)
	}
	if id2 != v2.ID {
		t.Fatalf("post-rotate Sign must use v2")
	}

	// But v1's *existing* signature still verifies — the whole point of
	// the ring is that anchored signatures survive rotation.
	ok, err := r.Verify(v1.ID, []byte("payload"), sig1)
	if err != nil {
		t.Fatalf("verify v1 post-rotate: %v", err)
	}
	if !ok {
		t.Fatalf("v1 signature must still verify after v1 is retired")
	}
}

func TestKeyRing_MigrateSignatureUpgradesAlgo(t *testing.T) {
	r := NewKeyRing()
	// Old world: an ed25519 signature anchored somewhere.
	_, err := r.CreateKey(AlgoEd25519)
	if err != nil {
		t.Fatalf("create ed25519: %v", err)
	}
	msg := []byte("anchored proof digest")
	oldSig, oldID, err := r.Sign(AlgoEd25519, msg)
	if err != nil {
		t.Fatalf("sign old: %v", err)
	}

	// New world: a hybrid key is now the target for future anchoring.
	_, err = r.CreateKey(AlgoHybrid)
	if err != nil {
		t.Fatalf("create hybrid: %v", err)
	}

	newSig, newID, err := r.MigrateSignature(oldID, msg, oldSig, AlgoHybrid)
	if err != nil {
		t.Fatalf("MigrateSignature: %v", err)
	}
	if newID == oldID {
		t.Fatalf("migrated key ID must differ from old")
	}
	ok, err := r.Verify(newID, msg, newSig)
	if err != nil {
		t.Fatalf("verify migrated: %v", err)
	}
	if !ok {
		t.Fatalf("migrated signature must verify under the new hybrid key")
	}

	// Refusing to migrate a bogus old signature.
	_, _, err = r.MigrateSignature(oldID, msg, []byte("nope"), AlgoHybrid)
	if err == nil {
		t.Fatalf("bogus old signature must be rejected")
	}
}

func TestKeyRing_MarshalPublicRoundTrip_VerifyOnly(t *testing.T) {
	r := NewKeyRing()
	// Seed a mix of algos, one with rotation history.
	for _, algo := range []Algo{AlgoEd25519, AlgoMLDSA65, AlgoLamport, AlgoHybrid} {
		if _, err := r.CreateKey(algo); err != nil {
			t.Fatalf("seed %s: %v", algo, err)
		}
	}
	if _, err := r.RotateKey(AlgoEd25519); err != nil {
		t.Fatalf("rotate ed25519: %v", err)
	}

	// Sign under active ed25519 (v2) and verify it via a snapshot.
	sig, activeID, err := r.Sign(AlgoEd25519, []byte("snapshot test"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	snapshot, err := r.MarshalPublic()
	if err != nil {
		t.Fatalf("MarshalPublic: %v", err)
	}
	loaded, err := LoadPublicKeyRing(snapshot)
	if err != nil {
		t.Fatalf("LoadPublicKeyRing: %v", err)
	}

	// Loaded ring can verify but not sign.
	ok, err := loaded.Verify(activeID, []byte("snapshot test"), sig)
	if err != nil {
		t.Fatalf("verify loaded: %v", err)
	}
	if !ok {
		t.Fatalf("loaded snapshot must verify signature produced under original ring")
	}
	if _, _, err := loaded.Sign(AlgoEd25519, []byte("nope")); err == nil {
		t.Fatalf("loaded (verify-only) ring must refuse to Sign")
	}

	// The loaded ring must know the active key IDs for each algo.
	for _, algo := range []Algo{AlgoEd25519, AlgoMLDSA65, AlgoLamport, AlgoHybrid} {
		if _, ok := loaded.ActiveKeyID(algo); !ok {
			t.Fatalf("loaded ring missing active id for %s", algo)
		}
	}
}

func TestKeyRing_VersionMonotonicUnderClock(t *testing.T) {
	r := NewKeyRing()
	frozen := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.SetClock(func() time.Time { return frozen })
	m1, _ := r.CreateKey(AlgoMLDSA65)
	m2, _ := r.CreateKey(AlgoMLDSA65)
	if m1.CreatedAt != frozen || m2.CreatedAt != frozen {
		t.Fatalf("clock override must apply to CreatedAt: %v / %v", m1.CreatedAt, m2.CreatedAt)
	}
	if m2.Version != m1.Version+1 {
		t.Fatalf("versions must be monotonic: got %d then %d", m1.Version, m2.Version)
	}
}

func TestKeyRing_ListSorted(t *testing.T) {
	r := NewKeyRing()
	_, _ = r.CreateKey(AlgoMLDSA65)
	_, _ = r.CreateKey(AlgoEd25519)
	_, _ = r.RotateKey(AlgoEd25519)
	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(list))
	}
	if list[0].Algo != AlgoEd25519 || list[0].Version != 1 {
		t.Fatalf("first should be ed25519 v1, got %+v", list[0])
	}
	if list[1].Algo != AlgoEd25519 || list[1].Version != 2 {
		t.Fatalf("second should be ed25519 v2, got %+v", list[1])
	}
	if list[2].Algo != AlgoMLDSA65 {
		t.Fatalf("third should be mldsa65, got %+v", list[2])
	}
}

func TestKeyRing_UnknownKeyIDsRejected(t *testing.T) {
	r := NewKeyRing()
	if _, err := r.GetMeta("bogus"); err == nil {
		t.Fatalf("GetMeta bogus must error")
	}
	if _, err := r.SignWithKey("bogus", []byte("x")); err == nil {
		t.Fatalf("SignWithKey bogus must error")
	}
	if _, err := r.Verify("bogus", []byte("x"), []byte("y")); err == nil {
		t.Fatalf("Verify bogus must error")
	}
}

func TestKeyRing_HybridSignatureIntegrity(t *testing.T) {
	r := NewKeyRing()
	if _, err := r.CreateKey(AlgoHybrid); err != nil {
		t.Fatalf("create hybrid: %v", err)
	}
	msg := []byte("hybrid integrity")
	sig, id, err := r.Sign(AlgoHybrid, msg)
	if err != nil {
		t.Fatalf("sign hybrid: %v", err)
	}
	ok, err := r.Verify(id, msg, sig)
	if err != nil || !ok {
		t.Fatalf("hybrid must verify: ok=%v err=%v", ok, err)
	}
	// Flip a middle byte and expect rejection.
	tampered := bytes.Clone(sig)
	tampered[len(tampered)/2] ^= 0x01
	ok, _ = r.Verify(id, msg, tampered)
	if ok {
		t.Fatalf("tampered hybrid signature must not verify")
	}
}
