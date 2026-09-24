export default function NotFound() {
  return (
    <div className="min-h-screen bg-cp-black flex items-center justify-center px-4 relative overflow-hidden">
      {/* Grid bg */}
      <div className="absolute inset-0 opacity-[0.02]" style={{
        backgroundImage: 'linear-gradient(#3b82f6 1px, transparent 1px), linear-gradient(90deg, #3b82f6 1px, transparent 1px)',
        backgroundSize: '80px 80px',
      }} />

      <div className="relative z-10 flex flex-col md:flex-row items-center gap-6 md:gap-12 max-w-4xl w-full">
        {/* Left mascot — thinking */}
        <div className="relative shrink-0 w-40 md:w-56">
          <div className="absolute inset-0 bg-blue-500/15 rounded-full blur-[50px] pointer-events-none" />
          <img
            src="/images/mascot/maskot_phink.png"
            alt="Thinking"
            className="w-full relative z-10 animate-float-left"
          />
        </div>

        {/* Center content */}
        <div className="text-center flex-1">
          <p className="text-red-500 font-mono text-xs tracking-[0.3em] mb-4">PROOF_NOT_FOUND</p>
          <h1
            className="text-7xl md:text-8xl font-black text-white mb-4"
            style={{ textShadow: '0 0 60px rgba(59,130,246,0.5), 0 0 120px rgba(59,130,246,0.2)' }}
          >
            404
          </h1>
          <p className="text-gray-500 mb-2 font-mono text-sm">
            This path has no valid Merkle proof.
          </p>
          <p className="text-gray-600 mb-8 text-sm">
            The prover checked all leaves — nothing here.
          </p>
          <a
            href="/"
            className="inline-flex items-center gap-2 px-6 py-3 bg-red-600 text-white font-semibold rounded-xl hover:bg-red-500 transition-colors"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 17l-5-5m0 0l5-5m-5 5h12" />
            </svg>
            Return to root
          </a>
        </div>

        {/* Right mascot — reading */}
        <div className="relative shrink-0 w-40 md:w-56">
          <div className="absolute inset-0 bg-blue-500/15 rounded-full blur-[50px] pointer-events-none" />
          <img
            src="/images/mascot/maskot_read.png"
            alt="Reading"
            className="w-full relative z-10 animate-float-right"
          />
        </div>
      </div>
    </div>
  )
}
