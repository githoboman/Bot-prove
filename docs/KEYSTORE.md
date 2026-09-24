# PQ Keystore — backends, threat model, gateway contract

> **Scope note:** this document describes the KEYSTORE ABSTRACTION added in
> commit I on top of the KeyRing added in commit H (see `docs/PQ_KEYRING.md`
> and `docs/roadmap/KEY_MANAGEMENT.md` for the rotation/versioning contract
> that sits underneath).

The engine used to hold every PQ signing key in process memory. That was
acceptable for local dev and integration tests but unfit for production:
a process restart wiped the ring; a compromised engine memory disclosed the
key. This commit introduces a small `Keystore` interface (see
`internal/crypto/keystore/keystore.go`) with three reference backends.

## Backends at a glance

| backend | selector                       | private keys live in… | survives restart | HSM-backed |
|---------|--------------------------------|-----------------------|------------------|------------|
| memory  | `CP_KEYSTORE_KIND=memory` (default) | process memory  | ❌               | ❌         |
| file    | `CP_KEYSTORE_KIND=file`        | ChaCha20-Poly1305 file, KDF Argon2id | ✅       | ❌         |
| remote  | `CP_KEYSTORE_KIND=remote` (stub) | HSM/KMS gateway (out of tree) | ✅          | ✅ (when wired) |
| vault-transit | `CP_KEYSTORE_KIND=vault-transit` | HashiCorp Vault Transit engine (private key never leaves Vault) | ✅ | ✅ (Ed25519 only) |

**Every write endpoint** on the engine surface (`/v1/pq/keys` and
`/v1/pq/keys/…/rotate`, `/sign`, `/migrate`) is gated by
`CP_KEYRING_ENABLE=1` on top of the keystore backend selection. That gate
exists so a fresh deployment doesn't accidentally sign anything with a
memory-only keystore.

## Environment variables

| env var                  | who reads it                                   | meaning |
|--------------------------|------------------------------------------------|---------|
| `CP_KEYRING_ENABLE=1`    | `keyring_handlers.go` — gate on every endpoint | Opt-in switch. Off → 503 on write, and 503 on read too when `CP_STRICT=1`. |
| `CP_KEYSTORE_KIND`       | `keystore.FromEnv`                             | `memory` (default), `file`, or `remote`. |
| `CP_KEYSTORE_PATH`       | file backend                                   | Filesystem path for the encrypted ring. Parent dirs created with 0700. |
| `CP_KEYSTORE_PASSPHRASE` | file backend                                   | Passphrase used with Argon2id to derive the wrap key. **Wire from a secret manager**, not a config file. |
| `CP_KEYSTORE_URL`        | remote stub                                    | HTTPS base URL of the HSM/KMS gateway. `""` leaves the stub inert. |
| `CP_KEYSTORE_TOKEN`      | remote stub                                    | Bearer token forwarded on every gateway request. `""` leaves the stub inert. |
| `CP_STRICT=1`            | keyring gate                                   | Fail-closed on reads too when the gate is off. |

## FileKeystore — encryption at rest

`FileKeystore` persists the full ring (public metadata + private key
material) to a single file with the following layout:

```
+--------+---------+-----------+------------------+--------------------+
| "CPFK" | ver(u16)| salt(16B) | nonce(12B, XCC20)| ciphertext + tag   |
+--------+---------+-----------+------------------+--------------------+
```

* **Cipher:** ChaCha20-Poly1305 (`golang.org/x/crypto/chacha20poly1305`).
  Authenticated additional data = the literal magic `CPFK` so a
  version-header tamper is detected on decrypt.
* **KDF:** Argon2id (`golang.org/x/crypto/argon2`) with parameters
  `time=3`, `memory=64 MiB`, `threads=4`, `keyLen=32`. Pinned in file
  format version 1; a future v2 can raise them.
* **Persistence:** every state-changing call (`CreateKey`, `RotateKey`,
  `MigrateSignature`) serializes the ring, encrypts, and writes to
  `{path}.tmp` then atomic-renames onto `{path}`.
* **Rewrap:** `FileKeystore.Rewrap(newPass)` re-encrypts the ring under a
  new passphrase. Old ciphertext is atomically overwritten.

**What FileKeystore protects against:**

* Process restarts wiping the ring — the file persists.
* Passive theft of the file (disk backups, snapshots, image exfiltration) —
  ciphertext is useless without the passphrase.
* Casual tampering — the AEAD tag catches every non-trivial bit flip.

**What FileKeystore does NOT protect against:**

* A compromised running engine process — to sign, the ring is unwrapped
  into engine memory, and from that moment the threat model equals
  `MemoryKeystore`.
* Passphrase leakage. If the passphrase lands in a git repo or a shell
  history file, everything the ciphertext contains is compromised.
* Side-channel attacks on the engine host. That's what an HSM/KMS is for.

FileKeystore is a **stepping stone**, not a substitute for a real HSM.

## RemoteKeystoreStub — HSM/KMS gateway contract

The stub is intentionally minimal — it documents an HTTP contract without
shipping a driver for any specific HSM/KMS. The engine talks to a
per-deployment gateway; the gateway talks to the hardware.

### Wire format

* Transport: HTTPS.
* Auth: `Authorization: Bearer <token>` on every request.
* Content-Type: `application/json` in and out.
* Errors: non-2xx status with body `{"error":"..."}` — the engine surfaces
  the body verbatim.

### Endpoints the gateway MUST implement

| method | path                       | request                                                                 | response |
|--------|----------------------------|-------------------------------------------------------------------------|----------|
| POST   | `/keys`                    | `{"algo":"ed25519\|mldsa65\|lamport\|hybrid_ed25519_mldsa65"}`          | `KeyMeta` |
| POST   | `/keys/{id}/rotate`        | (empty)                                                                 | `KeyMeta` (new active for that algo) |
| GET    | `/keys`                    | (empty)                                                                 | `[]KeyMeta` |
| GET    | `/keys/{id}`               | (empty)                                                                 | `KeyMeta` |
| POST   | `/keys/{id}/sign`          | `{"message_hex":"..."}`                                                 | `{"signature_hex":"..."}` |
| POST   | `/keys/{id}/verify`        | `{"message_hex","signature_hex"}`                                       | `{"valid":true\|false}` |
| POST   | `/keys/migrate`            | `{"old_key_id","message_hex","old_signature_hex","to_algo"}`            | `{"signature_hex","new_key_id"}` |

`KeyMeta` matches `internal/crypto.KeyMeta` byte-for-byte:

```json
{
  "id": "hybrid_ed25519_mldsa65:v3:a1b2c3d4",
  "algo": "hybrid_ed25519_mldsa65",
  "version": 3,
  "public_key_hex": "…",
  "created_at": "2026-07-26T14:30:00Z",
  "retired_at": null,
  "active": true
}
```

### What the stub does with the response

* `CreateKey`: mirrors `KeyMeta` into an in-memory verify-only ring so
  `Verify` can short-circuit and never round-trips the gateway.
* `Sign`: two-step — resolve active key ID locally (from the mirrored
  ring), then `POST /keys/{id}/sign`. A production driver may fuse this
  into a single `POST /algos/{algo}/sign` call; the harness stays
  explicit.
* `Verify`: local-only, from the mirrored ring. This is safe because
  Verify uses only public material.

### Building a real driver

The `RemoteKeystoreStub` type is not the driver; it's the **client** the
driver is expected to talk to. Two things sit outside this repo:

1. **The gateway process.** A small HTTP server that terminates the JSON
   contract above and translates each call into a hardware operation.
   Reference implementations most teams reach for:
   * AWS KMS (asymmetric CMK, `Sign` API, key policies pinned per algo).
   * Google Cloud KMS (asymmetric EC/PQ where available; fall back to
     an HSM behind the KMS façade).
   * HashiCorp Vault Transit (`sign` and `verify` primitives).
   * YubiHSM 2 or Nitrokey HSM 2 via PKCS#11.
2. **A hardware-appropriate KDF/key-storage strategy** for Lamport and
   the ML-DSA half of hybrid — most COTS HSMs support Ed25519 today,
   ML-DSA and Lamport support is nascent. When the HSM cannot hold the
   PQ half, the driver typically keeps a sealed-file fallback for the
   PQ half plus a hardware-backed Ed25519 half, and honestly reports
   `hardware_backed=false` in `Info` for those keys. Never overstate.

### Failure modes the stub already handles

* Unconfigured base URL or token → every non-`Info` call returns
  `ErrNotSupported`. Silent no-op is impossible.
* Non-2xx gateway response → surfaced as a Go error with the response
  body attached, no swallowing.
* Malformed hex in the gateway's response → surfaced as an error.

### Failure modes a real driver still needs

* Retries with jittered backoff for `429` and `503`.
* Circuit breaker so a dead HSM doesn't stall the engine's signing
  queue indefinitely.
* Rate-limit metrics + Prometheus counters for gateway calls (present
  on the engine side but not populated until a driver runs).
* An audit trail on the gateway. The engine records `key_id` on every
  sign; the gateway should record `caller` + `key_id` + timestamp so a
  break-glass audit doesn't rely on engine logs alone.

## Threat model summary

| threat                                          | memory | file | remote (real HSM) |
|-------------------------------------------------|:------:|:----:|:-----------------:|
| process restart wipes the ring                  |   ⚠️   |  ✅  |         ✅        |
| passive disk theft / backup exfiltration        |   n/a  |  ✅  |         ✅        |
| passphrase-in-git                               |   n/a  |  ⚠️  |         ✅        |
| compromised running engine (memory read)        |   ⚠️   |  ⚠️  |         ✅        |
| passive network eavesdropping (HTTPS assumed)   |   n/a  |  n/a |         ✅        |
| stolen HSM API token (short-lived, then rotate) |   n/a  |  n/a |         ✅        |

✅ = mitigated ⚠️ = not mitigated — plan for defence-in-depth.

## Migration path from Memory → File → Remote

1. Deploy with `CP_KEYSTORE_KIND=memory` while onboarding
   (`CP_KEYRING_ENABLE=0` in prod).
2. Once you need signatures to survive a restart, switch to
   `CP_KEYSTORE_KIND=file` and wire `CP_KEYSTORE_PATH` +
   `CP_KEYSTORE_PASSPHRASE` from a secret manager. Rewrap on cadence.
3. When you have an HSM/KMS gateway, switch to
   `CP_KEYSTORE_KIND=remote`. Backfill by importing the file
   keystore's private keys into the HSM using the gateway's admin
   surface (out of scope for the engine — the gateway owns key import).
4. Delete the file keystore once every key is anchored via the HSM.

## Out of scope for this commit

* An actual HSM/KMS driver — see "Building a real driver" above.
* A sealed-enclave backend (SGX / Confidential Compute). The Keystore
  interface accommodates it; the enclave loader itself is a separate
  effort.
* KMIP support. If your ops team requires KMIP, wire it in the gateway
  process — the engine deliberately stays HTTP-only.
* Distributed threshold signing over the HSM (t-of-n co-signers). See
  `docs/roadmap/KEY_MANAGEMENT.md` for the co-signing plan.
