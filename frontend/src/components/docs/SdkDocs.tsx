import { ArrowLeft, ExternalLink } from 'lucide-react'
import { Link } from 'react-router-dom'

const methods = [
  { category: 'Proofs', items: ['SubmitProof', 'BatchProofs', 'ListProofs', 'GetProof', 'VerifyProof', 'RevokeProof', 'ExportProof'] },
  { category: 'Inference', items: ['InferenceProve', 'InferenceVerify', 'RegisterModel', 'GetModelInfo'] },
  { category: 'ZK Proofs', items: ['VerifyGroth16', 'ZKBatchVerify', 'Groth16RealProve', 'Groth16RealVerify', 'ZKChallenge', 'ZKGetChallenge'] },
  { category: 'Aggregation', items: ['CreateBatch', 'AddProofToBatch', 'FinalizeBatch', 'GetBatch', 'VerifyBatch'] },
  { category: 'PQ Crypto', items: ['PQSignSPHINCS', 'PQVerifySPHINCS', 'HybridSign', 'HybridVerify'] },
  { category: 'KYC', items: ['KYCCheck', 'KYCGrant', 'KYCWhitelist'] },
  { category: 'Proof Chain', items: ['ValidateProofChain'] },
  { category: 'System', items: ['Health', 'GetStats'] },
]

export default function SdkDocs() {
  return (
    <div className="min-h-screen bg-[#0a0a14] text-white">
      <div className="max-w-4xl mx-auto px-6 py-16">
        <Link to="/" className="inline-flex items-center gap-1.5 text-sm text-gray-500 hover:text-gray-300 transition-colors mb-8">
          <ArrowLeft className="w-4 h-4" /> Back to home
        </Link>

        <div className="mb-12">
          <p className="text-xs font-mono text-red-500 tracking-widest mb-3">SDK</p>
          <h1 className="text-4xl font-extrabold mb-4">Go SDK</h1>
          <p className="text-gray-400 max-w-2xl leading-relaxed">
            Native Go client with 34 methods covering all 32 API endpoints. Import as a standalone module.
          </p>
          <div className="mt-4">
            <a href="https://github.com/anna-stolbovskaja/Bot Prove/tree/main/sdk" target="_blank" rel="noreferrer"
               className="inline-flex items-center gap-1.5 text-sm text-gray-400 hover:text-white transition-colors">
              <ExternalLink className="w-3.5 h-3.5" /> Source on GitHub
            </a>
          </div>
        </div>

        {/* Install */}
        <div className="mb-10">
          <h2 className="text-lg font-bold text-white mb-4">Installation</h2>
          <div className="bg-black/60 rounded-xl border border-cp-border p-5">
            <pre className="text-sm font-mono text-gray-300"><code>go get github.com/anna-stolbovskaja/Bot Prove/sdk</code></pre>
          </div>
        </div>

        {/* Quick start */}
        <div className="mb-10">
          <h2 className="text-lg font-bold text-white mb-4">Quick Start</h2>
          <div className="bg-black/60 rounded-xl border border-cp-border p-5 overflow-x-auto">
            <pre className="text-sm font-mono text-gray-300 leading-relaxed"><code>{`package main

import (
    "context"
    "fmt"
    "github.com/anna-stolbovskaja/Bot Prove/sdk"
)

func main() {
    ctx := context.Background()
    c := sdk.NewClient(
        sdk.WithBaseURL("https://botprove-api-ylsh.onrender.com"),
        sdk.WithAPIKey("your-api-key"),   // for mutating endpoints
    )

    // Generate a proof
    proof, _ := c.SubmitProof(ctx, sdk.SubmitProofRequest{
        Agent: "agent-alpha", Model: "gpt-4o",
        Input: "loan_decision", Output: "approved",
    })
    fmt.Println("Proof ID:", proof["id"])

    // Verify the proof
    result, _ := c.VerifyProof(ctx, proof["id"].(string))
    fmt.Println("Valid:", result["valid"])

    // Real Groth16 ZK proof
    zkProof, _ := c.Groth16RealProve(ctx, 42)
    fmt.Println("ZK proof:", zkProof["proof_hex"])

    // Post-quantum hybrid signature
    sig, _ := c.HybridSign(ctx, "message to sign")
    fmt.Println("PQ signature:", sig["valid"])
}`}</code></pre>
          </div>
        </div>

        {/* Python client */}
        <div className="mb-10">
          <h2 className="text-lg font-bold text-white mb-4">Python Client</h2>
          <div className="bg-black/60 rounded-xl border border-cp-border p-5 overflow-x-auto">
            <pre className="text-sm font-mono text-gray-300 leading-relaxed"><code>{`from sdk.python_client import ProverClient

client = ProverClient("https://botprove-api-ylsh.onrender.com")
proof = client.submit("agent-1", b"input", b"output", b"model", "inference")
print(proof["id"], proof["proof_hash"])

ok = client.verify(proof["id"])
print("valid:", ok)`}</code></pre>
          </div>
        </div>

        {/* Methods table */}
        <div className="mb-10">
          <h2 className="text-lg font-bold text-white mb-6">All Methods ({methods.reduce((a, c) => a + c.items.length, 0)})</h2>
          <div className="space-y-6">
            {methods.map((cat) => (
              <div key={cat.category}>
                <h3 className="text-sm font-semibold text-gray-400 mb-2 flex items-center gap-2">
                  <span className="w-1.5 h-1.5 rounded-full bg-red-500" />
                  {cat.category}
                </h3>
                <div className="bg-cp-card rounded-lg border border-cp-border p-4">
                  <div className="flex flex-wrap gap-2">
                    {cat.items.map((m) => (
                      <code key={m} className="text-xs font-mono text-gray-300 bg-black/40 px-2.5 py-1 rounded border border-cp-border">
                        c.{m}()
                      </code>
                    ))}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Client options */}
        <div className="mb-10">
          <h2 className="text-lg font-bold text-white mb-4">Client Options</h2>
          <div className="bg-cp-card rounded-xl border border-cp-border overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-cp-border">
                  <th className="text-left px-5 py-3 text-xs text-gray-500 font-mono">Option</th>
                  <th className="text-left px-5 py-3 text-xs text-gray-500 font-mono">Description</th>
                </tr>
              </thead>
              <tbody className="text-gray-300">
                <tr className="border-b border-cp-border/50"><td className="px-5 py-3 font-mono text-red-400">WithBaseURL(url)</td><td className="px-5 py-3">API base URL (default: http://localhost:9090)</td></tr>
                <tr className="border-b border-cp-border/50"><td className="px-5 py-3 font-mono text-red-400">WithAPIKey(key)</td><td className="px-5 py-3">API key for mutating endpoints</td></tr>
                <tr><td className="px-5 py-3 font-mono text-red-400">WithHTTPClient(c)</td><td className="px-5 py-3">Custom *http.Client</td></tr>
              </tbody>
            </table>
          </div>
        </div>

        <div className="mt-12 border-t border-cp-border pt-8 text-center">
          <p className="text-sm text-gray-600">
            Full SDK source and docs at{' '}
            <a href="https://github.com/anna-stolbovskaja/Bot Prove/tree/main/sdk" target="_blank" rel="noreferrer" className="text-red-400 hover:text-red-300">
              github.com/anna-stolbovskaja/Bot Prove/sdk
            </a>
          </p>
        </div>
      </div>
    </div>
  )
}
