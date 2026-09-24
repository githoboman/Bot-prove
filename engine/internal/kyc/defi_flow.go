package kyc

import (
	"fmt"
	"log/slog"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
)

type DeFiFlow struct {
	eng  *prover.ProofEngine
	demo *DemoKYC
}

func NewDeFiFlow(eng *prover.ProofEngine) *DeFiFlow {
	return &DeFiFlow{eng: eng, demo: NewDemo(eng)}
}

func (f *DeFiFlow) RunDemo(agentID string) error {
	input := []byte(`{"user":"alice","doc_type":"passport"}`)
	output := []byte(`{"verified":true,"score":95}`)
	model := []byte("kyc-model-v1")

	p := f.eng.Generate(agentID, input, output, model, "kyc")
	slog.Info("proof generated", "proof_id", p.ID)

	_, err := f.demo.GrantAccess("alice", p.ID)
	if err != nil {
		return fmt.Errorf("grant: %w", err)
	}

	ok := f.demo.IsWhitelisted("alice")
	slog.Info("KYC demo completed", "user", "alice", "whitelisted", ok)
	return nil
}
