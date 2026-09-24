# Trusted-Setup Ceremony — Design

Ref: `handoff/CP_FINAL_TASKS_V2.md` §D / §E.

## Problem

The current gnark Groth16 pipeline generates the proving/verifying key pair
on engine start. That is fine for a demo, but:

- It is not reproducible across restarts unless the RNG is seeded and the
  circuit is byte-identical.
- The verifying key is not published, so external verifiers cannot check a
  proof.
- The setup is not multi-party (a single entity holds all randomness).

## Design overview

Two-phase ceremony:

1. **Universal SRS reuse** — piggyback on an existing published Groth16 SRS
   (e.g. the Semaphore or ZK-EVM ceremony transcripts) when the circuit
   family allows. For MiMC-preimage the circuit is small enough that a
   dedicated ceremony is realistic; for larger circuits, reuse is
   preferable.
2. **Circuit-specific setup** — take the SRS from phase 1, run
   `gnark.Setup()` (or the equivalent for the target scheme) with N ≥ 3
   independent contributors, each adding entropy to the proving key.

## Artefact model

Each ceremony produces:

- `pk.bin` — the proving key. Persisted in object storage; content-addressed
  by SHA-256; served by the engine on start.
- `vk.json` — the verifying key. Public. Committed to git under
  `crypto/ceremony/vk/<circuit>-<version>.json`.
- `vk_hash` — 32-byte hash of the canonical serialisation. Anchored
  on-chain via `proof-registry` under a dedicated `ceremony` namespace so
  clients can verify they hold the "right" verifying key.
- `transcript.md` — human-readable log of each contribution, including who,
  when, hash of their contribution, and their signature over
  `(prev_hash, contribution_hash)`.

## Ceremony operations

- **Coordinator:** the CP maintainer (Anna Stolbovskaja for the initial
  round). Sets circuit version, publishes `pk_next` for each participant,
  collects contributions in-order.
- **Participants:** at least three independent parties. Each must:
  1. Verify `prev_hash` against the transcript.
  2. Generate a random beacon (VRF or public-randomness source such as
     drand).
  3. Contribute using `gnark`'s ceremony helper (or equivalent).
  4. Publish `(their_hash, sig_over_prev_hash_and_their_hash)`.
- **Sealing:** once N contributions are in, the coordinator burns the
  intermediate keys (write "burned at HH:MM UTC" into the transcript) and
  publishes the final `vk` + on-chain `vk_hash`.

## Reproducibility

- The circuit source at the version tag must be deterministic and vendored.
- The gnark version must be pinned in `go.mod` and cross-referenced in
  `transcript.md`.
- The Go build must be reproducible: `-trimpath -buildvcs=false`.
- A helper CLI `scripts/verify-ceremony.mjs` reconstructs `vk_hash` from
  the transcript + contributions and compares to the on-chain anchor.

## Key management

- Coordinator key: hardware-backed (YubiHSM / cloud KMS). Never touches
  disk in cleartext.
- Participant keys: their responsibility; the transcript records only
  their public key and signature.
- Storage of `pk.bin`: SHA-256 content-addressed; two independent copies
  (primary + cold backup); integrity check on engine start.

## Milestones

1. **Design + circuit versioning (3 days).** `crypto/ceremony/README.md`
   describing the model; `crypto/ceremony/manifest.json` pinning the
   circuit-family version.
2. **Coordinator CLI (5 days).** `crypto/ceremony/cmd/coordinator/` binary;
   fixture-participant testing.
3. **First live ceremony (2 days).** Three contributors; publish transcript.
4. **Consumer path (2 days).** Engine starts by loading `pk.bin` from
   content-addressed storage; verifies against the on-chain `vk_hash`.

## Non-goals

- Continuous / auto-triggered ceremonies. Version bumps are manual.
- Ceremony for non-Groth16 schemes (Halo2, Nova). Roadmap.
- Post-quantum-safe ceremony. Groth16 is not PQ; the ML-DSA/Lamport surface
  is a separate track (see `docs/PQ_HONESTY.md`).

## Acceptance criteria

- [ ] `crypto/ceremony/README.md` with the artefact model.
- [ ] `crypto/ceremony/manifest.json` pinning circuit + gnark versions.
- [ ] `scripts/verify-ceremony.mjs` reproduces `vk_hash` from an artefact
      directory.
- [ ] `docs/roadmap/CEREMONY.md` cross-linked from `30-DAY.md` and
      `PQ_HONESTY.md`.
