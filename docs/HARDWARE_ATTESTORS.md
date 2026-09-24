# Hardware attestors — design spec and stub interfaces (AT / backlog 6.1–6.4)

**Status: DRAFT design + compile-time stubs. No hardware SDK is
linked in. No real attestation is produced. Every stub honestly
returns `attestor: unavailable` at runtime.**

## Why this document exists

Backlog items 6.1–6.4 target hardware root-of-trust attestation for
CasperProver agents:

- 6.1 TPM 2.0 (Trusted Platform Module) — commodity x86/ARM boards.
- 6.2 Intel SGX (Software Guard Extensions) — enclave attestation.
- 6.3 AMD SEV-SNP (Secure Encrypted Virtualization) — VM attestation.
- 6.4 ARM TrustZone — mobile / edge devices.

Each requires vendor SDKs, kernel modules, provisioning of a specific
hardware platform, and — critically — a per-vendor **remote-attestation
service** (Intel IAS/DCAP, AMD KDS, TPM CA…) whose availability and
pricing model are not compatible with a solo-founder hackathon.

Two honest options at this stage:

1. Skip the surface entirely and label it *not-supported*.
2. Ship a compile-time interface + a stub implementation per vendor
   that returns `attestor: unavailable` unambiguously, plus a spec
   document that pins the eventual real design.

Option 2 is what this pack does. Nothing runtime changes; the surface
is a *promise* the auditor can review. When a real HSM/enclave is
ever wired in, it plugs into the same interface without breaking any
call site.

## Non-goals (deliberately excluded)

- Any actual hardware call. No `/dev/tpm0`, no `SGX_URTS`, no
  `sev-guest`, no OP-TEE. Every stub returns "unavailable" and the
  package tests assert that.
- Vendor SDK integration. No `go get` on any vendor module.
- Remote-attestation service integration (Intel DCAP quoting service,
  AMD KDS, TPM manufacturer CA).
- KMS/HSM key wrap — that is the AJ pack's territory
  (`docs/HSM_AND_KEY_CEREMONY.md`), which this design defers to.

## Interface (real code, in `engine/internal/attestor`)

The package ships a small, vendor-agnostic surface:

```go
package attestor

type Kind string

const (
    KindTPM       Kind = "tpm2"
    KindSGX       Kind = "sgx"
    KindSEVSNP    Kind = "sev-snp"
    KindTrustZone Kind = "trustzone"
)

// Attestor is what every hardware backend implements. Zero
// runtime deps; every method must be safe to call on any host.
type Attestor interface {
    Kind() Kind
    // Available reports whether the backend can actually produce a
    // real attestation on this host. Stubs return false.
    Available() bool
    // Attest produces a signed attestation over challenge. On
    // stub implementations it MUST return an error that includes
    // the string "unavailable" so callers cannot mistake a stub
    // for a real attestation.
    Attest(challenge []byte) (*Quote, error)
    // Verify checks a quote produced elsewhere. Stubs refuse.
    Verify(q *Quote) error
}

// Quote is the wire format for an attestation. Deliberately opaque
// vendor-blob-in-envelope so an auditor cannot mistake it for a
// generic signed message.
type Quote struct {
    Kind       Kind      // which backend produced it
    Vendor     string    // human-readable vendor id, e.g. "Intel SGX DCAP v3"
    Blob       []byte    // vendor-specific attestation bytes
    ChallengeH []byte    // sha256(challenge) — the caller can prove replay-freshness
    IssuedAt   time.Time // populated by the caller for freshness checks
    KindLabel  string    // stringified Kind for JSON friendliness
    Simulated  bool      // stubs set this true; real backends leave false
}
```

Handlers that want an attestation call:

```go
q, err := attestor.Default().Attest(challenge)
if err != nil {
    // stub path — surfaces "unavailable"
}
```

`attestor.Default()` returns whichever backend is registered in
`init()` for the current build. In a hackathon build, that is
always `StubAll` — an aggregate stub that reports every backend as
unavailable.

## Stub behaviour (what actually ships)

Package `engine/internal/attestor` contains:

- `tpm.go`, `sgx.go`, `sev.go`, `trustzone.go` — one stub per vendor.
  Each implements `Attestor`, sets `Available() == false`, and its
  `Attest` returns `(nil, ErrAttestorUnavailable)` where the error
  string contains `"attestor: unavailable"` verbatim.
- `default.go` — `StubAll` aggregate + `Default()` selector.
- `attestor_test.go` — tests that assert:
  * Every stub returns `Available() == false`.
  * Every stub's `Attest` and `Verify` error contains
    `"unavailable"`.
  * The wire type `Quote.Simulated` is set to `true` on any quote
    a stub would emit (even though stubs error out; the property
    is: *if you ever get a Quote from this build, Simulated is
    true*).

The stubs are ~200 LOC total. They compile everywhere Go compiles
(no cgo, no build tags), so CI stays hermetic.

## Real-implementation checklist (post-invest, per vendor)

### TPM 2.0

- Kernel `/dev/tpm0` or `/dev/tpmrm0` present.
- Go binding: `github.com/google/go-tpm/tpm2` (Apache-2.0, healthy).
- Requires provisioning: platform EK (Endorsement Key) certificate
  chain from the TPM manufacturer (Infineon, ST, NationZ, …).
- Remote verification: certificate-chain walk to the platform CA.
  No paid service, but manufacturer roots must be pinned per model.

### Intel SGX (DCAP)

- SGX-capable CPU (Xeon-SP or newer; consumer Skylake+ 6th gen…);
  BIOS support enabled.
- Go binding: `github.com/edgelesssys/ego` or direct `cgo` to
  `sgx_urts`.
- **Requires** Intel PCS (Provisioning Certification Service) or a
  self-hosted DCAP. Free tier exists but has quota; production
  usage triggers commercial licensing considerations that must be
  cleared before deploy.
- Verification: DCAP quote validation against Intel root CA.

### AMD SEV-SNP

- Milan+ EPYC. Kernel 5.19+ with `sev-guest` module.
- Go binding: `github.com/google/go-sev-guest` (Apache-2.0).
- Verification: AMD KDS (Key Distribution Service) — free but
  latency-sensitive; production deployments cache the VCEK.

### ARM TrustZone (OP-TEE)

- ARMv8 with OP-TEE OS. Rare on server platforms; common on
  mobile / edge.
- Go binding: none first-party; typically driven from C or Rust.
  For a Go engine this means an out-of-process helper.
- Deferred as low-priority for the CasperProver server-side path;
  relevant if we ever ship an on-device agent.

## Interaction with the rest of the engine

Currently: none. No handler calls `attestor.*`. This pack lands the
package in the tree so:

- Future work can wire it into the proof-submission path
  (`POST /proofs` gains an optional `attestation` block per agent).
- The AL pack's confidential-storage layer, if activated, can gate
  KEK unwrap on a fresh attestation.
- The BC multi-verifier layer, if activated, can require each
  verifier to attach an attestation to its registration.

None of that is wired today. The package sits on the shelf, waiting.

## Honesty ladder

- **REAL (interfaces + package tests)** — the `Attestor` interface,
  the `Quote` wire type, the `StubAll` aggregator, and the
  per-vendor stub implementations are real Go code. Tests assert
  the stubs cannot silently return a real-looking attestation.
- **NOT-RUNTIME-ACTIVE** — no engine handler calls this package
  today. Landing the interface is the whole scope. Wiring is
  post-invest.
- **NOT-ON-CHAIN** — hardware attestation is a Service-layer
  concept. Anchoring an attestation hash on Casper is
  contemplated in the rollout section but out of scope for AT.
- **NO-PAID-SERVICES** — no vendor SDK linked in. No PCS / KDS /
  TPM-CA call in the build. Compile is hermetic and does not
  require internet.

## Rollout sequence (post-invest)

1. Pick the first vendor to enable (TPM is the least painful —
   commodity hardware, no per-quote paid service).
2. Add a build-tag `attestor_tpm` that swaps in a real backend
   built on `github.com/google/go-tpm`. Stub build stays default.
3. Wire the `Attestor` into `POST /proofs` as an optional
   agent-signed field; verify on receipt; refuse to gate anything
   until dozen-agent field tests pass.
4. Repeat for SGX, then SEV-SNP.
5. Ratchet: turn on gate G4 of `MAINNET_LAUNCH_PLAN.md` only when at
   least two independent vendor paths are green in production.

## Open questions

1. **Nested attestations.** For SEV-SNP-inside-a-cloud, do we
   accept the cloud provider's attestation as sufficient, or
   require our own guest attestation on top? The current spec
   assumes guest-attestation; cloud-only attestation is a
   downgrade path documented but not encouraged.
2. **Attestation freshness window.** The `Quote.IssuedAt` field
   is caller-populated; freshness has to be enforced on
   verification. Default: 5 minutes, matching the SIWE-like
   challenge window in `docs/SIWE_CHALLENGE.md`. Configurable per
   deployment.
3. **Quote size budget.** Real SGX DCAP quotes are ~4 KB. The
   current `POST /proofs` body limit (1 MB) leaves plenty of
   room; documented here so a future body-limit tightening
   doesn't accidentally break attestations.
4. **Anchor the quote hash on-chain?** Optional; adds tamper-
   evidence but doubles the anchor cost per proof. Deferred.

## References

- `engine/internal/attestor/attestor.go` — interface + wire type.
- `engine/internal/attestor/default.go` — StubAll aggregate,
  `Default()` selector.
- `engine/internal/attestor/tpm.go` / `sgx.go` / `sev.go` /
  `trustzone.go` — per-vendor stubs.
- `engine/internal/attestor/attestor_test.go` — property tests.
- `docs/HSM_AND_KEY_CEREMONY.md` (AJ) — the KMS/HSM story this
  design's `Wrapper` interface deferred to.
- `docs/MULTI_VERIFIER_GOSSIP.md` (BC) — future consumer.
- `docs/MAINNET_LAUNCH_PLAN.md` (AK) § G4 — gate.
- `docs/KNOWN_LIMITATIONS.md` — where AT is enumerated as
  post-hackathon deferred work with a stubbed surface.
