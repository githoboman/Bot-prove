package kyc

type KYCResult struct {
	User      string `json:"user"`
	ProofID   string `json:"proof_id"`
	Verified  bool   `json:"verified"`
	TS        int64  `json:"timestamp"`
	GrantedAt int64  `json:"granted_at,omitempty"`
}

type DeFiAccess struct {
	User        string `json:"user"`
	Whitelisted bool   `json:"whitelisted"`
	ProofID     string `json:"proof_id"`
}
