package sdk

// Typed request/response shapes for the v1 primitives.
//
// These mirror the JSON the CasperProver API returns; unknown fields on the
// wire are ignored (Go's json decoder default).

// ProveRequest is the body of POST /v1/proofs.
type ProveRequest struct {
	Agent   string `json:"agent"`
	Input   string `json:"input"`
	Output  string `json:"output"`
	Model   string `json:"model"`
	UseCase string `json:"use_case,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

// ProveResponse is what the server returns for a successful prove.
type ProveResponse struct {
	ID         string `json:"id"`
	ProofHash  string `json:"proof_hash,omitempty"`
	VKHash     string `json:"vk_hash,omitempty"`
	InputHash  string `json:"input_hash,omitempty"`
	OutputHash string `json:"output_hash,omitempty"`
	ModelHash  string `json:"model_hash,omitempty"`
	Verdict    string `json:"verdict,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	// Raw retains the full server response so callers can reach for fields
	// the SDK has not surfaced yet.
	Raw map[string]any `json:"-"`
}

// VerifyResponse is the shape returned by POST /v1/verify.
type VerifyResponse struct {
	Valid   bool           `json:"valid"`
	ProofID string         `json:"proof_id,omitempty"`
	Reason  string         `json:"reason,omitempty"`
	Raw     map[string]any `json:"-"`
}

// BatchResponse is the shape returned by POST /v1/batch/verify-zk.
type BatchResponse struct {
	Verified []string       `json:"verified,omitempty"`
	Failed   []string       `json:"failed,omitempty"`
	Total    int            `json:"total,omitempty"`
	Mode     string         `json:"mode,omitempty"`
	Raw      map[string]any `json:"-"`
}

// AnchorResponse is the shape returned by POST /v1/proofs/{id}/anchor.
type AnchorResponse struct {
	ProofID       string         `json:"proof_id,omitempty"`
	TxHash        string         `json:"tx_hash,omitempty"`
	BlockHash     string         `json:"block_hash,omitempty"`
	AnchoredAt    string         `json:"anchored_at,omitempty"`
	StrictMode    bool           `json:"strict_mode,omitempty"`
	DeployerKeyID string         `json:"deployer_key_id,omitempty"`
	Raw           map[string]any `json:"-"`
}
