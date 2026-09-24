# Pedersen Fold — Intermediate Cryptographic Upgrade

**Status:** SHIPPED as `pedersen-fold-v1` alongside the existing
`hash-fold-v1`. Pack AS, backlog #2.11.

Purpose: give callers a real cryptographic accumulator on top of an
elliptic curve today, while the ecosystem's Go Nova (Pallas/Vesta
cycle, `arecibo`) matures. **This is NOT a Nova folding scheme.** It
does not reduce k R1CS instances into one R1CS instance whose
satisfiability implies satisfiability of the originals. Consumers
that need that property must look for a witness carrying a real Nova
label — `nova-go-v1` — which is reserved but not implemented.

## Construction

Two independent generators `G, H` in BLS12-381 G1 are derived at
package init:

- `G` is the canonical G1 generator (`bls.Generators()`).
- `H` is `HashToG1("CP_PED_H_V1_generator", DST="CP_PED_H_V1")` — an
  SSWU + isogeny map (RFC 9380). Because `H` comes from hashing an
  arbitrary tag to the curve, no party knows the discrete log of `H`
  with respect to `G`. This is critical: knowing `dlog_G(H) = k`
  breaks binding of the whole commitment scheme.

For each step `i`, two scalars are derived deterministically from
the caller's opaque bytes with disjoint domain-separation tags:

```
m_i = SHA256("CP_PED_M_V1" || 0x1e || instance_i)   mod r
r_i = SHA256("CP_PED_R_V1" || 0x1e || witness_i)    mod r
```

`r` is the order of the G1 subgroup (`fr.Element`). The reduction
bias from a 256-bit hash into a ~381-bit field is negligible.

The accumulator is:

```
C = Σ_i ( m_i · G + r_i · H )
```

`C` is emitted as the compressed 48-byte G1 encoding, hex-serialised
(96 chars).

## Security properties (what IS real here)

- **Binding** under the Discrete Logarithm assumption on BLS12-381
  G1. Given `C`, finding a *different* `(m'_i, r'_i)` such that
  `Σ (m'_i·G + r'_i·H) = C` requires solving DLP.
- **Hiding** under DLP given `r` is drawn from a distribution
  independent of `m`. In our construction `r_i` is derived from a
  separate DST hash of the witness digest — so if the caller keeps
  the witness digest private, hiding holds; if the caller publishes
  both `instance_i` and `witness_digest_i`, hiding does not hold
  (the commitment is fully re-derivable). This is documented on the
  wire as the `witness_digest` field name — callers who need hiding
  MUST pass a value the verifier does not know.
- **Homomorphism:** for any split `1 ≤ k < n`,
  `Commit(steps[:k]) + Commit(steps[k:]) = Commit(steps)` as curve
  points. Enforced by `PedersenHomomorphismCheck`. This is the
  practical advantage over `hash-fold-v1`: an aggregator can fold
  partial batches independently and add the results.
- **Determinism:** repeated fold of the same sequence produces
  bit-identical `Root`, verified by `TestPedersenDeterminism`.

## What is NOT here

- **Not a Nova/SuperNova/HyperNova/ProtoStar folding scheme.** No
  R1CS reduction, no arithmetic circuit, no cryptographic proof
  that the aggregate's satisfiability implies satisfiability of the
  originals. If a downstream consumer requires that property, they
  must wait for the `nova-go-v1` label (reserved in `nova.go`) which
  will ship when a Go Pallas/Vesta implementation is production-
  ready.
- **Not a KZG polynomial commitment.** The scheme commits to a
  fixed-arity `(m_i, r_i)` per step, not to a polynomial's
  coefficients. Batch openings, KZG-style, are out of scope.
- **Not a general-purpose commitment.** The scalars are derived from
  hash-of-input; the caller cannot commit to a value it does not
  publish elsewhere (there is no separate randomness source). If
  the caller needs true hiding, we recommend they pass a
  `witness_digest` that is itself a commitment to a private value
  they hold — that value's binding is out of scope for this
  package.

## Wire format

The scheme upgrades `AggregateProof` in-place:

```json
{
  "scheme": "pedersen-fold-v1",
  "steps": 3,
  "root_hex": "b1a…e7f",           // 96 hex chars (compressed G1)
  "step_hashes_hex": ["…", "…", "…"]
}
```

The `step_hashes_hex` array is unchanged from `hash-fold-v1` —
per-step SHA-256 tags for structural check on verify. The
cryptographic check happens on `root_hex`.

## HTTP surface

Existing endpoints:

- `POST /v1/aggregation/fold` — pass `"scheme": "pedersen-fold-v1"`
  in the request body to select this scheme; omitting the field
  falls back to `hash-fold-v1` (backwards-compatible).
- `POST /v1/aggregation/verify-fold` — dispatches on the `scheme`
  field inside the supplied aggregate.

## Comparison to `hash-fold-v1`

|                                | hash-fold-v1  | pedersen-fold-v1        |
| ------------------------------ | ------------- | ----------------------- |
| Determinism                    | ✓             | ✓                       |
| Verify by re-play              | ✓             | ✓                       |
| Homomorphic across splits      | ✗             | ✓                       |
| Real cryptographic hardness    | pre-image     | DLP on BLS12-381 G1     |
| Real Nova folding              | ✗             | ✗ (labelled honestly)   |

## Testing

- `internal/aggregator/pedersen_fold_test.go` — 8 tests: empty
  refusal, happy-path fold+verify, tamper detection, scheme
  mismatch, step-count mismatch, determinism, homomorphism across 6
  splits, distinct root vs hash-fold, and `Folder` interface
  satisfaction.
- `internal/api/pedersen_fold_test.go` — 3 handler tests:
  HTTP round-trip fold+verify, unknown-scheme 400, default fallback
  to hash-fold when scheme omitted.

## Migration path to real Nova

1. Implement `NovaFolder` matching the `Folder` interface using a
   Go-native Pallas/Vesta cycle (blocked on ecosystem maturity).
2. Emit scheme label `nova-go-v1`.
3. Existing `pedersen-fold-v1` aggregates remain verifiable — the
   scheme label is embedded in every witness, so a mixed-scheme
   consumer routes on the label.
