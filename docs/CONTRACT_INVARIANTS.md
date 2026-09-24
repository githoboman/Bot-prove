# Contract Invariants

Concise model of the reentrancy and cross-contract invariants that hold across
the original four deployed contracts (`proof-registry`, `verifier-gate`,
`defi-mock`, `stake-slashing`).

*(Status update, 2026-07-27: `model-registry`, `proof-aggregation`,
`proof-of-inference` and `governance` were compiled-but-not-deployed when this
doc was written — all four are now live on testnet too, see
`deploy-out/onchain.json` / `TX_MANIFEST.md`. Their invariant analysis has not
been folded into this doc yet; treat the below as covering the original four
only, not the full current set of eight.)*

Scope is intentionally narrow: this is a **living reference for reviewers and
future contributors**, not a formal proof. Everything here is enforced in
source (`contracts/*/src/main.rs`) and, where noted, in the API middleware
that fronts these contracts (`engine/internal/api/*`, `engine/internal/submitter/casper.go`).

## Trust model

Two principals, no more:

| Principal | Represents | Cardinality |
|-----------|------------|-------------|
| `owner`   | Deployer of the contract package. Sole account allowed to mutate configuration and rotate keys. | Exactly one live account per contract. Owner rotation goes through the timelock described in `OWNER_LIFECYCLE.md`. |
| `caller`  | Any signed deploy that hits an entry point. Constrained by per-entry-point access control. | Unbounded. |

No admin, no multisig, no proxy on any of the four deployed contracts. The
absence of these constructs is a deliberate simplification for the hackathon
scope — see `OWNER_LIFECYCLE.md` for what a shipping design would add.

## Global invariants

**I-1 · Owner isolation.** Only `owner` can call `record_stake`, `unstake`,
`report_and_slash` (stake-slashing), `grant_access`, `revoke_access`
(defi-mock), `revoke_proof` (proof-registry), `register_agent`
(proof-registry). Every write path validates `runtime::get_caller() == owner`
before touching a named key.

**I-2 · No reentrancy.** Casper's execution model is not EVM: a contract
cannot call itself recursively within a single deploy, and cross-contract
calls are executed as separate host-visible sessions rather than nested
call frames. We still respect the CEI pattern (checks → effects →
interactions) so a future migration to a chain with nested calls doesn't
break invariants. Every state mutation completes before an external
transfer (`stake-slashing::unstake`, `stake-slashing::report_and_slash`).

**I-3 · Monotonic reputation.** `proof-registry.get_reputation(agent)` is
monotonic non-decreasing per agent within a proof lifecycle. `revoke_proof`
does not decrease the counter — instead it flips a `revoked` flag on the
proof entry and the reputation function ignores revoked entries when
aggregating. This means an agent's "trust score" is a floor, never a
retracted claim.

**I-4 · Amount arithmetic is checked.** All U512 additions and subtractions
in `stake-slashing` go through `checked_add` / `checked_sub` (or
`saturating_sub` where saturation is intentional). This closes the
overflow class that shipped in `stake-slashing v1` and is enforced by the
hardened redeploy at `1ad1b3d94be6...` — see the ADR entry in
`docs/JUDGE_GUIDE.md` and the CHANGELOG for the pre/post hashes.

**I-5 · Zero-value slash is a tombstone, not a no-op.** `report_and_slash`
with amount == 0 records a slash *event* with amount 0 and increments the
per-agent slash count, but touches no purse. This preserves the invariant
that "an agent with ≥1 slash is publicly reportable" independent of the
economic penalty.

## Cross-contract invariants

**X-1 · verifier-gate → proof-registry read-only.** `verify` reads
`is_valid` from `proof-registry` via URef lookup; it never writes back.
This makes the gate hot-path idempotent and safe to call from a client SDK
that retries with the same `Idempotency-Key`.

**X-2 · defi-mock → verifier-gate is advisory.** `defi-mock.check_kyc`
consults `verifier-gate` for the caller's most recent verified proof but
does NOT hard-fail on absence. It returns a boolean; the client (or a
front-end policy layer) decides how to act. Missing proof ≠ malicious;
callers may be pre-registration.

**X-3 · stake-slashing → proof-registry write-back on slash.** When
`report_and_slash` succeeds it writes a `slashed_proof` tombstone into a
named-key dictionary on the stake-slashing contract itself and emits an
event. `proof-registry.get_reputation` reads the tombstone dictionary
across contracts by contract package hash — the address of
`stake-slashing` is captured at `proof-registry` install time and not
mutable, closing address-substitution attacks.

**X-4 · No unbounded loops.** No entry point iterates over an unbounded
collection. Reputation aggregation reads the tombstone dictionary via
direct key lookup (`get_dictionary_item_key`), not a scan. This bounds
gas even as the number of agents grows.

## Frontend / API guarantees

**F-1 · Contract hashes are read from `onchain.json`, not typed in code.**
The frontend build fails if `deploy-out/onchain.json` isn't present at
build time (`frontend/vite.config.ts`). The API server falls back to
compile-time defaults ONLY if the corresponding env var is empty — the
defaults are logged on startup so an operator running against a fresh
deploy immediately sees which hash is being used.

**F-2 · CP_STRICT=1 fails closed on anchor errors.** When
`CP_STRICT=1`, the API refuses any request that would otherwise return a
non-anchored receipt (Casper node unreachable, contract hash unset,
signing failure). See `engine/internal/api/server.go` for the guard and
`.env.example` for the switch.

## Out of scope (deliberately)

- Fuzzing / property testing. Odra unit tests exist but nothing runs on
  a persistent testnet forker.
- Formal verification. See `FORMAL_VERIFICATION.md` for what's actually
  been done (small-model TLC pass) and what a full effort would look like.
- Cross-contract *upgrade* invariants. Contracts are installed by
  package hash, never upgraded in-place. A new deploy overwrites the
  `contract_hash` in `onchain.json`; SDKs cached against the old hash
  will 404 gracefully.

## Change control

Every change to an invariant listed here MUST land alongside the code
change in the same commit, and MUST be reflected in `docs/JUDGE_GUIDE.md`
if the change is visible to a judge running `verify.sh` or the demo
script. If you're breaking I-*, X-*, or F-*, that's a security event —
open an issue titled `INVARIANT BREAK: <id>` first.
