# SDK Versioning Strategy

**Status**: `DRAFT — versioning policy`. Establishes the contract
CasperProver's SDKs, MCP server, and API surface follow so downstream
Operators can depend on them without surprise breakage. **No code is
changed by this document. No dependency is added.**

Cross-refs:
- `docs/INTEGRATIONS_ROADMAP.md` (AP) — SDK inventory and preconditions.
- `docs/HASH_ALGORITHM_ANALYSIS.md` (AN) Q2 — canonical serialisation
  version tag; any breaking receipt-schema change is a versioning
  event.
- `LEGAL/TOS.md` (AI) — an SDK version's honesty-ladder contract is
  part of the TOS surface it inherits.
- `docs/MAINNET_LAUNCH_PLAN.md` (AK) G6 — ops readiness includes SDK
  publish process discipline.

---

## 1. Framing — what versioning is for

An SDK version number is a promise about compatibility. If the promise
is wrong (e.g. a minor bump that breaks call sites, or a major bump
without a written migration path), downstream Operators pay for the
mismatch. This document specifies the promise precisely so nobody has
to guess.

CasperProver has three publish surfaces:

1. **API contract** — the wire format Operators talk to. Anchored at
   `api/v<major>/...`.
2. **SDK** — code Operators embed. Follows semver over the API contract.
3. **Receipt schema** — the canonical serialisation that gets hashed
   and signed. Version-tagged (`"cp:receipt:v1"`, `"cp:receipt:v2"`).

The three axes are related but not identical. A receipt-schema bump
forces an API major; an SDK feature-addition without a wire change is
an SDK minor. Confusing them is the top failure mode this document
prevents.

---

## 2. Semantic versioning contract

CasperProver SDKs and API contracts follow **SemVer 2.0.0** with the
following clarifications:

- **MAJOR** — breaks the API contract, the receipt schema, or a
  documented honesty-ladder label. Any of the three forces a MAJOR.
- **MINOR** — adds a wire endpoint, adds an SDK method, or widens
  the accepted input schema without narrowing existing outputs.
  Backward-compatible.
- **PATCH** — fixes a bug, tightens documentation, upgrades a
  transitively-safe internal dependency. No user-visible surface
  change.
- **PRE-RELEASE tags** — `-alpha.N`, `-beta.N`, `-rc.N`. Never used
  in production integrations. `-alpha` is `SIMULATION`-equivalent by
  default.

**"Backward-compatible" is a strict test, not a marketing term.** A
minor is backward-compatible iff every valid v.MINOR-1 call is still
valid at v.MINOR and produces a result that satisfies the
v.MINOR-1 contract. If a minor bump introduces a new REQUIRED field
anywhere, that is a MAJOR by this policy, no exceptions.

---

## 3. Version scoping across artefacts

| Artefact               | Versioned as    | Change trigger                               |
|------------------------|-----------------|-----------------------------------------------|
| API contract           | `/api/v<n>/`    | Wire change or receipt-schema change          |
| Receipt schema         | `cp:receipt:vN` | Any change to hashed canonical serialisation  |
| Go SDK                 | `vX.Y.Z`        | Follows API-contract compatibility windows    |
| Python SDK (planned)   | `vX.Y.Z`        | Follows API-contract compatibility windows    |
| MCP server             | `vX.Y.Z`        | Follows API-contract compatibility windows    |
| CLI (`verify.sh`)      | `vX.Y.Z`        | Follows receipt-schema compatibility windows  |
| Contracts (on-chain)   | See AK §6       | Change control per MAINNET_LAUNCH_PLAN        |

**Independence of receipt schema from SDK.** An SDK MAJOR can happen
without a receipt-schema MAJOR (e.g. changing an SDK method signature
while keeping the wire format). But a receipt-schema MAJOR *always*
forces an SDK MAJOR (because the serialisation changed).

---

## 4. Support windows

- **API contract**: last two MAJOR versions supported concurrently.
  N-2 is deprecated with 6 months notice; deletion after another
  6 months. Deletion is a stop-the-world event and must appear in the
  changelog with hard-red formatting.
- **Receipt schema**: **support all prior schemas indefinitely for
  verification**. This is non-negotiable — a receipt signed under
  `cp:receipt:v1` must remain verifiable even after `v2` ships;
  otherwise historical anchored proofs stop meaning anything and the
  whole system's premise breaks. Verifier code and CLI must retain
  read paths for every historical schema.
- **SDK MAJOR**: latest and previous MAJOR supported concurrently.
  Older MAJORs get security patches only for 12 months from the
  release of the next MAJOR.
- **PATCH windows**: security patches for 12 months on any supported
  MAJOR.

**Publish policy.** No CasperProver SDK is published to a public
package registry until:
1. AK G6 (ops readiness) has been reached, OR
2. The publication is explicitly tagged `-alpha`/`-beta` and
   documented as `SIMULATION` in `LEGAL/TOS.md`.

Until then, SDKs are consumed by direct source-tree reference.

---

## 5. Breaking-change checklist

Every proposed change goes through this before merge:

1. Does it change the wire format Operators talk to? → API MAJOR.
2. Does it change the canonical serialisation that gets hashed? →
   receipt-schema MAJOR (which forces API MAJOR).
3. Does it change an honesty-ladder label (`REAL` → `SIMULATION`,
   or vice-versa)? → API MAJOR + `LEGAL/TOS.md` update + counsel-review
   flag.
4. Does it change SDK method signatures downstream Operators depend
   on? → SDK MAJOR.
5. Does it add a new required parameter to any existing endpoint? →
   API MAJOR (there is no such thing as a "safe required addition").
6. Does it deprecate a supported endpoint or method? → deprecation
   notice at MINOR bump, deletion at the next MAJOR minimum.

Merging a change without walking this checklist is a defect. The
checklist itself is an artefact required by AK G6.

---

## 6. Deprecation flow

- **Announce**: changelog entry with hard-red formatting; SDK method
  or endpoint marked `@Deprecated` in the language's native way.
- **Emit warning**: SDK logs a warning on first call in each process.
  Endpoint emits a warning HTTP header (`Warning: 299 cp "deprecated,
  see docs/CHANGELOG.md#..."`).
- **Sunset date**: minimum 6 months from announcement for API
  MAJOR-scoped deletions; minimum 12 months for anything with a
  receipt-schema implication.
- **Delete**: at the next MAJOR after the sunset date. Not on the
  sunset date itself.

Deletion of a *receipt schema* is **explicitly forbidden**. Deletion
of an API endpoint that used to accept that schema is fine as long as
the schema itself can still be verified offline via the shipped tools.

---

## 7. Version tag placement

- **HTTP endpoints**: `/api/v<n>/...`.
- **HTTP response headers**: `X-CP-API-Version: v<n>`, `X-CP-Schema:
  cp:receipt:v<n>`.
- **Receipt canonical serialisation**: leading `"cp:receipt:v<n>"`
  purpose tag as required by AN Q2.
- **SDK packages**: `github.com/anna-stolbovskaja/casperprover-go/v<n>`
  (Go module semantic import path); PyPI `casperprover-py==vX.Y.Z`
  (planned); npm `@casperprover/sdk-ts@vX.Y.Z` (planned).
- **MCP server**: version reported via `initialize` response
  `serverInfo.version`.
- **CLI**: `verify.sh --version` returns the compatibility window.

Any of these missing = defect.

---

## 8. Honesty-ladder interaction

A change in the honesty label of an existing surface is **always** a
breaking change, no matter how small the underlying implementation
change is. Rationale: an Operator depending on `REAL` semantics for
audit compliance cannot silently be downgraded to `SIMULATION`; that
would launder a real regression as a marketing tweak.

Concretely:
- `REAL` → `SIMULATION` (downgrade): forbidden without a MAJOR bump
  and a `LEGAL/TOS.md` amendment.
- `SIMULATION` → `REAL` (upgrade): requires all four conditions from
  the relevant honesty verdict (e.g. `ZKML_HONEST_VERDICT.md` §4 for
  ZK-ML claims). Not a MAJOR by itself, but must be documented as
  the fulfilment of specific gate criteria.

---

## 9. Changelog discipline

- **`CHANGELOG.md`** in-repo, one entry per version, dated,
  categorised (Added / Changed / Deprecated / Removed / Security).
- **Migration notes** in-line for any MAJOR; separate `MIGRATION-vN.md`
  files if the notes exceed a screen.
- **No silent changes**: any user-visible change must be in the
  changelog, no exceptions.
- **PATCH releases**: still get a changelog entry, even if it's a
  one-liner.

---

## 10. Package-registry publish policy (deferred, but recorded)

When publish becomes gated on AK G6:

- **PyPI**: sign every release with `sigstore-python`; provide SBOM in
  `sbom.spdx.json`.
- **npm**: publish with `--provenance`; SBOM ditto.
- **Go modules**: semantic import versioning (`/v<N>` for `N ≥ 2`)
  as required by the toolchain; module checksum in `go.sum`; SBOM
  ditto.
- **Container images** (if any): signed with cosign, keyless via
  OIDC on the build system; SBOM in image annotations.
- **No vendor-lock artefact formats**: nothing proprietary in the
  publish path.

Until G6, none of the above is authorised. Direct source-tree
consumption is the only supported path.

---

## 11. Open questions (routed to `docs/KNOWN_LIMITATIONS.md`)

**Q1** — When does the CHANGELOG.md file get created in-tree? Not by
this document (which is the versioning policy). It is a G6
prerequisite.

**Q2** — Does the honesty-ladder-downgrade prohibition (§8) require a
counsel-review of the entire honesty ladder itself? Preliminary:
yes, at G5 (legal readiness).

**Q3** — How is receipt-schema `v1 → v2` migration proven? Preliminary:
a mechanised test that takes every leaf under `test/cp-merkle-provenance-
vectors` (AE) and verifies it under both v1 and v2 verifier code
paths. Required for any future MAJOR bump.

**Q4** — Does the SDK compatibility window shift when a per-chain
adapter changes (AP §5.2)? Preliminary: yes — per-chain writes are
part of the API contract surface.

**Q5** — Is a compatibility-matrix generated automatically or by
hand? Preliminary: by hand for the hackathon; automated by G6.

---

## 12. What this document does not do

- It does not change any code.
- It does not publish any SDK.
- It does not choose a registry.
- It does not commit to a schedule.
- It does not authorise a paid service.
- It does not weaken any honesty-ladder label.

The single deliverable is a written versioning contract Operators can
rely on before any publish happens. Its purpose is to make version
promises auditable.

---

*This is a versioning policy. It ships no code and publishes no SDK.
Its only purpose is to make CasperProver's version promises auditable
across API, receipt schema, and SDK simultaneously.*
