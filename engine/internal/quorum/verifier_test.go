package quorum

import (
	"errors"
	"testing"
)

// Helpers -------------------------------------------------------------

type committeeMember struct {
	id  string
	sk  *SecretKey
	pk  *PublicKey
}

func newCommittee(t *testing.T, n int) []committeeMember {
	t.Helper()
	out := make([]committeeMember, 0, n)
	for i := 0; i < n; i++ {
		sk, pk, err := GenerateSecretKey()
		if err != nil {
			t.Fatalf("GenerateSecretKey: %v", err)
		}
		out = append(out, committeeMember{
			id: string(rune('A' + i)),
			sk: sk,
			pk: pk,
		})
	}
	return out
}

// Registry -----------------------------------------------------------

func TestRegistryRegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	_, pk, _ := GenerateSecretKey()
	if err := r.Register("A", pk, 100); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register("A", pk, 100); err == nil {
		t.Fatal("duplicate Register accepted")
	}
	if err := r.Register("", pk, 100); err == nil {
		t.Fatal("empty id accepted")
	}
	if err := r.Register("B", nil, 100); err != ErrEmptyPubKey {
		t.Fatalf("nil pk: err=%v, want %v", err, ErrEmptyPubKey)
	}
}

func TestRegistrySlashIdempotent(t *testing.T) {
	r := NewRegistry()
	_, pk, _ := GenerateSecretKey()
	_ = r.Register("A", pk, 100)
	if err := r.Slash("A", "equivocation"); err != nil {
		t.Fatalf("Slash: %v", err)
	}
	// Second slash is a no-op
	if err := r.Slash("A", "another"); err != nil {
		t.Fatalf("second Slash: %v", err)
	}
	rec := r.Get("A")
	if rec.Status != StatusSlashed {
		t.Fatalf("Status = %s, want slashed", rec.Status)
	}
	if err := r.Slash("nonexistent", ""); !errors.Is(err, ErrUnknownSigner) {
		t.Fatalf("Slash(unknown) err=%v, want %v", err, ErrUnknownSigner)
	}
}

func TestRegistryListSorted(t *testing.T) {
	r := NewRegistry()
	_, pkA, _ := GenerateSecretKey()
	_, pkB, _ := GenerateSecretKey()
	_, pkC, _ := GenerateSecretKey()
	_ = r.Register("C", pkC, 0)
	_ = r.Register("A", pkA, 0)
	_ = r.Register("B", pkB, 0)
	list := r.List("")
	if len(list) != 3 || list[0].ID != "A" || list[1].ID != "B" || list[2].ID != "C" {
		t.Fatalf("sort order broken: got %+v", list)
	}
	// Filter
	_ = r.Slash("B", "test")
	act := r.List(StatusActive)
	if len(act) != 2 || act[0].ID != "A" || act[1].ID != "C" {
		t.Fatalf("filter broken: got %+v", act)
	}
}

// VerifyQuorum ------------------------------------------------------------

func setupQuorum(t *testing.T, n int) (*Registry, []committeeMember) {
	t.Helper()
	reg := NewRegistry()
	cs := newCommittee(t, n)
	for _, c := range cs {
		if err := reg.Register(c.id, c.pk, 1000); err != nil {
			t.Fatalf("Register(%s): %v", c.id, err)
		}
	}
	return reg, cs
}

func TestVerifyQuorumHappyPath(t *testing.T) {
	reg, cs := setupQuorum(t, 5)
	msg := []byte("evidence-root-happy")

	sigs := make([]*Signature, 0, 4)
	ids := make([]string, 0, 4)
	for _, c := range cs[:4] { // 4/5 signers → above ceil(10/3)+1=4 threshold
		sig, _ := c.sk.Sign(msg)
		sigs = append(sigs, sig)
		ids = append(ids, c.id)
	}
	aggSig, _ := Aggregate(sigs)

	w, err := VerifyQuorum(reg, msg, ids, aggSig, 0) // 0 = auto-threshold
	if err != nil {
		t.Fatalf("VerifyQuorum: %v", err)
	}
	if w.Verdict != "APPROVE" || w.Scheme != SchemeBLS12381G1V1 {
		t.Fatalf("witness = %+v", w)
	}
	if w.WitnessHashHex == "" {
		t.Fatal("witness hash empty")
	}
}

func TestVerifyQuorumBelowThreshold(t *testing.T) {
	reg, cs := setupQuorum(t, 5)
	msg := []byte("evidence")
	// Only 2 signers of 5 — threshold = 4
	sigs := []*Signature{}
	ids := []string{}
	for _, c := range cs[:2] {
		sig, _ := c.sk.Sign(msg)
		sigs = append(sigs, sig)
		ids = append(ids, c.id)
	}
	aggSig, _ := Aggregate(sigs)
	if _, err := VerifyQuorum(reg, msg, ids, aggSig, 0); !errors.Is(err, ErrThresholdNotMet) {
		t.Fatalf("err=%v, want %v", err, ErrThresholdNotMet)
	}
}

func TestVerifyQuorumTamperedSignature(t *testing.T) {
	reg, cs := setupQuorum(t, 5)
	msg := []byte("evidence-orig")
	sigs := []*Signature{}
	ids := []string{}
	for _, c := range cs[:4] {
		sig, _ := c.sk.Sign(msg)
		sigs = append(sigs, sig)
		ids = append(ids, c.id)
	}
	aggSig, _ := Aggregate(sigs)
	// Verify against a DIFFERENT message with the same aggregate
	if _, err := VerifyQuorum(reg, []byte("evidence-tampered"), ids, aggSig, 0); !errors.Is(err, ErrPairingCheckFail) {
		t.Fatalf("tampered msg: err=%v, want ErrPairingCheckFail", err)
	}
}

func TestVerifyQuorumSlashedSigner(t *testing.T) {
	reg, cs := setupQuorum(t, 5)
	msg := []byte("evidence")
	// signer A is slashed but tries to participate
	_ = reg.Slash("A", "equivocation")

	sigs := []*Signature{}
	ids := []string{}
	for _, c := range cs[:4] {
		sig, _ := c.sk.Sign(msg)
		sigs = append(sigs, sig)
		ids = append(ids, c.id)
	}
	aggSig, _ := Aggregate(sigs)
	if _, err := VerifyQuorum(reg, msg, ids, aggSig, 0); !errors.Is(err, ErrInactiveSigner) {
		t.Fatalf("err=%v, want ErrInactiveSigner", err)
	}
}

func TestVerifyQuorumUnknownSigner(t *testing.T) {
	reg, cs := setupQuorum(t, 3)
	msg := []byte("evidence")
	sigs := []*Signature{}
	for _, c := range cs {
		sig, _ := c.sk.Sign(msg)
		sigs = append(sigs, sig)
	}
	aggSig, _ := Aggregate(sigs)
	// Include an id not in the registry
	if _, err := VerifyQuorum(reg, msg, []string{"A", "B", "Z_UNKNOWN"}, aggSig, 0); !errors.Is(err, ErrUnknownSigner) {
		t.Fatalf("err=%v, want ErrUnknownSigner", err)
	}
}

func TestVerifyQuorumDuplicateBitset(t *testing.T) {
	reg, cs := setupQuorum(t, 3)
	msg := []byte("evidence")
	sig, _ := cs[0].sk.Sign(msg)
	aggSig, _ := Aggregate([]*Signature{sig, sig, sig})
	if _, err := VerifyQuorum(reg, msg, []string{"A", "A", "A"}, aggSig, 0); !errors.Is(err, ErrDuplicateSigner) {
		t.Fatalf("err=%v, want ErrDuplicateSigner", err)
	}
}

func TestVerifyQuorumBitsetOrderInvariant(t *testing.T) {
	reg, cs := setupQuorum(t, 5)
	msg := []byte("evidence-order")
	sigs := []*Signature{}
	for _, c := range cs[:4] {
		sig, _ := c.sk.Sign(msg)
		sigs = append(sigs, sig)
	}
	aggSig, _ := Aggregate(sigs)

	w1, err := VerifyQuorum(reg, msg, []string{"A", "B", "C", "D"}, aggSig, 0)
	if err != nil {
		t.Fatalf("VerifyQuorum(sorted): %v", err)
	}
	w2, err := VerifyQuorum(reg, msg, []string{"D", "A", "C", "B"}, aggSig, 0)
	if err != nil {
		t.Fatalf("VerifyQuorum(shuffled): %v", err)
	}
	if w1.WitnessHashHex != w2.WitnessHashHex {
		t.Fatalf("witness hash not order-invariant:\n  sorted=%s\n  shuffled=%s",
			w1.WitnessHashHex, w2.WitnessHashHex)
	}
	if w1.AggregatePubKeyHex != w2.AggregatePubKeyHex {
		t.Fatal("agg pubkey not order-invariant")
	}
}

func TestVerifyQuorumRejectsSchemeMismatch(t *testing.T) {
	// This is a sanity test that the witness always carries our current
	// scheme label — future work will introduce SchemeBLS12381TSSv1 as a
	// separate implementation; verifiers refusing an unknown scheme
	// happens at the callsite, not inside VerifyQuorum.
	reg, cs := setupQuorum(t, 3)
	msg := []byte("evidence")
	sigs := []*Signature{}
	for _, c := range cs {
		sig, _ := c.sk.Sign(msg)
		sigs = append(sigs, sig)
	}
	aggSig, _ := Aggregate(sigs)
	w, err := VerifyQuorum(reg, msg, []string{"A", "B", "C"}, aggSig, 0)
	if err != nil {
		t.Fatalf("VerifyQuorum: %v", err)
	}
	if w.Scheme != SchemeBLS12381G1V1 {
		t.Fatalf("scheme = %q, want %q", w.Scheme, SchemeBLS12381G1V1)
	}
	if w.Scheme == SchemeBLS12381TSSv1 {
		t.Fatal("scheme label is reserved future-work; must not appear in real witnesses")
	}
}
