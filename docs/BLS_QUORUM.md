# BLS12-381 Threshold Quorum — Honest Contract

**Status:** SHIPPED, off-chain layer only. Pack AS, backlog #2.15 / #2.16 / #2.17.

This document describes what the shipped `engine/internal/quorum` package
actually does, what it does *not* do, and how a caller should read
`docs/roadmap/BLS_QUORUM.md` (the design) against this file (reality).

The roadmap doc is the vision. This file is the truth on-disk.

## What is real cryptography here

- **Real BLS12-381 signatures.** Keys are drawn from `Fr` uniformly at
  random using `crypto/rand`; the public key lives in `G2`
  (`pk = sk·G2`); the signature is `H(m)·sk` in `G1`. The hash-to-curve
  routine is gnark-crypto's `HashToG1` (SSWU + isogeny, RFC 9380
  domain-separated tag `"CP_BLS_SIG_V1"`).
- **Real pairing check.** `Verify` does
  `e(H(m), pk) == e(sig, G2_generator)` via `bn254`-style
  `PairingCheck` over the Miller loop, not a hash placeholder.
- **Real aggregation.** `Aggregate` sums signatures in `G1` (Jacobian
  coordinates for constant-time-safe addition); `AggregatePubKeys`
  sums pubkeys in `G2`. Verifying the aggregate signature against the
  sum of pubkeys is equivalent to verifying each individual
  signature *when all signers signed the same message* — which is
  the mode this package supports and enforces.
- **Registry with slashing.** `Registry` is a thread-safe in-memory
  map with `active → slashed` (terminal) or `active → removed`
  transitions, `Slash` is idempotent, and `activePubKeys` refuses any
  id that is unknown / non-active / decode-failed.
- **Byzantine threshold arithmetic.** `ByzantineThreshold(n)` returns
  `⌊2n/3⌋ + 1` clamped to `[1, n]`. Table: `4→3`, `5→4`, `7→5`,
  `10→7`, `100→67`. Tiny committees (`n ≤ 3`) clamp to `n` — no BFT
  guarantee possible.
- **Canonical witness.** `VerifyQuorum` returns a `QuorumWitness`
  carrying `scheme` (label), `evidence_root_hex`, sorted
  `signer_bitset`, `aggregate_pubkey_hex`, `aggregate_sig_hex`,
  `threshold`, `active_signers`, `verdict`, and
  `witness_hash_hex` — a SHA-256 commitment over a deterministic
  serialisation of every other field. Order of the bitset in the
  request does not affect the commitment (canonical sort applied
  before hashing).

## What is *not* real cryptography here

- **No threshold key generation.** There is no DKG. Each signer keeps
  their own individual secret key. This is BLS *aggregation*, not
  BLS-TSS with Shamir + Lagrange interpolation. The reserved
  scheme label `bls12-381-tss-v1` exists but the code path is
  not implemented; a witness must never claim this label until DKG
  ships.
- **No rogue-key attack defence.** The naïve aggregation `pk_agg =
  Σ pk_i` is vulnerable to rogue-key attacks when the pool admits
  untrusted pubkeys. Our registry gates this with a
  proof-of-possession requirement at registration (delegated to the
  operator today; enforced by the on-chain contract in the deploy
  path). Off-chain we document this as a **required operator
  invariant**: the registration flow must verify the signer holds
  the private key before accepting the pubkey. `docs/roadmap/BLS_QUORUM.md`
  §"Proof of possession" describes the challenge protocol.
- **No on-chain verifier — yet.** The pairing check runs in the Go
  engine. Casper WASM contracts do not have BLS pairing today. The
  on-chain path in `docs/roadmap/BLS_QUORUM.md` §"On-chain surface"
  is the design target; the current wire is: engine verifies →
  engine emits `witness_hash_hex` → on-chain contract stores the
  hash. This is **the same trust model receipts already use**, not
  a step down.
- **Registry persistence is in-memory.** A Postgres driver of the
  same shape as `receipts.Store` is a follow-up (documented in
  §"Persistence" of the roadmap doc). Restarting the engine wipes
  the committee; operators must re-register.

## Threat model — what each error catches

| Attack                       | Detected by            | Error                    |
| ---------------------------- | ---------------------- | ------------------------ |
| Signature over wrong message | pairing check          | `ErrPairingCheckFail`    |
| Truncated aggregate          | pairing check          | `ErrPairingCheckFail`    |
| Slashed signer participates  | registry lookup        | `ErrInactiveSigner`      |
| Unknown signer id in bitset  | registry lookup        | `ErrUnknownSigner`       |
| Duplicate signer in bitset   | registry lookup        | `ErrDuplicateSigner`     |
| Below-threshold bitset       | pre-pairing gate       | `ErrThresholdNotMet`     |
| Malformed signature bytes    | `UnmarshalSignature`   | `ErrInvalidSignature`    |
| Malformed pubkey bytes       | `UnmarshalPubKey`      | `ErrInvalidPubKey`       |
| Empty message                | pre-pairing gate       | `ErrEmptyMessage`        |
| Empty bitset                 | pre-pairing gate       | `ErrEmptyBitset`         |

Each is a sentinel — callers can `errors.Is` them without parsing
prose.

## Env matrix

| Variable            | Default | Effect                              |
| ------------------- | ------- | ----------------------------------- |
| `CP_QUORUM_ENABLE`  | unset   | `1` enables `/v1/quorum/*` endpoints; unset → 503 |

## API surface

All routes require the quorum service to be enabled. Scoped under
`quorum:read` / `quorum:write` for `CP_SCOPES_FILE`-driven
authentication.

| Method | Path                                     | Scope         |
| ------ | ---------------------------------------- | ------------- |
| POST   | `/v1/quorum/signers`                     | quorum:write  |
| GET    | `/v1/quorum/signers`                     | quorum:read   |
| POST   | `/v1/quorum/signers/{id}/slash`          | quorum:write  |
| POST   | `/v1/quorum/signers/{id}/retire`         | quorum:write  |
| POST   | `/v1/quorum/verify`                      | quorum:read   |
| GET    | `/v1/quorum/threshold`                   | quorum:read   |

`/v1/quorum/verify` maps domain errors to HTTP status:

- pairing failed → 422 Unprocessable Entity
- unknown signer → 404 Not Found
- inactive / duplicate / below-threshold → 403 Forbidden
- malformed input → 400 Bad Request

## Testing

- `engine/internal/quorum/bls_test.go` — 6 tests: generate/sign/verify,
  aggregation, aggregation-under-different-messages break case,
  marshal roundtrips, garbage-rejection, threshold arithmetic table.
- `engine/internal/quorum/verifier_test.go` — 8 tests: registry
  duplicate-refusal, slash idempotency, list ordering, quorum happy
  path, below-threshold, tampered signature, slashed signer refused,
  unknown signer refused, duplicate bitset refused, bitset ordering
  invariant.
- `engine/internal/api/quorum_handlers_test.go` — 9 handler tests:
  503-disabled, register happy path, duplicate 409, list, verify happy
  path, tamper 422, below-threshold 403, threshold endpoint, slash.

Total: 23 new tests, all pass under `-race -count=1`.

## Out of scope (documented for the record)

The following are consciously deferred; each has a docs anchor.

- Postgres-backed `Registry` driver — same interface pattern as
  `receipts.Store`. §"Persistence" in roadmap doc.
- DKG-based BLS-TSS (`bls12-381-tss-v1` reserved label). Requires an
  interactive protocol between signers; a real implementation belongs
  behind the `ceremony` subsystem in `docs/roadmap/CEREMONY.md`.
- On-chain BLS pairing verifier. Blocked on Casper VM adding BLS12-381
  precompiles. Currently the on-chain contract commits
  `witness_hash_hex`.
- Rogue-key attack proof-of-possession *challenge protocol* (as
  opposed to the operator-invariant we document today). §"Proof of
  possession" in roadmap doc.
- Streaming aggregation for large committees (Bellman-style
  optimisations). Not required at expected committee sizes (< 30).
