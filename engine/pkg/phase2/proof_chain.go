package phase2

// ChainStatus indicates the verification state of a proof chain.
type ChainStatus int

const (
	ChainPending           ChainStatus = iota
	ChainPartiallyVerified             // some steps verified
	ChainFullyVerified                 // all steps verified
	ChainFailed                        // a step failed verification
	ChainBroken                        // input/output hash mismatch between steps
)

// ProofChain represents a directed acyclic graph of linked proofs.
type ProofChain struct {
	ID          string      `json:"id"`
	RootProofID string      `json:"root_proof_id"`
	Depth       int         `json:"depth"`
	TotalSteps  int         `json:"total_steps"`
	Status      ChainStatus `json:"status"`
	Steps       []ChainStep `json:"steps"`
	CreatedAt   int64       `json:"created_at"`
}

// ChainStep is one node in the proof DAG.
type ChainStep struct {
	ProofID    string   `json:"proof_id"`
	ParentIDs  []string `json:"parent_ids"`  // empty for root
	ModelHash  string   `json:"model_hash"`
	InputHash  string   `json:"input_hash"`  // must equal parent's OutputHash
	OutputHash string   `json:"output_hash"`
	StepIndex  int      `json:"step_index"`
	Verified   bool     `json:"verified"`
}
