// Package submitter — retry & idempotency tests.
//
// These tests exercise the exponential-backoff retry loop AND the
// idempotency guarantee that distinguishes a real "tx got lost in
// transit" case from an ambiguous "5xx returned but the node might
// actually have accepted the tx" case. The behavioural contract:
//
//   - Success on the first PutTransactionV1 call → no lookup.
//   - Non-retryable error (4xx, signing failure, malformed payload) →
//     surfaces immediately; NO retry, NO lookup.
//   - Transient error + a subsequent lookup that says "landed" →
//     treated as success (idempotency win, no re-submission).
//   - Transient error + lookup says "not found" → sleep backoff, retry.
//   - Transient error + lookup itself is broken (not a not-found) →
//     ABORT retry loop entirely, because retrying now risks a double-
//     submission of the same nonce.
//   - Exhausted attempts + a final lookup that says "landed" → treated
//     as success (very last attempt may have landed but response lost).
//   - Context cancellation during backoff → surfaces ctx.Err().
//
// The full engine suite continues to pass; these tests target only the
// submitter package.
package submitter

import (
	"os"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/make-software/casper-go-sdk/v2/rpc"
	"github.com/make-software/casper-go-sdk/v2/types"
	"github.com/make-software/casper-go-sdk/v2/types/key"
)

// -----------------------------------------------------------------------------
// fake txSubmitter
// -----------------------------------------------------------------------------

// programmed represents a scripted PutTransactionV1 response.
type programmed struct {
	err error
}

type fakeSubmitter struct {
	// putScript is consumed one entry per Put call.
	putScript []programmed
	putCalls  int

	// lookupScript is consumed one entry per Get call.
	lookupScript []programmed
	lookupCalls  int

	// fixedHash is what synthesized results should carry.
	fixedHash key.Hash
}

func (f *fakeSubmitter) PutTransactionV1(ctx context.Context, tx types.TransactionV1) (rpc.PutTransactionResult, error) {
	if f.putCalls >= len(f.putScript) {
		return rpc.PutTransactionResult{}, errors.New("fake: put script exhausted")
	}
	step := f.putScript[f.putCalls]
	f.putCalls++
	if step.err != nil {
		return rpc.PutTransactionResult{}, step.err
	}
	return rpc.PutTransactionResult{
		TransactionHash: types.TransactionHash{TransactionV1: &f.fixedHash},
	}, nil
}

func (f *fakeSubmitter) GetTransactionByTransactionHash(ctx context.Context, txHash string) (rpc.InfoGetTransactionResult, error) {
	if f.lookupCalls >= len(f.lookupScript) {
		return rpc.InfoGetTransactionResult{}, errors.New("fake: lookup script exhausted")
	}
	step := f.lookupScript[f.lookupCalls]
	f.lookupCalls++
	if step.err != nil {
		return rpc.InfoGetTransactionResult{}, step.err
	}
	return rpc.InfoGetTransactionResult{}, nil
}

// makeCasperWithFake builds a CasperSubmitter wired to the fake without
// touching the real network. Retry timing is compressed to keep tests fast.
func makeCasperWithFake(t *testing.T, fake *fakeSubmitter) *CasperSubmitter {
	t.Helper()
	// Deterministic dummy hash so synthesizePutResult produces a valid key.
	h, err := key.NewHash("0000000000000000000000000000000000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("dummy hash: %v", err)
	}
	fake.fixedHash = h
	return &CasperSubmitter{
		chain:  "test-net",
		client: fake,
		retry: retryConfig{
			maxAttempts:    3,
			initialBackoff: 1 * time.Millisecond,
			backoffFactor:  2.0,
		},
	}
}

// dummyTx returns a zero-value TransactionV1 that the fake ignores. Real
// tx construction is exercised in the (network-touching) integration
// tests / end-to-end submit path; these unit tests focus on retry
// mechanics.
func dummyTx() types.TransactionV1 { return types.TransactionV1{} }

// stableHexHash is a valid 64-char hex string used as the local tx
// hash across the retry tests. Only used by synthesizePutResult, which
// requires the hash to parse successfully via key.NewHash.
const stableHexHash = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"

// -----------------------------------------------------------------------------
// isRetryableRPCError
// -----------------------------------------------------------------------------

func TestIsRetryableRPCError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline_exceeded", context.DeadlineExceeded, true},
		{"connection_reset", errors.New("read tcp 127.0.0.1: connection reset by peer"), true},
		{"connection_refused", errors.New("dial tcp: connection refused"), true},
		{"broken_pipe", errors.New("write: broken pipe"), true},
		{"eof", errors.New("EOF"), true},
		{"io_timeout", errors.New("i/o timeout"), true},
		{"generic_timeout", errors.New("request timeout"), true},
		{"503", errors.New("HTTP 503 service unavailable"), true},
		{"502", errors.New("bad gateway"), true},
		{"504", errors.New("gateway timeout"), true},
		{"500", errors.New("internal server error"), true},
		{"generic_5xx", errors.New("upstream returned status 5xx"), true},
		{"400_bad_request", errors.New("HTTP 400 bad request"), false},
		{"401_unauthorized", errors.New("HTTP 401 unauthorized"), false},
		{"404_not_found", errors.New("HTTP 404 not found"), false},
		{"422_invalid_payload", errors.New("HTTP 422 invalid transaction"), false},
		{"signing_failure", errors.New("failed to sign transaction: bad key"), false},
		{"validation_error", errors.New("invalid pricing mode"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableRPCError(tc.err); got != tc.want {
				t.Fatalf("isRetryableRPCError(%v) = %v; want %v", tc.err, got, tc.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// isNotFoundError
// -----------------------------------------------------------------------------

func TestIsNotFoundError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not_found_lower", errors.New("transaction not found"), true},
		{"not_found_upper", errors.New("Transaction Not Found"), true},
		{"unknown_transaction", errors.New("unknown transaction hash"), true},
		{"no_such_transaction", errors.New("no such transaction"), true},
		{"failed_to_get", errors.New("failed to get transaction: missing"), true},
		{"method_not_found_pre_condor", errors.New("json-rpc error -32601"), true},
		{"generic_5xx_is_not_notfound", errors.New("500 internal server error"), false},
		{"connection_reset_is_not_notfound", errors.New("connection reset by peer"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNotFoundError(tc.err); got != tc.want {
				t.Fatalf("isNotFoundError(%v) = %v; want %v", tc.err, got, tc.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// putWithIdempotentRetry — happy paths
// -----------------------------------------------------------------------------

func TestPutWithIdempotentRetry_FirstAttemptSucceeds(t *testing.T) {
	fake := &fakeSubmitter{
		putScript: []programmed{{err: nil}},
	}
	s := makeCasperWithFake(t, fake)

	_, err := s.putWithIdempotentRetry(context.Background(), dummyTx(), stableHexHash, "submit_proof")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if fake.putCalls != 1 {
		t.Fatalf("put calls = %d; want 1", fake.putCalls)
	}
	if fake.lookupCalls != 0 {
		t.Fatalf("lookup calls = %d; want 0 (no lookup on happy path)", fake.lookupCalls)
	}
}

// A transient failure followed by success on the second attempt is the
// classic retry win: two puts, two lookups (one after the failed put,
// then no lookup after the success).
func TestPutWithIdempotentRetry_TransientThenSuccess(t *testing.T) {
	fake := &fakeSubmitter{
		putScript: []programmed{
			{err: errors.New("connection reset by peer")},
			{err: nil},
		},
		lookupScript: []programmed{
			{err: errors.New("transaction not found")}, // not landed yet
		},
	}
	s := makeCasperWithFake(t, fake)

	_, err := s.putWithIdempotentRetry(context.Background(), dummyTx(), stableHexHash, "submit_proof")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if fake.putCalls != 2 {
		t.Fatalf("put calls = %d; want 2", fake.putCalls)
	}
	if fake.lookupCalls != 1 {
		t.Fatalf("lookup calls = %d; want 1 (one lookup after the transient failure)", fake.lookupCalls)
	}
}

// -----------------------------------------------------------------------------
// putWithIdempotentRetry — idempotency wins
// -----------------------------------------------------------------------------

// The core reason this whole file exists: a transient error where the
// tx *actually landed* on the chain must be recognized as success, NOT
// blindly re-submitted. Otherwise the retry poisons the nonce.
func TestPutWithIdempotentRetry_IdempotencyRecovery(t *testing.T) {
	fake := &fakeSubmitter{
		putScript: []programmed{
			{err: errors.New("i/o timeout")}, // ambiguous transient
		},
		lookupScript: []programmed{
			{err: nil}, // tx IS on-chain despite the transient error
		},
	}
	s := makeCasperWithFake(t, fake)

	res, err := s.putWithIdempotentRetry(context.Background(), dummyTx(), stableHexHash, "submit_proof")
	if err != nil {
		t.Fatalf("expected idempotent-recovery success, got %v", err)
	}
	if fake.putCalls != 1 {
		t.Fatalf("put calls = %d; want 1 (must NOT re-submit after idempotency hit)", fake.putCalls)
	}
	if fake.lookupCalls != 1 {
		t.Fatalf("lookup calls = %d; want 1", fake.lookupCalls)
	}
	if res.TransactionHash.TransactionV1 == nil {
		t.Fatalf("expected synthesized result to carry a tx hash")
	}
}

// Same idempotency win, but the recovery happens on the FINAL attempt:
// last put transiently fails, the after-loop lookup says "landed",
// success is returned.
func TestPutWithIdempotentRetry_FinalAttemptLookupLanded(t *testing.T) {
	// All 3 attempts transiently fail; each of the intermediate lookups
	// says "not found"; the FINAL post-loop lookup says "landed".
	fake := &fakeSubmitter{
		putScript: []programmed{
			{err: errors.New("service unavailable")},
			{err: errors.New("service unavailable")},
			{err: errors.New("service unavailable")},
		},
		lookupScript: []programmed{
			{err: errors.New("transaction not found")},
			{err: errors.New("transaction not found")},
			{err: nil}, // post-loop check: it actually landed
		},
	}
	s := makeCasperWithFake(t, fake)

	_, err := s.putWithIdempotentRetry(context.Background(), dummyTx(), stableHexHash, "submit_proof")
	if err != nil {
		t.Fatalf("expected post-loop idempotent recovery, got %v", err)
	}
	if fake.putCalls != 3 {
		t.Fatalf("put calls = %d; want 3", fake.putCalls)
	}
	if fake.lookupCalls != 3 {
		t.Fatalf("lookup calls = %d; want 3 (two mid-loop + one post-loop)", fake.lookupCalls)
	}
}

// -----------------------------------------------------------------------------
// putWithIdempotentRetry — non-retryable terminal errors
// -----------------------------------------------------------------------------

func TestPutWithIdempotentRetry_NonRetryableStopsFirstCall(t *testing.T) {
	fake := &fakeSubmitter{
		putScript: []programmed{
			{err: errors.New("HTTP 422 invalid transaction: bad payment")},
		},
	}
	s := makeCasperWithFake(t, fake)

	_, err := s.putWithIdempotentRetry(context.Background(), dummyTx(), stableHexHash, "submit_proof")
	if err == nil {
		t.Fatalf("expected terminal error to propagate")
	}
	if fake.putCalls != 1 {
		t.Fatalf("put calls = %d; want 1 (no retry on terminal)", fake.putCalls)
	}
	if fake.lookupCalls != 0 {
		t.Fatalf("lookup calls = %d; want 0 (no lookup on terminal)", fake.lookupCalls)
	}
}

// -----------------------------------------------------------------------------
// putWithIdempotentRetry — broken lookup aborts to avoid double-submit
// -----------------------------------------------------------------------------

// If the lookup itself is broken (not a clean not-found), we can't
// distinguish landed-vs-not, so retrying now would risk a double-submit
// of the same nonce. Contract: abort the retry loop and surface the
// combined error.
func TestPutWithIdempotentRetry_BrokenLookupAborts(t *testing.T) {
	fake := &fakeSubmitter{
		putScript: []programmed{
			{err: errors.New("i/o timeout")},
		},
		lookupScript: []programmed{
			{err: errors.New("HTTP 500 internal server error")}, // lookup itself unhealthy
		},
	}
	s := makeCasperWithFake(t, fake)

	_, err := s.putWithIdempotentRetry(context.Background(), dummyTx(), stableHexHash, "submit_proof")
	if err == nil {
		t.Fatalf("expected abort when lookup unhealthy")
	}
	if fake.putCalls != 1 {
		t.Fatalf("put calls = %d; want 1 (must NOT retry when lookup broken)", fake.putCalls)
	}
	if fake.lookupCalls != 1 {
		t.Fatalf("lookup calls = %d; want 1", fake.lookupCalls)
	}
}

// -----------------------------------------------------------------------------
// putWithIdempotentRetry — exhausted attempts
// -----------------------------------------------------------------------------

// Every put transiently fails AND every lookup cleanly reports not-found:
// after maxAttempts we give up with the last-put error.
func TestPutWithIdempotentRetry_MaxAttemptsExhausted(t *testing.T) {
	fake := &fakeSubmitter{
		putScript: []programmed{
			{err: errors.New("connection reset by peer")},
			{err: errors.New("connection reset by peer")},
			{err: errors.New("connection reset by peer")},
		},
		lookupScript: []programmed{
			{err: errors.New("transaction not found")},
			{err: errors.New("transaction not found")},
			{err: errors.New("transaction not found")}, // final post-loop
		},
	}
	s := makeCasperWithFake(t, fake)

	_, err := s.putWithIdempotentRetry(context.Background(), dummyTx(), stableHexHash, "submit_proof")
	if err == nil {
		t.Fatalf("expected exhaustion error after 3 attempts")
	}
	if fake.putCalls != 3 {
		t.Fatalf("put calls = %d; want 3", fake.putCalls)
	}
	if fake.lookupCalls != 3 {
		t.Fatalf("lookup calls = %d; want 3 (2 mid-loop + 1 final)", fake.lookupCalls)
	}
}

// -----------------------------------------------------------------------------
// putWithIdempotentRetry — context cancellation
// -----------------------------------------------------------------------------

// Cancelling the context during backoff must abort promptly.
func TestPutWithIdempotentRetry_ContextCancellationAbortsBackoff(t *testing.T) {
	fake := &fakeSubmitter{
		putScript: []programmed{
			{err: errors.New("connection reset by peer")},
			{err: nil}, // never reached
		},
		lookupScript: []programmed{
			{err: errors.New("transaction not found")},
		},
	}
	s := makeCasperWithFake(t, fake)
	// Deliberately-slow backoff so the cancellation path is exercised.
	s.retry.initialBackoff = 10 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel almost immediately so the backoff <-time.After never fires.
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := s.putWithIdempotentRetry(ctx, dummyTx(), stableHexHash, "submit_proof")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected ctx-cancel error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("cancel should be prompt; took %v", elapsed)
	}
}

// -----------------------------------------------------------------------------
// synthesizePutResult — hash preservation
// -----------------------------------------------------------------------------

func TestSynthesizePutResult_PreservesHash(t *testing.T) {
	h := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	res := synthesizePutResult(h)
	if res.TransactionHash.TransactionV1 == nil {
		t.Fatalf("expected TransactionV1 set")
	}
	got := res.TransactionHash.TransactionV1.ToHex()
	if got != h {
		t.Fatalf("hash roundtrip: got %s, want %s", got, h)
	}
}

// -----------------------------------------------------------------------------
// zk-verifier submitter env-var contract tests
// -----------------------------------------------------------------------------

// These tests exercise the env-var contract of the zk-verifier submitters
// without hitting a live Casper RPC endpoint. Missing CONTRACT_ZK_VERIFIER
// must fail fast with a clear message - this is the guardrail that stops
// a mis-configured engine from silently NOT anchoring verdicts.

func TestRegisterZkVk_MissingEnvFails(t *testing.T) {
	_ = os.Unsetenv("CONTRACT_ZK_VERIFIER")
	s := &CasperSubmitter{}
	_, err := s.RegisterZkVk("mimc_v1", "aa", "bn254", "groth16", 0)
	if err == nil {
		t.Fatal("expected error when CONTRACT_ZK_VERIFIER unset")
	}
	if err.Error() != "CONTRACT_ZK_VERIFIER env var not set" {
		t.Fatalf("unexpected err message: %v", err)
	}
}

func TestRecordZkVerdict_MissingEnvFails(t *testing.T) {
	_ = os.Unsetenv("CONTRACT_ZK_VERIFIER")
	s := &CasperSubmitter{}
	_, err := s.RecordZkVerdict("mimc_v1", "aa", "bb", "gpt-4o-mini", true)
	if err == nil {
		t.Fatal("expected error when CONTRACT_ZK_VERIFIER unset")
	}
}

func TestAddZkVerifier_MissingEnvFails(t *testing.T) {
	_ = os.Unsetenv("CONTRACT_ZK_VERIFIER")
	s := &CasperSubmitter{}
	_, err := s.AddZkVerifier("00000000000000000000000000000000000000000000000000000000deadbeef")
	if err == nil {
		t.Fatal("expected error when CONTRACT_ZK_VERIFIER unset")
	}
}

func TestAddZkVerifier_BadAccountHash(t *testing.T) {
	_ = os.Setenv("CONTRACT_ZK_VERIFIER", "00")
	defer func() { _ = os.Unsetenv("CONTRACT_ZK_VERIFIER") }()
	s := &CasperSubmitter{}
	_, err := s.AddZkVerifier("not-hex")
	if err == nil {
		t.Fatal("expected error on malformed account hash")
	}
}
