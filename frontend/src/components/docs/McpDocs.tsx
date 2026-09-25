import { ArrowLeft, ExternalLink } from 'lucide-react'
import { Link } from 'react-router-dom'

const tools = [
  { category: 'Proofs (7)', items: [
    { name: 'generate_proof', desc: 'Create a cryptographic proof of an AI agent decision' },
    { name: 'verify_proof', desc: 'Verify proof integrity — hash check + Merkle path' },
    { name: 'get_proof', desc: 'Fetch a proof by ID' },
    { name: 'list_proofs', desc: 'List all proofs, optionally filter by agent' },
    { name: 'revoke_proof', desc: 'Revoke (invalidate) a proof' },
    { name: 'export_proof', desc: 'Export proof with full chain metadata' },
    { name: 'batch_proofs', desc: 'Generate up to 50 proofs in one call' },
  ]},
  { category: 'Inference (4)', items: [
    { name: 'inference_prove', desc: 'Record a model inference with input/output hashes' },
    { name: 'inference_verify', desc: 'Verify an inference proof' },
    { name: 'register_model', desc: 'Register an AI model in the registry' },
    { name: 'get_model_info', desc: 'Get model metadata by ID' },
  ]},
  { category: 'ZK Proofs (6)', items: [
    { name: 'verify_groth16', desc: 'Conceptual Groth16 verification' },
    { name: 'groth16_real_prove', desc: 'Real Groth16 proof — BN254 R1CS via gnark' },
    { name: 'groth16_real_verify', desc: 'Verify a real Groth16 proof' },
    { name: 'zk_batch_verify', desc: 'Batch-verify multiple ZK proofs' },
    { name: 'zk_challenge', desc: 'Create a dispute challenge' },
    { name: 'zk_get_challenge', desc: 'Get dispute challenge status' },
  ]},
  { category: 'Aggregation (5)', items: [
    { name: 'create_batch', desc: 'Create a new aggregation batch' },
    { name: 'add_proof_to_batch', desc: 'Add a proof to an open batch' },
    { name: 'finalize_batch', desc: 'Finalize — compute aggregated hash + persist' },
    { name: 'get_batch', desc: 'Get batch details with proof IDs' },
    { name: 'verify_batch', desc: 'Verify aggregated batch integrity' },
  ]},
  { category: 'Post-Quantum (4)', items: [
    { name: 'pq_sign_sphincs', desc: 'Lamport OTS signing (hash-based PQ)' },
    { name: 'pq_verify_sphincs', desc: 'Verify a Lamport OTS signature' },
    { name: 'pq_hybrid_sign', desc: 'Hybrid Ed25519 + ML-DSA-65 (FIPS 204)' },
    { name: 'pq_hybrid_verify', desc: 'Verify hybrid classical + post-quantum signature' },
  ]},
  { category: 'KYC (3)', items: [
    { name: 'kyc_check', desc: 'Check KYC eligibility' },
    { name: 'kyc_grant', desc: 'Grant KYC clearance' },
    { name: 'kyc_whitelist', desc: 'Check whitelist status for a user' },
  ]},
  { category: 'Proof Chain (1)', items: [
    { name: 'validate_proof_chain', desc: 'DAG validation — cycles, input continuity, single-root' },
  ]},
  { category: 'System (2)', items: [
    { name: 'health_check', desc: 'API health, uptime, deployed contracts' },
    { name: 'get_stats', desc: 'Proof statistics and use-case breakdown' },
  ]},
]

export default function McpDocs() {
  return (
    <div className="min-h-screen bg-[#0a0a14] text-white">
      <div className="max-w-4xl mx-auto px-6 py-16">
        <Link to="/" className="inline-flex items-center gap-1.5 text-sm text-gray-500 hover:text-gray-300 transition-colors mb-8">
          <ArrowLeft className="w-4 h-4" /> Back to home
        </Link>

        <div className="mb-12">
          <p className="text-xs font-mono text-red-500 tracking-widest mb-3">INTEGRATION</p>
          <h1 className="text-4xl font-extrabold mb-4">MCP Server</h1>
          <p className="text-gray-400 max-w-2xl leading-relaxed">
            Connect any AI agent to Bot Prove via{' '}
            <a href="https://modelcontextprotocol.io" target="_blank" rel="noreferrer" className="text-red-400 hover:text-red-300">
              Model Context Protocol
            </a>. 32 tools, stdio transport. All backed by real API endpoints.
          </p>
          <div className="mt-4">
            <a href="https://github.com/githoboman/Bot-prove/tree/main/sdk/cmd/mcpserver" target="_blank" rel="noreferrer"
               className="inline-flex items-center gap-1.5 text-sm text-gray-400 hover:text-white transition-colors">
              <ExternalLink className="w-3.5 h-3.5" /> Source on GitHub
            </a>
          </div>
        </div>

        {/* Setup */}
        <div className="mb-10">
          <h2 className="text-lg font-bold text-white mb-4">Setup</h2>
          <div className="bg-black/60 rounded-xl border border-cp-border p-5 overflow-x-auto">
            <pre className="text-sm font-mono text-gray-300 leading-relaxed"><code>{`# Build the MCP server binary
go build -o bot-prove-mcp ./sdk/cmd/mcpserver

# Run with environment config
CASPERPROVER_API_URL=https://botprove-api-ylsh.onrender.com \\
CASPERPROVER_API_KEY=your-api-key \\
  ./bot-prove-mcp`}</code></pre>
          </div>
        </div>

        {/* Claude Desktop */}
        <div className="mb-10">
          <h2 className="text-lg font-bold text-white mb-4">Claude Desktop Config</h2>
          <div className="bg-black/60 rounded-xl border border-cp-border p-5 overflow-x-auto">
            <pre className="text-sm font-mono text-gray-300 leading-relaxed"><code>{`// ~/Library/Application Support/Claude/claude_desktop_config.json
{
  "mcpServers": {
    "bot-prove": {
      "command": "bot-prove-mcp",
      "env": {
        "CASPERPROVER_API_URL":
          "https://botprove-api-ylsh.onrender.com",
        "CASPERPROVER_API_KEY": "your-api-key"
      }
    }
  }
}`}</code></pre>
          </div>
        </div>

        {/* Cursor / Windsurf */}
        <div className="mb-10">
          <h2 className="text-lg font-bold text-white mb-4">Cursor / Windsurf / Any MCP Client</h2>
          <div className="bg-cp-card rounded-xl border border-cp-border p-5">
            <p className="text-sm text-gray-400 leading-relaxed">
              Any IDE or AI agent that supports MCP can connect to Bot Prove.
              Point the MCP client to the <code className="text-gray-300">bot-prove-mcp</code> binary
              with <code className="text-gray-300">stdio</code> transport. The server advertises all 32 tools
              via <code className="text-gray-300">tools/list</code> and handles calls via <code className="text-gray-300">tools/call</code>.
            </p>
          </div>
        </div>

        {/* Tools */}
        <div className="mb-10">
          <h2 className="text-lg font-bold text-white mb-6">All Tools (32)</h2>
          <div className="space-y-6">
            {tools.map((cat) => (
              <div key={cat.category}>
                <h3 className="text-sm font-semibold text-gray-400 mb-3 flex items-center gap-2">
                  <span className="w-1.5 h-1.5 rounded-full bg-red-500" />
                  {cat.category}
                </h3>
                <div className="space-y-1.5">
                  {cat.items.map((t) => (
                    <div key={t.name} className="bg-cp-card border border-cp-border rounded-lg px-4 py-2.5 flex items-center gap-3 hover:border-gray-700 transition-colors">
                      <code className="text-sm font-mono text-red-400 shrink-0">{t.name}</code>
                      <span className="text-xs text-gray-500">{t.desc}</span>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* JSON-RPC example */}
        <div className="mb-10">
          <h2 className="text-lg font-bold text-white mb-4">JSON-RPC Example</h2>
          <div className="bg-black/60 rounded-xl border border-cp-border p-5 overflow-x-auto">
            <pre className="text-sm font-mono text-gray-300 leading-relaxed"><code>{`→ {"jsonrpc":"2.0","id":1,"method":"tools/call",
   "params":{"name":"generate_proof","arguments":{
     "agent":"agent-alpha","model":"gpt-4o",
     "input":"loan_decision","output":"approved"
   }}}

← {"jsonrpc":"2.0","id":1,"result":{
     "content":[{"type":"text","text":"{...proof JSON...}"}]
   }}`}</code></pre>
          </div>
        </div>

        <div className="mt-12 border-t border-cp-border pt-8 text-center">
          <p className="text-sm text-gray-600">
            MCP server source at{' '}
            <a href="https://github.com/githoboman/Bot-prove/tree/main/sdk" target="_blank" rel="noreferrer" className="text-red-400 hover:text-red-300">
              github.com/githoboman/Bot-prove/sdk
            </a>
          </p>
        </div>
      </div>
    </div>
  )
}
