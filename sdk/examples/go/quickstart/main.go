// quickstart — smallest-possible Go example.
//
// Run:
//     go run ./sdk/examples/go/quickstart
//
// Expects the API up locally (make api-run or `cd engine && go run
// ./cmd/casperprover serve`). Two-minute end-to-end: submit a proof,
// list, get, verify — no auth token needed unless you started the
// server with API_KEY.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	sdk "github.com/anna-stolbovskaja/CasperProver/sdk"
)

func main() {
	base := os.Getenv("CP_API_URL")
	if base == "" {
		base = "http://localhost:9090"
	}
	token := os.Getenv("CP_API_KEY")

	opts := []sdk.ClientOption{sdk.WithBaseURL(base)}
	if token != "" {
		opts = append(opts, sdk.WithAuthToken(token))
	}
	client := sdk.NewClient(opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Health check first — makes failure obvious.
	h, err := client.Health(ctx)
	if err != nil {
		log.Fatalf("health: %v (is the API running at %s?)", err, base)
	}
	pretty("health", h)

	// 2. Submit a demo proof.
	req := sdk.SubmitProofRequest{
		Agent:  "quickstart-agent",
		Input:  "quickstart demo input",
		Output: "quickstart demo output",
		Model:  "demo-model-v1",
	}
	sub, err := client.SubmitProof(ctx, req)
	if err != nil {
		log.Fatalf("submit_proof: %v", err)
	}
	pretty("submit_proof", sub)

	pid, _ := sub["proof_id"].(string)
	if pid == "" {
		log.Fatalf("no proof_id in response: %v", sub)
	}

	// 3. Fetch it back.
	got, err := client.GetProof(ctx, pid)
	if err != nil {
		log.Fatalf("get_proof: %v", err)
	}
	pretty("get_proof", got)

	// 4. Verify it (server-side merkle check).
	ver, err := client.VerifyProof(ctx, pid)
	if err != nil {
		log.Fatalf("verify_proof: %v", err)
	}
	pretty("verify_proof", ver)

	fmt.Println("quickstart OK.")
}

func pretty(label string, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Printf("--- %s ---\n%s\n", label, b)
}
