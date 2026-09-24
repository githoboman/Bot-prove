# PQ Signature Key Rotation & Versioning

**Status:** Real. Demo-grade key storage (in-memory only). API endpoints are gated by `CP_KEYRING_ENABLE=1`.

## What this is

`internal/crypto/keyring.go` adds a rotation + versioning layer on top of the four signature primitives already in this repo:

| Algo slug                     | Real primitive                                   | Notes |
|-------------------------------|--------------------------------------------------|-------|
| `ed25519`                     | Go stdlib `crypto/ed25519`                       | Classical, NOT post-quantum. |
| `mldsa65`                     | `github.com/cloudflare/circl/sign/mldsa/mldsa65` | Real ML-DSA-65 (FIPS 204, NIST security category 3). |
| `lamport`                     | own implementation, see `pq.go`                  | Real Lamport one-time signature (Lamport 1979). **Single-use per keypair.** Not SPHINCS+. |
| `hybrid_ed25519_mldsa65`      | `HybridSign`                                     | Ed25519 || ML-DSA-65 side-by-side. Verifier requires BOTH components to verify. |

The keyring adds:

- Monotonic **versions** per algo (`v1`, `v2`, …).
- Exactly **one active key** per algo at a time. Rotation retires the previous active key atomically.
- **Retired keys keep verifying.** The whole point of the ring is that signatures anchored under old keys survive rotation.
- Stable **key IDs**: `<algo>:v<version>:<sha256(pub)[:8]>` — safe to embed in anchored proofs and audit logs.
- A **migration primitive**: `MigrateSignature(oldKeyID, msg, oldSig, toAlgo)` — verifies the old signature, then re-signs the same message under the currently-active key of `toAlgo`. This is the "upgrade path" for anchored artefacts.
- **Public snapshot / verify-only reconstruction**: `MarshalPublic()` serializes only metadata + public keys; `LoadPublicKeyRing()` builds a ring that can verify but NOT sign.

## API surface

All under scope `pq:read` / `pq:write` (see `internal/api/scopes.go`).

```
POST /v1/pq/keys                        pq:write   create a key for {algo}
POST /v1/pq/keys/{algo}/rotate          pq:write   rotate active key for {algo}
GET  /v1/pq/keys[?algo=…]               pq:read    list all keys (optionally filtered)
GET  /v1/pq/keys/{id}                   pq:read    fetch one key's public metadata
POST /v1/pq/keys/sign                   pq:write   sign under {algo} or a specific {key_id}
POST /v1/pq/keys/verify                 pq:read    verify (key_id, message, signature)
POST /v1/pq/keys/migrate                pq:write   verify old sig then re-sign under to_algo
```

`ed25519`, `mldsa65`, `lamport`, `hybrid_ed25519_mldsa65` are the four accepted `algo` values.

Every write-side endpoint is gated by `CP_KEYRING_ENABLE=1`. When the gate is off:

- Write endpoints return HTTP 503 with an explanatory error.
- Read endpoints return normally (in non-strict mode) or 503 (in `CP_STRICT=1`).

## Threat model & honesty

**In-memory only.** Private keys live in the process heap. There is **no on-disk persistence layer**. A process restart destroys every private key. Only public metadata can be exported/imported via `MarshalPublic` / `LoadPublicKeyRing`.

**Why the gate:** the endpoints exist so downstream code, tests, and demo notebooks can exercise the rotation contract end-to-end. They are not safe for production signing of real assets. A production deployment MUST:

1. Wire an HSM, KMS, or sealed-enclave signer that implements the same `Sign` / `Verify` contract, without ever surfacing private-key material to Go heap memory.
2. Move the key lifecycle events (create / rotate / retire) behind the m-of-n approval workflow described in `docs/roadmap/KEY_MANAGEMENT.md`.
3. Ship the append-only audit log to cold storage.

None of these three items ship in this commit.

**Non-goals of this commit:**

- Persistence of the private-key store.
- HSM/KMS integration.
- Multi-party or threshold signing.
- Automatic rotation cadence.
- Approver workflow.

**Kept honest by tests.** `internal/crypto/keyring_test.go` covers:

- Round-trip sign/verify per algo (including hybrid).
- Rotation retires the previous key and verifies old signatures still validate.
- Migration refuses to re-sign a message whose old signature does not verify.
- Marshalled public snapshot round-trips: the loaded ring verifies signatures produced by the original ring and refuses to sign under any key.
- Monotonic versioning under a frozen clock.
- Hybrid tamper detection.

`internal/api/keyring_test.go` covers the HTTP surface, including the disabled gate returning 503 and unknown algo returning 400.

## Migration playbook (worked example)

You have anchored proofs signed with `ed25519:v1:…`. You want to migrate to `hybrid_ed25519_mldsa65:v1:…` without regenerating the underlying proofs.

1. `POST /v1/pq/keys {"algo":"hybrid_ed25519_mldsa65"}` — the ring now has both an ed25519 key and a hybrid key.
2. For each anchored proof:
   - `POST /v1/pq/keys/migrate` with the old key ID, the original message, the old signature, and `to_algo=hybrid_ed25519_mldsa65`.
   - Store the returned `{new_signature, new_key_id}` next to the proof.
3. Once every artefact has been migrated, `POST /v1/pq/keys/ed25519/rotate` to force any future signing operations off the retired ed25519 v1. The old ed25519 key remains available for verification of anything not yet migrated.

The migration path deliberately does NOT allow re-signing a message whose original signature does not verify — this prevents accidentally laundering a bad old signature into a good new one.
