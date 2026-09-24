# CasperProver — Data Room Index

*Backlog 14.4, 14.5.* Single-page pointer to every artefact a judge
(or investor / auditor / integrator) needs to independently verify
the project. Path-based, no dead links.

## 1. Positioning & claims

| Artefact                                | Path                                    | Purpose                                                                 |
|-----------------------------------------|-----------------------------------------|-------------------------------------------------------------------------|
| One-line pitch                          | `README.md` § tagline                 | REAL / ON-CHAIN / SIMULATION labels are stated up front                 |
| Judge guide                             | `docs/JUDGE_GUIDE.md`                   | 8-criteria map + step-by-step reproduce                                 |
| Claim boundary                          | `docs/KNOWN_LIMITATIONS.md`             | What is honestly limited                                                |
| Regulatory posture                      | `docs/JUDGE_GUIDE.md` § regulatory   | MiCA / EU AI Act / GDPR / FinCEN / NIST AI RMF alignment                |

## 2. Reproducibility

| Artefact                                | Path                                    | Purpose                                                                 |
|-----------------------------------------|-----------------------------------------|-------------------------------------------------------------------------|
| One-command judge script                | `scripts/judge_demo.py`                 | Runs the end-to-end vertical slice                                      |
| Verification harness                    | `verify.sh`                             | 8/8 pass locally without secrets                                        |
| Canonical on-chain manifest             | `deploy-out/onchain.json`               | Source of truth for deployed hashes                                     |
| SBOM                                    | `sbom/` (CycloneDX)                     | Dependency graph for Go / Node / Rust                                   |

## 3. Contracts

| Artefact                                | Path                                    | Purpose                                                                 |
|-----------------------------------------|-----------------------------------------|-------------------------------------------------------------------------|
| Deployed contracts index                | `docs/TX_MANIFEST.md`                   | 9 deployed, 0 undeployed                                                |
| Owner lifecycle design                  | `docs/OWNER_LIFECYCLE.md`               | Recovery / timelock instead of irreversible renounce                    |
| Contract invariants                     | `docs/CONTRACT_INVARIANTS.md`           | Global + cross-contract invariants                                      |
| Formal verification status              | `docs/FORMAL_VERIFICATION.md`           | What is (and isn't) verified today                                      |

## 4. Cryptography & engine

| Artefact                                | Path                                    | Purpose                                                                 |
|-----------------------------------------|-----------------------------------------|-------------------------------------------------------------------------|
| ZK real vs sim                          | `engine/internal/zkverifier/`           | Gnark BN254 Groth16 real; sim endpoints self-labeled                    |
| PQ crypto                               | `engine/internal/crypto/`               | SPHINCS+ reference + hybrid                                             |
| Decision audit                          | `engine/internal/decision/`             | Hashed request/response + chain-rooted tamper-evidence                  |

## 5. API & SDKs

| Artefact                                | Path                                    | Purpose                                                                 |
|-----------------------------------------|-----------------------------------------|-------------------------------------------------------------------------|
| OpenAPI                                 | `docs/openapi.yaml`                     | Machine-readable API contract                                           |
| API versioning + changelog              | `docs/API_CHANGELOG.md`                 | `/v1` + deprecation timeline                                            |
| SDK — Go                                | `sdk/go/`                               |                                                                          |
| SDK — Python                            | `sdk/python/`                           |                                                                          |
| SDK — TypeScript                        | `sdk/typescript/`                       |                                                                          |

## 6. Security posture

| Artefact                                | Path                                    | Purpose                                                                 |
|-----------------------------------------|-----------------------------------------|-------------------------------------------------------------------------|
| Threat model                            | `SECURITY.md`                           | STRIDE-shaped surface                                                   |
| Secret handling policy                  | `docs/SECRET_HANDLING.md`               | Classification / storage / rotation                                     |
| Security review checklist               | `docs/SECURITY_REVIEW_CHECKLIST.md`     | Per-PR gate                                                             |
| SLO catalogue                           | `docs/SLO.md`                           | Availability / latency / correctness targets                            |
| Compliance baseline                     | `docs/COMPLIANCE.md` (planned)          | MiCA / EU AI Act / GDPR / NIST AI RMF checklist                         |

## 7. Team

| Artefact                                | Path                                    | Purpose                                                                 |
|-----------------------------------------|-----------------------------------------|-------------------------------------------------------------------------|
| Owner                                   | `README.md` § authors                 | anna-stolbovskaja                                                       |
| Contribution guide                      | `CONTRIBUTING.md`                       |                                                                          |
| License                                 | `LICENSE`                               |                                                                          |

## 8. Roadmap & governance

| Artefact                                | Path                                    | Purpose                                                                 |
|-----------------------------------------|-----------------------------------------|-------------------------------------------------------------------------|
| Roadmap                                 | `ROADMAP.md`                            | Post-hackathon direction                                                |
| Backlog                                 | `docs/BACKLOG.md` (planned surface)     | Consolidated task universe                                              |
| Changelog                               | `CHANGELOG.md`                          | Every user-visible change                                               |
| Originality map                         | `docs/ORIGINALITY_MAP.md`               | What we built vs what we integrated                                     |

---

**Freshness:** any judge should be able to open a file, hash it, and
match the hash against `deploy-out/onchain.json` (for on-chain items)
or the commit SHA (for docs/code). If anything on this page is stale,
open a "Data-room drift" ticket and flag which row.
