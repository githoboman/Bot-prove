# CasperProver SDK

`sdk/` is a standalone Go module (`github.com/anna-stolbovskaja/CasperProver/sdk`),
separate from `engine/`, so it can be published/imported independently. Every
`Client` method maps 1:1 to a real route in `engine/internal/api/server.go` -
see `docs/openapi.yaml` for the authoritative route list. Not every route has
a typed method yet; PRs welcome.

## Go client

```go
package main

import (
    "context"
    "fmt"

    "github.com/anna-stolbovskaja/CasperProver/sdk"
)

func main() {
    ctx := context.Background()
    c := sdk.NewClient(sdk.WithBaseURL("http://localhost:9090"))

    // generate proof
    proof, err := c.SubmitProof(ctx, sdk.SubmitProofRequest{
        Agent: "agent-1", Input: "input", Output: "output", Model: "model",
    })
    if err != nil {
        panic(err)
    }
    fmt.Println(proof["id"], proof["proof_hash"])

    // verify
    result, _ := c.VerifyProof(ctx, proof["id"].(string))
    fmt.Println("valid:", result["valid"])

    // list all
    all, _ := c.ListProofs(ctx)
    fmt.Println(all)
}
```

Client responses are `map[string]any` rather than fixed structs, because
several endpoints (proofs, aggregation batches) return optional fields that
vary by state - see `sdk/types.go` for the rationale.

## python client

```python
from sdk.python_client import ProverClient

client = ProverClient("http://localhost:9090")
proof = client.submit("agent-1", b"input", b"output", b"model", "inference")
print(proof["id"], proof["proof_hash"])

ok = client.verify(proof["id"])
print("valid:", ok)
```

## mcp server

The MCP tool manifest and stdio JSON-RPC loop are defined in
`sdk/mcp_server.go`; the runnable entry point that wires a subset of those
tools to a real running API instance is `sdk/cmd/mcpserver`:

```bash
CASPERPROVER_API_URL=http://localhost:9090 go run ./sdk/cmd/mcpserver
```

### tools

| tool | description | backed by real API? |
|------|-------------|----------------------|
| `health_check` | API health check | ✅ |
| `generate_proof` | create proof of AI inference | ✅ |
| `verify_proof` | check proof validity | ✅ |
| `get_proof` | fetch proof details | ✅ |
| `list_proofs` | list all proofs | ✅ |
| `revoke_proof` | invalidate a proof | ✅ |
| `export_proof` | export proof + chain metadata | ✅ |
| `get_stats` | engine-wide proof stats | ✅ |
| `kyc_check` / `kyc_grant` / `kyc_whitelist` | KYC flow | ✅ |
| `inference_prove` / `inference_verify` | inference proof lifecycle | ✅ |
| `get_model_info` / `register_model` | model registry | ✅ |
| `create_batch` / `add_proof_to_batch` / `finalize_batch` / `get_batch` / `verify_batch` | STARK aggregation batches | ✅ |
| `verify_groth16` | hash-based Groth16 check | ✅ |
| `groth16_real_prove` / `groth16_real_verify` | real BN254 Groth16 via gnark | ✅ |
| `pq_sign_sphincs` / `pq_verify_sphincs` | hash-based OTS signing (Lamport OTS, occupies the SPHINCS+ family slot) | ✅ |
| `pq_hybrid_sign` / `pq_hybrid_verify` | hybrid Ed25519 + ML-DSA-65 signing | ✅ |

| `zk_batch_verify` | batch-verify multiple ZK proofs | ✅ |
| `zk_challenge` / `zk_get_challenge` | dispute challenge lifecycle | ✅ |
| `batch_proofs` | bulk proof creation | ✅ |
| `validate_proof_chain` | DAG validation (Phase 2) | ✅ |

All 32 tools map 1:1 to live API endpoints. No stubs.

### Categories

| Category | Tools | Count |
|---|---|---|
| Proofs | generate_proof, verify_proof, get_proof, list_proofs, revoke_proof, export_proof, batch_proofs | 7 |
| Inference | inference_prove, inference_verify, register_model, get_model_info | 4 |
| ZK | verify_groth16, groth16_real_prove, groth16_real_verify, zk_batch_verify, zk_challenge, zk_get_challenge | 6 |
| Aggregation | create_batch, add_proof_to_batch, finalize_batch, get_batch, verify_batch | 5 |
| PQ Crypto | pq_sign_sphincs, pq_verify_sphincs, pq_hybrid_sign, pq_hybrid_verify | 4 |
| KYC | kyc_check, kyc_grant, kyc_whitelist | 3 |
| Proof Chain | validate_proof_chain | 1 |
| System | health_check, get_stats | 2 |
