import { useEffect, useRef, useState } from 'react'

const steps = [
  {
    num: '01',
    title: 'Capture AI Decision',
    desc: 'Agent submits the input data, model identifier, and output for proof generation.',
    code: `prover.capture({
  agent: "agent-alpha",
  model: "gpt-4o",
  input: "loan_approval_decision",
  output: "approved_with_conditions"
})`,
  },
  {
    num: '02',
    title: 'Hash & Compute Proof',
    desc: 'Inputs and outputs are independently hashed. A Merkle tree is constructed from all proof components.',
    code: `// SHA-256 hashing
input_hash  = hash(input)   // 0x8a7f...
output_hash = hash(output)  // 0xc3b2...
model_hash  = hash(model)   // 0x5e1d...

// Merkle tree construction
merkle_root = merkle([
  input_hash, output_hash,
  model_hash, timestamp
])`,
  },
  {
    num: '03',
    title: 'Verify Integrity',
    desc: 'The proof is verified against the Merkle root. Any tampering with inputs or outputs invalidates the proof.',
    code: `verify({
  proof_hash: "0x7f3a...",
  merkle_root: "0x2c91...",
  merkle_path: [3 nodes],
  leaf_index: 2,
  valid: true  // ✓ proof matches
})`,
  },
  {
    num: '04',
    title: 'Anchor On-Chain',
    desc: 'The proof hash and Merkle root are stored on BOT Chain. Immutable, timestamped, publicly verifiable.',
    code: `bot.deploy("store_proof", {
  proof_hash: "0x7f3a...",
  merkle_root: "0x2c91...",
  agent: "agent-alpha",
  // On-chain forever
})`,
  },
]

export default function HowItWorks() {
  const ref = useRef<HTMLElement>(null)
  const [vis, setVis] = useState(false)

  useEffect(() => {
    const obs = new IntersectionObserver(([e]) => { if (e.isIntersecting) setVis(true) }, { threshold: 0.05 })
    if (ref.current) obs.observe(ref.current)
    return () => obs.disconnect()
  }, [])

  return (
    <section ref={ref} id="how" className="py-24 bg-cp-card/30">
      <div className="cp-section">
        <div className="text-center mb-16">
          <p className="text-xs font-mono text-red-500 tracking-widest mb-3">PIPELINE</p>
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white mb-4">
            Four steps to provable AI.
          </h2>
          <p className="text-gray-500 max-w-lg mx-auto">
            From raw decision to on-chain proof. Each step is cryptographically linked.
          </p>
        </div>

        {/* Vertical timeline */}
        <div className="max-w-4xl mx-auto relative">
          {/* Timeline line */}
          <div className="absolute left-6 lg:left-1/2 top-0 bottom-0 w-px bg-gradient-to-b from-red-500/50 via-red-500/20 to-transparent" />

          <div className="space-y-12">
            {steps.map((s, i) => (
              <div key={i} className={`relative flex flex-col lg:flex-row gap-6 lg:gap-12 ${i % 2 === 0 ? '' : 'lg:flex-row-reverse'} transition-all duration-700 ${vis ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'}`} style={{ transitionDelay: `${i * 0.15}s` }}>
                {/* Dot on timeline */}
                <div className="absolute left-6 lg:left-1/2 -translate-x-1/2 w-3 h-3 rounded-full bg-red-500 border-2 border-cp-black z-10 mt-6" />

                {/* Text */}
                <div className="lg:w-1/2 pl-14 lg:pl-0 lg:pr-8">
                  <span className="text-red-500/60 font-mono text-sm">{s.num}</span>
                  <h3 className="text-xl font-bold text-white mt-1 mb-2">{s.title}</h3>
                  <p className="text-gray-400 leading-relaxed">{s.desc}</p>
                </div>

                {/* Code block */}
                <div className="lg:w-1/2 pl-14 lg:pl-0">
                  <div className="bg-black/60 rounded-xl border border-gray-800 overflow-hidden">
                    <div className="flex items-center gap-1.5 px-3 py-2 border-b border-gray-800">
                      <div className="w-2 h-2 rounded-full bg-red-500" />
                      <div className="w-2 h-2 rounded-full bg-yellow-500" />
                      <div className="w-2 h-2 rounded-full bg-green-500" />
                    </div>
                    <pre className="p-4 text-xs font-mono text-gray-300 overflow-x-auto leading-relaxed"><code>{s.code}</code></pre>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}
