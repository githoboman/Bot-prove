package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/receipts"
)

func enableReceiptsService(t *testing.T, s *Server) {
	t.Helper()
	if _, err := s.keyRing.CreateKey(pqcrypto.AlgoHybrid); err != nil {
		// Key may already exist from a prior test path — non-fatal.
		if err.Error() != "keyring: algo already has an active key" {
			t.Fatalf("CreateKey: %v", err)
		}
	}
	svc := receipts.NewService(receipts.NewInMemoryStore(), s.keystore)
	s.receipts = svc
}

func mustReceiptEmit(t *testing.T, s *Server, body string) receipts.DecisionReceipt {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/receipts/emit", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.receiptsEmit(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("emit: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var r receipts.DecisionReceipt
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return r
}

func TestReceiptsEmitDisabledReturns503(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest(http.MethodPost, "/v1/receipts/emit", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	s.receiptsEmit(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestReceiptsEmitAndFetch(t *testing.T) {
	s := newTestServer("")
	enableReceiptsService(t, s)

	body := receiptsEmitRequest{
		Commit: receiptsCommitWire{
			Submitter:  "sub-1",
			SpecID:     "policy/v1",
			PayloadHex: hex.EncodeToString([]byte("hello")),
			Nonce:      1,
			Aggregate:  "APPROVE",
			Facets: []facetWire{
				{Kind: "safety", Verdict: "APPROVE", Confidence: 0.9, Reason: "clean"},
				{Kind: "correctness", Verdict: "APPROVE", Confidence: 0.8, Reason: "ok"},
			},
		},
		EvidenceRoot: "abcdef",
		ModelID:      "model-x-v1",
	}
	b, _ := json.Marshal(body)
	rec := mustReceiptEmit(t, s, string(b))
	if rec.ID == "" || rec.Proof == nil || rec.Proof.Signature == "" {
		t.Fatalf("bad emit output: %+v", rec)
	}
	if rec.Subject == "" {
		t.Fatal("subject empty")
	}
	if rec.ModelID != "model-x-v1" {
		t.Fatalf("model id: %s", rec.ModelID)
	}

	// Fetch by id.
	fetchReq := httptest.NewRequest(http.MethodGet, "/v1/receipts/"+rec.ID, nil)
	fetchReq.SetPathValue("id", rec.ID)
	fetchRec := httptest.NewRecorder()
	s.receiptsGet(fetchRec, fetchReq)
	if fetchRec.Code != http.StatusOK {
		t.Fatalf("get: got %d: %s", fetchRec.Code, fetchRec.Body.String())
	}
	var got receipts.DecisionReceipt
	if err := json.Unmarshal(fetchRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if got.ID != rec.ID {
		t.Fatalf("get id mismatch: %s vs %s", got.ID, rec.ID)
	}
}

func TestReceiptsGetNotFound(t *testing.T) {
	s := newTestServer("")
	enableReceiptsService(t, s)
	req := httptest.NewRequest(http.MethodGet, "/v1/receipts/does-not-exist", nil)
	req.SetPathValue("id", "does-not-exist")
	rec := httptest.NewRecorder()
	s.receiptsGet(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestReceiptsW3CVCAndAgentReceipt(t *testing.T) {
	s := newTestServer("")
	enableReceiptsService(t, s)

	body := receiptsEmitRequest{
		Commit: receiptsCommitWire{
			Submitter:  "sub-1",
			SpecID:     "policy/v1",
			PayloadHex: hex.EncodeToString([]byte("data")),
			Nonce:      7,
			Aggregate:  "APPROVE",
			Facets: []facetWire{
				{Kind: "safety", Verdict: "APPROVE", Confidence: 0.9, Reason: "clean"},
			},
		},
	}
	b, _ := json.Marshal(body)
	rec := mustReceiptEmit(t, s, string(b))

	// W3C VC
	vcReq := httptest.NewRequest(http.MethodGet, "/v1/receipts/"+rec.ID+"/w3c-vc", nil)
	vcReq.SetPathValue("id", rec.ID)
	vcRec := httptest.NewRecorder()
	s.receiptsW3CVC(vcRec, vcReq)
	if vcRec.Code != http.StatusOK {
		t.Fatalf("vc: got %d: %s", vcRec.Code, vcRec.Body.String())
	}
	var vc map[string]any
	if err := json.Unmarshal(vcRec.Body.Bytes(), &vc); err != nil {
		t.Fatalf("unmarshal vc: %v", err)
	}
	if vc["id"] != "urn:uuid:"+rec.ID {
		t.Fatalf("vc id: %v", vc["id"])
	}
	proof, ok := vc["proof"].(map[string]any)
	if !ok || proof["proofValue"] != rec.Proof.Signature {
		t.Fatal("vc proof mismatch")
	}

	// Agent Receipt
	arReq := httptest.NewRequest(http.MethodGet, "/v1/receipts/"+rec.ID+"/agent-receipt", nil)
	arReq.SetPathValue("id", rec.ID)
	arRec := httptest.NewRecorder()
	s.receiptsAgentReceipt(arRec, arReq)
	if arRec.Code != http.StatusOK {
		t.Fatalf("ar: got %d", arRec.Code)
	}
	var ar map[string]any
	if err := json.Unmarshal(arRec.Body.Bytes(), &ar); err != nil {
		t.Fatalf("unmarshal ar: %v", err)
	}
	if ar["ar_version"] != "0.3" {
		t.Fatalf("ar version: %v", ar["ar_version"])
	}
	if ar["id"] != rec.ID {
		t.Fatalf("ar id: %v", ar["id"])
	}
}

func TestReceiptsLineage(t *testing.T) {
	s := newTestServer("")
	enableReceiptsService(t, s)

	upstreamBody, _ := json.Marshal(receiptsEmitRequest{
		Commit: receiptsCommitWire{
			Submitter: "sub-1", SpecID: "policy/v1",
			PayloadHex: hex.EncodeToString([]byte("upstream")), Nonce: 1,
			Aggregate: "APPROVE",
			Facets: []facetWire{
				{Kind: "safety", Verdict: "APPROVE", Confidence: 0.9},
			},
		},
	})
	up := mustReceiptEmit(t, s, string(upstreamBody))

	// Recompute upstream canonical hash the way the store did.
	unsigned := up
	unsigned.Proof = nil
	upHash := receipts.CanonicalHash(unsigned)

	downBody, _ := json.Marshal(receiptsEmitRequest{
		Commit: receiptsCommitWire{
			Submitter: "sub-1", SpecID: "policy/v1",
			PayloadHex: hex.EncodeToString([]byte("downstream")), Nonce: 2,
			Aggregate: "APPROVE",
			Facets: []facetWire{
				{Kind: "safety", Verdict: "APPROVE", Confidence: 0.9},
			},
		},
		Providers: []providerReceiptWire{
			{Provider: "safety-1", TrustLevel: "system", ReceiptHash: upHash},
		},
	})
	down := mustReceiptEmit(t, s, string(downBody))

	req := httptest.NewRequest(http.MethodGet, "/v1/receipts/"+down.ID+"/lineage", nil)
	req.SetPathValue("id", down.ID)
	rec := httptest.NewRecorder()
	s.receiptsLineage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("lineage: got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Root      receipts.DecisionReceipt   `json:"root"`
		Ancestors []receipts.DecisionReceipt `json:"ancestors"`
		Depth     int                        `json:"depth"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Root.ID != down.ID {
		t.Fatalf("root id: %s", out.Root.ID)
	}
	if len(out.Ancestors) != 1 || out.Ancestors[0].ID != up.ID {
		t.Fatalf("ancestors: %+v", out.Ancestors)
	}
}

func TestReceiptsEmitBadJSONBody(t *testing.T) {
	s := newTestServer("")
	enableReceiptsService(t, s)
	req := httptest.NewRequest(http.MethodPost, "/v1/receipts/emit", strings.NewReader(`not-json`))
	rec := httptest.NewRecorder()
	s.receiptsEmit(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
