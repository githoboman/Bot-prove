import { ArrowLeft, ExternalLink } from 'lucide-react'
import { Link } from 'react-router-dom'

const endpoints = [
  {
    category: 'System',
    items: [
      { method: 'GET', path: '/health', desc: 'Health check — uptime, version, deployed contracts, chain ID', auth: false },
      { method: 'GET', path: '/stats', desc: 'Proof statistics — totals, unique agents, use-case breakdown', auth: false },
    ],
  },
  {
    category: 'Proofs',
    items: [
      { method: 'POST', path: '/proofs', desc: 'Generate a cryptographic proof for an AI agent decision', auth: true },
      { method: 'POST', path: '/proofs/batch', desc: 'Generate up to 50 proofs in a single request', auth: true },
      { method: 'GET', path: '/proofs', desc: 'List all proofs with optional ?agent= filter', auth: false },
      { method: 'GET', path: '/proofs/{id}', desc: 'Get a single proof by ID', auth: false },
      { method: 'POST', path: '/verify', desc: 'Verify a proof by ID — checks hash integrity + Merkle path', auth: false },
      { method: 'DELETE', path: '/proofs/{id}/revoke', desc: 'Revoke a proof (requires X-Public-Key of original creator)', auth: true },
      { method: 'GET', path: '/proofs/{id}/export', desc: 'Export proof with full chain metadata for archival', auth: false },
    ],
  },
  {
    category: 'Inference',
    items: [
      { method: 'POST', path: '/inference/prove', desc: 'Record a model inference with input/output hashes', auth: true },
      { method: 'POST', path: '/inference/verify', desc: 'Verify an inference proof', auth: false },
      { method: 'POST', path: '/inference/register-model', desc: 'Register a model in the model registry', auth: true },
      { method: 'GET', path: '/inference/model/{id}', desc: 'Get model metadata by ID', auth: false },
    ],
  },
  {
    category: 'ZK Proofs',
    items: [
      { method: 'POST', path: '/zk/verify-groth16', desc: 'Conceptual Groth16 verification (hash-based simulation)', auth: false },
      { method: 'POST', path: '/zk/batch-verify', desc: 'Batch-verify multiple ZK proofs', auth: false },
      { method: 'POST', path: '/zk/groth16-real/prove', desc: 'Generate a real Groth16 proof (BN254 R1CS via gnark)', auth: true },
      { method: 'POST', path: '/zk/groth16-real/verify', desc: 'Verify a real Groth16 proof', auth: false },
      { method: 'POST', path: '/zk/challenge', desc: 'Create a dispute challenge for a proof', auth: true },
      { method: 'GET', path: '/zk/challenge/{id}', desc: 'Get dispute challenge status', auth: false },
    ],
  },
  {
    category: 'Aggregation',
    items: [
      { method: 'POST', path: '/aggregation/create-batch', desc: 'Create a new aggregation batch', auth: true },
      { method: 'POST', path: '/aggregation/add-proof', desc: 'Add a proof to an open batch', auth: true },
      { method: 'POST', path: '/aggregation/finalize', desc: 'Finalize batch — compute aggregated hash + persist', auth: true },
      { method: 'GET', path: '/aggregation/batch/{id}', desc: 'Get batch details including all proof IDs', auth: false },
      { method: 'POST', path: '/aggregation/verify-batch/{id}', desc: 'Verify aggregated batch integrity', auth: false },
    ],
  },
  {
    category: 'Post-Quantum Crypto',
    items: [
      { method: 'POST', path: '/pq/sign-sphincs', desc: 'Lamport OTS signing (hash-based, post-quantum)', auth: true },
      { method: 'POST', path: '/pq/verify-sphincs', desc: 'Verify a Lamport OTS signature', auth: false },
      { method: 'POST', path: '/pq/hybrid-sign', desc: 'Hybrid Ed25519 + ML-DSA-65 (FIPS 204) signing', auth: true },
      { method: 'POST', path: '/pq/hybrid-verify', desc: 'Verify hybrid classical + post-quantum signature', auth: false },
    ],
  },
  {
    category: 'KYC',
    items: [
      { method: 'GET', path: '/kyc/check', desc: 'Check KYC eligibility for a user', auth: false },
      { method: 'POST', path: '/kyc/grant', desc: 'Grant KYC clearance to a user', auth: true },
      { method: 'GET', path: '/kyc/whitelist/{user}', desc: 'Check whitelist status for a specific user', auth: false },
    ],
  },
  {
    category: 'Proof Chain',
    items: [
      { method: 'POST', path: '/proof-chain/validate', desc: 'Validate a DAG of dependent proofs — cycle detection, input continuity, single-root', auth: false },
    ],
  },
]

const methodColor: Record<string, string> = {
  GET: 'bg-green-500/15 text-green-400 border-green-500/20',
  POST: 'bg-blue-500/15 text-blue-400 border-blue-500/20',
  DELETE: 'bg-red-500/15 text-red-400 border-red-500/20',
}

export default function ApiDocs() {
  return (
    <div className="min-h-screen bg-[#0a0a14] text-white">
      <div className="max-w-4xl mx-auto px-6 py-16">
        <Link to="/" className="inline-flex items-center gap-1.5 text-sm text-gray-500 hover:text-gray-300 transition-colors mb-8">
          <ArrowLeft className="w-4 h-4" /> Back to home
        </Link>

        <div className="mb-12">
          <p className="text-xs font-mono text-red-500 tracking-widest mb-3">REFERENCE</p>
          <h1 className="text-4xl font-extrabold mb-4">API Documentation</h1>
          <p className="text-gray-400 max-w-2xl leading-relaxed">
            Bot Prove exposes 32 REST endpoints. Base URL:{' '}
            <code className="text-red-400 bg-red-500/10 px-2 py-0.5 rounded text-sm">https://casperprover-api-ylsh.onrender.com</code>
          </p>
          <div className="mt-4 flex gap-3">
            <a href="https://github.com/anna-stolbovskaja/Bot Prove/blob/main/docs/openapi.yaml" target="_blank" rel="noreferrer"
               className="inline-flex items-center gap-1.5 text-sm text-gray-400 hover:text-white transition-colors">
              <ExternalLink className="w-3.5 h-3.5" /> OpenAPI spec
            </a>
          </div>
        </div>

        <div className="mb-8 bg-cp-card rounded-xl border border-cp-border p-5">
          <h3 className="text-sm font-semibold text-gray-300 mb-2">Authentication</h3>
          <p className="text-sm text-gray-500 leading-relaxed">
            Mutating endpoints (POST/DELETE) require an <code className="text-gray-300">X-API-Key</code> header.
            Read-only endpoints (GET) are public. Rate limit: 60 requests/minute per IP.
          </p>
        </div>

        {endpoints.map((cat) => (
          <div key={cat.category} className="mb-10">
            <h2 className="text-lg font-bold text-white mb-4 flex items-center gap-2">
              <span className="w-1.5 h-1.5 rounded-full bg-red-500" />
              {cat.category}
              <span className="text-xs text-gray-600 font-normal">({cat.items.length})</span>
            </h2>
            <div className="space-y-2">
              {cat.items.map((ep) => (
                <div key={ep.method + ep.path} className="bg-cp-card border border-cp-border rounded-lg px-4 py-3 flex items-start gap-3 hover:border-gray-700 transition-colors">
                  <span className={`inline-block px-2 py-0.5 rounded text-[11px] font-mono font-bold border shrink-0 mt-0.5 ${methodColor[ep.method] || ''}`}>
                    {ep.method}
                  </span>
                  <div className="min-w-0">
                    <code className="text-sm text-gray-200 font-mono">{ep.path}</code>
                    <p className="text-xs text-gray-500 mt-0.5">{ep.desc}</p>
                  </div>
                  {ep.auth && (
                    <span className="ml-auto shrink-0 text-[10px] text-yellow-500/70 font-mono border border-yellow-500/20 rounded px-1.5 py-0.5">AUTH</span>
                  )}
                </div>
              ))}
            </div>
          </div>
        ))}

        <div className="mt-12 border-t border-cp-border pt-8 text-center">
          <p className="text-sm text-gray-600">
            Full OpenAPI 3.0 spec available at{' '}
            <a href="https://github.com/anna-stolbovskaja/Bot Prove/blob/main/docs/openapi.yaml" target="_blank" rel="noreferrer" className="text-red-400 hover:text-red-300">
              docs/openapi.yaml
            </a>
          </p>
        </div>
      </div>
    </div>
  )
}
