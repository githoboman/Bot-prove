package phase2

// TargetVM specifies the virtual machine target for proof verification.
type TargetVM int

const (
	// CasperVM is the native Casper Network Wasm VM.
	CasperVM TargetVM = iota
	// EVM is the Ethereum Virtual Machine (for cross-chain verifiers).
	EVM
	// Future represents extensible VM targets beyond CasperVM and EVM.
	Future
)

// String returns the name of the target VM.
func (t TargetVM) String() string {
	switch t {
	case CasperVM:
		return "casper-wasm"
	case EVM:
		return "evm"
	case Future:
		return "future"
	default:
		return "unknown"
	}
}
