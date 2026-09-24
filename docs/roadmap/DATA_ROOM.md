# Investor Data Room — Content Plan

Ref: `handoff/CP_FINAL_TASKS_V2.md` §E.

**Audience:** seed / seed-extension investors evaluating CasperProver as an
"AI accountability infrastructure" company for a regulated buyer profile.

**Constraint:** everything here is *plan*, not vapor. When a claim goes
into the data room it must be backed by an artefact linked from this doc
(a metric, a report, a case study). No hand-waving.

## Structure

```
docs/data-room/
  README.md              — this index
  company/
    ONE_PAGER.md         — 1-page summary
    TEAM.md              — founders + advisors + open roles
    TIMELINE.md          — company + product milestones
  product/
    ARCHITECTURE.md      — the same story as docs/DECISION_LAYER.md but
                            in investor-language
    ROADMAP.md           — 30/90/180-day, symlinked from the roadmap docs
    DEMO.md              — how to reproduce the demo in < 10 min
    HONESTY.md           — symlink to docs/PQ_HONESTY.md; investors love
                            teams that don't overclaim
  market/
    THESIS.md            — why regulated AI/DeFi needs a proof primitive
    ICP.md               — ideal customer profile with 2–3 named archetypes
    COMPETITION.md       — competitors + how we differ; no name-drop
                            without a linked artefact
    TAM_SAM_SOM.md       — market sizing with methodology, not a headline
  traction/
    DESIGN_PARTNERS.md   — signed partners + status + case studies
    METRICS.md           — cohort retention, verifications/mo, API
                            uptime, pipeline; rolled up monthly
    LOG.md               — dated log of major events, so an investor can
                            skim the pace
  financials/
    MODEL.xlsx           — bottom-up model: pricing × usage × cost
    UNIT_ECONOMICS.md    — per-verification COGS, gross margin walk
    HIRING_PLAN.md       — 12-month hiring plan
  legal/
    CAP_TABLE.md         — anonymised until a term sheet is in play
    IP.md                — code + patents; symlink to docs/PQ_HONESTY.md
                            §patents
    AUDIT_REPORTS/       — third-party audit(s) as they land
  ops/
    SLO.md               — symlink to docs/roadmap/SLO.md
    KEY_MANAGEMENT.md    — symlink to docs/roadmap/KEY_MANAGEMENT.md
    STATUS_PAGE_URL.txt  — public status page URL
```

Symlinks (or per-file "source of truth" pointers) keep the roadmap docs
and the data room in sync — a single artefact, two audiences.

## Content standards

- **Numbers require a source.** Every metric line includes the query or
  the artefact hash.
- **Comparisons require artefacts.** "We are faster than X" needs a
  benchmark file committed to the repo.
- **No name-dropping.** Halborn / Allium / NowNodes / any other vendor
  must not appear without a linked report or dashboard.
- **Redaction policy.** Design partners can request their name / metrics
  be redacted; the redacted version is what goes into the data room.

## Milestones

1. **Structure + one-pager (5 days).**
2. **Market thesis + ICP + competition (10 days).**
3. **Design-partner section (10 days).** Requires ≥ 1 signed partner.
4. **Metrics roll-up cron (5 days).**
5. **Financial model (15 days).**
6. **Ops section symlinks + status-page URL (2 days).**
7. **First investor share (soft — advisor review) (5 days).**
8. **First investor share (external — target VC list) (10 days).**

## Non-goals

- A polished pitch deck AS the data room. The deck is the front door;
  the data room is the store.
- Aspirational metrics ("would-be users"). Only shipped numbers.
- A tokenomics section. See `docs/roadmap/90-180-DAY.md#non-goals`.

## Acceptance criteria

- [ ] `docs/data-room/README.md` renders as a valid index.
- [ ] Every claim in the data room has a linked artefact.
- [ ] Every roadmap doc that has a data-room counterpart is symlinked
      (or explicitly cross-referenced).
- [ ] At least one external investor has read the data room and
      returned feedback captured in `docs/data-room/traction/LOG.md`.
