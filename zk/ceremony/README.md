# Groth16 Trusted-Setup Ceremony (Phase 1 + Phase 2)

Backlog item **2.5**. Real, gnark-native ceremony for the
`PreimageCircuit` (BN254 · MiMC) defined in
`engine/internal/zkverifier/gnarkzk/circuit.go`.

## Honesty label

**SINGLE-COORDINATOR CEREMONY.** The ceremony is executed end-to-end
inside one process by one coordinator that contributes N times (each
contribution independently seeded by gnark's RNG, verified against the
previous, and hashed into the transcript). That produces a real,
cryptographically verifiable ceremony transcript. It does **not**
provide the multi-party "1-of-N honesty" property of a live public MPC
where independent contributors run the software on independent
machines.

The upgrade path is intentionally small: the code chains contributions
by `WriteTo` / `ReadFrom` of the `Phase1` / `Phase2` objects, so a
production ceremony is a matter of running `Contribute()` on other
machines and dropping the resulting binaries into the same
`VerifyPhase{1,2}` chain — no code change required.

See also `docs/HONESTY_BADGES.md`.

## What's real vs. simulated

**Real:**

* Powers-of-Tau (Phase 1) with the actual gnark
  `github.com/consensys/gnark/backend/groth16/bn254/mpcsetup` primitives.
* Phase 2 (circuit-specific setup) via `mpcsetup.Phase2` /
  `VerifyPhase2`, sealed with a public beacon.
* Pairwise verification of every contribution.
* SHA-256 digests of every contribution, of the sealed Phase-1
  `SrsCommons`, and of the final proving / verifying keys — recorded in
  the attestation JSON.
* Executable proof that the resulting PK/VK pair proves and verifies
  the `PreimageCircuit` (`ceremony_test.go`
  `TestRunProducesUsableSetup`).
* The beacon is bound into the sealed keys — a different beacon value
  produces a different final PK/VK (`TestBeaconAffectsFinalKeys`).

**Simulated:**

* The multi-contributor topology. See "Honesty label" above.
* The beacon value in tests. Production must use a real public
  randomness source (drand / League of Entropy / a block hash)
  evaluated strictly after the last contribution.

## Reproduce

```bash
# Build the CLI.
cd engine
go build -o ../bin/ceremony ./cmd/ceremony

# Run the ceremony with N=1024 domain, 3 contributions per phase,
# writing binaries to ../zk/ceremony and the attestation to stdout.
../bin/ceremony \
    --out ../zk/ceremony \
    --n 1024 --p1 3 --p2 3 \
    --beacon "drand-round-<round>-<value>" \
  > ../zk/ceremony/attestations.json
```

Artifacts written under `zk/ceremony/`:

* `phase1_commons.bin` — sealed Phase-1 `SrsCommons` (circuit-agnostic).
* `groth16_pk.bin`     — final Groth16 proving key.
* `groth16_vk.bin`     — final Groth16 verifying key.
* `attestations.json`  — full transcript: circuit id, N, beacon,
  per-contribution challenge/digest/size, final PK/VK/commons digests,
  honesty label.

## Verify

Any third party can rebuild the transcript from the binaries plus the
beacon value and check that:

1. `mpcsetup.VerifyPhase1(N, beacon, phase1s...)` returns the same
   `SrsCommons` (same SHA-256).
2. `mpcsetup.VerifyPhase2(r1cs, commons, beacon, phase2s...)` returns
   PK/VK with the SHA-256s in `attestations.json`.
3. `groth16.Prove` on `PreimageCircuit` with a real witness followed by
   `groth16.Verify` succeeds against the published VK.

The gnark unit tests in
`engine/internal/zkverifier/ceremony/ceremony_test.go` already exercise
(1), (2) and (3) on every `go test` run.

## Multi-party upgrade — what changes

To move from single-coordinator to multi-party MPC:

1. Coordinator runs Phase-1 contribution #0, `WriteTo` the file, publishes.
2. Contributor `i` fetches contribution `i-1`, `ReadFrom`, calls
   `Contribute()` on their own machine with their own entropy source
   (independent from every other contributor), `WriteTo` the file,
   publishes.
3. After the last contributor, everyone runs
   `mpcsetup.VerifyPhase1(N, beacon, phase1_0, ..., phase1_k)` — this
   must succeed, otherwise the ceremony is invalid.
4. Same protocol for Phase 2.
5. Beacon is drawn from a public randomness source *after* the last
   contribution is published. Everyone recomputes the final Seal and
   checks the digests match `attestations.json`.

The `Config.Phase1Contributors` / `Phase2Contributors` fields are just
knobs that let the coordinator front-load contributions during
bootstrap; the same `ceremony.Run` code that ties them together also
verifies an externally-produced chain when you drop those binaries into
the pipeline.
