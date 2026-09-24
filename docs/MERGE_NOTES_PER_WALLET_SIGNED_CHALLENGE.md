# Merge notes — per-wallet API keys with signed wallet challenge (CP batch-3 revised, PR-4)

_Prepared for @curator review. Closes the final gap flagged in
`handoff/2026-07-20/CP_FINAL_TASKS_V2_new.md`: "per-wallet auth требует
signed wallet challenge, scope allowlist и реального подключения
middleware". PR-3 delivered the middleware split + real router wiring;
this PR delivers the signed-challenge + scope-allowlist + tenant
lookup layer on top._

**Depends on PR-3** (`feat/cp-public-admin-middleware-split`) — this
branch is cut from that one, not from main. Merge PR-3 first.

## What this changes

### Two-step wallet-signed issuance

**Step 1 — `POST /admin/keys/challenge`** (admin-gated):
Request `{"wallet": "<casper address>"}` → server returns
`{"nonce": "<32-byte hex>", "wallet": "<echo>", "message": "<exact bytes to sign>",
"expires_at": <unix>, "ttl_secs": 300}`.

The nonce is stored server-side with a 5-minute TTL, bound to that
wallet, not yet consumed.

**Step 2 — `POST /admin/keys/issue`** (admin-gated):
Request `{"wallet", "scope", "nonce", "pubkey_hex", "signature_hex"}`.
Server enforces, in order:

1. All fields present.
2. Scope ∈ `{submit, verify_only, admin_readonly}`.
3. Challenge with this nonce exists, unconsumed, unexpired, belongs to
   the same wallet.
4. `pubkey_hex` is 32-byte ed25519 and matches `wallet` via Casper's
   `01<pubkey_hex>` convention.
5. `signature_hex` is a valid ed25519 signature over
   `"cp-issue-key:" || nonce || ":" || wallet` under `pubkey_hex`.
6. Consume the nonce (atomic UPDATE — replay race is a hard fail).
7. Only then mint `sk_live_<64 hex>` and persist `sha256(key)`.

Plaintext is returned exactly once in the response and never logged.

### Scope allowlist

Closed enum in `apikey.go`:
- `submit` — full tenant write surface (`/proofs`, `/proofs/batch`,
  `/proofs/{id}/revoke`, `/inference/prove`, `/verify`, all `/zk/*`,
  all `/pq/*`, `/proof-chain/validate`, `/zk/challenge`,
  `/zk/groth16-real/prove`).
- `verify_only` — verification and read-only chain checks:
  `/verify`, `/zk/verify-groth16`, `/zk/batch-verify`,
  `/zk/groth16-real/verify`, `/pq/verify-sphincs`, `/pq/hybrid-verify`,
  `/proof-chain/validate`, `/inference/verify`.
- `admin_readonly` — diagnostic-only surface: currently `/kyc/check`.
  Cannot submit, cannot register, cannot finalize. Not a superset of
  submit — a distinct axis.

Free-form scopes are rejected at issuance time with 400 + the
enumeration in the error body.

### writeAuth accepts sk_live_ keys

`writeAuth` (introduced in PR-3) now recognises two credential shapes
under `X-API-Key`:

1. The shared `API_KEY` (constant-time compare) — treated as a
   super-scope, satisfies every `requireScope` check. Operator escape
   hatch.
2. `sk_live_<64 hex>` — hashed and looked up in `api_keys`; must be
   non-revoked. The row's `scope` is stashed in the request context.

`requireScope(allowed, next)` at each mutating route enforces that the
resolved scope is in `allowed`. Wired per route in `buildMux()`
(mirror of the admin/public split from PR-3).

### Revocation

`POST /admin/keys/revoke {"id": "..."}` (admin-gated) flips
`revoked=true` atomically. Any live sk_live_ tied to that id is
rejected by `writeAuth` on the next request. Double-revoke returns 404.

## Storage

Two new Postgres tables auto-provisioned by `schemaDDL`:

- `api_keys(id PK, key_hash UNIQUE, wallet_addr, scope, created_at,
  revoked, revoked_at)`. Indexed on `wallet_addr`.
- `wallet_challenges(nonce PK, wallet_addr, created_at, expires_at,
  consumed_at)`. Indexed on `wallet_addr`.

Both tables are additive and idempotent (`CREATE TABLE IF NOT EXISTS`).
Existing DBs upgrade in-place on next start; no manual migration needed.

## Security posture

- **No unsigned issuance path.** The plaintext key is only returned
  after a valid ed25519 signature over the exact challenge message.
  There is no `wallet=X` shortcut, no operator override that mints
  without a signature — even the shared `API_KEY` cannot bypass the
  admin-gated issuance handler (it is on the admin tier, requiring
  `X-Admin-API-Key`).
- **Nonce is single-use.** `MarkWalletChallengeConsumed` is an atomic
  UPDATE with `consumed_at=0` predicate; a race between two identical
  issue requests loses one at the DB layer, and the loser gets 401.
  Tests cover the replay explicitly.
- **5-minute TTL.** Short enough that a stolen nonce is worthless
  minutes later; long enough for a human to sign in a wallet UI.
- **Domain-separated signature.** The client signs
  `"cp-issue-key:" || nonce || ":" || wallet`. The `cp-issue-key:`
  prefix prevents an attacker from replaying a signature crafted for
  a different CP protocol message (proof submission, etc.) as a
  key-issuance signature.
- **Wallet↔pubkey binding.** The server refuses to issue if
  `wallet != "01"+hex(pubkey)`. Even a valid signature over the
  challenge is rejected if the pubkey doesn't own the claimed wallet.
- **Constant-time secret handling.** Shared-key compare uses
  `crypto/subtle`; sk_live_ path is a DB lookup on a fixed-length
  sha256 hex digest.
- **No plaintext ever persisted.** Only `sha256(key)` lands in
  `api_keys`. Startup logs and `INFO api key issued` log only id,
  wallet, scope — never the raw key.
- **Scope enforcement is fail-closed.** A verify_only key hitting a
  submit route returns 403; a revoked key returns 403.

## Not in this PR

- **secp256k1 wallets.** Casper supports two wallet flavours; this PR
  ships ed25519 only. secp256k1 issuance is a small follow-up (parse
  compressed pubkey, use `crypto/ecdsa`); the closed scope enum and
  challenge/consume machinery are unchanged. Documented in
  `KNOWN_LIMITATIONS.md`.
- **JWT / bearer tokens.** Out of scope; the sk_live_ shape mirrors
  the existing tenant convention and is the demo-appropriate primitive.
- **Rate-limiting the challenge endpoint.** The global rate limiter
  (60 req/min/IP from PR-3's server chain) covers it. A per-wallet
  challenge quota is a hardening follow-up.
- **Client updates in `sdk/client.go`.** The SDK still sends the shared
  `X-API-Key`. Public tenant flows work as before; a typed
  per-wallet client with the challenge/sign helper is a separate small
  commit.

## Tests

- `engine/internal/api/apikey_test.go` (new): 16 tests covering key
  generation, hashing, challenge issuance, full signed-issue happy
  path, nonce replay, expired nonce, wrong wallet, tampered signature,
  invalid scope, pubkey/wallet mismatch, revoke lifecycle,
  `writeAuth` accepting a fresh sk_live_ key, rejecting a revoked
  one, rejecting an unknown one, scope enforcement on `/proofs`
  vs `/verify` for each scope, and the shared-API_KEY super-scope
  path. All exercised through the fully wired `buildMux()`, not just
  middlewares in isolation.
- Existing PR-3 tests (`authz_split_test.go`, `server_test.go`) still
  green — the split/wiring guarantees they enforce are preserved.
- Full `go test ./...` green across every engine package.
- `go vet ./...` clean.

## Files touched

- `engine/internal/api/apikey.go` — new; sk_live_ format, hashAPIKey,
  ValidScopes enum.
- `engine/internal/api/apikey_scope.go` — new; request-context helpers
  for scope propagation.
- `engine/internal/api/apikey_store.go` — new; storeAPIKeyRecord +
  storeWalletChallengeRecord adapters, apiKeyStore interface,
  keyStore() fallback.
- `engine/internal/api/admin_keys.go` — new; challenge, issue
  (signed flow, all 6 checks above), revoke handlers.
- `engine/internal/api/server.go` — `Server.keys` field;
  `New()` reads no new env (secrets stay in-DB); `writeAuth` accepts
  sk_live_ + shared key; `requireScope` per-route wrapper;
  `buildMux()` gains scope allowlists per public write and three new
  admin routes (`/admin/keys/challenge|issue|revoke`).
- `engine/internal/api/apikey_test.go` — new; 16 tests.
- `engine/internal/store/pg.go` — two new tables (`api_keys`,
  `wallet_challenges`) in `schemaDDL`; `APIKeyRecord`/`WalletChallengeRecord`
  types; six new methods (Insert/Lookup/Revoke on api_keys;
  Insert/Lookup/Consume on wallet_challenges).
