// Package submitter builds and signs Casper 2.0 (Condor) TransactionV1
// payloads using the official casper-go-sdk/v2 and submits them to a
// Casper node via JSON-RPC.
package submitter

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/make-software/casper-go-sdk/v2/casper"
	"github.com/make-software/casper-go-sdk/v2/rpc"
	"github.com/make-software/casper-go-sdk/v2/types"
	"github.com/make-software/casper-go-sdk/v2/types/clvalue"
	"github.com/make-software/casper-go-sdk/v2/types/key"
	"github.com/make-software/casper-go-sdk/v2/types/keypair"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
)

const (
	defaultTTL       = 30 * time.Minute
	defaultGasAmount = 3_000_000_000 // 3 CSPR

	// Retry defaults for transient RPC failures (network timeouts, 5xx,
	// connection resets). Values chosen for interactive-latency submits:
	// at most 3 attempts with 200ms/400ms backoff between attempts, so
	// worst case adds ~600ms before surfacing a terminal error.
	defaultMaxAttempts    = 3
	defaultInitialBackoff = 200 * time.Millisecond
	defaultBackoffFactor  = 2.0
)

// txSubmitter is the subset of the casper-go-sdk rpc.Client surface the
// submitter depends on. Extracting an interface lets unit tests inject
// a mock RPC that returns programmed errors / successes / lookup results
// without hitting a real Casper node.
//
// The retry loop needs BOTH:
//   - PutTransactionV1: send the signed transaction
//   - GetTransactionByTransactionHash: look up whether an ambiguous
//     transient failure actually made it on-chain (idempotency check)
type txSubmitter interface {
	PutTransactionV1(ctx context.Context, tx types.TransactionV1) (rpc.PutTransactionResult, error)
	GetTransactionByTransactionHash(ctx context.Context, transactionHash string) (rpc.InfoGetTransactionResult, error)
}

// retryConfig controls how putWithIdempotentRetry escalates a transient
// failure into a retry, and how many attempts it makes.
type retryConfig struct {
	maxAttempts    int
	initialBackoff time.Duration
	backoffFactor  float64
}

// CasperSubmitter signs and submits TransactionV1 calls to on-chain
// CasperProver contracts via the casper-go-sdk RPC client.
type CasperSubmitter struct {
	chain  string
	keys   keypair.PrivateKey
	client txSubmitter
	retry  retryConfig
}

// New creates a CasperSubmitter. keyPath must point to a PEM file
// (ED25519 or SECP256K1) whose corresponding account is the contract
// deployer / has the named keys for the target contracts.
func New(nodeURL, chain, keyPath string) *CasperSubmitter {
	// Try secp256k1 first (the project's deployer keys are secp256k1),
	// then fall back to ed25519.
	keys, err := casper.NewSECP256k1PrivateKeyFromPEMFile(keyPath)
	if err != nil {
		keys, err = casper.NewED25519PrivateKeyFromPEMFile(keyPath)
		if err != nil {
			slog.Error("failed to load deployer key", "path", keyPath, "err", err)
			return nil
		}
		slog.Info("submitter loaded ED25519 key", "path", keyPath)
	} else {
		slog.Info("submitter loaded SECP256K1 key", "path", keyPath)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	rpcClient := rpc.NewClient(rpc.NewHttpHandler(nodeURL+"/rpc", httpClient))

	return &CasperSubmitter{
		chain:  chain,
		keys:   keys,
		client: rpcClient,
		retry: retryConfig{
			maxAttempts:    defaultMaxAttempts,
			initialBackoff: defaultInitialBackoff,
			backoffFactor:  defaultBackoffFactor,
		},
	}
}

// isRetryableRPCError returns true for errors that indicate a transient
// failure of the deploy-submission RPC call and are worth an idempotency
// re-check + retry:
//   - context.DeadlineExceeded / net.Error with Timeout==true
//   - connection resets / EOF / broken pipe / "connection refused"
//   - HTTP 5xx responses (surfaced by the SDK as textual errors)
//
// Terminal errors (bad payload, signing failure, 4xx from the node,
// invalid arguments) are NOT retried — retrying them would just waste
// a slot and delay the real error reaching the caller.
func isRetryableRPCError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	transientMarkers := []string{
		"connection reset",
		"connection refused",
		"broken pipe",
		"eof",
		"i/o timeout",
		"timeout",
		"temporarily unavailable",
		"service unavailable",    // 503
		"bad gateway",            // 502
		"gateway timeout",        // 504
		"internal server error",  // 500
		"status 5",               // catch-all for "HTTP status 5xx" style formatting
	}
	for _, m := range transientMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// isNotFoundError returns true when a GetTransactionByTransactionHash
// lookup indicates the transaction hasn't reached the node yet. The RPC
// error surface for "not found" varies across node versions, so we match
// the common substrings. A negative match here is safe: we simply keep
// retrying, and if the tx really is on-chain a subsequent lookup will
// confirm it.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	notFoundMarkers := []string{
		"not found",
		"unknown transaction",
		"transaction not found",
		"no such transaction",
		"failed to get transaction",
		"-32601", // method not found — some pre-Condor nodes; treat as "no info"
		"-32603", // internal — often "not present"
	}
	for _, m := range notFoundMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// putTransaction builds, signs, and submits a TransactionV1 that calls
// the given entry point on a contract identified by its hex-encoded hash.
func (s *CasperSubmitter) putTransaction(contractHash, entryPoint string, args *types.Args) (string, error) {
	pubKey := s.keys.PublicKey()

	hashBytes, err := hex.DecodeString(contractHash)
	if err != nil {
		return "", fmt.Errorf("decode contract hash: %w", err)
	}
	var hash key.Hash
	copy(hash[:], hashBytes)

	ep := entryPoint
	payload, err := types.NewTransactionV1Payload(
		types.InitiatorAddr{PublicKey: &pubKey},
		types.Timestamp(time.Now().UTC()),
		types.Duration(defaultTTL),
		s.chain,
		types.PricingMode{
			Limited: &types.LimitedMode{
				GasPriceTolerance: 1,
				StandardPayment:   true,
				PaymentAmount:     defaultGasAmount,
			},
		},
		types.NewNamedArgs(args),
		types.TransactionTarget{
			Stored: &types.StoredTarget{
				ID:      types.TransactionInvocationTarget{ByHash: &hash},
				Runtime: types.NewVmCasperV1TransactionRuntime(),
			},
		},
		types.TransactionEntryPoint{Custom: &ep},
		types.TransactionScheduling{Standard: &struct{}{}},
	)
	if err != nil {
		return "", fmt.Errorf("build payload: %w", err)
	}

	tx, err := types.MakeTransactionV1(payload)
	if err != nil {
		return "", fmt.Errorf("make transaction: %w", err)
	}

	if err := tx.Sign(s.keys); err != nil {
		return "", fmt.Errorf("sign transaction: %w", err)
	}

	// The transaction hash is deterministic and known BEFORE any RPC call —
	// this is exactly what makes idempotent retry possible: on a transient
	// failure we can ask the node "did you already accept tx X?" instead of
	// blindly re-submitting.
	txHash := tx.Hash.ToHex()

	res, err := s.putWithIdempotentRetry(context.Background(), *tx, txHash, entryPoint)
	if err != nil {
		return "", fmt.Errorf("put transaction: %w", err)
	}

	returnedHash := res.TransactionHash.TransactionV1.ToHex()
	if returnedHash != txHash {
		// Should never happen if the SDK is well-behaved, but log if it
		// does — the caller trusts the returned hash and this would be a
		// silent identity mismatch.
		slog.Warn("submitter hash mismatch after successful submit",
			"expected", txHash, "returned", returnedHash, "entry_point", entryPoint)
	}
	slog.Info("transaction submitted", "hash", returnedHash, "entry_point", entryPoint)
	return returnedHash, nil
}

// putWithIdempotentRetry calls PutTransactionV1 with exponential-backoff
// retries on transient RPC errors. Before every retry (and after the
// final attempt), it uses GetTransactionByTransactionHash to check
// whether the transaction actually made it on-chain despite the transient
// error — this is the *idempotency* guarantee: an ambiguous 5xx / socket
// reset that actually landed the tx is recognized as a success instead of
// being blindly re-submitted with the same nonce.
//
// The lookup uses the LOCALLY-COMPUTED tx hash (from the signed payload)
// so it is deterministic and doesn't depend on the flaky put response.
//
// On success returns a PutTransactionResult with the confirmed hash so
// callers can log a single value. On terminal (non-retryable) errors
// returns the raw error immediately without further retries.
func (s *CasperSubmitter) putWithIdempotentRetry(
	ctx context.Context,
	tx types.TransactionV1,
	txHash string,
	entryPoint string,
) (rpc.PutTransactionResult, error) {
	maxAttempts := s.retry.maxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	backoff := s.retry.initialBackoff
	if backoff <= 0 {
		backoff = defaultInitialBackoff
	}
	factor := s.retry.backoffFactor
	if factor <= 0 {
		factor = defaultBackoffFactor
	}

	var lastErr error
	var zero rpc.PutTransactionResult

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		res, err := s.client.PutTransactionV1(ctx, tx)
		if err == nil {
			if attempt > 1 {
				slog.Info("deploy submission succeeded after retry",
					"attempt", attempt,
					"tx_hash", txHash,
					"entry_point", entryPoint)
			}
			return res, nil
		}
		lastErr = err

		if !isRetryableRPCError(err) {
			slog.Warn("deploy submission failed with non-retryable error",
				"attempt", attempt,
				"tx_hash", txHash,
				"entry_point", entryPoint,
				"err", err)
			return zero, err
		}

		// Transient failure — before we retry (which would risk double-
		// submitting the same nonce), ask the node whether the transaction
		// actually landed despite the error.
		if landed, lookupErr := s.checkTxLanded(ctx, txHash); landed {
			slog.Info("deploy submission recovered via idempotency lookup",
				"attempt", attempt,
				"tx_hash", txHash,
				"entry_point", entryPoint,
				"put_err", err)
			return synthesizePutResult(txHash), nil
		} else if lookupErr != nil && !isNotFoundError(lookupErr) {
			// The lookup itself is broken (RPC down entirely). We can't
			// distinguish landed-vs-not, so we do NOT retry — retrying now
			// could double-spend if the tx was actually accepted.
			slog.Error("idempotency lookup failed; refusing to retry to avoid double-submit",
				"attempt", attempt,
				"tx_hash", txHash,
				"entry_point", entryPoint,
				"put_err", err,
				"lookup_err", lookupErr)
			return zero, fmt.Errorf(
				"idempotency lookup failed after transient put error (%v); refusing to retry: %w",
				err, lookupErr,
			)
		}

		if attempt == maxAttempts {
			break
		}

		slog.Warn("deploy submission transient failure, retrying after idempotency check",
			"attempt", attempt,
			"tx_hash", txHash,
			"next_backoff", backoff,
			"entry_point", entryPoint,
			"err", err)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return zero, ctx.Err()
		}
		backoff = time.Duration(float64(backoff) * factor)
	}

	// Exhausted attempts. One final idempotency check: maybe the very last
	// attempt actually landed and only the response was lost.
	if landed, lookupErr := s.checkTxLanded(ctx, txHash); landed {
		slog.Info("deploy submission landed on final-attempt idempotency check",
			"tx_hash", txHash, "entry_point", entryPoint, "last_err", lastErr)
		return synthesizePutResult(txHash), nil
	} else if lookupErr != nil && !isNotFoundError(lookupErr) {
		return zero, fmt.Errorf(
			"exhausted %d attempts (last err %v); final idempotency lookup also failed: %w",
			maxAttempts, lastErr, lookupErr,
		)
	}

	return zero, fmt.Errorf("exhausted %d attempts: %w", maxAttempts, lastErr)
}

// checkTxLanded consults the node about the tx hash. Returns (true, nil)
// when the transaction is present in any form, (false, nil) when the
// node clearly doesn't have it yet, or (false, err) when the lookup
// itself failed for an unrelated reason.
func (s *CasperSubmitter) checkTxLanded(ctx context.Context, txHash string) (bool, error) {
	// Bound the lookup so a slow node can't wedge the retry loop for
	// longer than the submit path itself was willing to wait.
	lookupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := s.client.GetTransactionByTransactionHash(lookupCtx, txHash)
	if err == nil {
		return true, nil
	}
	if isNotFoundError(err) {
		return false, nil
	}
	return false, err
}

// synthesizePutResult builds the minimum-viable rpc.PutTransactionResult
// callers depend on when we've confirmed success via idempotency lookup
// (the original PutTransactionV1 response was lost / errored). Only the
// TransactionHash.TransactionV1 field is consumed by putTransaction.
func synthesizePutResult(txHash string) rpc.PutTransactionResult {
	h, err := key.NewHash(txHash)
	if err != nil {
		// Should be unreachable — txHash came from tx.Hash.ToHex() upstream.
		slog.Error("synthesizePutResult: bad hex from local tx hash", "hash", txHash, "err", err)
		return rpc.PutTransactionResult{}
	}
	return rpc.PutTransactionResult{
		TransactionHash: types.TransactionHash{TransactionV1: &h},
	}
}

// Submit anchors a proof to the on-chain proof_registry contract.
// The contract hash is read from CONTRACT_PROOF_REGISTRY env var.
func (s *CasperSubmitter) Submit(p *prover.Proof) (string, error) {
	contractHash := os.Getenv("CONTRACT_PROOF_REGISTRY")
	if contractHash == "" {
		return "", fmt.Errorf("CONTRACT_PROOF_REGISTRY env var not set")
	}

	args := &types.Args{}
	args.AddArgument("proof_hash", *clvalue.NewCLString(p.PH)).
		AddArgument("input_hash", *clvalue.NewCLString(p.IH)).
		AddArgument("output_hash", *clvalue.NewCLString(p.OH)).
		AddArgument("model_hash", *clvalue.NewCLString(p.MH))

	return s.putTransaction(contractHash, "submit_proof", args)
}

// Revoke marks a proof as revoked on-chain.
func (s *CasperSubmitter) Revoke(pid, reason string) (string, error) {
	contractHash := os.Getenv("CONTRACT_PROOF_REGISTRY")
	if contractHash == "" {
		return "", fmt.Errorf("CONTRACT_PROOF_REGISTRY env var not set")
	}

	args := &types.Args{}
	args.AddArgument("proof_id", *clvalue.NewCLString(pid)).
		AddArgument("reason", *clvalue.NewCLString(reason))

	return s.putTransaction(contractHash, "revoke_proof", args)
}

// SubmitModelRegistration registers a model on the model_registry contract.
func (s *CasperSubmitter) SubmitModelRegistration(modelID, modelHash, verifierContract string, metadata map[string]string) (string, error) {
	contractHash := os.Getenv("CONTRACT_MODEL_REGISTRY")
	if contractHash == "" {
		return "", fmt.Errorf("CONTRACT_MODEL_REGISTRY env var not set")
	}

	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}

	args := &types.Args{}
	args.AddArgument("model_id", *clvalue.NewCLString(modelID)).
		AddArgument("model_hash", *clvalue.NewCLString(modelHash)).
		AddArgument("verifier_contract", *clvalue.NewCLString(verifierContract)).
		AddArgument("metadata", *clvalue.NewCLString(string(metaJSON)))

	return s.putTransaction(contractHash, "register_model", args)
}

// RegisterZkVk pins a verifying-key digest for a circuit_id on the on-chain
// zk-verifier contract (BACKLOG 1.8 + 2.6 anchor). governanceApproved=0 for
// direct-owner calls, 1 for calls proved off-chain against governance's
// is_executed(proposal_id)==1.
func (s *CasperSubmitter) RegisterZkVk(circuitID, vkHash, curve, backend string, governanceApproved uint64) (string, error) {
	contractHash := os.Getenv("CONTRACT_ZK_VERIFIER")
	if contractHash == "" {
		return "", fmt.Errorf("CONTRACT_ZK_VERIFIER env var not set")
	}
	args := &types.Args{}
	args.AddArgument("governance_approved", *clvalue.NewCLUInt64(governanceApproved)).
		AddArgument("circuit_id", *clvalue.NewCLString(circuitID)).
		AddArgument("vk_hash", *clvalue.NewCLString(vkHash)).
		AddArgument("curve", *clvalue.NewCLString(curve)).
		AddArgument("backend", *clvalue.NewCLString(backend))
	return s.putTransaction(contractHash, "register_vk", args)
}

// RecordZkVerdict anchors an off-chain Groth16 verification verdict on-chain
// against a specific (circuit_id, proof_hash). Public inputs are hashed off-chain
// (sha256 of their canonical encoding) so the on-chain record stays fixed-size.
func (s *CasperSubmitter) RecordZkVerdict(circuitID, proofHash, publicInputsHash, modelID string, valid bool) (string, error) {
	contractHash := os.Getenv("CONTRACT_ZK_VERIFIER")
	if contractHash == "" {
		return "", fmt.Errorf("CONTRACT_ZK_VERIFIER env var not set")
	}
	var verdict uint64
	if valid {
		verdict = 1
	}
	args := &types.Args{}
	args.AddArgument("circuit_id", *clvalue.NewCLString(circuitID)).
		AddArgument("proof_hash", *clvalue.NewCLString(proofHash)).
		AddArgument("public_inputs_hash", *clvalue.NewCLString(publicInputsHash)).
		AddArgument("model_id", *clvalue.NewCLString(modelID)).
		AddArgument("verdict", *clvalue.NewCLUInt64(verdict))
	return s.putTransaction(contractHash, "record_verdict", args)
}

// AddZkVerifier authorizes an account hash to record verdicts.
func (s *CasperSubmitter) AddZkVerifier(verifierAccountHash string) (string, error) {
	contractHash := os.Getenv("CONTRACT_ZK_VERIFIER")
	if contractHash == "" {
		return "", fmt.Errorf("CONTRACT_ZK_VERIFIER env var not set")
	}
	acc, err := key.NewAccountHash(verifierAccountHash)
	if err != nil {
		return "", fmt.Errorf("parse verifier account hash: %w", err)
	}
	args := &types.Args{}
	args.AddArgument("verifier", clvalue.NewCLByteArray(acc.Bytes()))
	return s.putTransaction(contractHash, "add_verifier", args)
}
