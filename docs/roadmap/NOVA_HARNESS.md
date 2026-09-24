# Nova / folding-scheme aggregation — status & harness

**TL;DR.** The repo now ships an aggregation **harness** — a `Folder` interface and an HTTP surface (`POST /v1/aggregation/fold`, `POST /v1/aggregation/verify-fold`) whose contract matches what a real Nova/SuperNova/HyperNova implementation would expose. The default implementation is a hash-chain stand-in labelled `hash-fold-v1`. **It is not a cryptographic folding scheme.**

An intermediate cryptographic upgrade `pedersen-fold-v1` has since shipped alongside the hash stand-in — real BLS12-381 G1 Pedersen commitment sum, homomorphic across splits, but still not a Nova reduction of R1CS instances. See `docs/PEDERSEN_FOLD.md`. The `nova-go-v1` label remains reserved for the eventual real Nova.

This file explains exactly what is and is not real, and what a real implementation would need.

## What ships

- `internal/aggregator/nova.go` — `Folder` interface with `Fold(step)`, `Compress()`, `Verify(steps, aggregate)`. Default implementation `HashFolder`.
- `internal/aggregator/nova_test.go` — deterministic round-trip, tamper detection (per-step, per-root, reorder), empty-step rejection, unknown-scheme rejection.
- `internal/api/nova_handlers.go` + `nova_test.go` — HTTP surface + handler tests.
- Scoped as `aggregation:read` / `aggregation:write` (see `internal/api/scopes.go`).
- Response envelope carries an explicit `scheme` field and a `disclosure` string so downstream code cannot silently confuse this stand-in with a real folding proof.

## What the harness actually does

For each folded step:

```
step_hash_i     = SHA256( instance_i || witness_digest_i )
accumulator_{i+1} = SHA256( accumulator_i || step_hash_i )
```

with the initial accumulator seeded from `SHA256("hash-fold-v1")` so future schemes don't collide with this one on identical inputs. The aggregate carries the final accumulator (`root_hex`) plus every per-step hash so a verifier can re-play the chain from public inputs.

This is a genuine, deterministic hash chain. Given the public inputs (`steps`), it is verifiable — tampering with any step, reordering steps, or forging a root all get caught. It also **composes cleanly**: the same aggregation contract can host a real folding scheme without changing the HTTP shape.

## What the harness does NOT do

A real folding scheme (Nova, SuperNova, HyperNova, ProtoStar) reduces the satisfiability of `k` R1CS instances `U_1, …, U_k` into the satisfiability of ONE R1CS instance `U*` — such that satisfying `U*` implies (with overwhelming probability under a computational hardness assumption) satisfaction of every `U_i`. The harness in this repo does none of that. Specifically:

- **No relaxed R1CS accumulator.** A hash of two commitments is not a folded R1CS instance.
- **No zero-knowledge property.** The witness digests submitted to `Fold` must already be commitments the caller wants public; the harness gives no privacy over the underlying witnesses.
- **No soundness reduction to a hardness assumption.** The chain's soundness reduces to SHA-256 preimage resistance for the specific values being chained — but that is not the same statement as "the k underlying computations were all executed correctly."
- **No sublinear verification.** Real Nova verification of a k-step chain runs at a cost roughly independent of k (post-Compress + a final SNARK). Our verify replays every step.

## What a real implementation would need

To honestly claim "Nova aggregation" we would ship all of:

1. A curve cycle (Pallas/Vesta or a Ristretto-based alternative) implemented against `gnark-crypto` or an equivalent Go dependency.
2. A relaxed R1CS accumulator that tracks the error term across folds.
3. A Fiat-Shamir transform for the folding challenges, tied to a domain-separated transcript hash.
4. A final SNARK (Spartan-style or a Groth16 wrap) proving satisfiability of the accumulated relaxed R1CS.
5. Interoperability tests against a reference implementation (Microsoft/nova, SuperNova/Arecibo, HyperNova) or a formally-specified test vector set.

None of the five items ship in this commit. Item 1 (the curve cycle) is the current blocker in Go: `gnark-crypto` supports BN254/BLS12-381 but not the Pallas/Vesta cycle that most reference Nova code uses. The Go ecosystem lacks a maintained folding-scheme library as of the date of this commit; a real integration would either need to (a) build the cycle in-tree, (b) FFI-call a Rust reference (arecibo), or (c) wait for gnark to expose a supported cycle.

## Scheme labels

The response envelope carries `scheme` deliberately. Downstream code checking for a real Nova aggregate should match on a specific label — not on the presence of the `AggregateProof` type.

| Label            | Semantics                                                   | Ships? |
|------------------|-------------------------------------------------------------|--------|
| `hash-fold-v1`   | Hash-chain stand-in described above.                        | ✅ this commit |
| `nova-go-v1`     | Reserved for a future real Nova implementation.             | ❌ not implemented |

`VerifyAll` refuses to verify an aggregate labelled with an unimplemented scheme. This is intentional — sending back a `hash-fold-v1` aggregate under a `nova-go-v1` label must fail loudly.

## What the harness IS good for

- **Contract stabilisation.** SDKs and downstream callers can be built against the final HTTP shape today.
- **Batch bookkeeping.** The step hashes + root give a compact commitment to a sequence of proofs that the anchor contract (`proof-aggregation`) can already accept.
- **Regression fence.** Tampering with any step or the root breaks verification — enough to catch integrity bugs in the batching pipeline, distinct from batching correctness of the underlying proofs.

Anything beyond that requires the real scheme.
