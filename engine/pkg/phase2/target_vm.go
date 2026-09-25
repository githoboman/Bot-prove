package phase2

// TargetVM specifies the virtual machine target for proof verification.
type TargetVM int

const (
	// BotChainVM is the native BOT Chain Network Wasm VM.
	BotChainVM TargetVM = iota
	// EVM is the Ethereum Virtual Machine (for cross-chain verifiers).
	EVM
	// Future represents extensible VM targets beyond BotChainVM and EVM.
	Future
)

// String returns the name of the target VM.
func (t TargetVM) String() string {
	switch t {
	case BotChainVM:
		return "bot-wasm"
	case EVM:
		return "evm"
	case Future:
		return "future"
	default:
		return "unknown"
	}
}
