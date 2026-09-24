# Changelog

All notable changes to CasperProver are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/).

---

## [Unreleased] — 2026-07-28

### Fixed — critical (P0)

- **`zk-verifier.register_vk`/`disable_vk` governance bypass closed and live.**
  `require_owner_or_gov` trusted a caller-supplied `governance_approved` flag
  with zero on-chain verification, letting any account register/disable
  arbitrary verifying keys. Fixed in source, covered by a new regression test,
  redeployed to a fresh contract (`4500da5d…dc96a1`) from a dedicated new
  wallet (redeploying from the original owner account failed with
  `ApiError::InvalidArgument` due to NamedKey/dictionary collisions — see
  `docs/roadmap/ZK_VERIFIER_REDEPLOY_2026-07-27.md`). Render's
  `CONTRACT_ZK_VERIFIER` env var updated and reconfirmed live via `/health`.
  Live-regression-tested: non-owner `register_vk(governance_approved=1)` now
  reverts `ERR_NOT_OWNER`. New owner account authorized as a verifier
  (`add_verifier`) before the env var switch so the live
  `POST /v1/zk/anchor-verdict` endpoint kept working uninterrupted. Full
  `judge_demo.py` suite: 13/13 passing against the live API, including a real
  Groth16 round-trip. See `docs/SECURITY_AUDIT.md` §2.10 for the complete
  writeup.

### Docs

- `docs/ARCHITECTURE.md` — added the ownership cheat sheet to the Security
  posture section (previously only linked to `SECURITY_AUDIT.md`, not
  summarized inline).
- `docs/OPS_RUNBOOKS.md` — added §4.3 documenting that `governance` guardians
  cannot directly unpause; they must complete `sign_recovery`/`execute_recovery`
  first if the owner key is the one lost during an incident.
- `docs/SECURITY_AUDIT.md` — follow-up items 4 and 5 (both P3, doc-only)
  marked done; items 2 and 3 (P2) explicitly marked "not recommended
  pre-submission" since both require a contract redeploy and risk repeating
  the owner-account fragmentation seen in this pass.

---

## [Unreleased] — Post-Deadline Review Pass (2026-07-27)

Full review/reconciliation pass, done after 07-26's judge/redeploy work.
Found and fixed real production issues, not just doc drift:

### Fixed — critical

- **Production Go build had been broken since commit `1744880`**: `go.mod`'s
  `go 1.25.7` directive (bumped alongside gnark 0.13→0.15) didn't match the
  Dockerfile's `golang:1.24-alpine` builder, so every deploy since then
  failed at `go mod tidy` with "go.mod requires go >= 1.25.7". The live
  API (`casperprover-api-ylsh.onrender.com`) had been silently frozen on a
  ~3.5-hour-old build serving stale contract hashes and missing the
  CP_STRICT auth block, while `TX_MANIFEST.md`, `judge_demo.py`, and 3 SDK
  releases all landed on `main` without ever going live. Bumped the
  Dockerfile and 3 GitHub Actions workflows (`check.yml`,
  `sdk-publish-go.yml`, `verify.yml`) to `1.25`. Confirmed live via a fresh
  Render deploy.
- `scripts/judge_demo.py` only checked 4 of 8 live contracts —
  `_MANIFEST_KEYS` was never extended to match `_FALLBACK_CONTRACTS` when
  the other 4 contracts (proof-aggregation, model-registry,
  proof-of-inference, governance) went live. Fixed; verified 11/1/0 (was
  7/1/0).
- `frontend/src/components/lab/Contracts.tsx`'s `PRESENTATION` array was
  missing `governance` — the flagship Contracts lab page reported "7 total,
  7 deployed" instead of 8/8.

### Fixed — docs/site

- `README.md`'s top badge/summary said 250+ testnet transactions while the
  detail table and `TX_MANIFEST.md` both said 248+ — standardized on 248+.
- `Benchmarks.tsx`, `Features.tsx`, `Footer.tsx` still said "7 contracts,
  4 deployed" on the live site.
- All 6 `docs/screenshots/*.png` referenced from README were stale (showing
  the old "4 deployed + 3 written" contract count) — regenerated from the
  live site.
- Rewrote `docs/VIDEO_SCRIPT.md` (deleted from history in an earlier docs
  cleanup) to match the current 10-page Lab UI instead of the old
  Generate/Proofs/Verify/Demo tab layout.

*Frontend polish, DoraHacks submission prep, docs hardening, backend hardening, CI/deps sweep, Casper 2.0 SDK migration.*

### Added (2026-07-19)
- **CP_STRICT=1 + `API_KEY` fail-closed** (`engine/internal/api/server.go`, feat/cp-api-key-fail-closed). `api.New()` now returns an error instead of a running server when CP_STRICT=1 is set with an empty API_KEY -- `main.go` turns that error into `os.Exit(1)`, so an operator who opted into strict mode gets an immediate crash instead of a silently-anonymous deployment. Loose mode + empty key still works (dev / demo). `/health` gained a structured `auth` block ({mode, enforced, strict}) so `verify.sh` and the frontend can gate on the deployment posture without parsing the log stream. `verify.sh` gained a `verify_auth` section that WARNs on unenforced auth and hard-FAILs on the impossible "strict + not enforced" state (fail-close bypass detection). 7 unit tests in `engine/internal/api/apikey_failclosed_test.go` cover the 2×2 (strict, key) precondition matrix and the three `/health.auth` shapes (enabled + enforced, disabled loose, prod strict). Closes CP_AGENT_SPEC v2 Gate 1.2 ("startup fails or prominently degrades if API_KEY missing").

### Fixed
- `proof-aggregation::create_batch` no longer silently overwrites an existing
  open batch — duplicate `batch_id` now reverts with `ApiError::User(22)`,
  empty `batch_id` reverts with `20`, and `max_proofs == 0` reverts with `21`.
  This closes the P1 finding in `docs/SECURITY_AUDIT.md` and unblocks Gate 2
  redeploy of the `proof-aggregation` crate. Contract rebuilds cleanly on
  `nightly-2025-01-01`; guard invariants mirrored in
  `contracts/tests/src/integration_tests.rs::proof_aggregation_tests`
  (6 new tests, all green).

### Added — Gate 1 hardening (2026-07-20)
- `GET /onchain.json` API endpoint — canonical contract manifest served over HTTP with 60s in-memory cache and mtime invalidation (`engine/internal/api/onchain.go`, 4 tests).
- Root `deploy-out/onchain.json` promoted to canonical single-source-of-truth for on-chain contract addresses; served verbatim by `/onchain.json`.
- `scripts/sync-onchain-manifest.sh` and `make sync-onchain` — one-liner that copies the canonical manifest into `frontend/public/onchain.json`; wired into frontend `prebuild` + `predev` hooks so the SPA can never drift from the API.
- `scripts/judge_demo.py` — reads contract hashes from the canonical manifest (with a loud stderr fallback) instead of hardcoding them.
- `verify.sh` — loads contract addresses from `deploy-out/onchain.json` via `jq`; pinned list retained only as a fallback for stripped environments.
- `CP_STRICT=1` production preflight in `cmd/casperprover/serve` — refuses to start with `API_KEY=""`; unauthenticated writes are only permitted in dev mode.

### Added
- `docs/SECURITY_AUDIT.md` — full owner/admin/renounce lifecycle audit + reentrancy/cross-contract invariant review for all 8 contract crates (Gate 1, item 4 of the deadline plan). No P0 findings; one P1 blocker for `proof-aggregation` (silent `create_batch` overwrite) filed as pre-Gate-2 follow-up.
- **Gate 4 — ZK primary path.** `/zk/groth16-real/*` (gnark BN254, real R1CS + trusted setup + pairing verification) promoted to the canonical ZK path in the router, README, `docs/KNOWN_LIMITATIONS.md` and `docs/JUDGE_GUIDE.md`. Hash-based `/zk/verify-groth16` and `/zk/batch-verify` are marked as `[sim]` — responses now carry `simulation:true, deprecated:true, use:"/zk/groth16-real/verify"` plus `Warning`, `Deprecation` and `Sunset` HTTP headers. Canonical simulation spellings `/zk/verify-groth16-sim` / `/zk/batch-verify-sim` added; the older paths remain as deprecated aliases for backwards compatibility. 3 new tests in `engine/internal/api/zk_gate4_test.go` (sim banner + real round-trip; passes with `-race`).
- `/v1/*` API alias with backward-compat: every existing endpoint reachable under `/v1/...`; legacy unprefixed paths keep working but respond with `X-CP-Deprecation`, `Sunset: 2027-01-01`, and `Link` successor-version headers (RFC 8594).
- `X-Idempotency-Key` server middleware for POST/PUT/PATCH/DELETE: 15-minute TTL cache; replayed responses carry `X-Idempotency-Replay: true`; same key + different payload returns `409 Conflict` (surface client bugs, don't hide them).
- `X-CP-API-Version: v1` response header on every API call for client-side introspection.
- `docs/API_CHANGELOG.md` — versioning policy, deprecation timeline, migration guide.
- Skeleton loader components (`Skeleton`, `CardSkeleton`, `TableSkeleton`) wired into Overview + Proofs.
- `EmptyState` component for lab list views (Proofs / Models / Aggregation / KYC).
- `CopyButton` with checkmark feedback and legacy-browser fallback.
- Full favicon set: `favicon.ico` (multi-size) + `favicon-16x16.png` + `favicon-32x32.png` + `apple-touch-icon.png`.
- Sitemap.xml expanded to every public Lab and Docs route.
- Verify / Secret Scan / Contracts / Proofs / PQ status badges in README.
- Telegram + X + GitHub social links row in footer.
- `docs/SUBMISSION_CHECKLIST.tmp` — DoraHacks submission tracker.
- `docs/RED_TEAM.tmp` — 18-vector red-team self-audit.
- `docs/API.tmp` — curl-first API reference for all 32 endpoints.
- `verify.sh` — single-command proof-of-deployment script that hits `/health`, checks all four contract hashes on CSPR.cloud, and prints a green/red matrix (`1f17c22`).
- `SECURITY.md` — self-audit table, threat model, and disclosure policy (`db0775e`).
- `frontend/public/onchain.json` — verified contract deploy data pulled from CSPR.cloud, consumed by the Contracts tab (`f60372a`).
- Dependabot configuration covering Go, npm, Cargo, and GitHub Actions (`56d9331`); SDK-side gomod ecosystem added separately (`694f024`).
- TruffleHog secret-scan workflow on push and PR — verified and unknown findings both surface (`d247ec0`, hardened in `9396a19`).
- CI job that uploads compiled contract WASM artifacts, needed by the stake-slashing redeploy pipeline (`517f28a`).
- Lab UX additions: breadcrumbs above lab content (`71a344c`), help-tooltips on section titles (`a2bf6de`), keyboard-shortcuts + help modal (`b614894`), `StatusBadge` extracted from `SectionIntro` for reuse (`625477d`).

### Changed
- All 20 `console.error` calls in Lab components guarded by `import.meta.env.DEV`; only `ErrorBoundary` (line 18) and `toast` fallback (line 122) keep unconditional logging by design.
- Main marketing navbar auto-hides on `/lab/*` routes (Lab has its own sticky top bar).
- `ErrorBoundary` auto-resets on route change via `key={location.pathname}` — no stale error screens across nav.
- Mobile Lab sidebar auto-closes on route change.
- `README.md` badges regrouped: CI status row + capability row.
- **Migrated `internal/submitter` to `casper-go-sdk/v2` and refactored to sign & submit real `TransactionV1` payloads (Condor); `gnark` bumped `0.12 → 0.13` (`212c429`).**
- Contract hashes moved out of hard-coded literals into env vars (`CONTRACT_PROOF_REGISTRY`, `CONTRACT_VERIFIER_GATE`, `CONTRACT_DEFI_MOCK`, `CONTRACT_STAKE_SLASHING`) with sane defaults; redeploys no longer require a code change (`be66ac4`).
- Logging migrated from ad-hoc `fmt.Printf` to structured `slog` across CLI and demo code (`907eec9`).
- `genID` now falls back gracefully on `crypto/rand` failure instead of `panic` (`0dc68a2`).
- CI: `go-version` bumped `1.22 → 1.24`, `golangci-lint` config migrated to v2 (`d0503f8`), remaining errcheck / staticcheck findings resolved (`d0596a5`).
- GitHub Actions bumps: `actions/setup-node` 4 → 7 (`775090c`), `golangci/golangci-lint-action` 6 → 9 (`8f497f6`), `actions/setup-go` 5 → 7 (`fff4897`).
- npm bumps: `typescript` 5.6 → 7.0.2, `react-router-dom` 6.28 → 7.18.1 (`7e28ee4`).
- Safe patch/minor bumps: `circl`, `lib/pq`, `actions/checkout`, `autoprefixer` (`b99d0e4`).

### Fixed
- Vite env types wired via `src/vite-env.d.ts` so `import.meta.env.DEV` type-checks under `strict`.
- **stake-slashing `record_stake` now self-verifies against the actual purse balance — the previous implementation trusted the caller-supplied amount, so out-of-band calls could inflate recorded stake with no backing funds** (`392e4f0`; regression discovered in the internal audit).
- **Casper 2.0 `Key` compatibility: contracts now use `into_entity_hash_addr` where the old code assumed the pre-Condor addressable-entity model** (`9f4c40a`).
- Checked arithmetic added in stake-slashing and proof-registry to eliminate silent overflow (`a5e2aa4`).
- RED_TEAM audit: corrected 2 inaccurate agent-report claims (nonexistent CORS env-var, wrong function name) and added the missing API_KEY-unset auth-gap vector (`6d3d434`).
- Small lab polish: removed cross-project reference in a comment, replaced ✕ emoji with SVG X icon (`0347127`); replaced 💡 emoji with `Lightbulb` SVG in `LabLayout` (`462eb6c`).

### Planned
- Deploy `proof-of-inference`, `model-registry`, `proof-aggregation` contracts to mainnet
- Full model-inference ZK circuit (beyond MiMC preimage)
- STARK recursive aggregation
- Grafana/Prometheus metrics
- Proof expiry and rotation policies

---

## [1.2.0] — 2026-07-08

*Wallet integration, UX overhaul, stake-slashing contract, CI fix*

### Added
- **stake-slashing** contract deployed on testnet — 20% CSPR slash + permissionless bounty
- **defi-mock** contract redeployed with hardened `is_whitelisted(user: ByteArray32)` signature
- CSPR.click wallet integration (Casper Wallet, Ledger, MetaMask Snap, Google SSO)
- On-chain proof anchoring via wallet signing (`casper-js-sdk ^5.0.12`)
- Real Groth16 ZK-SNARK proofs via gnark (BN254 pairing-based cryptography)
- ML-DSA-65 post-quantum signing (FIPS 204, cloudflare/circl) + Lamport OTS
- 11 interactive Lab pages with sticky header, toast system, demo/live mode
- SectionIntro blocks, click-to-copy, search/filter, confirm modals
- Mobile-responsive menu
- Proof Pipeline merged into Playground

### Changed
- API expanded to 32 endpoints, SDK to 34 methods, MCP to 32 tools
- Agent field max length increased to 128 for wallet public keys

### Fixed
- SDK build: `Model` → `ModelID` in MCP server (CI was broken)
- Table refresh before wallet signing
- Revoke without X-Public-Key header
- Toast ordering: wallet popup before success toast

---

## [1.1.0] — 2026-07-02

*Major feature expansion — pre-submission hardening*

### Added

#### Smart Contracts (Rust/Wasm)
- **ProofOfInference** (498 LOC) — individual proof anchoring with Merkle root on-chain verification
- **ProofAggregation** (179 LOC) — batch-aggregated proofs for gas-efficient on-chain anchoring
- **ModelRegistry** (372 LOC) — on-chain registry of approved AI models with version tracking

#### Backend Modules (Go)
- **Model Registry** (`engine/internal/model/registry.go`, 283 LOC) — model CRUD, versioning, metadata management, and framework validation
- **Complexity Analyzer** (`engine/internal/prover/complexity.go`, 186 LOC) — proof complexity estimation based on model parameters and input size
- **Distributed Worker** (`engine/internal/worker/distributed.go`, 438 LOC) — task distribution, worker pool management, and fault-tolerant scheduling

#### MCP Server
- Expanded from 15 to 23 tools — new tools for model registry, complexity analysis, and distributed task management

#### Lab
- 4 new tabs: Models, Complexity, Workers, Contracts
- Model registry browser with version and framework info
- Complexity metrics: average generation time, Merkle depth, gas estimates
- Worker node status with real-time load and region info

#### Tests
- 51 new business logic tests (3 files, 1,373 LOC)
- `registry_test.go`: 16 tests — model CRUD, versioning, metadata
- `complexity_test.go`: 15 tests — complexity estimation, boundary analysis
- `distributed_test.go`: 20 tests — task distribution, worker pool, fault tolerance

### Security
- Secure temp file creation for deployer keys (os.CreateTemp)
- X-Public-Key header validation for revoke operations
- Capped pagination limits
- Request body size limits
- Bounds checking in Merkle tree operations

---

## [1.0.0] — 2026-06-30

### Added
- **Proof Registry contract** deployed on Casper testnet ([96e97c4d...a10708](https://testnet.cspr.live/contract/e11088f1f15a719f21c0c318d1f34d0b96419a22d60ac8fa384ecf5285fa7bc5)) — stores Merkle roots, proof metadata, verification status
- **Verifier Gate contract** ([a37f9cde...9f77d3](https://testnet.cspr.live/contract/06d69182b13c4d041613fe7e6e0805cdb06f099eff4291b40154d78cc0c79b66)) — checks inclusion proofs, manages access control
- **DeFi Mock contract** ([b9b11a97...b81d3](https://testnet.cspr.live/contract/b9b11a976af20b4b5d128c44e5ee118b8830c26a79f4b603cdf0a00e537b81d3)) — sample vault gated by verifier-gate, demonstrating KYC-gated DeFi flow
- **Merkle tree builder** in Go engine — SHA-256 leaf hashing over `{H(input), H(output), H(model)}` triplets, binary tree construction, path serialization
- **Four proof types supported**: `merkle-inclusion`, `kyc-eligibility`, `balance-range`, `transaction-membership`
- **REST API** at `https://casperprover-api-ylsh.onrender.com` — endpoints: `POST /api/v1/proof/submit`, `POST /api/v1/proof/verify`, `GET /api/v1/proofs`, `GET /api/v1/stats`
- **Lab** at `casperprover.xyz/lab` — 72 live proofs registered on testnet, proof type breakdown, verification status badges
- **Go SDK** (`sdk/`) — `client.go` with submit/verify helpers, Python client (`python_client.py`)
- **MCP server** (`sdk/mcp_server.go`) — Model Context Protocol adapter so AI frameworks (Claude, LangChain) can call CasperProver as a tool
- **Contract test suite** (`contracts/tests/`) — integration tests covering registry, gate, and mock interactions
- Configuration via `config.toml` — node URL, chain name, API port, rate limit, hash algorithm, precomputed proof directory

### Infrastructure
- Deployed on Casper testnet (chain: `casper-test`)
- API hosted on Render
- Frontend hosted on Vercel
- CI via GitHub Actions (`check.yml`)


