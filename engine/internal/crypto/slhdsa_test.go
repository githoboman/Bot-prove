package crypto

import (
	"testing"
)

func TestSLHDSA_Roundtrip_All128s(t *testing.T) {
	// 128s is the fastest to key-gen; test the full roundtrip.
	kp, err := SLHDSAKeygen(SLHDSA128s)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if len(kp.Public) == 0 || len(kp.Private) == 0 {
		t.Fatal("empty key bytes")
	}
	msg := []byte("cp-slh-dsa-probe")
	sig, err := SLHDSASign(SLHDSA128s, kp.Private, msg, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := SLHDSAVerify(SLHDSA128s, kp.Public, msg, nil, sig); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestSLHDSA_TamperedSig_Rejected(t *testing.T) {
	kp, err := SLHDSAKeygen(SLHDSA128s)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	msg := []byte("tamper-probe")
	sig, err := SLHDSASign(SLHDSA128s, kp.Private, msg, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig[100] ^= 0xff
	if err := SLHDSAVerify(SLHDSA128s, kp.Public, msg, nil, sig); err == nil {
		t.Fatal("expected verify to fail on tampered signature")
	}
}

func TestSLHDSA_TamperedMessage_Rejected(t *testing.T) {
	kp, err := SLHDSAKeygen(SLHDSA128s)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	sig, err := SLHDSASign(SLHDSA128s, kp.Private, []byte("m1"), nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := SLHDSAVerify(SLHDSA128s, kp.Public, []byte("m2"), nil, sig); err == nil {
		t.Fatal("expected verify to fail on different message")
	}
}

func TestSLHDSA_WrongKey_Rejected(t *testing.T) {
	kpA, err := SLHDSAKeygen(SLHDSA128s)
	if err != nil {
		t.Fatalf("keygen A: %v", err)
	}
	kpB, err := SLHDSAKeygen(SLHDSA128s)
	if err != nil {
		t.Fatalf("keygen B: %v", err)
	}
	msg := []byte("m")
	sig, err := SLHDSASign(SLHDSA128s, kpA.Private, msg, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := SLHDSAVerify(SLHDSA128s, kpB.Public, msg, nil, sig); err == nil {
		t.Fatal("expected verify to fail with unrelated pk")
	}
}

func TestSLHDSA_Determinism(t *testing.T) {
	kp, err := SLHDSAKeygen(SLHDSA128s)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	msg := []byte("determinism")
	s1, err := SLHDSASign(SLHDSA128s, kp.Private, msg, nil)
	if err != nil {
		t.Fatalf("sign1: %v", err)
	}
	s2, err := SLHDSASign(SLHDSA128s, kp.Private, msg, nil)
	if err != nil {
		t.Fatalf("sign2: %v", err)
	}
	if len(s1) != len(s2) {
		t.Fatalf("sig length mismatch: %d vs %d", len(s1), len(s2))
	}
	// SignDeterministic is deterministic: two signatures of the same
	// (priv, msg, ctx) must be byte-identical.
	for i := range s1 {
		if s1[i] != s2[i] {
			t.Fatalf("deterministic signature mismatch at byte %d", i)
		}
	}
}

func TestSLHDSA_UnknownParamSet_Errors(t *testing.T) {
	if _, err := SLHDSAKeygen("bogus-param"); err == nil {
		t.Fatal("expected error on unknown parameter set")
	}
}
