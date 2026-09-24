package crypto

import "testing"

// These are real sign-then-verify round trips against distinct, freshly
// generated key pairs. A prior version of this package had Sign and Verify
// derive their internal hash input from different key halves (Sign used the
// private key, Verify used the public key, which is not a prefix of the
// private key) so Verify(pub, msg, Sign(priv, msg)) never returned true for
// two independently generated keys - that bug is why these tests exist.

func TestClassicEd25519_SignVerifyRoundTrip(t *testing.T) {
	priv, pub, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}
	msg := []byte("hello casperprover")
	sig, err := SignClassic(priv, msg)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	valid, err := VerifyClassic(pub, msg, sig)
	if err != nil || !valid {
		t.Fatalf("expected valid signature, got valid=%v err=%v", valid, err)
	}
}

func TestClassicEd25519_RejectsTamperedMessageAndSignature(t *testing.T) {
	priv, pub, _ := GenerateEd25519KeyPair()
	msg := []byte("hello casperprover")
	sig, _ := SignClassic(priv, msg)

	if valid, _ := VerifyClassic(pub, []byte("tampered message"), sig); valid {
		t.Error("expected tampered message to be rejected")
	}
	tamperedSig := append([]byte(nil), sig...)
	tamperedSig[0] ^= 0xFF
	if valid, _ := VerifyClassic(pub, msg, tamperedSig); valid {
		t.Error("expected tampered signature to be rejected")
	}
}

func TestMLDSA_SignVerifyRoundTrip(t *testing.T) {
	priv, pub, err := GenerateMLDSAKeyPair()
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}
	msg := []byte("hello casperprover")
	sig, err := SignMLDSA(priv, msg)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	valid, err := VerifyMLDSA(pub, msg, sig)
	if err != nil || !valid {
		t.Fatalf("expected valid signature, got valid=%v err=%v", valid, err)
	}
}

func TestMLDSA_RejectsTamperedMessageAndSignature(t *testing.T) {
	priv, pub, _ := GenerateMLDSAKeyPair()
	msg := []byte("hello casperprover")
	sig, _ := SignMLDSA(priv, msg)

	if valid, _ := VerifyMLDSA(pub, []byte("tampered message"), sig); valid {
		t.Error("expected tampered message to be rejected")
	}
	tamperedSig := append([]byte(nil), sig...)
	tamperedSig[0] ^= 0xFF
	if valid, _ := VerifyMLDSA(pub, msg, tamperedSig); valid {
		t.Error("expected tampered signature to be rejected")
	}
}

func TestMLDSA_RejectsWrongKeyPair(t *testing.T) {
	_, pub1, _ := GenerateMLDSAKeyPair()
	priv2, _, _ := GenerateMLDSAKeyPair()
	msg := []byte("hello casperprover")
	sig, _ := SignMLDSA(priv2, msg)
	if valid, _ := VerifyMLDSA(pub1, msg, sig); valid {
		t.Error("expected signature from a different key pair to be rejected")
	}
}

func TestLamportOTS_SignVerifyRoundTrip(t *testing.T) {
	priv, pub, err := GenerateLamportKeyPair()
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}
	msg := []byte("hello casperprover")
	sig, err := SignSPHINCS(priv, msg)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	valid, err := VerifySPHINCS(pub, msg, sig)
	if err != nil || !valid {
		t.Fatalf("expected valid signature, got valid=%v err=%v", valid, err)
	}
}

func TestLamportOTS_RejectsTamperedMessageAndSignature(t *testing.T) {
	priv, pub, _ := GenerateLamportKeyPair()
	msg := []byte("hello casperprover")
	sig, _ := SignSPHINCS(priv, msg)

	if valid, _ := VerifySPHINCS(pub, []byte("tampered message"), sig); valid {
		t.Error("expected tampered message to be rejected")
	}
	tamperedSig := append([]byte(nil), sig...)
	tamperedSig[0] ^= 0xFF
	if valid, _ := VerifySPHINCS(pub, msg, tamperedSig); valid {
		t.Error("expected tampered signature to be rejected")
	}
}

func TestLamportOTS_RejectsWrongKeyPair(t *testing.T) {
	_, pub1, _ := GenerateLamportKeyPair()
	priv2, _, _ := GenerateLamportKeyPair()
	msg := []byte("hello casperprover")
	sig, _ := SignSPHINCS(priv2, msg)
	if valid, _ := VerifySPHINCS(pub1, msg, sig); valid {
		t.Error("expected signature from a different key pair to be rejected")
	}
}

func TestHybridSignature_MarshalUnmarshalRoundTrip(t *testing.T) {
	hs := &HybridSignature{ClassicSig: []byte("classic-sig"), PQSig: []byte("pq-sig-bytes")}
	data, err := hs.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var out HybridSignature
	if err := out.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if string(out.ClassicSig) != string(hs.ClassicSig) || string(out.PQSig) != string(hs.PQSig) {
		t.Error("round-tripped hybrid signature does not match original")
	}
}

func TestHybridSignVerify_RoundTrip(t *testing.T) {
	classicPriv, classicPub, _ := GenerateEd25519KeyPair()
	pqPriv, pqPub, _ := GenerateMLDSAKeyPair()
	msg := []byte("hello casperprover")

	sig, err := HybridSign(classicPriv, pqPriv, msg)
	if err != nil {
		t.Fatalf("HybridSign failed: %v", err)
	}
	valid, classicValid, pqValid, err := HybridVerify(classicPub, pqPub, msg, sig)
	if err != nil || !valid || !classicValid || !pqValid {
		t.Fatalf("expected valid hybrid signature, got valid=%v classic=%v pq=%v err=%v", valid, classicValid, pqValid, err)
	}
}

func TestHybridVerify_RejectsIfEitherComponentInvalid(t *testing.T) {
	classicPriv, classicPub, _ := GenerateEd25519KeyPair()
	pqPriv, pqPub, _ := GenerateMLDSAKeyPair()
	otherPQPriv, _, _ := GenerateMLDSAKeyPair()
	msg := []byte("hello casperprover")

	// Valid classic component, PQ component signed with the wrong key.
	sig, err := HybridSign(classicPriv, otherPQPriv, msg)
	if err != nil {
		t.Fatalf("HybridSign failed: %v", err)
	}
	valid, classicValid, pqValid, _ := HybridVerify(classicPub, pqPub, msg, sig)
	if valid {
		t.Error("expected hybrid verification to fail when the PQ component doesn't match")
	}
	if !classicValid {
		t.Error("expected classic component to still be valid on its own")
	}
	if pqValid {
		t.Error("expected pq component to be invalid since it was signed with the wrong key")
	}
	_ = pqPriv
}
