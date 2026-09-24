package attestor

// SGXStub is the hackathon-time stub for an Intel SGX (DCAP) backend.
//
// A real implementation would use github.com/edgelesssys/ego or a
// direct cgo binding to sgx_urts, produce a DCAP quote inside an
// enclave, and validate the quote against Intel PCS / Intel Root CA.
// Real deployments must clear DCAP licensing before turning this
// on — see docs/HARDWARE_ATTESTORS.md.
//
// This stub does none of that.
type SGXStub struct{ stub }

// NewSGXStub returns a fresh Intel SGX stub.
func NewSGXStub() *SGXStub {
	return &SGXStub{stub: stub{kind: KindSGX, vendor: "Intel SGX DCAP (stub — no PCS call, no enclave)"}}
}
