# Security

## Reporting

Report security issues via [GitHub Security Advisories](https://github.com/anna-stolbovskaja/CasperProver/security/advisories/new).

Do not open public issues for vulnerabilities.

## Self-Audit Summary

| Layer | Component | Status | Notes |
|---|---|---|---|
| **Contracts** | proof-registry | ✅ Audited | checked arithmetic, u64 counters, input validation |
| | verifier-gate | ✅ Audited | rate-limit per block (`MAX_VERIFY_PER_BLOCK=100`), u64 counters |
| | stake-slashing | ✅ Audited | `checked_add`/`checked_sub` on U512 amounts, slash guard |
| | defi-mock | ✅ Audited | access-control, whitelisting only by contract owner |
| **Backend** | API server | ✅ Hardened | rate-limit middleware, CORS, auth middleware, slog structured logging |
| | genID() | ✅ Fixed | panic → graceful timestamp-based fallback |
| | Proof engine | ✅ | Merkle-tree integrity, SHA-256 leaf hashing |
| | ZK (gnark) | ✅ | BN254 MiMC circuit, real Groth16 prove+verify |
| | PQ crypto | ✅ | Ed25519+ML-DSA-65 hybrid sign, Lamport-OTS |
| | Submitter | ✅ | Casper RPC integration, secp256k1 signing |
| **Frontend** | XSS | ✅ | React auto-escaping, no `dangerouslySetInnerHTML` |
| | ErrorBoundary | ✅ | Catches Lab errors, prevents blank screens |
| | Secrets | ✅ | No secrets in frontend; API key is server-side only |
| **Infra** | Render | ✅ | HTTPS-only, env vars for secrets |
| | Vercel | ✅ | Static frontend, no server-side secrets |
| | CI | ✅ | `go test -race`, `go vet`, build verification |

## Threat Model

| Threat | Mitigation |
|---|---|
| Overflow in contract arithmetic | All U512 ops use `checked_add`/`checked_sub` with revert on overflow |
| Double-slash same proof | `SLASHED_DICT` tracks by `proof_id`, reverts on duplicate |
| Unauthorized revocation | `revoke_proof` checks caller == original submitter |
| API abuse | Rate-limit middleware (60 req/min per IP, sliding 1-min window) — `engine/internal/api/server.go:250-289` |
| Server crash on bad RNG | `genID()` falls back to timestamp instead of panicking |
| Log injection | All logging via `slog` structured JSON, no user-controlled format strings |

## Scope

- **Contracts:** proof-registry, verifier-gate, defi-mock, stake-slashing, proof-of-inference, model-registry, proof-aggregation, stake-slashing-session
- **Engine:** hasher, prover, verifier, submitter, ZK (gnark), PQ crypto (ML-DSA-65, Lamport-OTS)
- **SDK:** client.go, python_client.py, mcp_server.go

## Keys

Never commit deployer secrets. `.env.example` has placeholders only. Testnet keys are disposable.

## Full Threat Model

See [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md) for asset inventory, actor
trust boundaries, per-surface mitigations, and accepted-risk rationale.

## Limitations

See [docs/KNOWN_LIMITATIONS.md](docs/KNOWN_LIMITATIONS.md).
