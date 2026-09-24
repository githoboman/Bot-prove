# Architecture

## System Overview

```mermaid
graph TB
    subgraph "Frontend (Vite + TypeScript)"
        UI[11 Interactive Tabs]
        WC[CSPR.click Wallet]
    end

    subgraph "Proof Engine (Go)"
        API[32 REST Endpoints]
        MK[Merkle Builder]
        ZK[Groth16 Verifier<br/>gnark BN254]
        PQ[PQ Crypto<br/>ML-DSA-65 · Lamport OTS]
        AG[Batch Aggregator]
        PC[Proof Chain<br/>DAG Validator]
        INF[Inference Service]
        KYC[KYC Demo]
    end

    subgraph "Smart Contracts (Rust / Casper 2.x)"
        PR[proof-registry]
        VG[verifier-gate]
        DM[defi-mock]
        SS[stake-slashing]
        PA[proof-aggregation]
        MR[model-registry]
        POI[proof-of-inference]
        GOV[governance]
    end

    subgraph "Integration Layer"
        SDK[Go SDK · 32 methods]
        MCP[MCP Server · 32 tools]
    end

    subgraph "Storage"
        PG[(PostgreSQL)]
        CS[Casper Testnet]
    end

    UI --> API
    WC --> CS
    API --> MK & ZK & PQ & AG & PC & INF & KYC
    INF --> PR
    INF --> POI
    VG --> PR
    DM --> VG
    SS --> PR
    AG --> PA
    API --> MR
    API --> GOV
    API --> PG
    SDK --> API
    MCP --> API
    PR & VG & DM & SS & PA & MR & POI & GOV --> CS
```

## Proof Generation

```mermaid
sequenceDiagram
    participant A as AI Agent
    participant E as Engine
    participant H as Hasher (SHA-256)
    participant M as Merkle Tree
    participant C as Casper Network

    A->>E: (input, output, model)
    E->>H: hash(input), hash(output), hash(model)
    H-->>M: leaf_0, leaf_1, leaf_2
    M->>M: Build tree bottom-up
    Note over M: root = H(H(leaf_0||leaf_1) || leaf_2)
    M-->>E: Proof{root, path, leaves}
    E->>C: deploy(proof_hash, merkle_root)
    C-->>E: deploy_hash (immutable)
    E-->>A: Proof{id, root, path, verified, deploy_hash}
```

## ZK Proof Flow (Groth16)

```mermaid
sequenceDiagram
    participant C as Client
    participant E as Engine
    participant G as gnark (BN254)

    C->>E: POST /zk/groth16-real/prove {secret: 42}
    E->>G: Compile R1CS circuit (MiMC)
    G->>G: Groth16 Setup (pk, vk)
    G->>G: Groth16 Prove (pk, witness)
    G-->>E: π = (A, B, C) ∈ G1×G2×G1
    E-->>C: {proof, public_hash, verification_key}

    C->>E: POST /zk/groth16-real/verify {proof, vk, public_inputs}
    E->>G: Groth16 Verify (vk, π, public)
    G->>G: Pairing check: e(A,B) = e(α,β)·e(L,γ)·e(C,δ)
    G-->>E: valid = true
    E-->>C: {valid: true}
```

## KYC Whitelisting Flow

```mermaid
sequenceDiagram
    participant U as User Agent
    participant E as Engine
    participant PR as proof-registry
    participant VG as verifier-gate
    participant DM as defi-mock

    U->>E: prove(kyc_input, kyc_result, model)
    E->>PR: submit_proof(root, agent, metadata)
    PR-->>E: proof_id

    U->>DM: check_kyc(user, proof_id)
    DM->>VG: verify_proof(proof_id)
    VG->>PR: get_proof(proof_id)
    PR-->>VG: proof data
    VG-->>DM: is_valid = true

    DM->>DM: grant_access(user)
    Note over DM: User whitelisted for DeFi
```

## Stake & Slash Flow

```mermaid
sequenceDiagram
    participant Ag as Agent
    participant SS as stake-slashing
    participant PR as proof-registry
    participant Rp as Reporter

    Ag->>SS: stake(5 CSPR)
    Note over SS: Agent's stake recorded

    Ag->>PR: revoke_proof(proof_id)
    Note over PR: Proof marked as revoked

    Rp->>SS: report_and_slash(agent, proof_id)
    SS->>PR: get_proof_status(proof_id)
    PR-->>SS: status = revoked
    SS->>SS: slash 20% of stake
    SS->>Rp: transfer 1 CSPR bounty
    Note over SS: proof_id tombstoned (no re-slash)
```

## Verification Logic

```
Given: leaf L, path P = [p_0, p_1, ...], root R

verify(L, P, R):
    h = H(L)
    for p_i in P:
        h = H(h || p_i)   if index is even
        h = H(p_i || h)   if index is odd
    return h == R
```

## Security posture

Ownership model per contract, cross-contract call graph, reentrancy analysis and
renounce/timelock policy live in [`docs/SECURITY_AUDIT.md`](./SECURITY_AUDIT.md).
Short version: single-owner-per-contract, no irreversible `renounce_ownership`
anywhere, every cross-contract call is either read-only or a native purse
transfer (no callback surface). Any new privileged entry point MUST be added to
that audit before it merges.

Ownership cheat sheet (full detail + verdicts in `SECURITY_AUDIT.md` §1):

| Contract | Admin key | Mutable? | Renounce? | Verdict |
|---|---|---|---|---|
| `defi-mock` | `ADMIN_KEY` (deployer) | No | No | Safe |
| `verifier-gate` | none | — | — | Safe (read-only proxy) |
| `stake-slashing` | none | — | — | Safe (permissionless) |
| `proof-registry` | none (per-record `agent`) | per-record | No | Safe |
| `proof-of-inference` | `INSTALLER_KEY` | No | No | Advisory: hot deployer key |
| `model-registry` | `INSTALLER_KEY` + per-model `owner` | Per-model transferable | No | Safe, dual-tier |
| `proof-aggregation` | `INSTALLER_KEY` | No | No | Advisory: hot deployer key |
| `governance` | `owner`, 48h-timelocked transfer | Yes | No | Safe — see §2.9 |
| `zk-verifier` | `owner` (redeployed 2026-07-28 to dedicated wallet) | No direct rotation | No | Fixed and verified live — see §2.10 |

No contract exposes `renounce_ownership()` today — intentional, see
`SECURITY_AUDIT.md` §1 for the three preconditions that must ship before one
is ever added.
