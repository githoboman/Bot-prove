package attestor

// SEVSNPStub is the hackathon-time stub for an AMD SEV-SNP backend.
//
// A real implementation would use github.com/google/go-sev-guest to
// request an attestation report from the AMD-SP inside a
// confidential-VM guest, then validate the report against the AMD
// KDS (Key Distribution Service) chain. The VCEK cert must be
// cached to keep KDS calls out of the hot path.
//
// This stub does none of that.
type SEVSNPStub struct{ stub }

// NewSEVSNPStub returns a fresh AMD SEV-SNP stub.
func NewSEVSNPStub() *SEVSNPStub {
	return &SEVSNPStub{stub: stub{kind: KindSEVSNP, vendor: "AMD SEV-SNP (stub — no /dev/sev-guest call, no KDS)"}}
}
