# SIWE-like Challenge Authentication

**Status**: `REAL`. Ed25519 challenge-response authentication.
Complements the `X-API-Key` middleware. See
`engine/internal/api/siwe/` (primitive) and
`engine/internal/api/siwe_handlers.go` (HTTP surface).

Cross-refs:
- `docs/HASH_ALGORITHM_ANALYSIS.md` (AN) — canonical-message domain
  separation is the same primitive family used elsewhere in the tree.
- `docs/METADATA_PRIVACY.md` (AO) — the challenge/verify endpoints
  add a metadata surface (rate-limited via shared middleware).
- `LEGAL/TOS.md` (AI) — SIWE-authenticated operations are still
  covered by the TOS honesty invariants.

## Scope

SIWE-like challenge is an authentication **primitive**, not a session
layer. One challenge, one operation. Each nonce is single-use and
bounded to a 5-minute TTL by default.

Where an API-key identifies the API *client*, SIWE binds an operation
to a specific Ed25519 *identity* — so revocations, delegations, and
user-scoped actions (as opposed to service-scoped) can be
cryptographically authenticated without shipping the private key to
the server.

Not full EIP-4361 (SIWE). CasperProver does not require Ethereum
semantics. Only the challenge → nonce → signature shape is borrowed.

## HTTP surface

- `POST /auth/siwe/challenge`
  - Body: `{"pubkey": "<hex Ed25519 pubkey, 32 bytes>", "purpose": "<lowercase-hyphenated, ≤64 chars>"}`
  - 200: `{"message": "<canonical message>", "nonce": "<hex nonce>", "purpose": "...", "pubkey": "...", "ttl_seconds": 300}`
  - 400: invalid pubkey / purpose
  - 503: outstanding-challenge cap exceeded

- `POST /auth/siwe/verify`
  - Body: `{"pubkey": "<hex>", "purpose": "<same as challenge>", "message": "<exact canonical message from /challenge>", "signature": "<hex Ed25519 signature over message>"}`
  - 200: `{"ok": true, "purpose": "...", "pubkey": "..."}`
  - 400: malformed pubkey/signature/message
  - 401: expired, unknown, purpose/pubkey mismatch, or signature invalid

## Canonical message format

    cp:siwe:v1|<purpose>|<pubkey-hex>|<nonce-hex>|<issued-iso>

- `cp:siwe:v1` — protocol-and-version tag (domain separation).
- `<purpose>` — lowercase alphanumeric + hyphen, ≤64 chars.
  Domain-separates one operation from another. A nonce issued for
  `submit-batch` cannot be replayed for `revoke-proof`.
- `<pubkey-hex>` — 64-char hex of the Ed25519 public key the
  challenge is bound to.
- `<nonce-hex>` — 32-char hex of the 128-bit random nonce.
- `<issued-iso>` — RFC3339 UTC timestamp truncated to seconds.

The message is echoed back to the client so the client signs exactly
what the server stored — no client-side reconstruction of the message
is required, and no ambiguity in the signed payload is possible.

## Client flow

1. Client picks an Ed25519 keypair once (out of band).
2. Client POSTs `pubkey` + `purpose` to `/auth/siwe/challenge`.
3. Client Ed25519-signs the returned `message` verbatim.
4. Client POSTs `pubkey` + `purpose` + `message` + `signature` to
   `/auth/siwe/verify` within the TTL window.
5. On 200, the operation the `purpose` referred to is considered
   authenticated for this single call.

Nonces do NOT persist across server restarts. Clients must re-issue
challenges after a restart. The TTL is short (5 minutes default) so
the window is bounded.

## Security notes

- **Nonces are random 128-bit**, from `crypto/rand`. Not derived from
  client input.
- **Nonces are single-use.** Consumed on successful verify.
- **A failed verify does not consume the nonce**, so a legitimate
  retry after a signing bug within the TTL is still possible. The
  outstanding-challenge cap (`MaxOutstanding = 1024`) prevents this
  from being weaponised as a DoS surface.
- **Purpose tag is domain-separated** by construction (embedded in
  the canonical message before the nonce and pubkey).
- **Length-extension is not a concern**: Ed25519 signatures are over
  the whole message, not `H(key || message)`. See
  `HASH_ALGORITHM_ANALYSIS.md` Q7.
- **Metadata**: successful verify does not identify the client at the
  HTTP level (no session cookie is set); the pubkey used is the only
  identifier. See `METADATA_PRIVACY.md` §2.4 for the Verifier-side
  metadata implications of using SIWE.

## Non-goals

- **Not a session layer.** One challenge, one operation. If a client
  wants session semantics, it can issue N challenges up-front, but
  that is a client concern, not a server contract.
- **Not full EIP-4361.** No blockchain address semantics, no ERC-4361
  wire format compatibility.
- **Not a replacement for `X-API-Key`.** The two complement each
  other: API-key identifies the API client (rate-limits, quotas);
  SIWE binds a specific operation to a user/agent identity.
- **Not a persistent identity store.** The server does not track
  pubkeys across restarts.
- **Not a fingerprinting surface.** The server does not log pubkey
  values into observability streams beyond aggregate counters.

## Rate limiting

The endpoints are on the shared rate-limit middleware. In production
the limit should be tightened for `/auth/siwe/challenge` (issuance is
cheaper than verify but bounded by `MaxOutstanding`). Configuration
lives in the standard rate-limit environment variables described in
`docs/OPS_RUNBOOKS.md`.

## Reference implementation

- Primitive: `engine/internal/api/siwe/siwe.go`
- HTTP handlers: `engine/internal/api/siwe_handlers.go`
- Tests: `engine/internal/api/siwe/siwe_test.go`,
  `engine/internal/api/siwe_handlers_test.go` (18 tests total)
