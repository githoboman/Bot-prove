# Vault Transit keystore driver

The `vault-transit` backend is a real driver on top of HashiCorp Vault's
[Transit secret engine](https://developer.hashicorp.com/vault/api-docs/secret/transit).
Private key material is generated and stored INSIDE Vault; the engine
never sees it in plaintext. All signing operations are delegated to
Vault under a named key.

Package: [`engine/internal/crypto/keystore`](../engine/internal/crypto/keystore/vault_transit.go).

## Scope

**Supported algorithms.** Vault Transit natively supports Ed25519, so
that is the algorithm this driver serves. Post-quantum algorithms
(Dilithium / SPHINCS+ / Lamport) are NOT available in upstream Vault —
requesting them via `CreateKey(AlgoMLDSA65)` returns `ErrAlgoNotSupported`
so we never silently swap the caller's requested primitive.

Hybrid workflows that need a Vault-backed classical half plus a
software-backed PQ half live one layer up (compose two `Keystore`
instances). This driver stays narrow: real Vault Transit for real
Ed25519.

**Wire protocol.**

| Vault path (relative to mount) | Purpose |
|---|---|
| `POST /keys/{name}` | Create a Transit key |
| `GET /keys/{name}` | Read public key + latest version |
| `POST /sign/{name}` | Sign a message (private key never leaves Vault) |
| `POST /verify/{name}` | Verify a signature |

Auth: static token via the standard `X-Vault-Token` header. In
production this token comes from a short-lived auth backend (K8s
service-account, AWS IAM, OIDC) — the driver only sees whatever token
the operator gives it.

## Configuration

Three environment variables, following the same convention as the
other keystore backends:

| env var | required | meaning |
|---|---|---|
| `CP_KEYSTORE_KIND=vault-transit` | ✔ | Selects this backend |
| `CP_VAULT_ADDR` | ✔ | e.g. `https://vault.internal:8200` |
| `CP_VAULT_TOKEN` | ✔ | Vault auth token (see below) |
| `CP_VAULT_TRANSIT_MOUNT` |  | Transit engine mount path (default `transit`) |

Bootstrap example:

```bash
# One-time (Vault operator, not the engine):
vault secrets enable transit
vault policy write cp-signer - <<POLICY
path "transit/keys/cp-*"           { capabilities = ["create","read","update"] }
path "transit/sign/cp-*"           { capabilities = ["update"] }
path "transit/verify/cp-*"         { capabilities = ["update"] }
POLICY

# Then, for the engine process:
export CP_KEYSTORE_KIND=vault-transit
export CP_VAULT_ADDR=https://vault.internal:8200
export CP_VAULT_TOKEN="$(vault write -field=token auth/kubernetes/login role=cp-signer jwt=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token))"
export CP_KEYRING_ENABLE=1
./casperprover
```

## What happens when the engine calls Create/Sign/Verify

- **CreateKey** → `POST /keys/cp-ed25519-<nanos>` on Vault. Vault
  generates a fresh Ed25519 pair; the engine reads the public half via
  `GET /keys/…`, mints a short opaque ID (`vt-<hex8>` from the SHA-256
  of `algo || public_key`), and caches (id → Vault name, version).
  Rotating (`RotateKey`) creates a *new* Vault key rather than
  incrementing the existing key's version — this keeps the (engine
  ID) → (Vault name) mapping 1:1, which simplifies Verify (no version
  lookup needed).
- **Sign** → `POST /sign/cp-ed25519-…` with `input=<base64(message)>`.
  Vault returns `vault:v<N>:<base64(signature)>`. The driver strips the
  `vault:vN:` wrapper and returns raw Ed25519 signature bytes to the
  caller.
- **Verify** → `POST /verify/cp-ed25519-…` with the caller's raw
  signature re-wrapped into `vault:v<N>:…` for Vault. Verification is
  delegated to Vault (not done locally) because Vault owns the
  version-aware key state — clients that pin an old version would
  otherwise silently fail.
- **MigrateSignature** verifies the old signature via Vault, then signs
  again under the current active key.

## Threat model

**Confidentiality of the private key** is now delegated to Vault.
Compromising the engine process no longer leaks the private key
material — the attacker gets a Vault token instead. This is a real
improvement: a Vault token is scoped (Vault policy), auditable (Vault
audit log), and can be rotated at any time; a leaked in-memory
Ed25519 key cannot be un-leaked.

What the engine STILL has to protect:

- **The Vault token.** A leaked token grants sign-under-any-cp-key
  until revoked. Bind it to a short-lived auth method (K8s SA, AWS IAM)
  and give the policy the narrowest paths that work.
- **The signing surface itself.** The engine still exposes `POST /pq/…`
  endpoints — anyone who can call those can sign under Vault. The
  keystore does not gate authorization; upstream RBAC and the
  `CP_KEYRING_ENABLE` env-flag do.

## Testing

Because Vault Transit's wire protocol is documented, we can exercise
this driver against an in-process mock. See
[`vault_transit_test.go`](../engine/internal/crypto/keystore/vault_transit_test.go)
— an httptest server implements the Transit contract with **real**
Ed25519 crypto (no shortcuts) and every driver method is round-tripped
through the mock. 8 tests cover:

- unconfigured driver returns `ErrNotConfigured`
- create → sign → verify round-trip
- ML-DSA / other PQ algorithms return `ErrAlgoNotSupported`
- `RotateKey` mints a new key and retires the previous one
- `MigrateSignature` verifies old, signs new, verifies new
- migrate refuses a forged old signature
- signature wrapper `vault:vN:<b64>` parse edge cases

## Roadmap items closed

- Backlog **2.10 (partial)** — remote HSM/KMS driver. Vault Transit is
  now the reference wiring; the `remote` stub remains as a
  documentation-only harness for bespoke HSM gateways (YubiHSM,
  Nitrokey, AWS KMS Custom Key Store) that do NOT speak the Vault
  Transit protocol.
- Backlog **11.3 (partial)** — real HSM driver on top of the keystore
  stub. Vault Transit is one of the two acceptable choices (the other,
  AWS KMS, remains a future driver).

## Not yet

- **AWS KMS driver.** Same story, different wire protocol (SigV4 +
  AWS SDK). Deliberately deferred until we need it; Vault Transit
  covers on-prem + K8s deployments today.
- **Vault Transit key rotation in place.** We prefer mint-new-key over
  rotating an existing Vault key so the engine-side ID map stays 1:1.
  If a caller needs the Vault-side version counter for compliance,
  they can drive it out-of-band; the driver just sees new keys.
- **Native OIDC/K8s auth inside the driver.** Today the caller wires
  `CP_VAULT_TOKEN` from whatever auth flow they run. Direct auth-inside
  is a future add-on that would live behind a `CP_VAULT_AUTH_METHOD`
  switch.
