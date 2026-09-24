package phase2

// ProverConfig holds configuration for distributed proof generation.
type ProverConfig struct {
	// Whether to distribute proof generation across multiple workers.
	Distributed bool `json:"distributed"`
	// Number of worker nodes for parallel proof generation.
	Workers uint `json:"workers"`
	// MPC threshold for distributed key generation.
	MPCThreshold uint `json:"mpc_threshold"`
	// Total MPC parties.
	MPCTotal uint `json:"mpc_total"`
	// Maximum proof generation timeout in seconds.
	TimeoutSecs int64 `json:"timeout_secs"`
}

// DefaultSingleProver returns a config for local single-node proving.
func DefaultSingleProver() ProverConfig {
	return ProverConfig{
		Distributed:  false,
		Workers:      1,
		MPCThreshold: 0,
		MPCTotal:     0,
		TimeoutSecs:  120,
	}
}
