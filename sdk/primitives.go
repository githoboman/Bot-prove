package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// This file exposes the four high-level primitives shared across the Go,
// Python and TypeScript SDKs: Prove, Verify, Batch, Anchor + VerifyReceipt.
//
// They are thin wrappers over the lower-level route methods in client.go
// with two properties:
//
//   1. All requests go through the /v1/ prefix by default (unless the client
//      was configured otherwise). This means SDK users get RFC-8594 stability
//      guarantees on the wire.
//   2. Every write primitive accepts RequestOption values, so callers can
//      pin X-Idempotency-Key for safe retries without threading the header
//      through each shape-specific request struct.
//
// The primitives return typed responses so callers do not have to reach into
// map[string]any values manually. The types are declared in types_v1.go.

// Prove submits a proof-of-inference request. It routes to POST /v1/proofs.
//
// The Idempotency-Key on retries is safe: same key + body → bit-identical
// replay from the server-side cache; different body under the same key → 409.
func (c *Client) Prove(ctx context.Context, req ProveRequest, opts ...RequestOption) (*ProveResponse, error) {
	var out ProveResponse
	if err := c.doRequestWithOpts(ctx, http.MethodPost, c.prefix()+"/proofs", req, &out, opts...); err != nil {
		return nil, err
	}
	return &out, nil
}

// Verify checks a proof by ID. Routes to POST /v1/verify.
//
// Verify is idempotent server-side by design; the option is accepted for
// symmetry with Prove.
func (c *Client) Verify(ctx context.Context, proofID string, opts ...RequestOption) (*VerifyResponse, error) {
	var out VerifyResponse
	body := map[string]string{"proof_id": proofID}
	if err := c.doRequestWithOpts(ctx, http.MethodPost, c.prefix()+"/verify", body, &out, opts...); err != nil {
		return nil, err
	}
	return &out, nil
}

// Batch verifies a set of proofs in one round-trip. Routes to POST /v1/batch/verify-zk.
//
// The `mode` field is optional; the server picks a sensible default when empty.
func (c *Client) Batch(ctx context.Context, proofs []BatchItem, mode string, opts ...RequestOption) (*BatchResponse, error) {
	// Server-side handler expects []map[string]any; keep on-the-wire shape stable.
	items := make([]map[string]any, 0, len(proofs))
	for _, p := range proofs {
		items = append(items, p.toMap())
	}
	body := map[string]any{"proofs": items}
	if mode != "" {
		body["mode"] = mode
	}
	var out BatchResponse
	if err := c.doRequestWithOpts(ctx, http.MethodPost, c.prefix()+"/batch/verify-zk", body, &out, opts...); err != nil {
		return nil, err
	}
	return &out, nil
}

// Anchor requests that a proof be anchored on-chain. Routes to
// POST /v1/proofs/{id}/anchor.
//
// This is a strict-mode operation on the server: with CP_STRICT=1 a missing
// deployer key returns an error rather than a fabricated hash. Always retry
// with the same Idempotency-Key so the caller cannot double-submit.
func (c *Client) Anchor(ctx context.Context, proofID string, opts ...RequestOption) (*AnchorResponse, error) {
	var out AnchorResponse
	path := fmt.Sprintf("%s/proofs/%s/anchor", c.prefix(), proofID)
	if err := c.doRequestWithOpts(ctx, http.MethodPost, path, nil, &out, opts...); err != nil {
		return nil, err
	}
	return &out, nil
}

// VerifyReceipt re-derives the proof-identity hashes locally and, on success,
// returns a normalized ProofReceipt view of the response. This is a strict
// client-side check that does not talk to the server: it exists so callers
// can validate a receipt they received out-of-band (e.g. through a webhook)
// without a round-trip.
//
// The exact hash re-derivation is delegated to package `receipt` so the same
// implementation is shared across all three SDKs (Go / Python / TS).
func (c *Client) VerifyReceipt(payload []byte) (*ProofReceipt, error) {
	return VerifyReceiptBytes(payload)
}

// BatchItem is one entry in a Batch call. All fields are optional except at
// least one of ProofID / (ModelID + Input + Output) must be present.
type BatchItem struct {
	ProofID string `json:"proof_id,omitempty"`
	ModelID string `json:"model_id,omitempty"`
	Input   string `json:"input,omitempty"`
	Output  string `json:"output,omitempty"`
	// Extra passthrough fields the server may accept.
	Extra map[string]any `json:"-"`
}

// toMap flattens BatchItem into the wire shape the server expects.
func (b BatchItem) toMap() map[string]any {
	m := map[string]any{}
	if b.ProofID != "" {
		m["proof_id"] = b.ProofID
	}
	if b.ModelID != "" {
		m["model_id"] = b.ModelID
	}
	if b.Input != "" {
		m["input"] = b.Input
	}
	if b.Output != "" {
		m["output"] = b.Output
	}
	for k, v := range b.Extra {
		if _, dup := m[k]; !dup {
			m[k] = v
		}
	}
	return m
}

// MarshalJSON supports Extra passthrough on encode paths that hit BatchItem
// directly (e.g. debug logging).
func (b BatchItem) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.toMap())
}
