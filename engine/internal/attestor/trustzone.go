package attestor

// TrustZoneStub is the hackathon-time stub for an ARM TrustZone /
// OP-TEE backend.
//
// A real implementation would drive OP-TEE from a small C or Rust
// helper (there is no first-party Go binding today) and package the
// resulting attestation into Quote.Blob. Relevant mostly for
// on-device / edge agents, not server-side CasperProver deployments.
//
// This stub does none of that.
type TrustZoneStub struct{ stub }

// NewTrustZoneStub returns a fresh ARM TrustZone stub.
func NewTrustZoneStub() *TrustZoneStub {
	return &TrustZoneStub{stub: stub{kind: KindTrustZone, vendor: "ARM TrustZone / OP-TEE (stub — no TA invocation)"}}
}
