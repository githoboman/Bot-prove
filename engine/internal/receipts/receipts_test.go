package receipts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto/keystore"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/decision/attest"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/hitl"
)

func mustKeystore(t *testing.T) keystore.Keystore {
	t.Helper()
	ring := pqcrypto.NewKeyRing()
	if _, err := ring.CreateKey(pqcrypto.AlgoHybrid); err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	return keystore.NewMemory(ring)
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
}



func newService(t *testing.T) *Service {
	t.Helper()
	svc := NewService(NewInMemoryStore(), mustKeystore(t))
	svc.Now = fixedNow
	counter := 0
	svc.NewID = func() string {
		counter++
		return fmt.Sprintf("test-receipt-%04d", counter)
	}
	return svc
}

func sampleCommit(agg attest.Verdict, vetoed attest.FacetKind) attest.DecisionCommit {
	dec := attest.Decision{Submitter: "sub", SpecID: "policy/v1", Nonce: 42}
	return attest.DecisionCommit{
		Decision:   dec,
		DecisionID: dec.ID(),
		FacetVerdicts: []attest.FacetVerdict{
			{Kind: attest.FacetSafety, Verdict: attest.VerdictApprove, Confidence: 0.9, Reason: "clean"},
			{Kind: attest.FacetCorrectness, Verdict: agg, Confidence: 0.7, Reason: "ok"},
			{Kind: attest.FacetSpecCompliance, Verdict: agg, Confidence: 0.6, Reason: "matches"},
		},
		Aggregate: agg,
		VetoedBy:  vetoed,
	}
}

// -------------------- canonical hash --------------------

func TestCanonicalHashDeterministic(t *testing.T) {
	svc := newService(t)
	r1, err := svc.Emit(context.Background(), EmitInput{Commit: sampleCommit(attest.VerdictApprove, "")})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	// Recompute over the receipt (Proof stripped) — must match what Verify uses.
	unsigned := r1
	unsigned.Proof = nil
	h1 := CanonicalHash(unsigned)
	h2 := CanonicalHash(unsigned)
	if h1 != h2 {
		t.Fatalf("hash not deterministic: %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("hash length: got %d, want 64", len(h1))
	}
}

func TestCanonicalHashIgnoresOrder(t *testing.T) {
	base := DecisionReceipt{
		ID:        "id-1",
		IssuedAt:  fixedNow(),
		Issuer:    "did:cp:x",
		Subject:   "sub",
		SpecID:    "s",
		Aggregate: VerdictApprove,
		Facets: []FacetOutput{
			{Kind: "safety", Verdict: VerdictApprove, Confidence: 0.9},
			{Kind: "correctness", Verdict: VerdictApprove, Confidence: 0.8},
		},
	}
	reordered := base
	reordered.Facets = []FacetOutput{base.Facets[1], base.Facets[0]}
	if CanonicalHash(base) != CanonicalHash(reordered) {
		t.Fatalf("hash depends on facet order — should not")
	}
}

// -------------------- emit + verify --------------------

func TestEmitProducesVerifiableProof(t *testing.T) {
	svc := newService(t)
	r, err := svc.Emit(context.Background(), EmitInput{
		Commit:       sampleCommit(attest.VerdictApprove, ""),
		EvidenceRoot: "abcdef",
		ModelID:      "gpt-x-v1",
	})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if r.Proof == nil || r.Proof.Signature == "" {
		t.Fatalf("proof missing")
	}
	if _, err := base64.StdEncoding.DecodeString(r.Proof.Signature); err != nil {
		t.Fatalf("signature is not base64: %v", err)
	}
	if !strings.HasPrefix(r.Proof.VerificationMethod, "did:cp:") {
		t.Fatalf("verification method: %s", r.Proof.VerificationMethod)
	}
	ok, err := svc.Verify(context.Background(), r)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatal("verify returned false on unmodified receipt")
	}
}

func TestVerifyRejectsTamperedReceipt(t *testing.T) {
	svc := newService(t)
	r, err := svc.Emit(context.Background(), EmitInput{Commit: sampleCommit(attest.VerdictApprove, "")})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	// Tamper: flip the aggregate. Signature covers CanonicalHash, so
	// this MUST invalidate.
	r.Aggregate = VerdictReject
	ok, err := svc.Verify(context.Background(), r)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Fatal("verify returned true for tampered receipt")
	}
}

func TestEmitRejectsMissingCommitAggregate(t *testing.T) {
	svc := newService(t)
	badCommit := attest.DecisionCommit{DecisionID: "x", Aggregate: attest.VerdictUnknown}
	if _, err := svc.Emit(context.Background(), EmitInput{Commit: badCommit}); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// -------------------- store --------------------

func TestInMemoryStoreGetByIDAndHash(t *testing.T) {
	svc := newService(t)
	r, err := svc.Emit(context.Background(), EmitInput{Commit: sampleCommit(attest.VerdictApprove, "")})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	got, err := svc.Store.GetByID(context.Background(), r.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != r.ID {
		t.Fatalf("GetByID mismatch: %s vs %s", got.ID, r.ID)
	}
	unsigned := r
	unsigned.Proof = nil
	got2, err := svc.Store.GetByHash(context.Background(), CanonicalHash(unsigned))
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if got2.ID != r.ID {
		t.Fatalf("GetByHash mismatch: %s vs %s", got2.ID, r.ID)
	}
	if _, err := svc.Store.GetByID(context.Background(), "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAncestorsGathersUpstreamLineage(t *testing.T) {
	svc := newService(t)
	// Upstream receipt.
	upstream, err := svc.Emit(context.Background(), EmitInput{Commit: sampleCommit(attest.VerdictApprove, "")})
	if err != nil {
		t.Fatalf("emit upstream: %v", err)
	}
	unsigned := upstream
	unsigned.Proof = nil
	upstreamHash := CanonicalHash(unsigned)
	// Downstream receipt referencing upstream.
	downCommit := sampleCommit(attest.VerdictApprove, "")
	downCommit.Decision.Nonce = 99
	downCommit.DecisionID = downCommit.Decision.ID()
	down, err := svc.Emit(context.Background(), EmitInput{
		Commit: downCommit,
		ProviderReceipts: []ProviderReceipt{
			{Provider: "safety-1", TrustLevel: "system", ReceiptHash: upstreamHash},
		},
	})
	if err != nil {
		t.Fatalf("emit down: %v", err)
	}
	ancs, err := svc.Store.Ancestors(context.Background(), down.ID, 5)
	if err != nil {
		t.Fatalf("Ancestors: %v", err)
	}
	if len(ancs) != 1 {
		t.Fatalf("expected 1 ancestor, got %d", len(ancs))
	}
	if ancs[0].ID != upstream.ID {
		t.Fatalf("ancestor mismatch: %s vs %s", ancs[0].ID, upstream.ID)
	}
}

// -------------------- HITL merge --------------------

func TestEmitCarriesHITLResolution(t *testing.T) {
	svc := newService(t)
	commit := sampleCommit(attest.VerdictApprove, "")
	commit.FacetVerdicts[0].Verdict = attest.VerdictAbstain // safety ABSTAIN → escalate
	commit.FacetVerdicts[0].Reason = "unsure"
	// Feed a synthetic HITL response so we don't couple the test to
	// hitl.Service internals.
	resp := &hitl.Response{
		Action:   hitl.ActionEscalate,
		Reason:   "critical facet safety ABSTAINed: unsure",
		TicketID: "ticket-abc",
	}
	r, err := svc.Emit(context.Background(), EmitInput{
		Commit:   commit,
		HITL:     resp,
		Reviewer: "reviewer-1",
	})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if r.HITL == nil {
		t.Fatal("HITL resolution missing")
	}
	if r.HITL.TicketID != "ticket-abc" {
		t.Fatalf("ticket id: %s", r.HITL.TicketID)
	}
	if r.HITL.Reviewer != "reviewer-1" {
		t.Fatalf("reviewer: %s", r.HITL.Reviewer)
	}
	ok, err := svc.Verify(context.Background(), r)
	if err != nil || !ok {
		t.Fatalf("verify with HITL: ok=%v err=%v", ok, err)
	}
}

// -------------------- W3C VC + Agent Receipt --------------------

func TestToW3CVCShape(t *testing.T) {
	svc := newService(t)
	r, err := svc.Emit(context.Background(), EmitInput{
		Commit:       sampleCommit(attest.VerdictApprove, ""),
		EvidenceRoot: "root",
		ModelID:      "model",
	})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	vc, err := ToW3CVC(r)
	if err != nil {
		t.Fatalf("ToW3CVC: %v", err)
	}
	// Round-trip through JSON to guarantee the shape is serialisable
	// and every required VC-2.0 field is present.
	b, err := json.Marshal(vc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := map[string]any{}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["id"] != "urn:uuid:"+r.ID {
		t.Fatalf("id: %v", got["id"])
	}
	ctx, ok := got["@context"].([]any)
	if !ok || len(ctx) < 2 {
		t.Fatalf("context: %v", got["@context"])
	}
	ctx0, ok := ctx[0].(string)
	if !ok || ctx0 != "https://www.w3.org/ns/credentials/v2" {
		t.Fatalf("context[0]: %v", ctx[0])
	}
	proof, ok := got["proof"].(map[string]any)
	if !ok {
		t.Fatal("no proof")
	}
	if proof["proofValue"] != r.Proof.Signature {
		t.Fatal("proofValue does not match receipt signature")
	}
	if proof["proofPurpose"] != "assertionMethod" {
		t.Fatal("proofPurpose")
	}
}

func TestToAgentReceiptShape(t *testing.T) {
	svc := newService(t)
	r, err := svc.Emit(context.Background(), EmitInput{Commit: sampleCommit(attest.VerdictApprove, "")})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	ar, err := ToAgentReceipt(r)
	if err != nil {
		t.Fatalf("ToAgentReceipt: %v", err)
	}
	if ar.Version != "0.3" {
		t.Fatalf("ar version: %s", ar.Version)
	}
	if ar.ID != r.ID {
		t.Fatalf("ar id: %s vs %s", ar.ID, r.ID)
	}
	if ar.Signature.Value != r.Proof.Signature {
		t.Fatalf("ar signature: %s vs %s", ar.Signature.Value, r.Proof.Signature)
	}
	if len(ar.Facets) != 3 {
		t.Fatalf("facets: %d", len(ar.Facets))
	}
}

func TestUnsignedReceiptEmittersRefuse(t *testing.T) {
	unsigned := DecisionReceipt{ID: "x", IssuedAt: fixedNow(), Aggregate: VerdictApprove}
	if _, err := ToW3CVC(unsigned); err == nil {
		t.Fatal("expected W3C-VC refusal on unsigned receipt")
	}
	if _, err := ToAgentReceipt(unsigned); err == nil {
		t.Fatal("expected agent-receipt refusal on unsigned receipt")
	}
}

// -------------------- OTel JSONL sink --------------------

func TestJSONLSinkWritesOnePerReceipt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "receipts.jsonl")
	sink, err := NewJSONLSink(path)
	if err != nil {
		t.Fatalf("NewJSONLSink: %v", err)
	}
	defer func() { _ = sink.Close() }()

	svc := newService(t)
	svc.Sink = sink
	r1, err := svc.Emit(context.Background(), EmitInput{Commit: sampleCommit(attest.VerdictApprove, "")})
	if err != nil {
		t.Fatalf("emit r1: %v", err)
	}
	c2 := sampleCommit(attest.VerdictApprove, "")
	c2.Decision.Nonce = 43
	c2.DecisionID = c2.Decision.ID()
	r2, err := svc.Emit(context.Background(), EmitInput{Commit: c2})
	if err != nil {
		t.Fatalf("emit r2: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	var got1 map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &got1); err != nil {
		t.Fatalf("unmarshal line 1: %v", err)
	}
	if got1["cp.receipt_id"] != r1.ID {
		t.Fatalf("line 1 receipt id: %v", got1["cp.receipt_id"])
	}
	if got1["cp.verdict"] != "APPROVE" {
		t.Fatalf("verdict: %v", got1["cp.verdict"])
	}
	_ = r2
}
