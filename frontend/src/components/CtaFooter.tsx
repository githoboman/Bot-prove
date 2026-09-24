import { useEffect, useMemo, useState } from 'react'
import { getCachedManifest, loadManifest } from '../lib/onchain'

// Fallback deploy-hash prefix for the typewriter demo. Real value comes from
// the canonical /onchain.json (Gate 1.5). This is a visual demo, not a clickable
// on-chain link, so a stale fallback only ever changes the terminal aesthetic.
const FALLBACK_DEPLOY_HASH_PREFIX = '96e97c4d564fe7374ba4e938355fb89f5b'

function buildLines(deployHashPrefix: string) {
  return [
    { delay: 0, prefix: '$ ', text: 'bot-prove prove --agent loan-bot --model gpt-4o', color: 'text-green-400' },
    { delay: 1200, prefix: '', text: 'hashing inputs...        SHA-256  OK', color: 'text-gray-500' },
    { delay: 1800, prefix: '', text: 'building merkle tree...   depth=2 OK', color: 'text-gray-500' },
    { delay: 2400, prefix: '', text: 'generating proof...       P-42    OK', color: 'text-gray-500' },
    { delay: 3000, prefix: '', text: 'anchoring on-chain...     deploy  OK', color: 'text-gray-500' },
    { delay: 3600, prefix: '', text: '', color: '' },
    { delay: 3700, prefix: '  ', text: 'proof_hash   3d807fab6719562d774058d32cb4fb2319...', color: 'text-red-400' },
    { delay: 4000, prefix: '  ', text: 'merkle_root  85fcc3dd7145066239ddc90f67b92bc041...', color: 'text-orange-300' },
    { delay: 4300, prefix: '  ', text: `deploy_hash  ${deployHashPrefix}...`, color: 'text-yellow-300' },
    { delay: 4600, prefix: '  ', text: 'status       VERIFIED', color: 'text-green-400' },
    { delay: 5200, prefix: '', text: '', color: '' },
    { delay: 5400, prefix: '$ ', text: 'echo "Your AI is now accountable."', color: 'text-green-400' },
  ]
}

export default function CtaFooter() {
  const [visible, setVisible] = useState(0)
  const [deployPrefix, setDeployPrefix] = useState<string>(() => {
    const h = getCachedManifest()?.contracts.proof_registry?.contract_hash
    return h ? h.slice(0, 34) : FALLBACK_DEPLOY_HASH_PREFIX
  })

  useEffect(() => {
    let alive = true
    loadManifest()
      .then((m) => {
        if (!alive) return
        const h = m.contracts.proof_registry?.contract_hash
        if (h) setDeployPrefix(h.slice(0, 34))
      })
      .catch(() => { /* keep fallback */ })
    return () => { alive = false }
  }, [])

  const lines = useMemo(() => buildLines(deployPrefix), [deployPrefix])

  useEffect(() => {
    const timers = lines.map((l, i) =>
      setTimeout(() => setVisible(i + 1), l.delay)
    )
    return () => timers.forEach(clearTimeout)
  }, [lines])

  return (
    <section className="py-16 relative">
      {/* Subtle grid */}
      <div className="absolute inset-0 opacity-[0.02]" style={{
        backgroundImage: 'linear-gradient(#E53935 1px, transparent 1px), linear-gradient(90deg, #E53935 1px, transparent 1px)',
        backgroundSize: '40px 40px',
      }} />

      <div className="cp-section relative z-10">
        <div className="grid lg:grid-cols-2 gap-10 items-center">
          {/* Terminal output */}
          <div className="bg-black/80 backdrop-blur rounded-2xl border border-gray-800 p-6 font-mono text-xs sm:text-sm">
            <div className="flex items-center gap-2 mb-4">
              <div className="w-2.5 h-2.5 rounded-full bg-red-500" />
              <div className="w-2.5 h-2.5 rounded-full bg-yellow-500" />
              <div className="w-2.5 h-2.5 rounded-full bg-green-500" />
              <span className="text-gray-600 text-[10px] ml-2">proof-generation</span>
            </div>
            <div className="space-y-0.5 min-h-[220px]">
              {lines.slice(0, visible).map((l, i) => (
                <div key={i} className={`${l.color} transition-opacity duration-300`}>
                  {l.prefix && <span className="text-gray-500">{l.prefix}</span>}
                  {l.text}
                </div>
              ))}
              {visible < lines.length && (
                <span className="text-red-400 animate-pulse">_</span>
              )}
            </div>
          </div>

          {/* CTA content */}
          <div className="flex flex-col items-start">
            <div className="flex items-center gap-4 mb-6">
              <img src="/images/mascot/pose3.webp" alt="" className="w-20 animate-fire-glow" />
              <div>
                <p className="text-red-500 font-mono text-xs tracking-widest mb-1">READY TO PROVE?</p>
                <h2 className="text-3xl sm:text-4xl font-extrabold text-white leading-tight">
                  Make your AI<br />accountable.
                </h2>
              </div>
            </div>

            <p className="text-gray-400 mb-8 max-w-md leading-relaxed">
              Generate your first cryptographic proof in under a minute. No GPU, no complex setup. Just SHA-256, Merkle trees, and on-chain anchoring.
            </p>

            <div className="flex flex-wrap gap-3">
              <a href="/lab" className="group inline-flex items-center gap-2 px-7 py-3.5 bg-red-600 text-white font-semibold rounded-xl hover:bg-red-500 transition-all shadow-lg shadow-red-600/20">
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg>
                Enter Lab
              </a>
              <a href="https://github.com/anna-stolbovskaja/Bot Prove" target="_blank" rel="noreferrer" className="inline-flex items-center gap-2 px-7 py-3.5 border border-gray-700 text-gray-300 rounded-xl hover:border-red-500/40 hover:text-white transition-colors font-mono text-sm">
                git clone
              </a>
            </div>

            {/* Contract badges */}
            <div className="flex flex-wrap gap-2 mt-6">
              {['proof-registry', 'verifier-gate', 'defi-mock', 'stake-slashing'].map(c => (
                <span key={c} className="px-2 py-1 rounded bg-cp-card border border-cp-border text-[10px] font-mono text-gray-500">
                  {c}
                </span>
              ))}
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
