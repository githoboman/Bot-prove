// Package receipts implements the provenance-lineage receipt layer.
//
// A DecisionReceipt is the durable, signed record of one decision cycle:
// the aggregated verdict, the per-facet outputs, the upstream provider
// receipts (for lineage graphs), an optional HITL resolution, and a
// cryptographic proof binding all of the above under the engine's active
// signing key.
//
// The receipt is designed to be emitted in three interoperable shapes:
//
//   * INTERNAL — the canonical Go / JSON representation this package
//                stores and hashes. Every other format is derived from it,
//                so the internal shape is the source of truth for the
//                proof.signature calculation.
//   * W3C-VC   — W3C Verifiable Credentials 2.0 JSON. Suitable for
//                consumption by any VC verifier: the receipt maps onto a
//                VC whose credentialSubject.id is the decision id, whose
//                issuer is the engine's DID, and whose proof carries the
//                Ed25519 / ML-DSA signature.
//   * AGENT-RECEIPT — the draft agentreceipts.org shape. A superset of
//                the facet outputs and an evidence pointer at the Merkle
//                batch receipt from engine/internal/prover.
//
// The three shapes are *lossless in one direction*: internal → W3C-VC and
// internal → agent-receipt. Round-tripping W3C-VC → internal is not
// supported — receipts are produced, not consumed, by CasperProver, so
// unmarshalling is intentionally omitted. This keeps the surface small and
// makes the canonical hash the only thing that ever needs to be recomputed.
//
// The package intentionally does NOT depend on an OpenTelemetry SDK. The
// receipt service exposes an OtelSink interface which packages a receipt
// as a span record; the default implementation writes JSONL to a
// configurable file. A production deployment plugs in an OTel-native sink
// by implementing the same interface — see docs/PROVENANCE_LINEAGE.md for
// the exact contract and a reference collector wiring.
package receipts

import (
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/decision/attest"
)

// Verdict mirrors attest.Verdict as its canonical string form. Copied
// (not aliased) so this package stays a leaf dependency of decision and
// can be imported by API handlers without a cycle.
type Verdict string

const (
	VerdictApprove Verdict = "APPROVE"
	VerdictAbstain Verdict = "ABSTAIN"
	VerdictReject  Verdict = "REJECT"
)

// FromDecision maps a attest.Verdict onto the receipts.Verdict string.
func FromDecision(v attest.Verdict) Verdict {
	switch v {
	case attest.VerdictApprove:
		return VerdictApprove
	case attest.VerdictAbstain:
		return VerdictAbstain
	case attest.VerdictReject:
		return VerdictReject
	default:
		return VerdictAbstain
	}
}

// FacetOutput is one facet evaluator's judgement, stripped down to the
// receipt-relevant fields. Confidence is only meaningful for
// non-critical facets; the receipt keeps the number so a downstream
// verifier can re-run the aggregation policy off the receipt alone.
type FacetOutput struct {
	Kind       string  `json:"kind"`
	Verdict    Verdict `json:"verdict"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason,omitempty"`
}

// ProviderReceipt is a lineage pointer: an upstream provider's receipt
// hash (a hex sha256), plus a short human-readable name. The hash is
// what makes receipts form a DAG — a downstream receipt embeds one
// ProviderReceipt per upstream provider it consulted.
type ProviderReceipt struct {
	Provider    string `json:"provider"`
	TrustLevel  string `json:"trust_level"`
	ReceiptHash string `json:"receipt_hash"`
}

// HITLResolution records the human review outcome, when a receipt was
// gated by the hitl service. Absent for non-escalated paths.
type HITLResolution struct {
	TicketID   string    `json:"ticket_id"`
	Action     string    `json:"action"`
	Reviewer   string    `json:"reviewer,omitempty"`
	ResolvedAt time.Time `json:"resolved_at"`
	Note       string    `json:"note,omitempty"`
}

// Proof is the cryptographic binding: an algorithm identifier that
// matches one of internal/crypto.Algo, the signature bytes (base64), and
// a verification_method — a DID-URL that names the engine's active key.
//
// The signature covers CanonicalHash(receipt) — see canonical.go — not
// the JSON serialisation. This makes the receipt hash reproducible
// byte-for-byte across languages, which the W3C-VC and agent-receipt
// emitters rely on so the derived shapes carry the SAME proof.signature.
type Proof struct {
	Scheme             string `json:"scheme"`
	Signature          string `json:"signature"`
	VerificationMethod string `json:"verification_method"`
	SignedAt           time.Time
}

// DecisionReceipt is the canonical receipt shape.
type DecisionReceipt struct {
	// ID is a UUIDv4-shaped hex string minted at emit time. It appears
	// in W3C-VC as credentialSubject.id and in agent-receipt as the
	// top-level id field.
	ID string `json:"id"`
	// IssuedAt is the RFC 3339 emit time in UTC.
	IssuedAt time.Time `json:"issued_at"`
	// Issuer is the engine's DID. The default is derived from the
	// keystore's active key id; a deployment MAY override via the
	// receipts.Service.IssuerDID knob.
	Issuer string `json:"issuer"`
	// Subject is the decision id (sha256-hex over the decision fields,
	// see attest.Decision.ID). Downstream verifiers use this to
	// correlate the receipt with the on-chain proof-registry entry.
	Subject string `json:"subject"`
	// SpecID is the decision's declared spec.
	SpecID string `json:"spec_id"`
	// EvidenceRoot is the merkle root of the batch of prover events
	// that produced this decision, hex-encoded. Empty when the receipt
	// is emitted outside a prover batch.
	EvidenceRoot string `json:"evidence_root,omitempty"`
	// ModelID names the model version consulted for this attest. The
	// on-chain verifier-gate binds evidence_root to model_id, and the
	// receipt keeps a copy so an off-chain verifier can re-derive the
	// binding.
	ModelID string `json:"model_id,omitempty"`
	// Aggregate is the final verdict.
	Aggregate Verdict `json:"aggregate"`
	// VetoedBy is set when a critical facet forced a REJECT. Empty
	// otherwise. Exposed here so a downstream that only reads the
	// receipt can distinguish veto from quorum-driven REJECT.
	VetoedBy string `json:"vetoed_by,omitempty"`
	// Confidence is the mean confidence over non-critical facets. Kept
	// here so the receipt is self-contained for policy replay.
	Confidence float64 `json:"confidence"`
	// Facets is the sorted list of per-facet outputs (sorted by Kind,
	// ascending). Sorting is enforced by CanonicalHash — see
	// canonical.go — so a downstream that re-serialises the JSON in a
	// different order still gets the same hash.
	Facets []FacetOutput `json:"facets"`
	// ProviderReceipts is the sorted list of upstream provider
	// receipts. Sorted by ReceiptHash (ascending). Empty when no
	// upstream lineage exists.
	ProviderReceipts []ProviderReceipt `json:"provider_receipts,omitempty"`
	// HITL is the human-review record, present only for escalated
	// paths.
	HITL *HITLResolution `json:"hitl_resolution,omitempty"`
	// Proof is populated by Service.Emit after signing. Absent until
	// the receipt has been signed; a receipt without Proof MUST NOT
	// cross the API boundary.
	Proof *Proof `json:"proof,omitempty"`
}
