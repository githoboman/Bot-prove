# Merge notes — public/admin API auth split (CP batch-3 revised, PR-3)

_Prepared for @curator review. Fills the gap flagged in
`handoff/2026-07-20/CP_FINAL_TASKS_V2_new.md`: "per-wallet auth требует
signed wallet challenge, scope allowlist и реального подключения
middleware" — this PR delivers the **real router wiring** part; the
signed-challenge + scope-allowlist part ships in PR-4._

## What this changes

Two orthogonal API keys instead of one:

- `API_KEY` (existing) — gates **public write** routes: `POST /proofs`,
  `POST /proofs/batch`, `POST /proofs/{id}/revoke`, `POST /verify`,
  `POST /kyc/check`, `POST /inference/prove`, `POST /inference/verify`,
  all `POST /zk/*`, `POST /proof-chain/validate`, all `POST /pq/*`.
- `ADMIN_API_KEY` (new) — gates **admin** routes: `POST /kyc/grant`,
  `POST /inference/register-model`, `POST /aggregation/create-batch`,
  `POST /aggregation/add-proof`, `POST /aggregation/finalize`.
- GET/HEAD/OPTIONS on every route stays public.

Both keys are optional. When either is unset the corresponding tier
passes through — same local-dev posture as before, and it's called out
loudly in the startup log.

## Why this is different from the earlier `fix/agent-batch-3` T2 commit

The reviewer's complaint on the old commit was accurate: it declared a
split but never wired the admin gate to specific routes — the global
`authMiddleware` still checked one shared secret for every write. This
version:

1. Adds `writeAuth` and `adminAuth` as **distinct** per-route
   middlewares.
2. Replaces the global `authMiddleware` wrap with a `buildMux()` that
   explicitly wraps each mutating route in `writeAuth(...)` or
   `adminAuth(...)`. The route table is the source of truth; no
   route silently inherits the wrong tier.
3. Retains `authMiddleware` as a thin backward-compat shim over
   `writeAuth` so no external caller / existing test breaks.

## Security posture

- **Admin routes never fall back to `API_KEY`.** An operator whose
  public key leaks to a tenant still cannot grant KYC or finalize an
  aggregation batch. Guarded by `TestAdminAuth_RejectsPublicKey` and
  `TestRouter_AdminRoutesAreAdminGated`.
- **Public routes never accept `ADMIN_API_KEY` in `X-API-Key`.** The
  admin secret is not usable as a super-key for tenant endpoints; PR-4
  will layer per-wallet scoping on top of `writeAuth` and this
  guarantee is what keeps that scoping meaningful. Guarded by
  `TestPublicWrite_RejectsAdminKey`.
- **401 vs 403 split (RFC 7235).** Missing header returns 401
  (unauthenticated); a value that fails constant-time compare returns
  403 (rejected). Clients can distinguish "I forgot the header" from
  "my key is stale/wrong" without a probing oracle.
- **Constant-time compare** on both keys via `crypto/subtle`.

## Tests

- `engine/internal/api/authz_split_test.go` (new): 7 tests, 20 subcases.
  - `writeAuth` 401/403/200 split.
  - `adminAuth` 401/403/200 split.
  - Public-key rejected on admin routes.
  - Admin-key rejected on public routes.
  - Full admin route table asserted admin-gated (5 routes).
  - Sample public writes assert reachable with only `X-API-Key`.
  - GETs bypass both gates.
- Existing `server_test.go`: `TestAuthMiddleware_KeyConfigured_RejectsMissingOrWrongKey`
  now asserts the new 401/403 split (was 401 for both). One-line
  behaviour update, documented in the test.

Full run: `go test ./...` → all engine packages green. `go vet ./...`
→ clean.

## Not in this PR

- Per-wallet key issuance with signed wallet challenge + scope enum
  (`submit | verify_only | admin_readonly`). That is PR-4 in this same
  series — will hang off the same `writeAuth` entry point.
- JWT / OIDC. Deliberately out of scope for the hackathon window; the
  shared-secret model is documented in `KNOWN_LIMITATIONS.md`.
- Client-side updates (`sdk/client.go` still sends the single
  `X-API-Key`). Public tenant flows keep working; admin flows expect
  an operator to set `X-Admin-API-Key` explicitly. If a follow-up
  wants a typed admin client that's a separate small commit.

## Files touched

- `engine/internal/api/server.go` — `Server.adminKey`, `New()` reads
  `ADMIN_API_KEY`, `Start()` delegates to new `buildMux()` which does
  per-route wrapping; `authMiddleware` demoted to shim over
  `writeAuth`; `writeAuth` + `adminAuth` added.
- `engine/internal/api/authz_split_test.go` — new, 7 tests.
- `engine/internal/api/server_test.go` — updated one existing
  subtest to reflect the 401/403 split, added a code comment
  pointing at the RFC 7235 rationale.
