# Multi-verifier gossip layer (BC / backlog 12.2)

**Status: DRAFT design spec, no code. This is a post-hackathon,
post-invest capability tracked in KNOWN_LIMITATIONS.md.**

## Why this document exists

The backlog labels BC as *deferred*: multi-verifier gossip requires a
real peer-to-peer network layer, peer discovery, and Byzantine-tolerant
propagation. Shipping any of that in the hackathon window would be
either honest-but-toy (a single-region libp2p mesh with no adversarial
model) or dishonest-but-flashy (a claim of "decentralised verifier
network" backed by a couple of hardcoded peer URLs).

Instead, we distil the design here so:

- the tradeoffs are pinned before any code is written;
- an auditor and an investor can read the intended architecture
  without inferring it from a partial prototype;
- when we do ship it (Gate G4 in `docs/MAINNET_LAUNCH_PLAN.md`,
  post-invest) the target is fixed.

**Nothing about the CasperProver hackathon submission depends on this
document. The current verifier surface — `internal/verifier` + the
`/verify` endpoint — is a single-node, single-verifier implementation
and is honestly labelled as such.**

## Goal

A CasperProver proof is currently verified locally by whichever engine
the caller happens to hit. That is fine for a hackathon MVP and honest
about its centralisation. Multi-verifier gossip changes the trust
model to:

> A verification is valid iff a threshold `t` of `n` independent
> verifier nodes, chosen from a live-and-registered set, agree on the
> outcome and gossip that agreement to a quorum of the network.

The properties we want:

1. **Attester dishonesty is publicly detectable.** If an attester
   emits two conflicting proofs (same input, different output hash),
   any two honest verifiers can gossip both proofs and any
   verifier — including a fresh one — can adjudicate.
2. **Verifier dishonesty is publicly detectable.** If a verifier
   signs a "verified" attestation on a proof that fails another
   verifier's replay, the disagreement itself is gossiped and both
   attestations are on record for slashing.
3. **Liveness under partial failure.** As long as `> t` verifiers are
   live and connected to the gossip mesh, verification finalises
   within a bounded window.
4. **Sybil-resistance.** A single actor can't unilaterally add
   verifiers to the live set. Membership is either PoS-anchored
   (stake tied on Casper via `stake_slashing`) or governance-anchored
   (M-of-N Adjudicator quorum from `REPUTATION_ECONOMICS.md` § 4).
5. **Deterministic replay.** A verifier node that comes back online
   after a network partition can replay the gossip log and land on
   the same finalised outcome as everyone else, no reconciliation
   protocol needed.

## Non-goals (deliberately excluded from this design)

- A currency-of-account or fee market. Verifier reward economics
  live in `REPUTATION_ECONOMICS.md` and are referenced by id here,
  not duplicated.
- Confidential verification (verify without seeing the payload).
  That is the ZK-ML rung, tracked in `ZKML_HONEST_VERDICT.md`.
- A NAT-traversal / hole-punching layer. Verifier nodes are assumed
  to run in operator-controlled infrastructure with routable
  addresses. Bootstrap discovery uses a small registry of seed
  hosts, not DHT crawling.

## Network layer (draft, not built)

Two candidate substrates evaluated:

### Candidate A — libp2p gossipsub

- Mature Go implementation, actively used by Ethereum consensus
  clients, IPFS, and Filecoin.
- Gossipsub v1.1 has score-based mesh maintenance which is close to
  what we want for verifier reputation gating.
- Cost: heavy dep tree (~90 transitive Go modules), non-trivial
  observability surface, has known DoS-hardening lore we would have
  to absorb.

### Candidate B — CRDT-first minimal mesh

- Handwritten HTTP-long-poll + append-only log per verifier, with a
  merkleised join over the log for reconciliation.
- Cost: we would be writing our own gossip layer. History of
  wrong-in-subtle-ways-until-year-three is discouraging.
- Benefit: zero external dep, deterministic replay is trivial
  (CRDT semantics), no gossipsub-specific attack surface.

**Preliminary verdict:** libp2p gossipsub is the least-bad choice
for a real deployment. It buys us peer discovery, mesh maintenance,
and message deduplication for free. The CRDT alternative is only
credible if we discover a specific libp2p attack we cannot mitigate.

## Adversarial model

We assume a partially-synchronous model with Byzantine faults up to
`f < n/3` where `n` is the size of the current verifier live set.
The threshold `t` is set to `2f + 1` — the classical BFT boundary.

Attackers can:

- Sign arbitrary messages with their own verifier keys.
- Refuse to broadcast some messages.
- Reorder incoming messages.
- Withhold responses until a deadline.

Attackers can NOT:

- Forge signatures of honest verifiers (standard crypto assumption).
- Break the underlying Casper anchor's finality (that is the base
  trust root, already assumed by the whole engine).

## Verifier lifecycle

1. **Registration.** Prospective verifier posts a stake bond on
   `stake_slashing` (structural stub today, formal slashing spec
   in `SLASH_EQUIVOCATION_DRAFT.md`) and publishes its libp2p peer
   id + advertised endpoint through the tenant admin API. Bond size
   and required attestations of identity are governance parameters
   in `REPUTATION_ECONOMICS.md` § 8.
2. **Warm-up.** A newly-registered verifier joins the gossip mesh
   in *observer* mode for a warm-up window (target: 24 h). It does
   not sign attestations during warm-up. This is anti-Sybil: a
   flash-mob of verifiers cannot immediately vote.
3. **Active.** After warm-up, the verifier's signed attestations
   count towards the threshold `t`. It receives per-verification
   fees via the payoff matrix in `REPUTATION_ECONOMICS.md` § 6.
4. **Exit.** A verifier can voluntarily exit; its stake unlocks
   after the challenge window (§ 4). Involuntary exit follows a
   slash execution from adjudication.

## Verification flow

Given a proof `P` submitted to any engine:

```
Client   Engine A (verifier V_1)   Gossip mesh   Verifiers V_2..V_n
  |             |                       |               |
  |--- P ------>|                       |               |
  |             |-- verify(P) -->       |               |
  |             |<-- ok / not-ok        |               |
  |             |                       |               |
  |             |-- publish(A_1) ------>|               |
  |             |                       |-- fanout ---->|
  |             |                       |               |-- verify(P)
  |             |                       |<-- A_i -------|
  |             |<-- collect A_1..A_t --|               |
  |             |                       |               |
  |<-- Verdict --|                      |               |
```

Every attestation `A_i` is a signed statement of the form:

```
A_i = Sign_i {
    proof_hash: h,
    outcome:    "verified" | "failed",
    reason?:    string,           // only on "failed"
    seen_at:    RFC3339 timestamp,
    verifier_id: v_i,
    engine_version: sv_i,          // reproducibility gate
}
```

The engine that first sees the proof (V_1 above) is the *collator*
for that verification round. It publishes `A_1` to the mesh, waits
for at least `t - 1` further attestations, and issues a
`Verdict` — a signed envelope over the sorted set of attestations —
as soon as the threshold is reached.

The `Verdict` itself is gossiped, so any node that missed the round
can catch up on replay.

## Adjudication (equivocation and disagreement)

Two triggers:

- **Attester equivocation.** Two attestations `A_i, A_j` refer to
  proofs `P_a, P_b` with the same input hash and same attester id
  but different output hashes. This is exactly the case the
  `slash_equivocation(evidence)` entrypoint sketched in
  `SLASH_EQUIVOCATION_DRAFT.md` targets: the *evidence* payload is
  `(A_i, A_j)`, and the on-chain entrypoint verifies both signatures
  and the collision before executing the slash.
- **Verifier disagreement.** A `Verdict` for proof `P` disagrees
  with a subsequent replay attestation from another verifier on
  the same `proof_hash + engine_version`. The disagreement itself
  is gossiped, an M-of-N Adjudicator quorum
  (`REPUTATION_ECONOMICS.md` § 4) is convened, and if the quorum
  finds the majority `Verdict` incorrect, the majority-signing
  verifiers are slashed.

Both triggers land as a public artifact — verifiable by anyone with
a copy of the mesh's gossip log — so an outside observer does not
have to trust CasperProver to detect misbehaviour.

## Data structures

- **Gossip log.** Append-only per-verifier log of `(attestation |
  verdict | equivocation-evidence)`. Merkleised by the receiver on
  arrival so a partitioned node can request a range and verify it
  on catch-up.
- **Live set.** A registry of currently-active verifier ids +
  advertised endpoints + stake status. Bootstrap by RPC-fetching
  from the `stake_slashing` contract; kept in sync via a "verifier
  membership changed" gossip topic.
- **Version tag.** Every attestation carries the engine version. A
  `Verdict` is only valid if all `t` attestations carry the same
  version tag. Different versions land in separate quorums — this
  is a deliberate anti-upgrade-race guard.

## Reproducibility gate

Two verifiers must be able to independently arrive at the same
outcome on the same proof, or the model breaks.

Sources of non-determinism to eliminate up-front:

- **Model-inference determinism.** Deterministic decoding, fixed
  seeds, no temperature > 0 in the verifier's re-run. Owned by
  `internal/inference`.
- **Merkle-root determinism.** Fixed leaf-ordering rule (already in
  place in `internal/hasher`).
- **Timestamp inclusion.** Timestamps go in the attestation
  envelope, not the signed content. The signed content is the
  proof hash + outcome + verifier id + engine version only.

## Rollout stages (post-invest)

1. **Prototype:** libp2p node, hard-coded 3-verifier live set,
   local devnet only, no stake bonding.
2. **Shadow deploy:** the gossip mesh runs alongside the
   single-verifier `/verify` endpoint. Attestations are recorded
   but the client-facing verdict is still the single-node result.
   Discrepancies are logged for calibration.
3. **Cutover:** `/verify` publishes to the mesh and blocks on the
   threshold. Single-node fallback removed.
4. **Full slashing:** the `slash_equivocation` entrypoint from
   `SLASH_EQUIVOCATION_DRAFT.md` is deployed and gated on the
   G7 governance gate from `MAINNET_LAUNCH_PLAN.md`.

Each stage has a rollback plan — if stage N misbehaves, we drop
back to stage N-1 in one config flip.

## Threat inventory (§8 from REPUTATION_ECONOMICS.md, mapped here)

- **Sybil live-set flooding** — 24 h warm-up + PoS bond required.
- **Wash-attestation attacks** — attestations don't earn fees
  until a `Verdict` finalises; a lone verifier attesting to
  itself is a no-op.
- **Collusion (verifiers agree to sign a wrong verdict)** — bounded
  by the `f < n/3` assumption; detectable by a single honest
  verifier's replay attestation → Adjudicator convene.
- **Bribe attack** — reputation slashing on top of stake slashing
  makes the payoff table for a one-shot bribe negative unless the
  bribe exceeds the verifier's entire future stream of fees.
- **Capture (a majority coalition)** — 14-day time-locked
  governance parameter changes + separate M-of-N Governance quorum
  from § 8 of REPUTATION_ECONOMICS.md, deliberately non-overlapping
  membership with the Adjudicator quorum.

## Honesty ladder

- **NOT-BUILT (BLOCKED-ON-INVEST)** — this is a design document. No
  network code, no libp2p integration, no verifier registry, no
  gossip protocol implementation. `internal/verifier` remains a
  single-node local verifier and is honestly labelled that way in
  the top of that package's doc comment.
- **NOT-ON-CHAIN** — the design *interacts* with on-chain contracts
  (stake_slashing, Adjudicator quorum) but is itself an off-chain
  P2P layer. Anchoring the mesh's Verdict roots on Casper (for
  extra tamper-evidence) is contemplated in § "Rollout stages"
  stage 4, not now.
- **NO-CODE-SHIPPED, NO-DEPS-ADDED, NO-PAID-SERVICES** — no changes
  to go.mod, no external services required. This file is prose.

## Open questions

1. **Live-set size scaling** — the BFT `2f+1` threshold gets
   expensive above ~30 verifiers. Do we need a committee-selection
   sub-protocol (VRF-based, tied to AC's VRF spec)? Answer belongs
   at prototype stage, not now.
2. **Attestation batching** — for high-throughput deployments, do
   we bundle multiple proofs into one attestation? Interaction with
   the AE `merkle-provenance-vectors` primitive is favourable but
   unverified.
3. **Fee-market interaction with proof volume** — if verifier fees
   are proportional to attestations, an attester can DoS by
   submitting cheap-to-verify proofs; needs a floor from
   `REPUTATION_ECONOMICS.md`.
4. **Governance-quorum overlap ban** — how do we technically
   enforce non-overlapping membership between Adjudicator quorum
   and Governance quorum, given both are elected on-chain?

## References

- `docs/SLASH_EQUIVOCATION_DRAFT.md` — the on-chain slashing
  entrypoint this design's equivocation-detection path calls into.
- `docs/REPUTATION_ECONOMICS.md` — payoff matrix, quorum
  definitions, anti-manipulation mechanisms this design assumes.
- `docs/MAINNET_LAUNCH_PLAN.md` § G4, § G7 — gates that gate the
  rollout stages above.
- `docs/KNOWN_LIMITATIONS.md` — where BC is enumerated as
  post-hackathon deferred work.
- `internal/verifier` — current single-node verifier this design
  eventually replaces.
