# Groth16 Key Ceremony Plan (DRAFT)

> **Status:** DRAFT — the code path exists (see Pack AF: `feat/cp-phase2-ceremony`)
> and is **honestly labelled** in-repo as `SINGLE-COORDINATOR CEREMONY`. This
> document is the playbook for upgrading that single-coordinator ceremony to a
> genuine multi-party ceremony ahead of any mainnet cutover. **No multi-party
> ceremony has been executed yet.**
>
> **Honesty ladder:** current — `REAL CRYPTO`, single-coordinator. Target —
> `REAL CRYPTO`, multi-party sealed with a public randomness beacon.
> Cross-refs: `zk/ceremony/README.md`, `docs/HSM_PLAN.md`,
> `docs/MAINNET_LAUNCH_PLAN.md` (forward — Pack AK), `LEGAL/TOS.md`.

## 1. Why a ceremony at all

Groth16 requires a **structured reference string (SRS)** — a pair of
cryptographic keys (`pk`, `vk`) that must be generated in a way where at least
one contributor was honest, or every subsequently generated proof is
unsound. The ceremony is the process that generates that SRS with public,
verifiable transcript, so no single party can silently trapdoor the system.

## 2. What is already in place

- `engine/cmd/ceremony` (`feat/cp-phase2-ceremony` @ 48666ad) — real Phase-1
  Powers-of-Tau + Phase-2 circuit-specific ceremony via gnark `mpcsetup`.
- Transcript emits SHA-256 of each contribution, sealed SrsCommons, and
  final `pk`/`vk` in a JSON-serialisable envelope.
- Beacon-binding proven by test: changing the beacon changes the final
  `pk`/`vk` (test in `engine/internal/ceremony/ceremony_test.go`).
- `zk/ceremony/README.md` documents the single-coordinator honesty label and
  the multi-party upgrade path.

This document turns the "upgrade path" into a scheduled, checklist-driven
operation.

## 3. Roles

| Role | Count | Responsibility |
|---|---|---|
| **Coordinator** | 1 | Runs the `ceremony` binary, publishes intermediate transcripts, verifies pairwise linking, seals final artifacts. Cannot see any single contributor's secret randomness. |
| **Contributors** | N ≥ 5 for pilot, N ≥ 11 for mainnet | Each contributes randomness derived from an independent entropy source. |
| **Auditors** | ≥ 2, independent of Contributors | Re-verify the transcript end-to-end after the ceremony closes. |
| **Beacon steward** | 1 | Publishes the public randomness beacon value (see §6). |

**Independence rule:** Contributors and Auditors MUST NOT share machines,
custody, or organisational chain. The Coordinator MAY be one of the
Contributors but MUST NOT be the beacon steward.

## 4. Entropy sources (per contributor)

Each Contributor MUST combine **at least three independent** sources into the
seed handed to their local `ceremony contribute` invocation. Acceptable
examples (an audit will spot check):

- OS CSPRNG (`/dev/urandom` on Linux, `SystemRandom` in Python, `crypto/rand` in Go).
- Physical dice throw or hardware RNG (photograph the roll, hash the image).
- Recent public block hash from a chain the Contributor does not operate
  (belt-and-braces bind: attacker would have to compromise multiple
  independent chains at the exact same slot).

**Never acceptable:** a single OS CSPRNG on its own, `time.Now()`,
`os.Hostname()`, or reused seeds. A single-source contribution invalidates
the transcript.

## 5. Commit-and-verify contract

Every contribution round emits:

- `contribution_N.bin` — the sealed contribution artifact.
- `contribution_N.sha256` — the SHA-256 digest of the artifact.
- `contribution_N.attest.json` — signed statement from the Contributor
  asserting entropy source list, timestamp (ISO-8601 UTC), and self-attested
  destruction of the secret seed.

The Coordinator MUST verify **pairwise linkage** before accepting each
contribution: contribution N+1 must chain to contribution N via the
`WriteTo`/`ReadFrom` primitive already exercised in the AF tests.

After the last contribution, the Coordinator publishes the transcript to a
public location (repo release + IPFS pin, or repo release + Casper
on-chain commit). The Auditor role independently re-runs the verification
step; a mismatch between Auditor and Coordinator MUST halt the ceremony
and trigger a restart.

## 6. Beacon binding

At `T_close + 24h`, the Coordinator ingests a public randomness beacon value
and re-seals the final `pk`/`vk`. Acceptable beacons:

- **NIST Randomness Beacon** (public draw at fixed hour).
- **Ethereum block hash at height H** where H is published *before* T_close
  (the future block hash is not predictable at close time).
- **Casper block hash** at a height announced before T_close.

The specific beacon MUST be announced in the transcript **before** T_close
so a Contributor cannot predict it and pre-compute a trapdoor.

## 7. Playbook (schedule, calendar days)

```
D-14  Announce ceremony window, invite Contributors and Auditors publicly.
D-7   Publish this plan + zk/ceremony/README.md + `engine/cmd/ceremony` binary
      hash + expected beacon source. No further code changes to ceremony/*.
D-3   Contributors dry-run the CLI locally. Coordinator publishes canonical
      contribution_0 (starting point).
D-2   Freeze — no code changes to engine/internal/ceremony or engine/cmd/ceremony.
D-1   Coordinator publishes environment digest (Go version, gnark version,
      OS, kernel) and host attestation.
D+0   Ceremony window opens. Contributors submit contributions serially, each
      pairwise verified.
D+1   Ceremony window closes. Transcript uploaded to a public store.
D+2   Beacon steward publishes the beacon value. Coordinator seals final
      pk/vk with the beacon. Sealed artifacts published.
D+3   Auditor 1 independently verifies. Auditor 2 independently verifies.
D+7   If both Auditors clear, ceremony is `FINAL`. Otherwise, ceremony is
      `INVALID` and the schedule restarts at D-14.
```

## 8. Failure modes and abort criteria

| Failure | Response |
|---|---|
| Pairwise verification fails between contributions N and N+1 | Halt. Contact Contributor N+1. If Contributor N+1 cannot re-submit within 24h, drop and continue with a public transcript entry. |
| Auditor disagrees with Coordinator | Halt. Publish both transcripts. Investigate before restart. |
| Beacon steward misses window | Extend window by 48h. If still missed, restart from D+0 with a new beacon. |
| Any Contributor withdraws attestation post-hoc | Ceremony becomes `INVALID`. Restart. |
| Environment digest at D-1 differs from what was announced at D-7 | Halt. Restart at D-14 with new digest. |

## 9. Storage & handoff

Sealed artifacts:
- `pk.sealed` — prover key (large, retained offline + committed hash on-chain).
- `vk.sealed` — verifier key (small, committed on-chain, replicated to
  `deploy-out/onchain.json`).
- `transcript.json` — full transcript (contributor list, hashes, beacon,
  environment digest).
- `attestations/*.json` — one per Contributor.

Retention: transcripts and attestations are **archival** under
`LEGAL/DATA_PROTECTION.md` §Retention — never rotated out.

## 10. Cross-references

- `zk/ceremony/README.md` — current honesty label, single-coordinator
  reproduction steps.
- `docs/HSM_PLAN.md` — HSM adapter that will custody the sealed anchor key
  used to publish the vk commit on-chain.
- `docs/OPS_RUNBOOKS.md` — response if a Contributor is credibly reported
  compromised after ceremony close.
- `docs/MAINNET_LAUNCH_PLAN.md` — the launch milestone that requires this
  ceremony to complete with `FINAL` status.
- `LEGAL/TOS.md` §Warranty — the ceremony is the sole basis on which
  Groth16 proofs are non-vacuous; a failed ceremony implies the Service
  cannot honour attestation claims that depend on it.

_Owner: maintainers. Reviewer: operator + external cryptography advisor.
Cadence: one ceremony per SRS epoch. New epoch triggered by circuit change,
suspected compromise, or scheduled 12-month rotation — whichever comes first._
