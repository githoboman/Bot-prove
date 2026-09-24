# Threat Model — CasperProver

Expands on the summary in [`SECURITY.md`](../SECURITY.md) with a structured
threat model: assets, actors, attack surfaces, and mitigations. Update this
document whenever a new contract, endpoint, or trust boundary is added.

## 1. Assets

| Asset | Description | Impact if compromised |
|---|---|---|
| Staked funds (U512) | Value locked in `stake-slashing` contract | Direct financial loss on incorrect/malicious slash |
| Proof registry state | Immutable on-chain proof records (`proof-registry`) | Loss of verifiability / provenance if forgeable |
| Verifier-gate rate state | Per-block verify counters (`MAX_VERIFY_PER_BLOCK=100`) | DoS on verification throughput if bypassable |
| DeFi-mock whitelist | Owner-controlled KYC whitelist | Unauthorized DeFi access if whitelist can be bypassed |
| PQ / hybrid signing keys | Ed25519 + ML-DSA-65 hybrid keys, Lamport-OTS | Forged proofs/signatures if key material leaks |
| ZK proving/verifying keys (gnark, BN254/MiMC) | Groth16 setup artifacts | Forged ZK proofs if trusted setup is compromised |
| Submitter credentials | secp256k1 keys used for Casper RPC submission | Unauthorized on-chain submission if leaked |
| API server | Go backend: verify, batch-verify, resilience layer | Availability loss, tampered verification results |

## 2. Actors / Trust Boundaries

| Actor | Trust level | Notes |
|---|---|---|
| Proof submitter | Untrusted (external) | Submits proof_id + input/output/model; server independently re-derives and checks, never trusts client-asserted validity |
| Verifier caller (judge, agent, dashboard) | Untrusted (external) | `/verify` and `/verify/batch` re-check existence, hash match, commit validity, and Merkle-path validity server-side |
| Contract owner (defi-mock admin) | Trusted infra | Sole party able to whitelist; no other privileged path |
| Casper network / validators | Trusted (base layer) | Standard blockchain consensus assumptions apply |
| Casper RPC node (global-state queries) | Semi-trusted, assumed unreliable | Treated as a flaky dependency, not a trust anchor — see resilience layer below |

## 3. Attack Surfaces & Mitigations

### 3.1 Smart contracts (Rust/Wasm)
- **Integer overflow/underflow** — all `stake-slashing` U512 arithmetic uses `checked_add`/`checked_sub` with revert on overflow.
- **Double-slash** — `SLASHED_DICT` tracks slashes by `proof_id`; a repeat slash on the same id reverts.
- **Unauthorized revocation** — `revoke_proof` requires `caller == original submitter`.
- **Unauthorized whitelisting** — `defi-mock` whitelist mutation is owner-only; enforced access-control checks in place.
- **Verification-rate abuse** — `verifier-gate` enforces `MAX_VERIFY_PER_BLOCK=100`, capping on-chain verify calls per block.

### 3.2 Proof engine correctness (Go)
- **Merkle proof forgery** — `engine/internal/prover` inclusion/exclusion paths verified against `Root`; property-based tests (`merkle_property_test.go`, gopter, 3 properties × 200 cases) assert every leaf's path verifies against root, tampering always breaks `VerifyPath`, and root generation is deterministic + order-sensitive.
- **Batch verification integrity** — `POST /verify/batch` (up to 50 tuples) reuses the *exact* per-proof checks from `/verify` per item; a missing `proof_id` is reported inline rather than silently passing or failing the whole batch.
- **RNG panic on ID generation** — `genID()` falls back to a timestamp-based ID instead of panicking on RNG failure.

### 3.3 On-chain query resilience (submitter layer)
- **Flaky/unreachable Casper RPC** — `engine/internal/submitter/resilience.go`'s `ResilientQuerier` wraps global-state queries with exponential-backoff retry (bounded attempts, context-cancellation aware) plus a three-state circuit breaker (closed → open → half-open) that fails fast (`ErrCircuitOpen`) while open and self-probes after cooldown. 8 unit tests cover retry exhaustion, breaker open/reopen, half-open probe success/failure, and context cancellation mid-backoff.
- Rationale: an untrusted/unreliable RPC endpoint should degrade the *submitter's* behavior (fail fast, retry, recover) — it must never be treated as ground truth for consensus.

### 3.4 Backend API
- **Authentication/authorization** — API-key auth on mutating endpoints (via `X-API-Key` header when `API_KEY` env is set); rate-limit middleware caps every request path at **60 requests per rolling 1-minute window per source IP** — single global cap, no separate/stricter POST-specific limit. Implementation: `engine/internal/api/server.go:250-289` (`rateLimitMiddleware` + `rateLimiter` struct with bounded per-IP counter map).
- **Log injection** — all logging via `slog` structured JSON; no user-controlled format strings.
- **XSS (frontend)** — React auto-escaping throughout; no `dangerouslySetInnerHTML` usage. Judge dashboard (`/judge` route) follows the same pattern.
- **Secret leakage** — no secrets in frontend bundle; API key is server-side only. `.env.example` ships placeholders only; testnet keys are disposable.

### 3.5 Cryptography
- **ZK soundness** — real BN254 Groth16 prove+verify (gnark) for the MiMC preimage circuit; conceptual/rapid-test verification path is explicitly labeled non-binding (see Known Limitations).
- **PQ transition risk** — hybrid Ed25519 + ML-DSA-65 signing means a break in either scheme alone does not forge a valid signature; Lamport-OTS available for one-time-use high-assurance paths.

### 3.6 Threat enumeration (STRIDE-style summary)

Compact index of the concrete threats sections 3.1–3.5 mitigate, with likelihood/impact ratings and pointer to the enforcing code. Ratings are hackathon-scope: post-mainnet deployment would re-rate several `L`-likelihood items as `M` under real adversarial pressure.

| # | Category | Threat | Asset | Likelihood | Impact | Mitigation | Enforcement |
|---|---|---|---|---|---|---|---|
| T-01 | Tampering | Double-slash on same `proof_id` | Staked funds | L | H | `SLASHED_DICT` tombstone reverts repeat slash | `contracts/stake-slashing/src/main.rs` |
| T-02 | Elevation of Privilege | Non-owner mutates DeFi-mock whitelist | KYC whitelist | L | H | `caller != admin` reverts | `contracts/defi-mock/src/main.rs:69-71,107-109` |
| T-03 | Elevation of Privilege | Non-submitter revokes a proof | Proof registry | L | M | `caller == original_submitter` check | `contracts/proof-registry/src/main.rs:126` |
| T-04 | Denial of Service | On-chain verify flood | Verifier throughput | M | M | `MAX_VERIFY_PER_BLOCK=100` per-block cap | `contracts/verifier-gate/src/main.rs:24` |
| T-05 | Denial of Service | API request flood | Backend availability | M | M | Rate-limit middleware, 60 req/min per IP | `engine/internal/api/server.go:250-289` |
| T-06 | Tampering | Integer overflow in slash arithmetic | Staked funds | L | H | `checked_add`/`checked_sub` with revert | `contracts/stake-slashing/src/main.rs:169,182,226` |
| T-07 | Spoofing | Forged proof accepted via `/verify` | Proof registry | L | H | Server independently re-derives hash + commit + Merkle-path; client `valid` claim never trusted | `engine/internal/api/server.go` `/verify`, `/verify/batch` |
| T-08 | Tampering | Forged Merkle inclusion path | Proof registry | L | H | Path verified against `Root`; property tests cover tamper detection | `engine/internal/prover`, `merkle_property_test.go` |
| T-09 | Repudiation | Verifier claims tampered result | Verification integrity | L | M | Structured `slog` audit log per request; deterministic re-derivation | `engine/internal/api/server.go` `logMiddleware` |
| T-10 | Information Disclosure | XSS via judge dashboard | User session | L | M | React auto-escaping, no `dangerouslySetInnerHTML` | `frontend/` |
| T-11 | Information Disclosure | API key leak via frontend bundle | Mutating endpoints | L | H | API key server-side only, `.env.example` placeholders | `.env.example`, frontend bundle |
| T-12 | Denial of Service | Casper RPC failure blocks verification | Backend availability | M | L | Circuit breaker + exponential backoff; verification correctness independent of RPC | `engine/internal/submitter/resilience.go` |
| T-13 | Spoofing | Forged signature via classical break | Signing keys | L (short-term) / M (long-term) | H | Hybrid Ed25519 + ML-DSA-65 — requires break of *both* schemes | proof-generation signing path |
| T-14 | Tampering | Log injection via user input | Audit trail | L | L | All logging via `slog` structured JSON, no format-string interpolation | `engine/internal/api/server.go` |
| T-15 | Denial of Service | RNG failure crashes ID generation | Backend availability | L | L | `genID()` falls back to timestamp-based ID, never panics | `engine/internal/model/registry.go:83-90` |
| T-16 | Tampering | Batch verify short-circuits on one bad item | Verification integrity | L | M | Per-item independent check; missing `proof_id` reported inline, batch still processes remainder | `engine/internal/api/server.go` `/verify/batch` |

Likelihood scale: `L` = requires adversary with privileged access or unlikely conditions; `M` = plausible under normal adversarial pressure; `H` = attempted routinely.
Impact scale: `L` = degrades UX; `M` = loss of verification/audit trust; `H` = direct financial loss or full compromise.

## 4. Explicit Non-Goals / Accepted Risk

See [`docs/KNOWN_LIMITATIONS.md`](KNOWN_LIMITATIONS.md) for the full list (real Groth16 model-inference circuit vs. current MiMC preimage stand-in, per-start trusted-setup generation vs. a production ceremony, etc.). These are accepted for the testnet/hackathon submission and tracked on the roadmap.

## 5. Structural Guarantees (unchanged from SECURITY.md)

- No proof can be marked valid without independently re-deriving hash/commit/Merkle-path checks server-side — client-asserted validity is never trusted.
- No slash can be applied twice to the same `proof_id`.
- No party other than the contract owner can mutate the DeFi-mock whitelist.
- An unreachable/misbehaving Casper RPC node degrades submitter *availability*, never verification *correctness* — the circuit breaker and retry logic live entirely below the trust boundary.

## 6. Review Cadence

Revisit this document whenever a new contract, endpoint, or external
integration (new RPC provider, new ZK circuit, new signing scheme) is added.
Last written: 2026-07-23, alongside SBOM (T14) and SECURITY.md hardening (T15).
