# Multi-chain anchoring — SIMULATION stubs

**Status:** interface + 3 simulation adapters. Live cross-chain anchoring
deferred to mainnet launch (see `docs/MAINNET_LAUNCH_PLAN.md`).

**Backlog items closed:** 5.4 (interface layer), 5.5 (Solana + Cosmos stubs).

## Trust labels

Every adapter self-labels via `Adapter.Label()`:

| Chain ID       | Adapter                | Label       |
|----------------|------------------------|-------------|
| `casper-test`  | `CasperAdapter`        | ON-CHAIN    |
| `eth-sim`      | `EthereumStubAdapter`  | SIMULATION  |
| `solana-sim`   | `SolanaStubAdapter`    | SIMULATION  |
| `cosmos-sim`   | `CosmosStubAdapter`    | SIMULATION  |

Only `casper-test` is a real anchor. The three stubs exist so the SDK,
frontend, and integration tests can prove the anchor path is
chain-agnostic without paying gas or requiring a live node. **Judges,
FE, and demos must render the SIMULATION badge for any receipt whose
`Label` is `SIMULATION`.**

## Determinism

Each stub emits a pseudo-tx-hash that is a pure function of the anchor
request fields (`ProofID`, `ProofHash`, `MerkleRoot`, `VKHash`,
`Verdict`, `ModelID`) plus the chain-id prefix. Same input → same
output, always. Prefixes differ per chain, so anchoring the same proof
via `eth-sim`, `solana-sim` and `cosmos-sim` yields three distinct
pseudo-hashes — a caller cannot conflate chains.

## Format-per-chain

| Chain      | Preimage prefix | Digest    | On-wire format                            |
|------------|-----------------|-----------|-------------------------------------------|
| eth-sim    | none            | SHA-256   | `0x` + 64 lowercase hex chars (66 total)  |
| solana-sim | `solana|`       | SHA-512   | base58 of 64 bytes (~86–88 chars)         |
| cosmos-sim | `cosmos|`       | SHA-256   | UPPERCASE 64 hex chars (Tendermint style) |

The on-wire format mirrors what each real chain exposes in its
explorer so downstream tests can catch a code path accidentally
routing through the wrong chain.

## When these stubs become real anchors

Blocked on:

- **Solana:** RPC key + a funded devnet keypair. Cost: 0 (devnet).
- **Cosmos:** RPC endpoint of a permissive testnet (Cosmos Hub 4
  testnet or Osmosis testnet). Cost: 0 (testnet).
- **Ethereum / Base / Polygon:** RPC provider (Alchemy / Infura free
  tier) + funded testnet keypair. Cost: 0 (Sepolia / Base Sepolia).

None of the above require paid services for a first live anchor — the
gate is a scheduled roll-out, not budget. Tracked in
`docs/MAINNET_LAUNCH_PLAN.md`.

## Router usage

```go
r := chainadapter.NewRouter("casper-test")
r.Register(NewCasperAdapter(...))
r.Register(NewEthereumStub("eth-sim"))
r.Register(NewSolanaStub("solana-sim"))
r.Register(NewCosmosStub("cosmos-sim"))

receipt, err := r.Anchor("solana-sim", chainadapter.AnchorRequest{
    ProofID:   "proof-123",
    ProofHash: "deadbeef...",
    Verdict:   "APPROVE",
    ModelID:   "model-42",
})
// receipt.Label == SIMULATION → FE must render the SIMULATION badge.
```
