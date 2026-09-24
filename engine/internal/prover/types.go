package prover

type Proof struct {
	ID              string   `json:"id"`
	Agent           string   `json:"agent"`
	PH              string   `json:"proof_hash"`
	IH              string   `json:"input_hash"`
	OH              string   `json:"output_hash"`
	MH              string   `json:"model_hash"`
	Root            string   `json:"merkle_root"`
	Path            []string `json:"merkle_path"`
	Idx             int      `json:"leaf_index"`
	TS              int64    `json:"timestamp"`
	Valid           bool     `json:"valid"`
	Revoked         bool     `json:"revoked"`
	UseCase         string   `json:"use_case"`
	PubKey          string   `json:"public_key,omitempty"`
	Deploy          string   `json:"deploy_hash,omitempty"`
	AnchorHash      string   `json:"anchor_hash,omitempty"`
	AnchoringStatus string   `json:"anchoring_status,omitempty"`
	GenMs           int64    `json:"generation_ms"`
	Mode            string   `json:"mode,omitempty"`
}

type MerkleNode struct {
	Hash  [32]byte
	Left  *MerkleNode
	Right *MerkleNode
}

type ProofBundle struct {
	Root  string   `json:"root"`
	Leafs []string `json:"leafs"`
	Count int      `json:"count"`
}

type Stats struct {
	Total    int            `json:"total_proofs"`
	Valid    int            `json:"valid_proofs"`
	Revoked  int            `json:"revoked_proofs"`
	Agents   int            `json:"unique_agents"`
	AvgGenMs float64        `json:"avg_generation_ms"`
	MaxDepth int            `json:"max_merkle_depth"`
	UseCases map[string]int `json:"use_cases"`
}
