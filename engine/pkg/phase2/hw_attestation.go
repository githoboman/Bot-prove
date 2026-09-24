package phase2

import (
	"crypto/sha256"
	"time"
)

// HWQuote represents a hardware attestation quote from a trusted execution environment.
type HWQuote struct {
	AttestationType AttestationType `json:"attestation_type"`
	// MRENCLAVE (SGX) or PCR values (TPM).
	EnclaveHash []byte `json:"enclave_hash"`
	// CPU-generated signature over the report data.
	CPUSignature []byte `json:"cpu_signature"`
	// User-supplied data bound to the quote (typically proof hash).
	ReportData []byte `json:"report_data"`
	Timestamp  int64  `json:"timestamp"`
	Platform   PlatformInfo `json:"platform_info"`
}

// PlatformInfo describes the hardware platform that generated the quote.
type PlatformInfo struct {
	Vendor    string `json:"vendor"`      // "Intel", "AMD", "ARM"
	Model     string `json:"model"`       // e.g. "Xeon E-2288G"
	FWVersion string `json:"fw_version"`  // firmware version
	TCBLevel  int    `json:"tcb_level"`   // Trusted Computing Base level
}

// HardwareAttestor generates and verifies hardware attestation quotes.
type HardwareAttestor interface {
	GenerateQuote(reportData []byte) (*HWQuote, error)
	VerifyQuote(quote *HWQuote) (bool, error)
	GetPlatformInfo() (*PlatformInfo, error)
}

// SoftwareAttestor is a fallback attestor that uses software-only hashing.
// It does NOT provide hardware security guarantees.
type SoftwareAttestor struct{}

// GenerateQuote creates a software-only attestation (not secure, API-compatible).
func (s *SoftwareAttestor) GenerateQuote(reportData []byte) (*HWQuote, error) {
	h := sha256.Sum256(reportData)
	return &HWQuote{
		AttestationType: AttestSoftware,
		EnclaveHash:     h[:],
		CPUSignature:    nil, // no hardware signature available
		ReportData:      reportData,
		Timestamp:       time.Now().Unix(),
		Platform: PlatformInfo{
			Vendor: "software",
			Model:  "none",
		},
	}, nil
}

// VerifyQuote checks a software attestation (always returns true for software mode).
func (s *SoftwareAttestor) VerifyQuote(quote *HWQuote) (bool, error) {
	if quote.AttestationType != AttestSoftware {
		return false, nil
	}
	h := sha256.Sum256(quote.ReportData)
	for i, b := range h {
		if quote.EnclaveHash[i] != b {
			return false, nil
		}
	}
	return true, nil
}

// GetPlatformInfo returns software platform info.
func (s *SoftwareAttestor) GetPlatformInfo() (*PlatformInfo, error) {
	return &PlatformInfo{Vendor: "software", Model: "none"}, nil
}
