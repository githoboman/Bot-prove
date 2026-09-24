# Architecture — Extensions (post-Gate 6)

Companion doc to [`ARCHITECTURE.md`](./ARCHITECTURE.md). Captures the
subsystems added *after* the initial four deployed contracts + the KYC
demo — trust boundaries, data flow, and the honest label (REAL vs
SIMULATION vs ON-CHAIN) for each.

Consumers of this doc: hackathon judges wanting a component map without
digging through code; new contributors deciding where a change belongs.

---

## Component Map

```
┌────────────────────────────────────────────────────────────────────┐
│                         External surface                          │
│                                                                    │
│   Frontend (SPA, PWA)     SDK (Go / Python / TS)     verify.sh    │
│         │                        │                       │        │
└─────────┼────────────────────────┼───────────────────────┼────────┘
          ▼                        ▼                       ▼
┌────────────────────────────────────────────────────────────────────┐
│                         engine/internal/api  (Go)                 │
│                                                                    │
│  auth ──▶ scope ──▶ idempotency ──▶ handler ──▶ decision ──▶ zk   │
│    ▲       ▲            ▲             │                  │        │
│    │       │            │             ▼                  ▼        │
│  X-API   scope     X-Idempot.       aggregator       prover/      │
│   Key    Registry     Store        (batch merkle)    verifier     │
└────────┬───────────────────────────────┬──────────────────────────┘
         │                               │
         ▼                               ▼
   Postgres store                Casper Testnet
   (aggregation_batches,        (proof-registry,
    proof_ledger,                verifier-gate,
    idempotency_keys)            stake-slashing,
                                 defi-mock)
```

## Trust Boundaries

| Boundary                         | Kind    | Enforcement                                             |
| -------------------------------- | ------- | ------------------------------------------------------- |
| SPA/PWA ↔ API                    | Network | HTTPS + CORS; API key on writes                         |
| SDK ↔ API                        | Network | X-API-Key + X-Idempotency-Key + `Accept: application/vnd.cp.v1+json` |
| API ↔ Postgres                   | Network | Password + TLS; least-privilege role                    |
| API ↔ Casper Testnet             | Network | Signed deploy with `defi_mock_owner` PEM                |
| Frontend `TrustBadge`            | UX      | Every crypto claim tagged REAL / ON-CHAIN / SIMULATION  |
| verify.sh                        | Client  | Reproduces `chain_root_sha256` against `onchain.json`   |

## New Middleware Stack

Order matters — each layer assumes the one above has run.

1. **`corsMiddleware`** — permissive for demo; tightens in prod via config.
2. **`authMiddleware`** — X-API-Key single-shared-secret (legacy).
3. **`scopeGate(required, …)`** — role-lite (7.11); per-key
   `{tenant_id, scopes[]}` from `ScopeRegistry`. No-op unless the
   operator populates the registry at startup. Read-only methods
   (GET/HEAD/OPTIONS) bypass — same policy as authMiddleware.
4. **`idempotencyMiddleware`** — `X-Idempotency-Key` (7.4/7.6/7.12/7.13);
   in-memory 15-min TTL; 409 on request-body drift. `X-CP-API-Version:
   v1` and `X-CP-Deprecation` land here.
5. **Handler** — route-specific logic.

## Decision Layer (3.2 / 3.7)

`engine/internal/decision/` is the tamper-evident audit trail:

- **Request hash** = canonical-JSON(sha256) over the decision request.
- **Response hash** = canonical-JSON(sha256) over the decision response.
- **Redaction** = `api_key`, `token`, `pii`, `email`, `borrower_name`
  are hard-redacted before logging.
- **Chain root** = SHA-256(SHA-256(entry_0) || SHA-256(entry_1) || …).
  Frontend re-verifies via `SubtleCrypto` (see `TrustBadge`).
- **Endpoints** — `POST /decisions/log`, `GET /decisions/log[/{id}[/lineage]]`.

Every decision is bit-reproducible via `cmd/cp-repro` (see [15.3](./JUDGE_GUIDE.md)).

## PWA / Offline Verify (9.3 / 9.4)

`frontend/src/OfflineVerify.tsx` + `frontend/public/manifest.json` +
`frontend/public/sw.js` — cache-first application shell, network-only
API. Judge can install the PWA, unplug the network, paste an
`onchain.json` blob, and re-verify SHA-256 chain root locally. No API
calls happen offline.

## Trust Labels

| Label        | Meaning                                                          | Where enforced |
| ------------ | ---------------------------------------------------------------- | -------------- |
| REAL         | Real cryptography executed at call time                          | `TrustBadge`   |
| ON-CHAIN     | State lives on Casper Testnet (query `deploy-out/onchain.json`)  | `TrustBadge`   |
| SIMULATION   | Deterministic stub; no external primitive; honest by-design      | `TrustBadge`   |

Every user-facing surface that advertises a crypto/on-chain claim must
attach the corresponding badge. When a subsystem is upgraded (e.g. a
mock lifted to a real primitive), the badge must be flipped in the
same commit.

## Contribution Signals

- Every new component **must** ship with a paired test (Go/Rust).
- Every new externally-facing surface **must** carry a trust label.
- Every new cross-boundary call **must** appear in [Trust Boundaries](#trust-boundaries).

---

Related:

- [`ARCHITECTURE.md`](./ARCHITECTURE.md) — original component map
- [`JUDGE_GUIDE.md`](./JUDGE_GUIDE.md) — how a judge runs the demo
- [`SLO.md`](./SLO.md) / [`SECRET_HANDLING.md`](./SECRET_HANDLING.md) — ops discipline
- [`CONTRIBUTING.md`](../CONTRIBUTING.md) — checklist for new contributors
