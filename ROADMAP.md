# CasperProver — Roadmap

> Cryptographic proof engine for AI inference verification on Casper Network

---

## ✅ Shipped

- [x] 7 smart contracts on Casper testnet (proof-registry, verifier-gate, defi-mock, stake-slashing, proof-aggregation, model-registry, proof-of-inference)
- [x] 3 additional contracts written, not yet deployed (zk-verifier, stake-slashing-session, governance)
- [x] Go API server — 32 endpoints, PostgreSQL persistence, rate limiting
- [x] Merkle tree builder (SHA-256, configurable depth, <50ms)
- [x] Real BN254 Groth16 ZK proofs via gnark (R1CS circuit, trusted setup, pairing verification)
- [x] Post-quantum signing: ML-DSA-65 (FIPS 204), hybrid Ed25519 + ML-DSA-65, Lamport OTS
- [x] Hash-chain batch aggregation with Postgres persistence
- [x] Proof-chain DAG validation (cycle detection, input continuity, single root)
- [x] KYC whitelisting via Merkle inclusion (cross-contract, persistent)
- [x] DeFi vault gated by verifier-gate (admin-only `grant_access`, typed `AccountHash`)
- [x] Stake-slashing — real CSPR economic penalty (20% slash, permissionless bounty)
- [x] Go SDK — 32 methods, 1:1 API mapping
- [x] MCP server — 32 tools with full InputSchema definitions
- [x] React lab at casperprover.xyz — 11 interactive tabs, all calling real API
- [x] Casper Wallet integration with demo fallback
- [x] 194 tests (172 Go + 22 Rust)
- [x] CI via GitHub Actions
- [x] API-key auth, rate limiting (60 req/min), input validation
- [x] 248+ testnet transactions

## 🔜 Next

- [x] Deploy `proof-of-inference`, `model-registry`, `proof-aggregation` contracts (see `docs/roadmap/DEPLOYMENT_VERIFY_2026-07-25.md`)
- [x] Smoke-call read-only entrypoints on the 3 new contracts (`docs/roadmap/SMOKE_VERIFY_2026-07-26.md`) — all three callable on-chain from Anna's account, `error_message: none`.
- [ ] Full model-inference ZK circuit (beyond MiMC preimage)
- [ ] Production trusted setup ceremony
- [ ] STARK recursive aggregation (pending mature Go STARK library)
- [x] Demo/Real data toggle in lab
- [x] PQ signature key rotation + versioning (`docs/PQ_KEYRING.md`) — keyring, migration primitive, HTTP surface. In-memory storage; HSM/KMS integration deferred.
- [x] PQ keystore adapter (`docs/KEYSTORE.md`) — `Keystore` interface (`engine/internal/crypto/keystore/`) + 3 backends: `memory` (default), `file` (ChaCha20-Poly1305 at rest, Argon2id KDF), `remote` HSM/KMS gateway stub with a documented HTTP contract. Real HSM driver still lives out of tree per deployment.
- [x] Nova / folding aggregation **harness** (`docs/roadmap/NOVA_HARNESS.md`) — API contract + hash-chain stand-in labelled `hash-fold-v1`. A real Nova is still a future item and requires a Pallas/Vesta curve cycle in Go.
- [x] A2A provider pool + HTTP provider adapter + HITL policy service (`docs/DECISION_A2A_HITL.md`) — `decision.ProviderPool`+`Router` with trust levels (`system`/`delegated`/`observational`) and per-facet capabilities; `HTTPProviderAdapter` (JSON-over-HTTP contract, fixture fallback); `hitl.Service` with declarative policy (veto on critical REJECT / escalate on critical ABSTAIN or low confidence) and a `TicketStore` interface (in-memory default; Postgres-backed drop-in). Opt-in via `CP_DECISION_ENABLE=1`; endpoints: `POST /v1/decision/evaluate`, `GET /v1/decision/pool`, `GET/POST /v1/hitl/tickets*`.
- [x] Merkle-recursion aggregation (`docs/MERKLE_RECURSION.md`) — SHA-256 Merkle tree over proof-commitment digests, labelled `merkle-recursion-v1`. Domain-separated leaf/interior tags (0x00 / 0x01); Bitcoin-style odd-count padding. Verifier does O(log n) SHA-256 hashes on an inclusion path against the aggregate root; aggregate itself is O(1) size regardless of k. NOT a STARK recursion (does not re-verify the underlying proof — only proves inclusion of the commitment); the `stark-recursion-v1` label remains reserved for a future real recursive STARK. HTTP surface: `POST /v1/aggregation/merkle-aggregate`, `POST /v1/aggregation/merkle-inclusion`, `POST /v1/aggregation/merkle-verify`.
- [x] Pedersen fold on BLS12-381 G1 (`docs/PEDERSEN_FOLD.md`) — intermediate cryptographic upgrade of `hash-fold-v1`, labelled `pedersen-fold-v1`. Two independent generators (`G` canonical, `H` = HashToG1("CP_PED_H_V1")); per-step scalars `m_i, r_i` derived by domain-separated SHA-256 hashes; accumulator `C = Σ (m_i·G + r_i·H)`. Real cryptographic binding under DLP + real homomorphism across splits (`PedersenHomomorphismCheck`). NOT a Nova folding scheme (does not reduce R1CS instances); the `nova-go-v1` label remains reserved for the eventual real Nova. HTTP surface reused: `POST /v1/aggregation/fold` and `POST /v1/aggregation/verify-fold` dispatch on the `scheme` field (`hash-fold-v1` default, `pedersen-fold-v1` opt-in).
- [x] BLS12-381 threshold quorum registry (`docs/BLS_QUORUM.md`) — real BLS aggregate signatures over BLS12-381 (`engine/internal/quorum/`): keypair generation over `Fr`, pubkey in `G2`, hash-to-curve via SSWU (RFC 9380), pairing check `e(H(m), pk_agg) == e(agg_sig, G2)`. Thread-safe `Registry` with `active → slashed`/`removed` lifecycle, idempotent `Slash`, active-count-aware `ByzantineThreshold(n) = ⌊2n/3⌋+1` (clamped). `VerifyQuorum` emits a canonical `QuorumWitness` (SHA-256 commitment over deterministic serialisation) whose hash is order-invariant across bitset shuffles. Opt-in via `CP_QUORUM_ENABLE=1`; endpoints: `POST /v1/quorum/signers`, `GET /v1/quorum/signers`, `POST /v1/quorum/signers/{id}/slash`, `POST /v1/quorum/signers/{id}/retire`, `POST /v1/quorum/verify`, `GET /v1/quorum/threshold`. Reserved scheme label `bls12-381-tss-v1` for future DKG-based TSS; today the shipped label is `bls12-381-g1-agg-v1`. On-chain BLS pairing verifier requires a Casper VM precompile and remains a follow-up — the current wire commits `witness_hash_hex` on-chain (same trust model as receipts).
- [x] Admin dashboard rollup endpoint (`docs/roadmap/ADMIN_SUMMARY.md`) — `GET /v1/admin/summary` returns a single read-only rollup (subsystems on/off, keystore info + key metadata, webhook aggregate state, scopes summary, contract addresses). Never leaks secrets; enforced by `TestAdminSummary_NoSecretsInPayload`. Scope: `admin:read`. This is the engine side of the FE admin dashboard — no FE changes ship in this slot.
- [x] Webhook subsystem Prometheus instrumentation (`docs/roadmap/WEBHOOK_METRICS.md`) — `cp_webhook_*` counters (enqueued/attempts/delivered/dead_lettered/replayed), attempt-duration histogram (label: event + status_class), live queue/dead-letter depth gauges. Same `/metrics` endpoint as `cp_http_*`; zero new module deps.
- [x] Formal-verification scaffolding (`docs/roadmap/FORMAL_VERIFICATION.md`) — 3 TLA+ specs (`ProofSystemSpec` — proof-registry state machine; `QuorumSpec` — BLS quorum registry; `ReceiptLineageSpec` — lineage DAG), a portable `specs/run-tlc.sh` driver, and a `.github/workflows/formal-verification.yml` workflow that runs TLC on every push and PR touching `specs/`. Full formal verification of the whole system remains a multi-month effort; this slot makes formal verification an always-green CI signal so drift is caught the moment a spec breaks.
- [x] Provenance-lineage receipts (`docs/PROVENANCE_LINEAGE.md`) — `receipts.Service` signs each decision under the active PQ key and emits it in three shapes: internal (`DecisionReceipt`), W3C Verifiable Credentials 2.0 (`ToW3CVC`), and Agent Receipt draft v0.3 (`ToAgentReceipt`). Canonical, sort-normalised, length-prefixed hash makes the receipt verifiable independent of JSON key order. In-memory `Store` (Postgres drop-in ready) tracks receipt lineage via `Ancestors()` DAG walk. `OtelSink` interface with a JSONL implementation (attribute names align with an OTel span). Opt-in via `CP_RECEIPTS_ENABLE=1`; endpoints: `POST /v1/receipts/emit`, `GET /v1/receipts/{id}`, `GET /v1/receipts/{id}/lineage`, `GET /v1/receipts/{id}/w3c-vc`, `GET /v1/receipts/{id}/agent-receipt`. Sink target set via `CP_RECEIPTS_JSONL`.

## 🔮 Future

- [ ] Multi-chain anchoring (EVM, Solana, Cosmos)
- [ ] Hardware attestation (TPM 2.0, Intel SGX, AMD SEV)
- [ ] Distributed prover network with MPC threshold
- [ ] Proof marketplace for verified computation
- [ ] Mainnet deployment with gas optimization
- [ ] Formal verification (TLA+ specification)
- [ ] Cross-chain proof bridging
- [ ] Integration with model registries (HuggingFace, Replicate)
