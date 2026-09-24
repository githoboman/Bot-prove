# CasperProver Go SDK

> **Status:** `v0.1.0`. High-level `Prove` / `Verify` / `Batch` / `Anchor`
> primitives + typed responses. The lower-level route methods in
> `sdk/client.go` remain as the compatibility surface.

## Install

```sh
go get github.com/anna-stolbovskaja/CasperProver/sdk
```

## Quickstart

```go
import "github.com/anna-stolbovskaja/CasperProver/sdk"

c := sdk.NewClient(
    sdk.WithBaseURL("https://casperprover-api-ylsh.onrender.com"),
    sdk.WithAuthToken("pk_..."),
)

ctx := context.Background()
proof, err := c.Prove(ctx, sdk.ProveRequest{
    Agent: "a", Model: "gpt-toy-v1",
    Input: "hello", Output: "42",
}, sdk.WithIdempotencyKey("run-1"))
if err != nil { log.Fatal(err) }

check, _ := c.Verify(ctx, proof.ID)
fmt.Println(check.Valid)
```

Every write primitive accepts `sdk.WithIdempotencyKey(key)` — safe retries
against the server-side dedup cache (24h TTL).

Legacy unversioned routes:

```go
sdk.NewClient(sdk.WithAPIVersion(sdk.APIVersionUnversioned))
```

## Receipt validator

`sdk.VerifyReceiptBytes(payload)` re-derives `input_hash`, `output_hash`,
and `model_hash` locally (SHA-256 of the UTF-8 plaintext) and returns
`*sdk.ReceiptValidationError` on any mismatch. Bit-identical output to the
Python and TypeScript implementations.

## Test

```sh
cd sdk && go test -race -count=1 ./...
```
