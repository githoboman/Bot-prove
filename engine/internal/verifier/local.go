package verifier

import (
	"encoding/hex"
	"fmt"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/hasher"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
)

type LocalVerifier struct{}

func New() *LocalVerifier { return &LocalVerifier{} }

func (v *LocalVerifier) VerifyProof(p *prover.Proof, input, output, model []byte) error {
	if p == nil {
		return fmt.Errorf("nil proof")
	}
	if p.Revoked {
		return fmt.Errorf("proof %s revoked", p.ID)
	}
	if !p.Valid {
		return fmt.Errorf("proof %s invalid", p.ID)
	}

	ih := hasher.HexHash(input)
	if ih != p.IH {
		return fmt.Errorf("input hash mismatch: got %s want %s", ih, p.IH)
	}

	oh := hasher.HexHash(output)
	if oh != p.OH {
		return fmt.Errorf("output hash mismatch")
	}

	mh := hasher.HexHash(model)
	if mh != p.MH {
		return fmt.Errorf("model hash mismatch")
	}

	ph := hasher.CommitHash(input, output, model)
	if ph != p.PH {
		return fmt.Errorf("proof hash mismatch")
	}

	if p.Root != "" && len(p.Path) > 0 {
		if !prover.VerifyPath(input, p.Path, p.Root, p.Idx) {
			return fmt.Errorf("merkle path invalid")
		}
	}

	return nil
}

func (v *LocalVerifier) QuickCheck(ph string, input, output, model []byte) bool {
	return hasher.CommitHash(input, output, model) == ph
}

func (v *LocalVerifier) ValidateHash(h string) bool {
	b, err := hex.DecodeString(h)
	return err == nil && len(b) == 32
}
