// Command mcpserver runs the CasperProver MCP (Model Context Protocol)
// server over stdio, backed by a real CasperProver API instance.
//
// Usage:
//
//	CASPERPROVER_API_URL=http://localhost:9090 go run ./sdk/cmd/mcpserver
//
// All tools map 1:1 to real, live API endpoints. No stubs.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/sdk"
)

func main() {
	baseURL := os.Getenv("CASPERPROVER_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:9090"
	}
	client := sdk.NewClient(sdk.WithBaseURL(baseURL))
	if token := os.Getenv("CASPERPROVER_API_KEY"); token != "" {
		client.SetAuthToken(token)
	}

	sdk.RunStdio(func(name string, args map[string]interface{}) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return dispatch(ctx, client, name, args)
	})
}

func dispatch(ctx context.Context, c *sdk.Client, name string, args map[string]interface{}) (string, error) {
	str := func(k string) string { s, _ := args[k].(string); return s }
	intArg := func(k string, def int) int {
		switch v := args[k].(type) {
		case float64:
			return int(v)
		case string:
			n, err := strconv.Atoi(v)
			if err == nil {
				return n
			}
		}
		return def
	}

	var (
		result interface{}
		err    error
	)

	switch name {

	// ── Core proof lifecycle ───────────────────────────────────
	case "health_check":
		result, err = c.Health(ctx)
	case "generate_proof":
		result, err = c.SubmitProof(ctx, sdk.SubmitProofRequest{
			Agent: str("agent"), Input: str("input"), Output: str("output"),
			Model: str("model"), UseCase: str("use_case"),
		})
	case "verify_proof":
		result, err = c.VerifyProof(ctx, str("proof_id"))
	case "get_proof":
		result, err = c.GetProof(ctx, str("proof_id"))
	case "list_proofs":
		result, err = c.ListProofs(ctx)
	case "revoke_proof":
		err = c.RevokeProof(ctx, str("proof_id"), str("reason"))
		result = map[string]any{"revoked": err == nil}
	case "export_proof":
		result, err = c.ExportProof(ctx, str("proof_id"))
	case "get_stats":
		result, err = c.Stats(ctx)

	// ── KYC ───────────────────────────────────────────────────
	case "kyc_check":
		result, err = c.KYCCheck(ctx, str("proof_id"))
	case "kyc_grant":
		result, err = c.KYCGrant(ctx, str("user"), str("proof_id"))
	case "kyc_whitelist":
		result, err = c.KYCWhitelist(ctx, str("user"))

	// ── Inference / model registry ────────────────────────────
	case "inference_prove":
		result, err = c.InferenceProve(ctx, sdk.InferenceProveRequest{
			Input: str("input"), Output: str("output"),
			ModelID: str("model"), Agent: str("agent"),
		})
	case "inference_verify":
		result, err = c.InferenceVerify(ctx, str("proof_id"))
	case "get_model_info":
		result, err = c.GetModel(ctx, str("model_id"))
	case "register_model":
		result, err = c.RegisterModel(ctx, sdk.RegisterModelRequest{
			ModelID: str("model_id"), ModelHash: str("model_hash"),
			VerifierContract: str("verifier_contract"),
		})

	// ── Aggregation (STARK batch) ─────────────────────────────
	case "create_batch":
		result, err = c.CreateAggregationBatch(ctx, str("batch_id"), intArg("max_proofs", 10))
	case "add_proof_to_batch":
		result, err = c.AddProofToBatch(ctx, str("batch_id"), str("proof_hash"), intArg("leaf_index", 0))
	case "finalize_batch":
		result, err = c.FinalizeBatch(ctx, str("batch_id"))
	case "get_batch":
		result, err = c.GetBatch(ctx, str("batch_id"))
	case "verify_batch":
		result, err = c.VerifyBatch(ctx, str("batch_id"))

	// ── ZK (Groth16) ──────────────────────────────────────────
	case "verify_groth16":
		result, err = c.VerifyGroth16Conceptual(ctx, str("proof"), str("vk_hash"), nil)
	case "groth16_real_prove":
		result, err = c.Groth16RealProve(ctx, str("preimage"))
	case "groth16_real_verify":
		result, err = c.Groth16RealVerify(ctx, str("hash"), str("proof_hex"))

	// ── ZK batch & challenges ──────────────────────────────────
	case "zk_batch_verify":
		var proofs []map[string]any
		if arr, ok := args["proofs"].([]interface{}); ok {
			for _, v := range arr {
				if m, ok := v.(map[string]interface{}); ok {
					pm := make(map[string]any, len(m))
					for k, v := range m { pm[k] = v }
					proofs = append(proofs, pm)
				}
			}
		}
		result, err = c.BatchVerifyZK(ctx, proofs)
	case "zk_challenge":
		result, err = c.ChallengeZK(ctx, str("proof_id"), str("reason"))
	case "zk_get_challenge":
		result, err = c.GetChallengeZK(ctx, str("id"))
	case "validate_proof_chain":
		chain := make(map[string]any)
		for k, v := range args { chain[k] = v }
		result, err = c.ValidateProofChain(ctx, chain)
	case "batch_proofs":
		var proofs []map[string]string
		if arr, ok := args["proofs"].([]interface{}); ok {
			for _, v := range arr {
				if m, ok := v.(map[string]interface{}); ok {
					pm := make(map[string]string, len(m))
					for k, v := range m { pm[k] = fmt.Sprintf("%v", v) }
					proofs = append(proofs, pm)
				}
			}
		}
		result, err = c.BatchProofs(ctx, proofs, str("mode"))

	// ── Post-quantum crypto ───────────────────────────────────
	case "pq_sign_sphincs":
		result, err = c.SignSPHINCS(ctx, str("message"))
	case "pq_verify_sphincs":
		result, err = c.VerifySPHINCS(ctx, str("message"), str("signature_hex"), str("public_key_hex"))
	case "pq_hybrid_sign":
		result, err = c.HybridSign(ctx, str("message"))
	case "pq_hybrid_verify":
		result, err = c.HybridVerify(ctx, str("message"), str("signature_hex"), str("classic_pub_hex"), str("pq_pub_hex"))

	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}

	if err != nil {
		return "", err
	}
	out, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return "", marshalErr
	}
	return string(out), nil
}
