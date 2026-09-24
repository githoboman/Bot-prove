import { useEffect, useRef, useState } from 'react'
import { Lock, Eye, Fingerprint, Zap, GitBranch, Server, Shield, Layers, Cpu, Key } from 'lucide-react'

const features = [
  { icon: Lock, title: 'Cryptographic Proofs', desc: 'SHA-256 hashing and Merkle trees ensure every proof is tamper-evident. Change one bit of input and the proof breaks.' },
  { icon: Eye, title: 'Verifiable by Anyone', desc: 'Merkle roots are public and on-chain. Anyone holding the original input/output can verify inclusion; the on-chain state itself stores only hashes.' },
  { icon: Fingerprint, title: 'Model Fingerprinting', desc: 'Each proof includes a hash of the model used. Track which model version made which decision across time.' },
  { icon: Zap, title: 'Sub-Second Generation', desc: 'Proof generation takes milliseconds. No GPU required. No heavy computation. Just cryptographic hashing.' },
  { icon: GitBranch, title: 'Merkle Path Verification', desc: 'Each proof includes its Merkle path for independent verification. Reconstruct the root from any leaf.' },
  { icon: Server, title: 'Casper Anchoring', desc: 'Proof hashes and Merkle roots are stored across 9 smart contracts, all live on BOT Chain testnet. Immutable, timestamped records.' },
  { icon: Shield, title: 'Real ZK Proofs (Groth16, off-chain)', desc: 'BN254 Groth16 via gnark — real R1CS circuits, trusted setup, pairing-based verification run in the engine (not simulation). On-chain Casper verifier for Groth16 is roadmap.' },
  { icon: Layers, title: 'Batch Aggregation', desc: 'Aggregate multiple proofs into a single batch with hash-chain verification and Postgres persistence.' },
  { icon: Cpu, title: 'Post-Quantum Ready', desc: 'ML-DSA-65 (FIPS 204, cloudflare/circl) + Lamport OTS + hybrid Ed25519→ML-DSA signing. Provides transitional PQ security under lattice (LWE/SIS) and hash-preimage assumptions; see docs/PQ_HONESTY.md.' },
  { icon: Key, title: 'Proof Chain DAG', desc: 'Validate chains of dependent proofs with cycle detection, input continuity checks, and single-root enforcement.' },
]

export default function Features() {
  const ref = useRef<HTMLElement>(null)
  const [vis, setVis] = useState(false)
  useEffect(() => {
    const obs = new IntersectionObserver(([e]) => { if (e.isIntersecting) setVis(true) }, { threshold: 0.1 })
    if (ref.current) obs.observe(ref.current)
    return () => obs.disconnect()
  }, [])

  return (
    <section ref={ref} id="features" className="py-24">
      <div className="cp-section">
        <div className="grid lg:grid-cols-3 gap-6">
          <div className={`lg:col-span-1 transition-all duration-700 ${vis ? 'opacity-100' : 'opacity-0 translate-y-8'}`}>
            <p className="text-xs font-mono text-red-500 tracking-widest mb-3">CAPABILITIES</p>
            <h2 className="text-3xl font-extrabold text-white mb-4 leading-tight">
              Why trust matters for AI decisions.
            </h2>
            <p className="text-gray-400 leading-relaxed mb-8">
              AI agents make decisions that move money, approve loans, and flag risks. Without proof, there's no accountability. Bot Prove provides cryptographic guarantees backed by on-chain immutability.
            </p>

            <div className="relative w-96 max-w-full mx-auto lg:mx-0">
              <div className="absolute inset-0 bg-red-500/10 rounded-full blur-[60px] pointer-events-none" />
              <img
                src="/images/mascot/pose2.webp"
                alt="Bot Prove"
                className="w-full animate-zoom-breathe"
                style={{ transform: 'scaleX(-1)' }}
              />
            </div>
          </div>

          <div className="lg:col-span-2 grid sm:grid-cols-2 gap-4">
            {features.map((f, i) => (
              <div key={i} className={`bg-cp-card border border-cp-border rounded-xl p-5 hover:border-red-500/30 transition-all duration-300 ${vis ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-6'}`} style={{ transitionDelay: `${i * 0.08}s` }}>
                <div className="w-10 h-10 rounded-lg bg-red-500/10 border border-red-500/20 flex items-center justify-center mb-3">
                  <f.icon className="w-5 h-5 text-red-400" />
                </div>
                <h3 className="text-white font-bold mb-1.5">{f.title}</h3>
                <p className="text-sm text-gray-500 leading-relaxed">{f.desc}</p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}
