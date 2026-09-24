# CP T1' (revised) — Deploy Submitter: Idempotent Retry with Tx-Lookup Pre-check

**Branch**: `fix/cp-deploy-submitter-idempotent-retry`
**Author**: anna-stolbovskaja
**Merge order**: standalone; independent of `fix/cp-changelog-gaps` and the two upcoming middleware/wallet PRs.

## Why this exists

Curator feedback on the original `fix/agent-batch-3` T1 (`e88991b fix: retry
logic for deploy submitter`):

> Retry требует idempotency/tx lookup.

The original commit only added exponential-backoff retry with terminal-vs-
transient error classification. That is safe when a transient error means
"the tx never left our socket", but it is **actively dangerous** when a
transient error means "the tx was accepted by the node, but the response
was lost" — a naive retry then replays the same signed payload with the
same nonce and can result in:

1. The node rejects the retry with "already known" (best case; wasted attempt).
2. On some setups the retry lands in a different bucket and a second
   inclusion is attempted (worst case; nonce/gas duplication).

The correct semantics for a submitter is **idempotent retry**: on a
transient failure, ask the chain whether the transaction actually landed
before deciding whether to re-submit.

## What this PR does

Replaces the goal-post — the retry loop now has **three legs**, not two:

1. **Submit** — `PutTransactionV1(tx)`.
2. **Classify error** — non-retryable (4xx, signing failure, bad payload)
   surfaces immediately. Only transient errors (5xx, timeouts, connection
   resets, EOF, broken pipe, deadline-exceeded) fall through to leg 3.
3. **Idempotency check + retry gate** — `GetTransactionByTransactionHash`
   using the locally-computed tx hash from the signed payload:
   * If the tx is on-chain → treat as success (synthesized `PutTransactionResult`
     carrying the confirmed hash). **No re-submit.**
   * If the node cleanly reports not-found → sleep the backoff, retry.
   * If the lookup itself is broken (500 / connection error, not not-found)
     → **abort the whole retry loop** — we can't distinguish landed from
     not-landed, so retrying would risk a double-submit. Better to surface
     an error to the caller than silently replay a nonce.

After all attempts are exhausted, one **final post-loop lookup** covers
the corner case where the *very last* attempt actually landed but the
response was lost.

The transaction hash used for idempotency is `tx.Hash.ToHex()` from the
signed `TransactionV1`, which is deterministic and known before any RPC
call — this is exactly what makes idempotent retry possible.

## Interface change

`CasperSubmitter.client` moved from the concrete `rpc.Client` to a small
`txSubmitter` interface exposing exactly the two methods the retry loop
needs:

```go
type txSubmitter interface {
    PutTransactionV1(ctx, tx) (rpc.PutTransactionResult, error)
    GetTransactionByTransactionHash(ctx, hash) (rpc.InfoGetTransactionResult, error)
}
```

The concrete `rpc.Client` from `make-software/casper-go-sdk/v2` satisfies
this interface without any adapter — the constructor is unchanged. The
interface exists only so unit tests can inject a `fakeSubmitter` that
plays scripted put/lookup responses without touching a real node.

No public API of the `submitter` package changed. `New`, `Submit`,
`Revoke`, `SubmitModelRegistration` all have identical signatures.

## Tests (13, all pass)

- `TestIsRetryableRPCError` — 19 sub-cases covering nil, deadline-exceeded,
  net.Error timeouts, all common 5xx strings, and every 4xx / signing /
  validation error that must NOT retry.
- `TestIsNotFoundError` — 8 sub-cases covering the various "not found"
  error strings across node versions (case-insensitive), plus negative
  cases (500 and connection-reset must NOT be misread as "not found").
- `TestPutWithIdempotentRetry_FirstAttemptSucceeds` — happy path: 1 put,
  0 lookups.
- `TestPutWithIdempotentRetry_TransientThenSuccess` — 2 puts, 1 lookup
  (the mid-loop lookup that says "not found").
- **`TestPutWithIdempotentRetry_IdempotencyRecovery`** — the reason this
  file exists: a transient put error whose subsequent lookup says "landed"
  is treated as success, **exactly one put call is made**, the returned
  hash matches the locally-computed one.
- `TestPutWithIdempotentRetry_FinalAttemptLookupLanded` — the post-loop
  idempotency check catches the case where the last attempt actually
  landed but the response was lost.
- `TestPutWithIdempotentRetry_NonRetryableStopsFirstCall` — 4xx / bad
  payload surfaces immediately; NO retry, NO lookup.
- **`TestPutWithIdempotentRetry_BrokenLookupAborts`** — the safety hatch:
  if the lookup is unhealthy (500, not not-found) we abort the retry
  loop to prevent a double-submit. Exactly 1 put, exactly 1 lookup.
- `TestPutWithIdempotentRetry_MaxAttemptsExhausted` — 3 puts + 3 lookups
  (2 mid-loop + 1 post-loop), then a clean exhaustion error.
- `TestPutWithIdempotentRetry_ContextCancellationAbortsBackoff` — cancel
  during backoff aborts promptly with `context.Canceled`.
- `TestSynthesizePutResult_PreservesHash` — the hash we synthesize for
  idempotent-recovery success round-trips correctly via `key.NewHash`.

Full engine suite: **every package green** (`go test ./...`) — no
regressions in aggregator, api, crypto, hasher, inference, kyc, model,
prover, submitter, verifier, worker, zkverifier, or gnarkzk.

`go vet ./...` clean.

## What is deliberately NOT here

- **No CHANGELOG entry.** The batch-3-withdrawal PR (`fix/cp-changelog-gaps`,
  already open) explicitly did not backfill retry logic to CHANGELOG,
  because retry was withdrawn. When THIS PR lands, a two-line CHANGELOG
  addition (`fix: idempotent retry for deploy submitter — exponential
  backoff with pre-retry tx-lookup to prevent nonce double-submission on
  ambiguous RPC failures`) should be included in the merge commit.

## Files changed

- `engine/internal/submitter/casper.go` — retry loop + idempotency
  lookup + safety hatch. Existing `Submit` / `Revoke` /
  `SubmitModelRegistration` untouched.
- `engine/internal/submitter/casper_test.go` — the 13 tests above.

## Verification (for the merge-agent)

```
cd engine
go build ./...
go vet ./...
go test ./internal/submitter/...   # 13 pass
go test ./...                      # all packages green
```

No Render / infra / contract changes. No new env vars. No dependency
bumps.
