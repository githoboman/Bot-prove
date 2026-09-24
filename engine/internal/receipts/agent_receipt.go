package receipts

import "time"

// Agent Receipt (agentreceipts.org draft) mapping.
//
// The draft is not a W3C REC; the shape below tracks the current
// agentreceipts.org spec (v0.3 as of 2026-07). Adjust the tag list and
// evidence shape if the spec evolves — the fields marked "cp_" are the
// engine's private extensions and are always safe to keep.

// AgentReceipt is the AR-shape emitted by ToAgentReceipt.
type AgentReceipt struct {
	Version   string             `json:"ar_version"`
	ID        string             `json:"id"`
	Issuer    string             `json:"issuer"`
	IssuedAt  time.Time          `json:"issued_at"`
	Subject   string             `json:"subject"`
	Verdict   string             `json:"verdict"`
	Facets    []ARFacet          `json:"facets"`
	Evidence  ARSection          `json:"evidence"`
	Lineage   []ARLineage        `json:"lineage,omitempty"`
	Review    *ARReview          `json:"review,omitempty"`
	Tags      []string           `json:"tags"`
	CPExtra   map[string]any     `json:"cp_extra"`
	Signature ARSignatureEnvelope `json:"signature"`
}

// ARFacet is the AR-flavoured facet output.
type ARFacet struct {
	Kind       string  `json:"kind"`
	Verdict    string  `json:"verdict"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason,omitempty"`
}

// ARSection points at the on-chain / off-chain evidence.
type ARSection struct {
	Root    string `json:"root,omitempty"`
	ModelID string `json:"model_id,omitempty"`
	Digest  string `json:"digest"`
}

// ARLineage points at an upstream provider receipt.
type ARLineage struct {
	Provider   string `json:"provider"`
	TrustLevel string `json:"trust_level"`
	Receipt    string `json:"receipt_hash"`
}

// ARReview mirrors the HITL resolution.
type ARReview struct {
	TicketID   string    `json:"ticket_id"`
	Action     string    `json:"action"`
	Reviewer   string    `json:"reviewer,omitempty"`
	Note       string    `json:"note,omitempty"`
	ResolvedAt time.Time `json:"resolved_at"`
}

// ARSignatureEnvelope carries the same signature bytes as the internal
// Proof, but in the AR-shape.
type ARSignatureEnvelope struct {
	Alg                string `json:"alg"`
	Value              string `json:"value"`
	VerificationMethod string `json:"verification_method"`
}

// ToAgentReceipt emits the receipt in agentreceipts.org draft shape.
// Returns an error when the receipt has not been signed.
func ToAgentReceipt(r DecisionReceipt) (AgentReceipt, error) {
	if r.Proof == nil {
		return AgentReceipt{}, errUnsigned("agent-receipt: receipt has no proof — refusing to emit unsigned envelope")
	}
	unsigned := r
	unsigned.Proof = nil
	digest := CanonicalHash(unsigned)

	facets := make([]ARFacet, 0, len(r.Facets))
	for _, f := range r.Facets {
		facets = append(facets, ARFacet{
			Kind:       f.Kind,
			Verdict:    string(f.Verdict),
			Confidence: f.Confidence,
			Reason:     f.Reason,
		})
	}
	lineage := make([]ARLineage, 0, len(r.ProviderReceipts))
	for _, pr := range r.ProviderReceipts {
		lineage = append(lineage, ARLineage{
			Provider:   pr.Provider,
			TrustLevel: pr.TrustLevel,
			Receipt:    pr.ReceiptHash,
		})
	}
	var review *ARReview
	if r.HITL != nil {
		review = &ARReview{
			TicketID:   r.HITL.TicketID,
			Action:     r.HITL.Action,
			Reviewer:   r.HITL.Reviewer,
			Note:       r.HITL.Note,
			ResolvedAt: r.HITL.ResolvedAt,
		}
	}

	tags := []string{"casper-prover", "decision-receipt", "spec-" + r.SpecID}
	if r.VetoedBy != "" {
		tags = append(tags, "vetoed-by-"+r.VetoedBy)
	}
	cpExtra := map[string]any{
		"aggregate":   string(r.Aggregate),
		"confidence":  r.Confidence,
		"spec_id":     r.SpecID,
		"vetoed_by":   r.VetoedBy,
		"engine_algo": r.Proof.Scheme,
	}

	return AgentReceipt{
		Version:  "0.3",
		ID:       r.ID,
		Issuer:   r.Issuer,
		IssuedAt: r.IssuedAt,
		Subject:  r.Subject,
		Verdict:  string(r.Aggregate),
		Facets:   facets,
		Evidence: ARSection{
			Root:    r.EvidenceRoot,
			ModelID: r.ModelID,
			Digest:  digest,
		},
		Lineage: lineage,
		Review:  review,
		Tags:    tags,
		CPExtra: cpExtra,
		Signature: ARSignatureEnvelope{
			Alg:                r.Proof.Scheme,
			Value:              r.Proof.Signature,
			VerificationMethod: r.Proof.VerificationMethod,
		},
	}, nil
}
