package receipts

import (
	"encoding/json"
	"time"
)

// W3C Verifiable Credentials 2.0 mapping.
//
// See https://www.w3.org/TR/vc-data-model-2.0/ . The receipt maps as:
//
//   context           = ["https://www.w3.org/ns/credentials/v2",
//                        "https://casperprover.io/context/cp-receipt-v1"]
//   type              = ["VerifiableCredential", "CasperProverDecisionReceipt"]
//   id                = receipt.id (urn:uuid:...)
//   issuer            = receipt.issuer                (did)
//   validFrom         = receipt.issued_at             (rfc3339)
//   credentialSubject = { id: receipt.subject, ... facet outputs ... }
//   proof.type        = "CasperProverProof2026"       (registered profile)
//   proof.cryptosuite = receipt.proof.scheme
//   proof.proofValue  = receipt.proof.signature       (base64)
//   proof.verification_method = receipt.proof.verification_method
//   proof.created     = receipt.proof.signed_at
//
// The generated document embeds `cp:canonical_hash` under
// credentialSubject so a verifier can re-derive the signed digest
// without pulling in this Go package.

// W3CCredential is the minimal VC 2.0 wire shape this package emits.
// Only fields the spec requires are included; the whole document is
// still a valid VC.
type W3CCredential struct {
	Context           []string        `json:"@context"`
	ID                string          `json:"id"`
	Type              []string        `json:"type"`
	Issuer            string          `json:"issuer"`
	ValidFrom         time.Time       `json:"validFrom"`
	CredentialSubject json.RawMessage `json:"credentialSubject"`
	Proof             W3CProof        `json:"proof"`
}

// W3CProof is the VC-2.0 proof envelope.
type W3CProof struct {
	Type               string    `json:"type"`
	Cryptosuite        string    `json:"cryptosuite"`
	Created            time.Time `json:"created"`
	VerificationMethod string    `json:"verificationMethod"`
	ProofPurpose       string    `json:"proofPurpose"`
	ProofValue         string    `json:"proofValue"`
}

// ToW3CVC emits the receipt in W3C Verifiable Credentials 2.0 shape.
// Returns an error when the receipt has not been signed.
func ToW3CVC(r DecisionReceipt) (W3CCredential, error) {
	if r.Proof == nil {
		return W3CCredential{}, errW3CUnsigned
	}
	unsigned := r
	unsigned.Proof = nil
	digest := CanonicalHash(unsigned)

	// credentialSubject holds the decision-shaped payload. Fields are
	// namespaced under "cp:" so a downstream that walks a VC without
	// the CP context does not mistake them for standard VC properties.
	subject := map[string]any{
		"id":                 r.Subject,
		"cp:spec_id":         r.SpecID,
		"cp:aggregate":       string(r.Aggregate),
		"cp:confidence":      r.Confidence,
		"cp:facets":          r.Facets,
		"cp:evidence_root":   r.EvidenceRoot,
		"cp:model_id":        r.ModelID,
		"cp:canonical_hash":  digest,
		"cp:receipt_version": "1.0",
	}
	if r.VetoedBy != "" {
		subject["cp:vetoed_by"] = r.VetoedBy
	}
	if len(r.ProviderReceipts) > 0 {
		subject["cp:provider_receipts"] = r.ProviderReceipts
	}
	if r.HITL != nil {
		subject["cp:hitl_resolution"] = r.HITL
	}
	subj, err := json.Marshal(subject)
	if err != nil {
		return W3CCredential{}, err
	}

	return W3CCredential{
		Context: []string{
			"https://www.w3.org/ns/credentials/v2",
			"https://casperprover.io/context/cp-receipt-v1",
		},
		ID:                "urn:uuid:" + r.ID,
		Type:              []string{"VerifiableCredential", "CasperProverDecisionReceipt"},
		Issuer:            r.Issuer,
		ValidFrom:         r.IssuedAt,
		CredentialSubject: subj,
		Proof: W3CProof{
			Type:               "CasperProverProof2026",
			Cryptosuite:        r.Proof.Scheme,
			Created:            r.Proof.SignedAt,
			VerificationMethod: r.Proof.VerificationMethod,
			ProofPurpose:       "assertionMethod",
			ProofValue:         r.Proof.Signature,
		},
	}, nil
}

var errW3CUnsigned = errUnsigned("w3c-vc: receipt has no proof — refusing to emit unsigned credential")

type errUnsigned string

func (e errUnsigned) Error() string { return string(e) }
