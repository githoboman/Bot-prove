// Package sdk provides an MCP (Model Context Protocol) server manifest and
// stdio JSON-RPC loop (RunStdio) for CasperProver.
//
// This file only defines the tool manifest and the stdio transport loop; it
// has no `func main` and cannot be run directly (`go run sdk/mcp_server.go`
// will fail - that was a documentation bug, now fixed). A working entry
// point that wires a subset of these tools to real API calls lives at
// sdk/cmd/mcpserver:
//
//	go run ./sdk/cmd/mcpserver
//
// Not every tool in the manifest below has a backing API endpoint yet
// (e.g. list_models, get_task_status, submit_batch_task - see
// docs/KNOWN_LIMITATIONS.md); cmd/mcpserver returns a clear "not
// implemented" error for those rather than pretending to execute them.
package sdk

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// MCPTool describes an MCP-compatible tool.
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// MCPManifest is returned by tools/list.
type MCPManifest struct {
	Name    string    `json:"name"`
	Version string    `json:"version"`
	Tools   []MCPTool `json:"tools"`
}

var mcpTools = []MCPTool{
	{
		Name:        "generate_proof",
		Description: "Generate a cryptographic proof of AI inference (input → output).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"agent_id": map[string]string{"type": "string", "description": "Agent identifier"},
				"input":    map[string]string{"type": "string", "description": "Inference input data"},
				"output":   map[string]string{"type": "string", "description": "Inference output data"},
				"model":    map[string]string{"type": "string", "description": "Model identifier"},
				"use_case": map[string]string{"type": "string", "description": "Use case tag (inference, kyc, etc.)"},
			},
			"required": []string{"agent_id", "input", "output", "model"},
		},
	},
	{
		Name:        "verify_proof",
		Description: "Verify that a proof is valid and not revoked.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"proof_id": map[string]string{"type": "string", "description": "Proof identifier (e.g. P-1)"},
			},
			"required": []string{"proof_id"},
		},
	},
	{
		Name:        "get_proof",
		Description: "Fetch full details of a proof by ID.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"proof_id": map[string]string{"type": "string"},
			},
			"required": []string{"proof_id"},
		},
	},
	{
		Name:        "list_proofs",
		Description: "List all generated proofs.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "revoke_proof",
		Description: "Revoke a proof, marking it as invalid.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"proof_id": map[string]string{"type": "string"},
				"reason":   map[string]string{"type": "string", "description": "Reason for revocation"},
			},
			"required": []string{"proof_id", "reason"},
		},
	},
	{
		Name:        "batch_proofs",
		Description: "Generate multiple proofs in a single batch request.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"proofs": map[string]interface{}{
					"type":        "array",
					"description": "Array of proof requests",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"agent_id": map[string]string{"type": "string"},
							"input":    map[string]string{"type": "string"},
							"output":   map[string]string{"type": "string"},
							"model":    map[string]string{"type": "string"},
						},
					},
				},
			},
			"required": []string{"proofs"},
		},
	},
	{
		Name:        "export_proof",
		Description: "Export a proof as a portable JSON bundle for external verification.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"proof_id": map[string]string{"type": "string"},
			},
			"required": []string{"proof_id"},
		},
	},
	{
		Name:        "get_stats",
		Description: "Get aggregate proof statistics: total proofs, verified count, models.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "kyc_check",
		Description: "Check whether an address passes KYC whitelist verification.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"address": map[string]string{"type": "string", "description": "Address to verify"},
			},
			"required": []string{"address"},
		},
	},
	{
		Name:        "kyc_grant",
		Description: "Grant KYC whitelist access to an address (admin only).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"address": map[string]string{"type": "string"},
				"admin":   map[string]string{"type": "string", "description": "Admin account hash"},
			},
			"required": []string{"address", "admin"},
		},
	},
	{
		Name:        "kyc_whitelist",
		Description: "Check KYC whitelist status for a specific user.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"user": map[string]string{"type": "string"},
			},
			"required": []string{"user"},
		},
	},
	{
		Name:        "health_check",
		Description: "Check API server, database, and blockchain connection health.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "get_model_info",
		Description: "Get information about a registered model by its hash.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"model_hash": map[string]string{"type": "string"},
			},
			"required": []string{"model_hash"},
		},
	},
	// --- Proof Chain (Phase 2) -------------------------------------------
	{
		Name:        "validate_proof_chain",
		Description: "Validate a proof chain DAG — checks for cycles, input continuity, and single root.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]string{"type": "string", "description": "Chain identifier"},
				"steps": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"proof_id":    map[string]string{"type": "string"},
							"parent_ids":  map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
							"model_hash":  map[string]string{"type": "string"},
							"input_hash":  map[string]string{"type": "string"},
							"output_hash": map[string]string{"type": "string"},
							"step_index":  map[string]interface{}{"type": "integer"},
						},
					},
				},
			},
			"required": []string{"steps"},
		},
	},
	// --- Inference -------------------------------------------------------
	{
		Name:        "inference_prove",
		Description: "Generate a proof of AI inference (model + input → output).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input":  map[string]string{"type": "string", "description": "Inference input data"},
				"output": map[string]string{"type": "string", "description": "Inference output data"},
				"model":  map[string]string{"type": "string", "description": "Model identifier"},
				"agent":  map[string]string{"type": "string", "description": "Agent identifier"},
			},
			"required": []string{"input", "output", "model"},
		},
	},
	{
		Name:        "inference_verify",
		Description: "Verify an inference proof by proof ID.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"proof_id": map[string]string{"type": "string"},
			},
			"required": []string{"proof_id"},
		},
	},
	// --- Aggregation (STARK batch) ------------------------------------
	{
		Name:        "create_batch",
		Description: "Create a new aggregation batch for STARK proof bundling.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"batch_id":   map[string]string{"type": "string", "description": "Batch identifier"},
				"max_proofs": map[string]interface{}{"type": "integer", "description": "Maximum proofs in batch", "default": 10},
			},
			"required": []string{"batch_id"},
		},
	},
	{
		Name:        "add_proof_to_batch",
		Description: "Add a proof hash to an open aggregation batch.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"batch_id":   map[string]string{"type": "string"},
				"proof_hash": map[string]string{"type": "string"},
				"leaf_index": map[string]interface{}{"type": "integer", "default": 0},
			},
			"required": []string{"batch_id", "proof_hash"},
		},
	},
	{
		Name:        "finalize_batch",
		Description: "Finalize an aggregation batch, producing the aggregate STARK proof.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"batch_id": map[string]string{"type": "string"},
			},
			"required": []string{"batch_id"},
		},
	},
	{
		Name:        "get_batch",
		Description: "Fetch the state of an aggregation batch by ID.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"batch_id": map[string]string{"type": "string"},
			},
			"required": []string{"batch_id"},
		},
	},
	{
		Name:        "verify_batch",
		Description: "Verify a finalized aggregation batch's aggregate STARK proof.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"batch_id": map[string]string{"type": "string"},
			},
			"required": []string{"batch_id"},
		},
	},
	// --- ZK (Groth16) -------------------------------------------------
	{
		Name:        "verify_groth16",
		Description: "Verify a Groth16 proof using hash-based conceptual verification.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"proof":         map[string]string{"type": "string"},
				"vk_hash":       map[string]string{"type": "string"},
				"public_inputs": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
			},
			"required": []string{"proof"},
		},
	},
	{
		Name:        "groth16_real_prove",
		Description: "Generate a real BN254 Groth16 proof of knowledge of a MiMC preimage.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"preimage": map[string]string{"type": "string", "description": "Base-10 integer preimage"},
			},
			"required": []string{"preimage"},
		},
	},
	{
		Name:        "groth16_real_verify",
		Description: "Verify a real BN254 Groth16 proof (from groth16_real_prove).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"hash":      map[string]string{"type": "string", "description": "MiMC hash (base-10 integer string)"},
				"proof_hex": map[string]string{"type": "string", "description": "Hex-encoded proof bytes"},
			},
			"required": []string{"hash", "proof_hex"},
		},
	},
	{
		Name:        "zk_batch_verify",
		Description: "Verify multiple ZK proofs in a single batch call.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"proofs": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"proof":         map[string]string{"type": "string"},
							"public_inputs": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
						},
					},
				},
			},
			"required": []string{"proofs"},
		},
	},
	{
		Name:        "zk_challenge",
		Description: "Open a dispute challenge against a proof.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"proof_id": map[string]string{"type": "string"},
				"reason":   map[string]string{"type": "string", "description": "Reason for challenge"},
			},
			"required": []string{"proof_id", "reason"},
		},
	},
	{
		Name:        "zk_get_challenge",
		Description: "Fetch a ZK challenge by its ID.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]string{"type": "string"},
			},
			"required": []string{"id"},
		},
	},
	// --- Post-quantum crypto ------------------------------------------
	{
		Name:        "pq_sign_sphincs",
		Description: "Sign a message with post-quantum SPHINCS+ (Lamport OTS fallback).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]string{"type": "string"},
			},
			"required": []string{"message"},
		},
	},
	{
		Name:        "pq_verify_sphincs",
		Description: "Verify a SPHINCS+ (Lamport OTS) signature.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message":        map[string]string{"type": "string"},
				"signature_hex":  map[string]string{"type": "string"},
				"public_key_hex": map[string]string{"type": "string"},
			},
			"required": []string{"message", "signature_hex", "public_key_hex"},
		},
	},
	{
		Name:        "pq_hybrid_sign",
		Description: "Sign a message with hybrid Ed25519 + ML-DSA-65.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]string{"type": "string"},
			},
			"required": []string{"message"},
		},
	},
	{
		Name:        "pq_hybrid_verify",
		Description: "Verify a hybrid Ed25519 + ML-DSA-65 signature.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message":        map[string]string{"type": "string"},
				"signature_hex":  map[string]string{"type": "string"},
				"classic_pub_hex": map[string]string{"type": "string"},
				"pq_pub_hex":     map[string]string{"type": "string"},
			},
			"required": []string{"message", "signature_hex", "classic_pub_hex", "pq_pub_hex"},
		},
	},
	// --- Model Registry tools ---
	{
		Name:        "register_model",
		Description: "Register a new AI model in the on-chain registry with its metadata and hash.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"model_name":     map[string]interface{}{"type": "string", "description": "Human-readable model name"},
				"model_hash":     map[string]interface{}{"type": "string", "description": "SHA-256 of model weights"},
				"framework":      map[string]interface{}{"type": "string", "description": "ML framework (pytorch, tensorflow, onnx)"},
				"version":        map[string]interface{}{"type": "string", "description": "Semantic version"},
				"max_input_size": map[string]interface{}{"type": "integer", "description": "Max input size in bytes"},
			},
			"required": []string{"model_name", "model_hash", "framework"},
		},
	},
}


// Manifest returns the MCP tool manifest.
func Manifest() MCPManifest {
	return MCPManifest{
		Name:    "casperprover",
		Version: "0.3.0",
		Tools:   mcpTools,
	}
}

type jsonrpcRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result"`
}

// RunStdio starts a JSON-RPC stdio loop for MCP.
// Tool calls are forwarded to the given handler function.
func RunStdio(handler func(name string, args map[string]interface{}) (string, error)) {
	fmt.Fprintln(os.Stderr, "CasperProver MCP server (stdio)")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		var req jsonrpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
			continue
		}

		var resp jsonrpcResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case "tools/list":
			resp.Result = map[string]interface{}{"tools": mcpTools}
		case "tools/call":
			name, _ := req.Params["name"].(string)
			args, _ := req.Params["arguments"].(map[string]interface{})
			if name == "" {
				resp.Result = map[string]interface{}{
					"content": []map[string]string{{"type": "text", "text": `{"error":"missing tool name"}`}},
				}
			} else {
				text, err := handler(name, args)
				if err != nil {
					// Sanitize error: don't expose internal details
					text = `{"error":"tool execution failed"}`
				}
				resp.Result = map[string]interface{}{
					"content": []map[string]string{{"type": "text", "text": text}},
				}
			}
		default:
			resp.Result = map[string]string{"error": "unknown method"}
		}

		out, _ := json.Marshal(resp)
		fmt.Println(string(out))
	}
}
