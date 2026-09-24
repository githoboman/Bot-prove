# RWA-Sentinel Salvage Plan

Ref: `handoff/CP_FINAL_TASKS_V2.md` §C.

## Source

RWA-Sentinel (`triumphkrug/RWA-Sentinel`) is an Apache-2.0-licensed
hackathon codebase that did not advance past round 1. It contains a
handful of independently reusable technical patterns. A salvage catalog
maintained separately from this repo — under
`/data/casper/casper_research/rwa-s-salvage/` — enumerates them.

**Nothing from RWA-Sentinel has been ported into CasperProver yet.** This
document is the plan; the actual salvage happens on branches downstream
of this one after cofounder + reviewer sign-off.

## Salvage discipline

Before any RWA-Sentinel pattern lands in this repo, the porting agent
must obey:

1. **No verbatim copy.** Patterns are algorithmic + architectural
   reference, not a paste source. Rename all identifiers, restructure to
   match CasperProver's Go / Rust style, rewrite comments in
   CasperProver's own voice.
2. **No RWA-Sentinel references in the code itself.** No "adapted from
   RWA-Sentinel" comments. Attribution belongs in the commit body and in
   this design doc, not in the shipped source.
3. **Independent decision.** RWA-Sentinel's Apache-2.0 license does NOT
   override the hackathon submission's originality rule. Every port must
   pass a substantial-rewrite test.
4. **Test parity.** Every ported pattern ships with tests written in
   CasperProver's own test style (`_test.go` for Go, Rust `#[test]`
   for contracts, `pytest` for Python).
5. **Reviewer + owner sign-off** before push.

## Patterns worth porting

### Safe / high value

- **BLS12-381 3-of-5 threshold consensus** — informs the
  `docs/roadmap/BLS_QUORUM.md` design, but does not become a drop-in
  port. RWA-Sentinel's implementation is TypeScript via `@noble/curves`;
  CasperProver's target is Go via `gnark-crypto`. What is portable is
  the *algorithmic shape* (signer registry, threshold check, aggregate
  verification) and the *challenge lifecycle*, not the code.
- **`oracle-slashing` challenge lifecycle and severity logic** — the
  state machine (challenge → attestation window → resolution → slash)
  is directly reusable as a design input for
  `docs/roadmap/GOVERNANCE.md` §emergency-pause and for the extended
  `stake-slashing` contract. Do not port the whole Rust crate; port the
  state machine.
- **`attestation-store` signer dedup, threshold invalidation, challenge
  rate limits** — informs the `signer-registry` contract described in
  `docs/roadmap/BLS_QUORUM.md`.
- **`merkleProvenance` algorithm / test reference** — informs the
  incremental Merkle batch receipt (already implemented in commit
  `9fa1481`; the RWA-Sentinel reference validates our approach and
  can seed additional property-based tests).
- **`kyc-gate` provider lifecycle / cross-contract pattern** — informs
  the cross-contract discipline in
  `docs/roadmap/GOVERNANCE.md`. CasperProver does NOT currently have a
  `kyc_oracle` contract (contrary to `CP_FINAL_TASKS_V2.md` v1); the
  pattern is aspirational for the compliance-gated flow.
- **42-line TLA+ spec stub** — CasperProver already ships a fuller TLA+
  spec (`crypto/formal/*.tla`) with a TLC pass on a small model (commit
  `b6218d8`). The RWA-Sentinel stub is inferior; use only as a source
  of additional invariant ideas, not as a template.

### Reference-only (do NOT port)

- **Deposit-session module.** CasperProver has no matching primitive.
- **UI / wallet integration.** Frontend is CasperProver's own.
- **RWA-specific modules** (asset-registry, custody adapters).
- **Generic MCP / SDK bootstrap.** CasperProver's SDK is already deeper.

## Originality guardrails

Even with the salvage discipline above, some patterns are close enough
to the originals that a reasonable reviewer would flag them as
derivative. To pre-empt this:

- **Provenance doc.** For every pattern ported, add a paragraph to
  `docs/data-room/product/HONESTY.md` (or a dedicated
  `docs/PROVENANCE_NOTES.md`) that names the pattern, cites the source
  repo, and describes the specific substantial rewrite.
- **Diff review.** The reviewing agent MUST diff the ported file
  side-by-side with the source and confirm ≥ 60% of identifiers, ≥ 60%
  of comments, and ≥ 30% of structural layout have been rewritten. If
  the threshold is not met, the port is rejected.
- **License compliance.** RWA-Sentinel is Apache-2.0. Add an
  attribution-only NOTICE entry (not in shipped source, but in the
  provenance doc) per Apache-2.0 §4.

## Milestones

Not scheduled against a fixed calendar — salvage happens if and when
the 30-day roadmap's `BLS_QUORUM.md`, `GOVERNANCE.md`, or
`PROVENANCE.md` implementation reaches the "informed by prior art"
phase. Each port is its own PR.

## Non-goals

- Wholesale merge of the RWA-Sentinel repo.
- A "compatibility layer" translating RWA-Sentinel primitives 1:1.
- Any implication in copy or marketing that CasperProver is a
  continuation of RWA-Sentinel.

## Acceptance criteria (per port)

- [ ] Pattern identified in this document.
- [ ] Diff review passes the ≥60/60/30 rewrite thresholds.
- [ ] Test parity: host-style tests present.
- [ ] Provenance paragraph committed.
- [ ] Reviewer + owner sign-off on the PR.
