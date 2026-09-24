package attestor

// TPMStub is the hackathon-time stub for a TPM 2.0 backend.
//
// A real implementation would wrap github.com/google/go-tpm/tpm2,
// speak to /dev/tpm0 (or /dev/tpmrm0), request a Quote over PCRs
// bound to a nonce derived from the challenge, and package the
// resulting attestation blob + endorsement-key certificate chain
// into Quote.Blob.
//
// This stub does none of that. Available() == false; Attest and
// Verify both return ErrAttestorUnavailable.
type TPMStub struct{ stub }

// NewTPMStub returns a fresh TPM 2.0 stub.
func NewTPMStub() *TPMStub {
	return &TPMStub{stub: stub{kind: KindTPM, vendor: "TPM 2.0 (stub — no /dev/tpm0 call)"}}
}
