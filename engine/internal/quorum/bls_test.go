package quorum

import (
	"bytes"
	"testing"
)

func TestGenerateAndSignVerify(t *testing.T) {
	sk, pk, err := GenerateSecretKey()
	if err != nil {
		t.Fatalf("GenerateSecretKey: %v", err)
	}
	msg := []byte("evidence-root-A")
	sig, err := sk.Sign(msg)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := Verify(pk, msg, sig); err != nil {
		t.Fatalf("Verify(good) = %v, want nil", err)
	}
	// tamper msg
	if err := Verify(pk, []byte("evidence-root-B"), sig); err == nil {
		t.Fatal("Verify(tampered msg) = nil, want error")
	}
	// tamper pk (fresh key)
	_, pk2, _ := GenerateSecretKey()
	if err := Verify(pk2, msg, sig); err == nil {
		t.Fatal("Verify(wrong pk) = nil, want error")
	}
}

func TestEmptyMessageRejected(t *testing.T) {
	sk, _, _ := GenerateSecretKey()
	if _, err := sk.Sign(nil); err != ErrEmptyMessage {
		t.Fatalf("Sign(nil) err=%v, want %v", err, ErrEmptyMessage)
	}
}

func TestPubKeyRoundtrip(t *testing.T) {
	_, pk, _ := GenerateSecretKey()
	b, err := pk.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if len(b) != 96 {
		t.Fatalf("pk bytes = %d, want 96", len(b))
	}
	pk2, err := UnmarshalPubKey(b)
	if err != nil {
		t.Fatalf("UnmarshalPubKey: %v", err)
	}
	if !pk.Point.Equal(&pk2.Point) {
		t.Fatal("roundtripped pk mismatch")
	}
}

func TestSignatureRoundtrip(t *testing.T) {
	sk, _, _ := GenerateSecretKey()
	sig, _ := sk.Sign([]byte("msg"))
	b, err := sig.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if len(b) != 48 {
		t.Fatalf("sig bytes = %d, want 48", len(b))
	}
	sig2, err := UnmarshalSignature(b)
	if err != nil {
		t.Fatalf("UnmarshalSignature: %v", err)
	}
	if !sig.Point.Equal(&sig2.Point) {
		t.Fatal("roundtripped sig mismatch")
	}
}

func TestUnmarshalGarbage(t *testing.T) {
	if _, err := UnmarshalPubKey([]byte{}); err == nil {
		t.Fatal("empty pk accepted")
	}
	if _, err := UnmarshalPubKey(bytes.Repeat([]byte{0xff}, 96)); err == nil {
		t.Fatal("garbage 96B pk accepted")
	}
	if _, err := UnmarshalSignature([]byte{}); err == nil {
		t.Fatal("empty sig accepted")
	}
	if _, err := UnmarshalSignature(bytes.Repeat([]byte{0xff}, 48)); err == nil {
		t.Fatal("garbage 48B sig accepted")
	}
}

// -----------------------------------------------------------------------------
// Aggregate semantics
// -----------------------------------------------------------------------------

func TestAggregateSameMessage(t *testing.T) {
	msg := []byte("evidence-root-quorum-1")
	skA, pkA, _ := GenerateSecretKey()
	skB, pkB, _ := GenerateSecretKey()
	skC, pkC, _ := GenerateSecretKey()
	sigA, _ := skA.Sign(msg)
	sigB, _ := skB.Sign(msg)
	sigC, _ := skC.Sign(msg)

	aggSig, err := Aggregate([]*Signature{sigA, sigB, sigC})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	aggPK, err := AggregatePubKeys([]*PublicKey{pkA, pkB, pkC})
	if err != nil {
		t.Fatalf("AggregatePubKeys: %v", err)
	}
	if err := Verify(aggPK, msg, aggSig); err != nil {
		t.Fatalf("Verify(agg) = %v, want nil", err)
	}
	// Order-invariance: sum is commutative
	aggSig2, _ := Aggregate([]*Signature{sigC, sigA, sigB})
	if !aggSig.Point.Equal(&aggSig2.Point) {
		t.Fatal("agg signature not order-invariant")
	}
	aggPK2, _ := AggregatePubKeys([]*PublicKey{pkB, pkC, pkA})
	if !aggPK.Point.Equal(&aggPK2.Point) {
		t.Fatal("agg pubkey not order-invariant")
	}
}

func TestAggregateOneMissingBreaksVerification(t *testing.T) {
	msg := []byte("evidence")
	skA, pkA, _ := GenerateSecretKey()
	skB, pkB, _ := GenerateSecretKey()
	_, pkC, _ := GenerateSecretKey()

	sigA, _ := skA.Sign(msg)
	sigB, _ := skB.Sign(msg)
	// Missing sigC in the aggregate
	aggSig, _ := Aggregate([]*Signature{sigA, sigB})
	// But the caller lies about who signed, includes C
	aggPK, _ := AggregatePubKeys([]*PublicKey{pkA, pkB, pkC})
	if err := Verify(aggPK, msg, aggSig); err == nil {
		t.Fatal("verify passed on missing signer — soundness broken")
	}
	// Test the honest case still passes.
	honestPK, _ := AggregatePubKeys([]*PublicKey{pkA, pkB})
	if err := Verify(honestPK, msg, aggSig); err != nil {
		t.Fatalf("honest verify failed: %v", err)
	}
}

func TestAggregateEmpty(t *testing.T) {
	if _, err := Aggregate(nil); err != ErrEmptySignatures {
		t.Fatalf("Aggregate(nil) err=%v", err)
	}
	if _, err := AggregatePubKeys(nil); err != ErrEmptyPubKey {
		t.Fatalf("AggregatePubKeys(nil) err=%v", err)
	}
}

// -----------------------------------------------------------------------------
// Threshold arithmetic
// -----------------------------------------------------------------------------

func TestByzantineThreshold(t *testing.T) {
	cases := []struct {
		n, want int
	}{
		{0, 0},
		{1, 1},   // clamped to n (no BFT for tiny committee)
		{2, 2},   // clamped to n
		{3, 3},   // clamped to n (floor(6/3)+1 = 3, == n)
		{4, 3},   // floor(8/3)+1 = 2+1 = 3 (tolerates f=1)
		{5, 4},   // floor(10/3)+1 = 3+1 = 4
		{7, 5},   // floor(14/3)+1 = 4+1 = 5 (tolerates f=2)
		{10, 7},  // floor(20/3)+1 = 6+1 = 7
		{100, 67}, // floor(200/3)+1 = 66+1 = 67
	}
	for _, c := range cases {
		if got := ByzantineThreshold(c.n); got != c.want {
			t.Errorf("ByzantineThreshold(%d)=%d, want %d", c.n, got, c.want)
		}
	}
}
