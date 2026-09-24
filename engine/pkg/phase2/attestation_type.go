package phase2

// AttestationType indicates the hardware environment where proof was generated.
type AttestationType int

const (
	// Software attestation (default) — no hardware guarantees.
	AttestSoftware AttestationType = iota
	// TPM 2.0 attestation — Trusted Platform Module quote.
	AttestTPM
	// Intel SGX attestation — enclave remote attestation via DCAP.
	AttestSGX
	// AMD SEV attestation — encrypted VM attestation.
	AttestSEV
	// ARM TrustZone attestation — secure world execution proof.
	AttestTrustZone
)

// String returns the human-readable name of the attestation type.
func (a AttestationType) String() string {
	switch a {
	case AttestSoftware:
		return "software"
	case AttestTPM:
		return "tpm2.0"
	case AttestSGX:
		return "intel-sgx"
	case AttestSEV:
		return "amd-sev"
	case AttestTrustZone:
		return "arm-trustzone"
	default:
		return "unknown"
	}
}
