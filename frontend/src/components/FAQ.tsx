import { useState } from 'react'
import { ChevronDown } from 'lucide-react'

const faqs = [
  { q: 'What exactly does Bot Prove prove?', a: 'It proves that a specific AI agent, using a specific model, received a specific input and produced a specific output at a specific time. The proof is cryptographic — any change to any parameter invalidates it.' },
  { q: 'Is this actual zero-knowledge?', a: 'Yes. Bot Prove includes real BN254 Groth16 ZK proofs powered by gnark — with genuine R1CS circuits, trusted setup, and pairing-based verification. It also provides Merkle tree-based proofs for lightweight use cases where full ZK is unnecessary.' },
  { q: 'Why anchor proofs on Casper?', a: 'Casper provides finality, low gas costs, and a WASM-based VM suitable for proof storage contracts. Proofs are immutable once deployed — no one can alter or delete them. Eight smart contracts are live on testnet, covering registration, verification, KYC gating, staking/slashing, proof aggregation, and governance.' },
  { q: 'What about quantum computing threats?', a: 'Bot Prove includes real post-quantum cryptography: ML-DSA-65 (FIPS 204, cloudflare/circl), Lamport one-time signatures (hash-based OTS), and hybrid Ed25519→ML-DSA signing. The Lamport OTS path occupies the hash-based (SPHINCS+ family) slot until a Go SLH-DSA implementation ships.' },
  { q: 'How does the SDK / MCP integration work?', a: 'The Go SDK provides 32 methods mapping 1:1 to all API endpoints. The MCP server exposes 32 tools for AI agent frameworks like Claude Desktop. Install with `go get` or connect via MCP stdio — no custom integration needed.' },
  { q: 'Can I verify a proof without the original data?', a: 'Yes. You only need the proof hash and Merkle root to verify integrity. The original input/output data is never required for verification — only their hashes.' },
  { q: 'What is proof-chain DAG validation?', a: 'For multi-step AI workflows, Bot Prove validates the entire chain of proofs as a directed acyclic graph. It checks for cycles, verifies input/output continuity between steps, and enforces a single root — ensuring no step was tampered with or skipped.' },
]

export default function FAQ() {
  const [open, setOpen] = useState<number | null>(0)

  return (
    <section id="faq" className="py-24">
      <div className="cp-section">
        <div className="max-w-3xl mx-auto">
          <div className="text-center mb-14">
            <p className="text-xs font-mono text-red-500 tracking-widest mb-3">FAQ</p>
            <h2 className="text-3xl font-extrabold text-white">Common questions.</h2>
          </div>

          <div className="space-y-2">
            {faqs.map((f, i) => (
              <div key={i} className="border border-cp-border rounded-xl overflow-hidden bg-cp-card">
                <button
                  onClick={() => setOpen(open === i ? null : i)}
                  className="w-full flex items-center justify-between p-5 text-left hover:bg-white/[0.02] transition-colors"
                >
                  <span className="font-semibold text-white pr-4">{f.q}</span>
                  <ChevronDown className={`w-5 h-5 text-gray-600 shrink-0 transition-transform ${open === i ? 'rotate-180' : ''}`} />
                </button>
                <div className={`overflow-hidden transition-all duration-300 ${open === i ? 'max-h-60' : 'max-h-0'}`}>
                  <p className="px-5 pb-5 text-gray-400 leading-relaxed text-sm">{f.a}</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}
