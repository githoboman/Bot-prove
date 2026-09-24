# CasperProver — Originality Map

*Backlog 15.2.* What is genuinely ours, what we borrow, and where the
boundary sits. Judges want to know the delta between "we integrated
X" and "we invented Y." This is that map.

## Legend

- **built** — designed and written by the CasperProver team.
- **wrapped** — built on top of an upstream library, but with
  non-trivial code / configuration / composition around it.
- **used-as-is** — upstream dependency, essentially untouched.

## Cryptography

| Component                              | Status      | Upstream / rationale                                                            |
|----------------------------------------|-------------|---------------------------------------------------------------------------------|
| Groth16 pairing check (BN254)          | wrapped     | `github.com/consensys/gnark` — vetted, do not hand-roll                        |
| MiMC-preimage circuit                  | built       | R1CS circuit definition, witness gen, key derivation                            |
| PQ signature (SPHINCS+)                | wrapped     | Reference impl; audited primitive                                               |
| Hybrid Ed25519+SPHINCS+                | built       | Composition + verification order                                                |
| Poseidon hash                          | used-as-is  | Standard implementation                                                         |
| Merkle-inclusion proof                 | built       | Path serialization + property-based tests                                       |

## Chain integration

| Component                              | Status      | Upstream / rationale                                                            |
|----------------------------------------|-------------|---------------------------------------------------------------------------------|
| Casper deploy submitter                | wrapped     | `casper-go-sdk` v2; retry + idempotency added on top                             |
| Contracts (Odra)                       | wrapped     | Odra framework; storage layout + entry-points ours                              |
| `onchain.json` canonical manifest      | built       | Design + generator + judge one-liner                                            |
| Deploy pipeline                        | built       | Reproducible via `scripts/deploy_all.sh`                                        |

## Engine

| Component                              | Status      | Upstream / rationale                                                            |
|----------------------------------------|-------------|---------------------------------------------------------------------------------|
| ProofEngine (hash-based)               | built       | Cache + generator + eviction                                                    |
| Real ZK path                           | wrapped     | Around gnark                                                                    |
| Decision audit sink                    | built       | Chain-rooted, tamper-evident, secret-redacted                                   |
| Aggregation (STARK-pack)               | built       | Hash-chain design; STARK primitive off-the-shelf                                |
| Prompt-injection fixtures              | built       | Curated attack surface                                                          |

## API / integrations

| Component                              | Status      | Upstream / rationale                                                            |
|----------------------------------------|-------------|---------------------------------------------------------------------------------|
| HTTP router                            | used-as-is  | `net/http` stdlib                                                               |
| `/v1` alias + idempotency middleware   | built       | Ours; documented in `docs/API_CHANGELOG.md`                                     |
| CORS / auth / rate-limit               | built       | Middleware chain composition                                                    |
| CSPR.click auth                        | used-as-is  | Vendor SDK                                                                      |

## Frontend

| Component                              | Status      | Upstream / rationale                                                            |
|----------------------------------------|-------------|---------------------------------------------------------------------------------|
| Vite + React + TS scaffold             | used-as-is  |                                                                                  |
| Lab dashboard components               | built       | Overview / Proofs / Aggregation / KYC / PQ / Decisions                          |
| TrustBadge + honest-claims labelling   | built       | REAL / ON-CHAIN / SIMULATION                                                    |
| Client-side chain-root re-verify       | built       | `frontend/src/components/lab/Decisions.tsx`                                     |

## Operations

| Component                              | Status      | Upstream / rationale                                                            |
|----------------------------------------|-------------|---------------------------------------------------------------------------------|
| SLO catalogue                          | built       | `docs/SLO.md`                                                                    |
| Secret handling policy                 | built       | `docs/SECRET_HANDLING.md`                                                        |
| Security review checklist              | built       | `docs/SECURITY_REVIEW_CHECKLIST.md`                                              |
| SBOM generator                         | wrapped     | CycloneDX generators; convention ours                                           |
| gitleaks / semgrep / trivy pipeline    | wrapped     | Upstream tools; policy ours                                                     |

## What we deliberately did NOT invent

- We do NOT claim a novel pairing / curve / hash. Real crypto rides on gnark + audited SPHINCS+.
- We do NOT claim a novel consensus / L2. We anchor to Casper mainnet-testnet.
- We do NOT claim a novel LLM. The audit layer covers ANY upstream LLM as a hashed request/response.

## Traceability

Every "built" row has:

1. A commit range or PR link in the repo history.
2. At least one test in the codebase named after the invariant.
3. A row in `docs/DATA_ROOM.md` pointing at the artefact.

If a "built" row lacks any of the three, that's a gap — open a
"originality debt" ticket.
