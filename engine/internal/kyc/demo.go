package kyc

import (
	"fmt"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
)

type DemoKYC struct {
	eng       *prover.ProofEngine
	whitelist map[string]*DeFiAccess
}

func NewDemo(eng *prover.ProofEngine) *DemoKYC {
	return &DemoKYC{eng: eng, whitelist: make(map[string]*DeFiAccess)}
}

func (d *DemoKYC) CheckKYC(pid string) (*KYCResult, error) {
	valid, err := d.eng.Verify(pid)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}

	return &KYCResult{
		ProofID:  pid,
		Verified: valid,
		TS:       time.Now().Unix(),
	}, nil
}

func (d *DemoKYC) GrantAccess(user, pid string) (*DeFiAccess, error) {
	res, err := d.CheckKYC(pid)
	if err != nil {
		return nil, err
	}
	if !res.Verified {
		return nil, fmt.Errorf("kyc failed for proof %s", pid)
	}

	access := &DeFiAccess{User: user, Whitelisted: true, ProofID: pid}
	d.whitelist[user] = access
	return access, nil
}

func (d *DemoKYC) IsWhitelisted(user string) bool {
	a, ok := d.whitelist[user]
	return ok && a.Whitelisted
}

// Restore rehydrates a whitelist entry loaded from persistent storage,
// bypassing the KYC proof re-check (the proof was already verified when the
// grant originally happened; re-verifying on every restart would also
// silently drop entries whose backing proof has since been pruned).
func (d *DemoKYC) Restore(user, proofID string) {
	d.whitelist[user] = &DeFiAccess{User: user, Whitelisted: true, ProofID: proofID}
}
