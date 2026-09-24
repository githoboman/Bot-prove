export default function SDKSection() {
  return (
    <section id="sdk" className="py-24 bg-cp-card/30">
      <div className="cp-section">
        <div className="text-center mb-14">
          <p className="text-xs font-mono text-red-500 tracking-widest mb-3">INTEGRATION</p>
          <h2 className="text-3xl font-extrabold text-white mb-4">SDK and MCP server.</h2>
          <p className="text-gray-500 max-w-lg mx-auto">Two ways to integrate proof generation into your stack. 32 tools, all backed by real API endpoints.</p>
        </div>

        <div className="grid lg:grid-cols-2 gap-6 max-w-5xl mx-auto">
          {/* Go SDK */}
          <div className="bg-cp-card rounded-2xl border border-cp-border overflow-hidden">
            <div className="p-6 border-b border-cp-border">
              <h3 className="text-white font-bold mb-1">Go SDK</h3>
              <p className="text-sm text-gray-500">Native Go client — 32 methods covering proofs, ZK, PQ crypto, aggregation, and more.</p>
              <div className="mt-3 bg-black/40 rounded-lg px-4 py-2 font-mono text-xs text-gray-400 inline-block">
                go get github.com/anna-stolbovskaja/Bot Prove/sdk
              </div>
            </div>
            <pre className="p-5 text-xs font-mono text-gray-300 overflow-x-auto leading-relaxed"><code>{`import "github.com/anna-stolbovskaja/Bot Prove/sdk"

client := sdk.New("https://casperprover-api-ylsh.onrender.com")

// Generate & verify proof
proof, _ := client.SubmitProof(ctx, sdk.ProofInput{
    Agent: "agent-alpha", Model: "gpt-4o",
    Input: "loan_decision", Output: "approved",
})

// Real Groth16 ZK proof
zkProof, _ := client.Groth16RealProve(ctx, 42)

// Post-quantum hybrid sign
sig, _ := client.HybridSign(ctx, "message")`}</code></pre>
          </div>

          {/* MCP */}
          <div className="bg-cp-card rounded-2xl border border-cp-border overflow-hidden">
            <div className="p-6 border-b border-cp-border">
              <h3 className="text-white font-bold mb-1">MCP Server</h3>
              <p className="text-sm text-gray-500">Connect any AI agent to Bot Prove via Model Context Protocol. 32 tools, stdio transport.</p>
              <div className="mt-3 bg-black/40 rounded-lg px-4 py-2 font-mono text-xs text-gray-400 inline-block">
                go run ./sdk/cmd/mcpserver
              </div>
            </div>
            <pre className="p-5 text-xs font-mono text-gray-300 overflow-x-auto leading-relaxed"><code>{`// claude_desktop_config.json
{
  "mcpServers": {
    "bot-prove": {
      "command": "bot-prove-mcp",
      "env": {
        "CASPERPROVER_API_URL":
          "https://casperprover-api-ylsh.onrender.com"
      }
    }
  }
}
// 32 tools: proofs, ZK, PQ, aggregation,
// KYC, models, proof-chain validation`}</code></pre>
          </div>
        </div>
      </div>
    </section>
  )
}
