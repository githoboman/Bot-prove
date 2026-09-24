# 30-Day Product Roadmap

Ref: `handoff/CP_FINAL_TASKS_V2.md` §D.

**Horizon:** 30 days after the hackathon submission window closes.
**Goal:** convert the demonstrated primitives (proof registry, verifier gate,
decision layer, PQ signing, Groth16 off-chain, TLC-checked spec) into
consumable SDKs, a hosted playground, and a governance surface that a design
partner can actually integrate.

## Objectives

1. Publish versioned SDKs in Go, Python, and TypeScript. Every SDK 1:1 with
   the engine's REST/MCP surface.
2. A hosted MCP playground where an evaluator can plug an LLM tool call into
   the CP proof pipeline without cloning the repo.
3. Threshold BLS quorum registry design ready for implementation (see
   `BLS_QUORUM.md`).
4. Timelocked-governance / emergency-pause / renounce lifecycle design ready
   for implementation (see `GOVERNANCE.md`).
5. Persisted trusted-setup ceremony strategy replacing per-start generation
   (see `CEREMONY.md`).
6. A2A workflow + HITL policy service design ready for prototyping (see
   `A2A_HITL.md`).
7. Provenance lineage (W3C VC / Agent Receipt / OTel) mapped onto the
   existing receipt schema (see `PROVENANCE.md`).
8. API lifecycle (versioning, webhooks, RBAC-lite) designed (see
   `API_LIFECYCLE.md`).
9. Installable PWA offline shell with explicit stale/offline badge and NO
   optimistic writes (see `PWA.md`).

## Semver policy (SDKs)

- SDKs start at `v0.1.0`. `v0.x` means the API surface may change between
  minor bumps; document the diff in each SDK's `CHANGELOG.md`.
- The engine's REST API version (`X-CP-API-Version` header) is decoupled from
  SDK versions. An SDK declares the range of API versions it supports.
- Breaking API changes bump the engine's major version and every SDK ships a
  matching minor bump with the compatibility declared.
- No SDK is `v1.0.0` until:
  - It has ≥90% coverage of the engine surface.
  - It ships in a language-native package registry (crates/npm/pypi).
  - It has a smoke-test workflow that runs against a live testnet-facing
    engine at least once per week.

## Publish flow

For each of the three SDKs:

1. Scaffold in-repo under `sdk/<lang>/` (see SDK skeletons committed alongside
   this doc).
2. Add a `sdk/<lang>/README.md` with install snippet, quickstart, and
   version-support table.
3. Add a `sdk/<lang>/CHANGELOG.md` starting at unreleased.
4. Add CI: build + smoke test on push; do NOT auto-publish.
5. First real release goes through a manual gate with:
   - Green smoke test against a live engine deploy.
   - `docs/PQ_HONESTY.md` audit clean.
   - Signed tag by a maintainer.

## Hosted MCP playground

- Deploy the existing `sdk/cmd/mcpserver` behind a public URL with per-IP
  rate limiting and an explicit "playground, not production" banner.
- Provide two demo LLM tool-call flows out of the box:
  1. Attestation-generation flow (produces a signed receipt).
  2. Attestation-verification flow (consumes a receipt, returns
     `{valid, evidence_root, verdict, model_id}`).
- Instrument with OTel; export a Grafana dashboard config alongside the
  playground.

## Acceptance criteria (day 30)

- [ ] `sdk/go/`, `sdk/python/`, `sdk/typescript/` in-repo with a smoke test each.
- [ ] `docs/roadmap/A2A_HITL.md`, `BLS_QUORUM.md`, `CEREMONY.md`,
      `PROVENANCE.md`, `GOVERNANCE.md`, `API_LIFECYCLE.md`, `PWA.md` all
      present and cross-linked.
- [ ] At least one design partner (research group, DeFi risk desk, or
      compliance vendor) has read the SDK quickstart and produced written
      feedback captured in `docs/roadmap/FEEDBACK.md`.
- [ ] The hosted MCP playground URL is linked from the top-level README.

## Non-goals for the 30-day window

- Mainnet deployment.
- Real trusted-setup ceremony (design only in this window; execution is
  90–180 day).
- Full multi-tenant billing (design only; see `docs/roadmap/MULTITENANCY.md`).
- Third-party cryptography audit (procurement is in the 90–180 day window).
