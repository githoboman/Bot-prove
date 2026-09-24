package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultBaseURL = "http://localhost:9090"
	defaultTimeout = 30 * time.Second
)

// Client is a Go client for the CasperProver REST API.
//
// Every method here maps 1:1 to a real route registered in
// engine/internal/api/server.go - see docs/openapi.yaml for the
// authoritative route list. Field names mirror the Go structs the server
// decodes/encodes, so requests built with this client are accepted as-is.
type Client struct {
	authToken  string
	baseURL    string
	httpClient *http.Client
	apiVersion APIVersion
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL overrides the default API base URL (http://localhost:9090).
func WithBaseURL(url string) ClientOption {
	return func(c *Client) { c.baseURL = url }
}

// WithHTTPClient supplies a custom *http.Client (e.g. custom timeouts/transport).
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) { c.httpClient = httpClient }
}

// WithAuthToken sets the X-API-Key header sent on every non-GET request.
// Only required if the server was started with the API_KEY env var set.
func WithAuthToken(token string) ClientOption {
	return func(c *Client) { c.authToken = token }
}

// NewClient creates a CasperProver API client. Defaults to APIVersionV1;
// override with WithAPIVersion(APIVersionUnversioned) for legacy routes.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
		apiVersion: APIVersionV1,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetAuthToken updates the API key used for subsequent requests.
func (c *Client) SetAuthToken(token string) { c.authToken = token }

func (c *Client) doRequest(ctx context.Context, method, path string, reqBody, respBody interface{}) error {
	return c.doRequestWithOpts(ctx, method, path, reqBody, respBody)
}

// doRequestWithOpts is the low-level dispatcher. All high-level methods route
// through it; per-request options (idempotency key, extra headers) are
// applied here so the client stays stateless between calls.
func (c *Client) doRequestWithOpts(ctx context.Context, method, path string, reqBody, respBody interface{}, opts ...RequestOption) error {
	var bodyReader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.authToken != "" {
		req.Header.Set("X-API-Key", c.authToken)
	}

	rc := buildRequestConfig(opts)
	if rc.idempotencyKey != "" {
		req.Header.Set("X-Idempotency-Key", rc.idempotencyKey)
	}
	for name, val := range rc.extraHeaders {
		req.Header.Set(name, val)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		return fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(data))
	}
	if respBody == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, respBody); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// Health calls GET /health.
func (c *Client) Health(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodGet, "/health", nil, &out)
	return out, err
}

// --- Proofs ------------------------------------------------------------

// SubmitProofRequest is the body for POST /proofs.
type SubmitProofRequest struct {
	Agent   string `json:"agent"`
	Input   string `json:"input"`
	Output  string `json:"output"`
	Model   string `json:"model"`
	UseCase string `json:"use_case,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

// SubmitProof creates a new proof. Maps to POST /proofs.
func (c *Client) SubmitProof(ctx context.Context, req SubmitProofRequest) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodPost, "/proofs", req, &out)
	return out, err
}

// GetProof fetches a proof by ID. Maps to GET /proofs/{id}.
func (c *Client) GetProof(ctx context.Context, proofID string) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodGet, "/proofs/"+proofID, nil, &out)
	return out, err
}

// ListProofs lists proofs, optionally filtered. Maps to GET /proofs.
func (c *Client) ListProofs(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodGet, "/proofs", nil, &out)
	return out, err
}

// VerifyProof checks a proof's validity. Maps to POST /verify.
func (c *Client) VerifyProof(ctx context.Context, proofID string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{"proof_id": proofID}
	err := c.doRequest(ctx, http.MethodPost, "/verify", req, &out)
	return out, err
}

// RevokeProof revokes a proof the caller owns. Maps to POST /proofs/{id}/revoke.
// Requires the client to send an X-Public-Key header matching the proof owner;
// use WithHTTPClient + a custom RoundTripper, or call doRequestWithHeader-style
// helpers if you need that - not yet exposed as a typed method here.
func (c *Client) RevokeProof(ctx context.Context, proofID, reason string) error {
	req := map[string]string{"reason": reason}
	return c.doRequest(ctx, http.MethodPost, "/proofs/"+proofID+"/revoke", req, nil)
}

// ExportProof fetches a proof bundled with chain/verify-URL metadata for
// external sharing. Maps to GET /proofs/{id}/export.
func (c *Client) ExportProof(ctx context.Context, proofID string) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodGet, "/proofs/"+proofID+"/export", nil, &out)
	return out, err
}

// Stats fetches engine-wide proof statistics. Maps to GET /stats.
func (c *Client) Stats(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodGet, "/stats", nil, &out)
	return out, err
}

// --- KYC ---------------------------------------------------------------

// KYCCheck checks whether a proof passes KYC. Maps to POST /kyc/check.
func (c *Client) KYCCheck(ctx context.Context, proofID string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{"proof_id": proofID}
	err := c.doRequest(ctx, http.MethodPost, "/kyc/check", req, &out)
	return out, err
}

// KYCGrant grants a user access based on a proof. Maps to POST /kyc/grant.
func (c *Client) KYCGrant(ctx context.Context, user, proofID string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{"user": user, "proof_id": proofID}
	err := c.doRequest(ctx, http.MethodPost, "/kyc/grant", req, &out)
	return out, err
}

// KYCWhitelist checks if a user is whitelisted. Maps to GET /kyc/whitelist/{user}.
func (c *Client) KYCWhitelist(ctx context.Context, user string) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodGet, "/kyc/whitelist/"+user, nil, &out)
	return out, err
}

// --- Inference -----------------------------------------------------------

// InferenceProveRequest is the body for POST /inference/prove.
type InferenceProveRequest struct {
	Agent   string `json:"agent,omitempty"`
	ModelID string `json:"model_id"`
	Input   string `json:"input"`
	Output  string `json:"output"`
	UseCase string `json:"use_case,omitempty"`
	PubKey  string `json:"public_key,omitempty"`
}

// InferenceProve generates a proof of an AI inference. Maps to POST /inference/prove.
func (c *Client) InferenceProve(ctx context.Context, req InferenceProveRequest) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodPost, "/inference/prove", req, &out)
	return out, err
}

// InferenceVerify checks an inference proof. Maps to POST /inference/verify.
func (c *Client) InferenceVerify(ctx context.Context, proofID string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{"proof_id": proofID}
	err := c.doRequest(ctx, http.MethodPost, "/inference/verify", req, &out)
	return out, err
}

// RegisterModelRequest is the body for POST /inference/register-model.
type RegisterModelRequest struct {
	ModelID          string            `json:"model_id"`
	ModelHash        string            `json:"model_hash"`
	VerifierContract string            `json:"verifier_contract,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// RegisterModel registers a model. Maps to POST /inference/register-model.
func (c *Client) RegisterModel(ctx context.Context, req RegisterModelRequest) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodPost, "/inference/register-model", req, &out)
	return out, err
}

// GetModel fetches a registered model. Maps to GET /inference/model/{id}.
func (c *Client) GetModel(ctx context.Context, modelID string) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodGet, "/inference/model/"+modelID, nil, &out)
	return out, err
}

// --- Aggregation -----------------------------------------------------------

// CreateAggregationBatch starts a new batch. Maps to POST /aggregation/create-batch.
func (c *Client) CreateAggregationBatch(ctx context.Context, batchID string, maxProofs int) (map[string]any, error) {
	var out map[string]any
	req := map[string]any{"batch_id": batchID, "max_proofs": maxProofs}
	err := c.doRequest(ctx, http.MethodPost, "/aggregation/create-batch", req, &out)
	return out, err
}

// AddProofToBatch adds a proof hash to an open batch. Maps to POST /aggregation/add-proof.
func (c *Client) AddProofToBatch(ctx context.Context, batchID, proofHash string, leafIndex int) (map[string]any, error) {
	var out map[string]any
	req := map[string]any{"batch_id": batchID, "proof_hash": proofHash, "leaf_index": leafIndex}
	err := c.doRequest(ctx, http.MethodPost, "/aggregation/add-proof", req, &out)
	return out, err
}

// FinalizeBatch closes a batch and produces the aggregate proof. Maps to POST /aggregation/finalize.
func (c *Client) FinalizeBatch(ctx context.Context, batchID string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{"batch_id": batchID}
	err := c.doRequest(ctx, http.MethodPost, "/aggregation/finalize", req, &out)
	return out, err
}

// GetBatch fetches batch state. Maps to GET /aggregation/batch/{id}.
func (c *Client) GetBatch(ctx context.Context, batchID string) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodGet, "/aggregation/batch/"+batchID, nil, &out)
	return out, err
}

// VerifyBatch verifies a finalized batch's aggregate proof. Maps to GET /aggregation/verify-batch/{id}.
func (c *Client) VerifyBatch(ctx context.Context, batchID string) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodGet, "/aggregation/verify-batch/"+batchID, nil, &out)
	return out, err
}

// --- Zero-knowledge --------------------------------------------------------

// VerifyGroth16Conceptual runs the hash-based conceptual Groth16 check.
// Maps to POST /zk/verify-groth16. For a real BN254 pairing check, see
// Groth16RealProve / Groth16RealVerify below.
func (c *Client) VerifyGroth16Conceptual(ctx context.Context, proof, vkHash string, publicInputs []string) (map[string]any, error) {
	var out map[string]any
	req := map[string]any{"proof": proof, "vk_hash": vkHash, "public_inputs": publicInputs}
	err := c.doRequest(ctx, http.MethodPost, "/zk/verify-groth16", req, &out)
	return out, err
}

// Groth16RealProve requests a real Groth16 proof of knowledge of a MiMC
// preimage. `preimage` is a base-10 integer string. Maps to POST /zk/groth16-real/prove.
func (c *Client) Groth16RealProve(ctx context.Context, preimage string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{"preimage": preimage}
	err := c.doRequest(ctx, http.MethodPost, "/zk/groth16-real/prove", req, &out)
	return out, err
}

// Groth16RealVerify verifies a proof returned by Groth16RealProve. `hash` is
// the base-10 integer string and `proofHex` the hex-encoded proof from the
// prove response. Maps to POST /zk/groth16-real/verify.
func (c *Client) Groth16RealVerify(ctx context.Context, hash, proofHex string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{"hash": hash, "proof_hex": proofHex}
	err := c.doRequest(ctx, http.MethodPost, "/zk/groth16-real/verify", req, &out)
	return out, err
}

// --- Post-quantum crypto ----------------------------------------------------

// SignSPHINCS signs a message with the Lamport OTS scheme used as the
// SPHINCS+ stand-in (see docs/KNOWN_LIMITATIONS.md). Maps to POST /pq/sign-sphincs.
// Returns the raw JSON response (signature + public_key, both hex-encoded).
func (c *Client) SignSPHINCS(ctx context.Context, message string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{"message": message}
	err := c.doRequest(ctx, http.MethodPost, "/pq/sign-sphincs", req, &out)
	return out, err
}

// VerifySPHINCS verifies a signature produced by SignSPHINCS. Maps to POST /pq/verify-sphincs.
func (c *Client) VerifySPHINCS(ctx context.Context, message, signatureHex, publicKeyHex string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{"message": message, "signature": signatureHex, "public_key": publicKeyHex}
	err := c.doRequest(ctx, http.MethodPost, "/pq/verify-sphincs", req, &out)
	return out, err
}

// HybridSign signs a message with real Ed25519 + real ML-DSA-65. Maps to POST /pq/hybrid-sign.
func (c *Client) HybridSign(ctx context.Context, message string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{"message": message}
	err := c.doRequest(ctx, http.MethodPost, "/pq/hybrid-sign", req, &out)
	return out, err
}

// HybridVerify verifies a signature produced by HybridSign. Maps to POST /pq/hybrid-verify.
func (c *Client) HybridVerify(ctx context.Context, message, signatureHex, classicPubHex, pqPubHex string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{
		"message": message, "signature": signatureHex,
		"classic_public_key": classicPubHex, "pq_public_key": pqPubHex,
	}
	err := c.doRequest(ctx, http.MethodPost, "/pq/hybrid-verify", req, &out)
	return out, err
}

// BatchProofs submits multiple proofs in a single batch. Maps to POST /proofs/batch.
func (c *Client) BatchProofs(ctx context.Context, proofs []map[string]string, mode string) (map[string]any, error) {
	var out map[string]any
	req := map[string]any{"proofs": proofs, "mode": mode}
	err := c.doRequest(ctx, http.MethodPost, "/proofs/batch", req, &out)
	return out, err
}

// BatchVerifyZK verifies multiple ZK proofs in a single call. Maps to POST /zk/batch-verify.
func (c *Client) BatchVerifyZK(ctx context.Context, proofs []map[string]any) (map[string]any, error) {
	var out map[string]any
	req := map[string]any{"proofs": proofs}
	err := c.doRequest(ctx, http.MethodPost, "/zk/batch-verify", req, &out)
	return out, err
}

// ChallengeZK opens a dispute challenge against a proof. Maps to POST /zk/challenge.
func (c *Client) ChallengeZK(ctx context.Context, proofID, reason string) (map[string]any, error) {
	var out map[string]any
	req := map[string]string{"proof_id": proofID, "reason": reason}
	err := c.doRequest(ctx, http.MethodPost, "/zk/challenge", req, &out)
	return out, err
}

// GetChallengeZK fetches a challenge by its ID. Maps to GET /zk/challenge/{id}.
func (c *Client) GetChallengeZK(ctx context.Context, id string) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodGet, "/zk/challenge/"+id, nil, &out)
	return out, err
}

// ValidateProofChain submits a proof chain for DAG validation. Maps to POST /proof-chain/validate.
func (c *Client) ValidateProofChain(ctx context.Context, chain map[string]any) (map[string]any, error) {
	var out map[string]any
	err := c.doRequest(ctx, http.MethodPost, "/proof-chain/validate", chain, &out)
	return out, err
}
