package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newRecordingServer returns an httptest.Server that records the last
// request method + path + headers + body, and replies with the given JSON.
type recordedRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Body    string
}

func newRecordingServer(t *testing.T, reply string) (*httptest.Server, *recordedRequest) {
	t.Helper()
	rec := &recordedRequest{Headers: http.Header{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.Method = r.Method
		rec.Path = r.URL.Path
		rec.Headers = r.Header.Clone()
		buf := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			_, _ = r.Body.Read(buf)
		}
		rec.Body = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

// -- Prove ----------------------------------------------------------------

func TestProve_UsesV1Prefix(t *testing.T) {
	srv, rec := newRecordingServer(t, `{"id":"pf-1","proof_hash":"deadbeef"}`)
	c := NewClient(WithBaseURL(srv.URL))
	got, err := c.Prove(context.Background(), ProveRequest{Agent: "a", Model: "m", Input: "in", Output: "out"})
	if err != nil {
		t.Fatalf("Prove returned error: %v", err)
	}
	if got.ID != "pf-1" {
		t.Fatalf("unexpected id %q", got.ID)
	}
	if rec.Path != "/v1/proofs" {
		t.Fatalf("expected /v1/proofs, got %q", rec.Path)
	}
	if rec.Method != http.MethodPost {
		t.Fatalf("expected POST, got %q", rec.Method)
	}
}

func TestProve_SendsIdempotencyKey(t *testing.T) {
	srv, rec := newRecordingServer(t, `{"id":"pf-1"}`)
	c := NewClient(WithBaseURL(srv.URL))
	_, err := c.Prove(context.Background(), ProveRequest{Agent: "a", Model: "m"}, WithIdempotencyKey("key-42"))
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if got := rec.Headers.Get("X-Idempotency-Key"); got != "key-42" {
		t.Fatalf("expected X-Idempotency-Key=key-42, got %q", got)
	}
}

// -- Verify ---------------------------------------------------------------

func TestVerify_PostsProofID(t *testing.T) {
	srv, rec := newRecordingServer(t, `{"valid":true,"proof_id":"pf-9"}`)
	c := NewClient(WithBaseURL(srv.URL))
	got, err := c.Verify(context.Background(), "pf-9")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !got.Valid || got.ProofID != "pf-9" {
		t.Fatalf("unexpected response: %+v", got)
	}
	if rec.Path != "/v1/verify" || rec.Method != http.MethodPost {
		t.Fatalf("wrong route: %s %s", rec.Method, rec.Path)
	}
	if !strings.Contains(rec.Body, `"proof_id":"pf-9"`) {
		t.Fatalf("body missing proof_id: %s", rec.Body)
	}
}

// -- Batch ----------------------------------------------------------------

func TestBatch_SendsAllItems(t *testing.T) {
	srv, rec := newRecordingServer(t, `{"verified":["a","b"],"total":2}`)
	c := NewClient(WithBaseURL(srv.URL))
	got, err := c.Batch(context.Background(), []BatchItem{
		{ProofID: "a"},
		{ProofID: "b"},
	}, "strict")
	if err != nil {
		t.Fatalf("Batch: %v", err)
	}
	if got.Total != 2 || len(got.Verified) != 2 {
		t.Fatalf("unexpected batch response: %+v", got)
	}
	if rec.Path != "/v1/batch/verify-zk" {
		t.Fatalf("wrong path %q", rec.Path)
	}
	// crude on-wire shape check
	var body map[string]any
	if err := json.Unmarshal([]byte(rec.Body), &body); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, rec.Body)
	}
	if body["mode"] != "strict" {
		t.Fatalf("mode not forwarded: %v", body["mode"])
	}
}

// -- Anchor ---------------------------------------------------------------

func TestAnchor_UsesProofPath(t *testing.T) {
	srv, rec := newRecordingServer(t, `{"proof_id":"pf-x","tx_hash":"aa11","strict_mode":true}`)
	c := NewClient(WithBaseURL(srv.URL))
	got, err := c.Anchor(context.Background(), "pf-x", WithIdempotencyKey("anchor-key"))
	if err != nil {
		t.Fatalf("Anchor: %v", err)
	}
	if got.TxHash != "aa11" || !got.StrictMode {
		t.Fatalf("unexpected anchor response: %+v", got)
	}
	if rec.Path != "/v1/proofs/pf-x/anchor" {
		t.Fatalf("wrong path %q", rec.Path)
	}
	if rec.Headers.Get("X-Idempotency-Key") != "anchor-key" {
		t.Fatalf("idempotency key not forwarded")
	}
}

// -- Unversioned override -------------------------------------------------

func TestUnversionedClient_KeepsLegacyPath(t *testing.T) {
	srv, rec := newRecordingServer(t, `{"id":"pf"}`)
	c := NewClient(WithBaseURL(srv.URL), WithAPIVersion(APIVersionUnversioned))
	if _, err := c.Prove(context.Background(), ProveRequest{Agent: "a"}); err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if rec.Path != "/proofs" {
		t.Fatalf("expected legacy /proofs, got %q", rec.Path)
	}
}

// -- Receipt validator ----------------------------------------------------

func TestVerifyReceipt_HappyPath(t *testing.T) {
	payload := map[string]any{
		"id":          "pf-1",
		"input":       "hello",
		"output":      "world",
		"model":       "gpt-toy-v1",
		"input_hash":  HashField("hello"),
		"output_hash": HashField("world"),
		"model_hash":  HashField("gpt-toy-v1"),
	}
	raw, _ := json.Marshal(payload)
	r, err := VerifyReceiptBytes(raw)
	if err != nil {
		t.Fatalf("VerifyReceiptBytes: %v", err)
	}
	if r.ID != "pf-1" {
		t.Fatalf("unexpected id %q", r.ID)
	}
}

func TestVerifyReceipt_DetectsTamper(t *testing.T) {
	payload := map[string]any{
		"id":         "pf-1",
		"input":      "hello",
		"input_hash": HashField("goodbye"), // tampered
	}
	raw, _ := json.Marshal(payload)
	_, err := VerifyReceiptBytes(raw)
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}
	var vErr *ReceiptValidationError
	if !errorsAs(err, &vErr) || vErr.Field != "input_hash" {
		t.Fatalf("expected ReceiptValidationError on input_hash, got %v", err)
	}
}

// tiny wrapper so we can errors.As without importing errors in every test
func errorsAs(err error, target any) bool {
	// stdlib errors.As is imported lazily to keep this file free of the
	// import when unused; but Go forces the import if referenced. Just
	// use it directly.
	return goErrorsAs(err, target)
}
