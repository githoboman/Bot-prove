import { useEffect, useRef, useState } from 'react'
import { Building2, Heart, Scale, Bot, Banknote, Globe } from 'lucide-react'

const cases = [
  {
    icon: Banknote,
    title: 'DeFi & Lending',
    desc: 'AI-driven loan approvals generate cryptographic proofs that auditors and regulators can verify without accessing private financial data.',
    tag: 'Live: KYC-gated vault on testnet',
  },
  {
    icon: Heart,
    title: 'Healthcare AI',
    desc: 'Prove that a diagnostic model produced a specific recommendation for a specific patient — without exposing medical records.',
    tag: 'Use case: diagnostic accountability',
  },
  {
    icon: Scale,
    title: 'Legal & Compliance',
    desc: 'Timestamped, on-chain proofs create an immutable audit trail for AI decisions subject to regulatory review.',
    tag: 'Use case: GDPR right to explanation',
  },
  {
    icon: Bot,
    title: 'Autonomous Agents',
    desc: 'Multi-step agent workflows produce a chain of proofs. DAG validation ensures every step is linked and tamper-evident.',
    tag: 'Live: proof-chain DAG validation',
  },
  {
    icon: Building2,
    title: 'Enterprise AI Governance',
    desc: 'Model fingerprinting + proof anchoring gives enterprises a verifiable record of which model version made which decision.',
    tag: 'Use case: model lifecycle tracking',
  },
  {
    icon: Globe,
    title: 'Cross-Chain Verification',
    desc: 'Proofs anchored on Casper can be verified by any chain or off-chain system. SDK and MCP make integration trivial.',
    tag: 'Live: 32-tool MCP server',
  },
]

export default function UseCases() {
  const ref = useRef<HTMLElement>(null)
  const [vis, setVis] = useState(false)

  useEffect(() => {
    const obs = new IntersectionObserver(([e]) => { if (e.isIntersecting) setVis(true) }, { threshold: 0.05 })
    if (ref.current) obs.observe(ref.current)
    return () => obs.disconnect()
  }, [])

  return (
    <section ref={ref} id="use-cases" className="py-24">
      <div className="cp-section">
        <div className="text-center mb-14">
          <p className="text-xs font-mono text-red-500 tracking-widest mb-3">USE CASES</p>
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white mb-4">
            Where provable AI matters.
          </h2>
          <p className="text-gray-500 max-w-lg mx-auto">
            Any industry where AI decisions have consequences needs cryptographic accountability.
          </p>
        </div>

        <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-5 max-w-6xl mx-auto">
          {cases.map((c, i) => (
            <div
              key={i}
              className={`bg-cp-card border border-cp-border rounded-xl p-6 hover:border-red-500/30 transition-all duration-500 ${vis ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`}
              style={{ transitionDelay: `${i * 0.1}s` }}
            >
              <div className="w-10 h-10 rounded-lg bg-red-500/10 border border-red-500/20 flex items-center justify-center mb-4">
                <c.icon className="w-5 h-5 text-red-400" />
              </div>
              <h3 className="text-white font-bold text-lg mb-2">{c.title}</h3>
              <p className="text-sm text-gray-400 leading-relaxed mb-3">{c.desc}</p>
              <span className="inline-block px-2.5 py-1 rounded-md bg-red-500/10 text-red-400 text-[11px] font-mono">
                {c.tag}
              </span>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
