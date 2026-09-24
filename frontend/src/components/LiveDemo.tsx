import { useState, useRef } from 'react'
import { Play, CheckCircle, XCircle, Loader2 } from 'lucide-react'
import { createProof, getProofs } from '../lib/api'

export default function LiveDemo() {
  const [agent, setAgent] = useState('agent-alpha')
  const [input, setInput] = useState('loan_approval_decision')
  const [model, setModel] = useState('gpt-4o')
  const [output, setOutput] = useState('approved_with_conditions')
  const [result, setResult] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const logRef = useRef<HTMLDivElement>(null)

  const handleGenerate = async () => {
    setLoading(true)
    setError(null)
    setResult(null)
    try {
      const proof = await createProof({ agent, input, output, model, use_case: 'merkle-inclusion', mode: 'local' })
      setResult(proof)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
    setLoading(false)
  }

  return (
    <section id="demo" className="py-24">
      <div className="cp-section">
        <div className="text-center mb-14">
          <p className="text-xs font-mono text-red-500 tracking-widest mb-3">LIVE DEMO</p>
          <h2 className="text-3xl sm:text-4xl font-extrabold text-white mb-4">
            Generate a proof. Right now.
          </h2>
          <p className="text-gray-500 max-w-lg mx-auto">
            Enter AI decision parameters below. Bot Prove will generate a cryptographic proof and return the verification data.
          </p>
        </div>

        <div className="max-w-4xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-6">
            {/* Input form */}
            <div className="bg-cp-card rounded-2xl border border-cp-border p-6">
              <h3 className="text-white font-bold mb-4 flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-red-500" /> Input Parameters
              </h3>
              <div className="space-y-4">
                {[
                  { label: 'Agent ID', val: agent, set: setAgent, ph: 'agent-alpha' },
                  { label: 'Input', val: input, set: setInput, ph: 'loan_approval_decision' },
                  { label: 'Model', val: model, set: setModel, ph: 'gpt-4o' },
                  { label: 'Output', val: output, set: setOutput, ph: 'approved_with_conditions' },
                ].map((f, i) => (
                  <div key={i}>
                    <label className="text-xs text-gray-500 font-mono mb-1 block">{f.label}</label>
                    <input
                      value={f.val}
                      onChange={e => f.set(e.target.value)}
                      placeholder={f.ph}
                      className="w-full px-4 py-2.5 bg-black/40 border border-gray-800 rounded-lg text-sm text-white font-mono placeholder:text-gray-600 focus:outline-none focus:border-red-500/50"
                    />
                  </div>
                ))}
                <button
                  onClick={handleGenerate}
                  disabled={loading}
                  className="w-full mt-2 inline-flex items-center justify-center gap-2 px-6 py-3 bg-red-600 text-white font-semibold rounded-xl hover:bg-red-500 disabled:opacity-50 transition-all"
                >
                  {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                  {loading ? 'Generating...' : 'Generate Proof'}
                </button>
              </div>
            </div>

            {/* Output */}
            <div className="bg-black/60 rounded-2xl border border-gray-800 overflow-hidden">
              <div className="flex items-center gap-2 px-4 py-3 border-b border-gray-800 bg-black/40">
                <div className="w-2.5 h-2.5 rounded-full bg-red-500" />
                <div className="w-2.5 h-2.5 rounded-full bg-yellow-500" />
                <div className="w-2.5 h-2.5 rounded-full bg-green-500" />
                <span className="text-gray-600 text-xs ml-2 font-mono">proof-output</span>
              </div>
              <div ref={logRef} className="p-5 font-mono text-xs min-h-[280px] max-h-96 overflow-y-auto">
                {!result && !error && !loading && (
                  <p className="text-gray-600">Waiting for proof generation...</p>
                )}
                {loading && (
                  <div className="space-y-2">
                    <p className="text-gray-400">Hashing inputs...</p>
                    <p className="text-gray-400">Computing Merkle tree...</p>
                    <p className="text-red-400 animate-pulse">Generating proof...</p>
                  </div>
                )}
                {error && (
                  <div className="flex items-start gap-2 text-red-400">
                    <XCircle className="w-4 h-4 mt-0.5 shrink-0" />
                    <span>{error}</span>
                  </div>
                )}
                {result && (
                  <div className="space-y-1">
                    <p className="text-green-400 flex items-center gap-1"><CheckCircle className="w-3 h-3" /> Proof generated</p>
                    <p className="text-gray-500">---</p>
                    <p className="text-gray-400">id: <span className="text-white">{result.id}</span></p>
                    <p className="text-gray-400">agentId: <span className="text-white">{result.agent}</span></p>
                    <p className="text-gray-400">proof_hash: <span className="text-red-400">{result.proof_hash}</span></p>
                    <p className="text-gray-400">input_hash: <span className="text-orange-300">{result.input_hash}</span></p>
                    <p className="text-gray-400">output_hash: <span className="text-orange-300">{result.output_hash}</span></p>
                    <p className="text-gray-400">merkle_root: <span className="text-yellow-300">{result.merkle_root}</span></p>
                    <p className="text-gray-400">leaf_index: <span className="text-white">{result.leaf_index}</span></p>
                    <p className="text-gray-400">valid: <span className={result.valid ? 'text-green-400' : 'text-red-400'}>{String(result.valid)}</span></p>
                    <p className="text-gray-500">---</p>
                    <p className="text-gray-400">merkle_path: [{result.merkle_path?.length || 0} nodes]</p>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
