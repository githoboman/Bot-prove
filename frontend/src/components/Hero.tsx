import { useEffect, useState, useRef } from 'react'

const floatingBlocks = [
  { text: 'proof_hash: 3d807f...', x: 8, y: 18, z: 1.15, angle: -6, speed: 3.2, phase: 0 },
  { text: 'merkle_root: 85fcc3...', x: 72, y: 12, z: 0.75, angle: 4, speed: 4.1, phase: 1.2 },
  { text: 'VERIFIED', x: 80, y: 55, z: 1.3, angle: -3, speed: 2.8, phase: 2.5 },
  { text: 'SHA-256', x: 5, y: 62, z: 0.6, angle: 8, speed: 3.6, phase: 0.8 },
  { text: 'leaf_index: 0', x: 68, y: 78, z: 0.85, angle: -5, speed: 5.0, phase: 3.1 },
  { text: 'botchain-mainnet', x: 15, y: 80, z: 1.05, angle: 3, speed: 3.9, phase: 1.7 },
  { text: 'input_hash: 7165...', x: 75, y: 35, z: 0.5, angle: -7, speed: 4.5, phase: 4.0 },
  { text: 'valid: true', x: 12, y: 42, z: 1.25, angle: 5, speed: 2.5, phase: 2.0 },
]

export default function Hero() {
  const [typed, setTyped] = useState('')
  const fullText = 'botchain-prover verify --model gpt-4o --input "loan_42" --anchor botchain-mainnet'
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const mascotRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let i = 0
    const interval = setInterval(() => {
      if (i <= fullText.length) { setTyped(fullText.slice(0, i)); i++ }
      else clearInterval(interval)
    }, 40)
    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const particles: { x: number; y: number; vx: number; vy: number; life: number; size: number }[] = []

    const resize = () => {
      canvas.width = canvas.offsetWidth * 2
      canvas.height = canvas.offsetHeight * 2
      ctx.scale(2, 2)
      particles.length = 0
    }
    resize()
    window.addEventListener('resize', resize)

    let raf: number
    const animate = () => {
      const W = canvas.offsetWidth
      const H = canvas.offsetHeight
      ctx.clearRect(0, 0, W, H)

      const isDesktop = W > 768
      const cx = isDesktop ? W * 0.75 : W * 0.5
      const cy = isDesktop ? H * 0.5 : H * 0.75

      if (Math.random() < 0.4) {
        particles.push({
          x: cx + (Math.random() - 0.5) * 100,
          y: cy + Math.random() * 30,
          vx: (Math.random() - 0.5) * 1,
          vy: -(2 + Math.random() * 4),
          life: 1,
          size: 1.5 + Math.random() * 3.5,
        })
      }

      for (let i = particles.length - 1; i >= 0; i--) {
        const p = particles[i]
        p.x += p.vx
        p.y += p.vy
        p.life -= 0.008
        if (p.life <= 0) { particles.splice(i, 1); continue }
        ctx.beginPath()
        ctx.arc(p.x, p.y, p.size * p.life, 0, Math.PI * 2)
        const r = 229 + Math.random() * 26
        const g = 57 * p.life
        ctx.fillStyle = `rgba(${r},${g},20,${p.life * 0.6})`
        ctx.fill()
      }

      raf = requestAnimationFrame(animate)
    }
    animate()
    return () => { cancelAnimationFrame(raf); window.removeEventListener('resize', resize) }
  }, [])

  return (
    <section id="home" className="relative min-h-screen flex items-center overflow-hidden">
      <div className="absolute inset-0 opacity-[0.03]" style={{
        backgroundImage: 'linear-gradient(#E53935 1px, transparent 1px), linear-gradient(90deg, #E53935 1px, transparent 1px)',
        backgroundSize: '60px 60px',
      }} />

      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="w-full h-px bg-gradient-to-r from-transparent via-red-500/30 to-transparent animate-scan" />
      </div>

      {/* Fire particles — behind mascot */}
      <canvas ref={canvasRef} className="absolute inset-0 w-full h-full pointer-events-none z-[3]" />

      {/* Floating 3D info blocks */}
      {floatingBlocks.map((b, i) => {
        const isBehind = b.z < 1
        return (
          <div
            key={i}
            className="absolute pointer-events-none font-mono hidden md:block"
            style={{
              left: `${b.x}%`,
              top: `${b.y}%`,
              zIndex: isBehind ? 3 : 15,
              transform: `scale(${b.z}) rotateX(${b.angle}deg) rotateY(${b.angle * 0.5}deg)`,
              opacity: 0.15 + b.z * 0.35,
              animation: `floatBlock ${b.speed}s ease-in-out infinite`,
              animationDelay: `${b.phase}s`,
            }}
          >
            <div className={`px-2.5 py-1.5 rounded-md border text-[10px] backdrop-blur-sm whitespace-nowrap ${
              b.text === 'VERIFIED'
                ? 'bg-green-500/10 border-green-500/20 text-green-400'
                : b.text === 'SHA-256'
                ? 'bg-orange-500/10 border-orange-500/20 text-orange-400'
                : 'bg-red-500/5 border-red-500/15 text-red-400/70'
            }`}>
              {b.text}
            </div>
          </div>
        )
      })}

      {/* Mascot — mobile: centered bottom, desktop: right + vertically centered */}
      <div ref={mascotRef} className="absolute z-[5] left-1/2 -translate-x-1/2 top-16 w-[45%] sm:w-[40%] opacity-30 sm:opacity-40 lg:opacity-100 lg:left-auto lg:translate-x-0 lg:right-[5%] lg:top-1/2 lg:-translate-y-1/2 lg:w-[38%] max-w-2xl">
        <img src="/images/mascot/hero.webp" alt="BOT Chain Spirit" className="w-full animate-fire-glow" loading="eager" />
      </div>

      <div className="cp-section relative z-20 py-32">
        <div className="max-w-2xl">
          <h1 className="text-4xl sm:text-5xl lg:text-6xl font-extrabold text-white leading-[1.08] mb-6 tracking-tight">
            Your AI made<br />a decision.<br />
            <span className="bg-clip-text text-transparent bg-gradient-to-r from-red-500 to-orange-400">Prove it.</span>
          </h1>

          <p className="text-lg text-gray-400 mb-10 leading-relaxed max-w-lg">
            BOT Chain commits cryptographic Merkle hashes of AI agent decisions (input, output, model) to BOT Chain Mainnet. Publicly verifiable, tamper-evident, on-chain.
          </p>

          <div className="bg-black/60 backdrop-blur rounded-xl border border-gray-800 p-5 mb-10 max-w-xl">
            <div className="flex items-center gap-2 mb-3">
              <div className="w-2.5 h-2.5 rounded-full bg-red-500" />
              <div className="w-2.5 h-2.5 rounded-full bg-yellow-500" />
              <div className="w-2.5 h-2.5 rounded-full bg-green-500" />
              <span className="text-gray-600 text-xs ml-2 font-mono">terminal</span>
            </div>
            <div className="font-mono text-sm">
              <span className="text-gray-500">$ </span>
              <span className="text-green-400">{typed}</span>
              <span className="animate-pulse text-red-400">_</span>
            </div>
          </div>

          {/*
            Three audience-scoped entry points. Every href resolves to an
            existing, working route in this repo:
              /lab/playground -> guided product demo (Playground.tsx)
              /docs/api       -> developer docs hub (API + SDK + MCP nav)
              /lab/contracts  -> deployed contracts + architecture proof
          */}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 max-w-2xl" role="group" aria-label="Choose your entry point">
            <a
              href="/lab/playground"
              className="group flex flex-col items-start gap-1 p-4 rounded-xl bg-red-600 text-white font-semibold hover:bg-red-500 transition-all shadow-lg shadow-red-600/20"
            >
              <span className="text-xs uppercase tracking-wider text-red-100/80">For users</span>
              <span className="text-base flex items-center gap-1.5">
                Try the product
                <svg className="w-4 h-4 opacity-90" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
              </span>
              <span className="text-xs text-red-50/80 font-normal">Guided demo in the Playground</span>
            </a>
            <a
              href="/docs/api"
              className="group flex flex-col items-start gap-1 p-4 rounded-xl border border-gray-700 text-gray-200 font-semibold hover:border-red-500/60 hover:text-white transition-colors"
            >
              <span className="text-xs uppercase tracking-wider text-gray-500">For developers</span>
              <span className="text-base flex items-center gap-1.5">
                API, SDK &amp; MCP
                <svg className="w-4 h-4 opacity-70" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
              </span>
              <span className="text-xs text-gray-400 font-normal">Docs, endpoints and client SDK</span>
            </a>
            <a
              href="/lab/contracts"
              className="group flex flex-col items-start gap-1 p-4 rounded-xl border border-gray-700 text-gray-200 font-semibold hover:border-red-500/60 hover:text-white transition-colors"
            >
              <span className="text-xs uppercase tracking-wider text-gray-500">For evaluators</span>
              <span className="text-base flex items-center gap-1.5">
                Proof &amp; architecture
                <svg className="w-4 h-4 opacity-70" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
              </span>
              <span className="text-xs text-gray-400 font-normal">Deployed contracts, roadmap, source</span>
            </a>
          </div>

          <div className="flex flex-wrap gap-4 mt-5 text-sm">
            <a href="/lab" className="text-gray-400 hover:text-gray-200 underline underline-offset-4 decoration-gray-700 hover:decoration-gray-500 transition-colors">
              Open the full Lab
            </a>
            <span className="text-gray-700">·</span>
            <a href="https://github.com/anna-stolbovskaja/Bot Prove" target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 text-gray-400 hover:text-gray-200 underline underline-offset-4 decoration-gray-700 hover:decoration-gray-500 transition-colors">
              <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" /></svg>
              Source on GitHub
            </a>
          </div>
        </div>
      </div>
    </section>
  )
}
