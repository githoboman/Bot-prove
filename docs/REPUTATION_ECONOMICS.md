# Reputation & Slashing Economics — DRAFT model

> **Status.** DRAFT. Off-repo economic design. Not shipped as economic
> primitive on testnet. Not counsel-reviewed. No paid services. Existing
> `stake-slashing` contract (see `docs/KNOWN_LIMITATIONS.md`) is a
> **structural stub** — this document is the roadmap for the actual
> economics that would live on top of it.

## 0. Why this plan exists

The Service already ships a `stake-slashing` contract on testnet (one
of the seven contracts noted in `docs/KNOWN_LIMITATIONS.md`). What it
does today is **mechanical**: it lets a curator flag a receipt and
subtract a fixed penalty from a bonded stake. What it does **not** yet
have is:

- an **economic model** — how much stake, at what price, denominated
  in what, with what payoff structure;
- a **reputation surface** — how honest attesters accumulate signal,
  and how that signal is trusted by counterparties;
- an **appeal path** — how a slashed party contests without depending
  on the same curator that slashed them;
- a **governance boundary** — who is allowed to change the parameters.

This document defines those four things as a *design*, not as code, and
maps them to gates in `docs/MAINNET_LAUNCH_PLAN.md` so no economic
primitive can go live without passing them.

## 1. Actors

- **Attester** — the party (usually the Operator's system) submitting
  Receipts. Owns a bonded stake and a reputation record.
- **Verifier** — any party running `verify.sh` against a Receipt. No
  stake required.
- **Challenger** — a party that formally disputes a Receipt. Posts a
  refundable challenge bond to prevent nuisance disputes.
- **Adjudicator quorum** — M-of-N holders that resolve challenges. On
  testnet: a single admin. On mainnet target: an M-of-N committee whose
  membership is public and rotates.
- **Governance quorum** — a separate M-of-N that can change parameters
  (§8). Never the same identities as the Adjudicator quorum in the same
  epoch — separation of powers.

## 2. Non-scope

- No token launch, no sale, no distribution schedule. The design is
  denominated in **abstract stake units** (`STAKE_UNIT`) and pinned to
  a fiat-equivalent band inside the parameters, not to a market price.
- No trading market. STAKE_UNITs are non-transferable in the bonded
  state; they move only via bonding, unbonding, and slashing.
- No mainnet activation. Everything in this document remains
  `SIMULATION` until Gate G8 of `docs/MAINNET_LAUNCH_PLAN.md`.

## 3. Reputation record

Each Attester has a public record with a small, fixed schema:

- `attestations_total` — count of Receipts submitted.
- `challenges_upheld` — count where Adjudicator ruled the Attester
  wrong (Receipt withdrawn or corrected).
- `challenges_dismissed` — count where Adjudicator ruled the
  Challenger wrong.
- `challenges_pending` — count currently open.
- `bond_current` — bonded STAKE_UNITs.
- `bond_history` — append-only log of bond movements.
- `epoch_first_seen` — provenance for age-weighted trust.

The **reputation score** exposed by the read API is a deterministic
function of those fields — no ML, no opaque model, no operator-tunable
knobs at query time. The formula ships as part of the contract and only
changes via §8 governance.

The default formula (subject to G7 review):

```
raw = attestations_total - k * challenges_upheld
score = clamp( raw * age_weight(epoch_first_seen) , 0 , MAX_SCORE )
```

with `k` heavily punitive (default `k = 20`) so one upheld challenge
wipes out ~20 clean attestations. Age weighting caps at a plateau to
prevent early-Attesters from becoming un-catchable incumbents.

**Score is advisory.** No Service endpoint changes behaviour based on
score; it is a signal for Verifiers and Counterparties, not a gate.

## 4. Bonding

- **Bond size band.** Attesters bond within `[BOND_MIN, BOND_MAX]`
  STAKE_UNITs. The band is a governance parameter (§8). Testnet band:
  `[10, 10_000]`. Mainnet target band: reviewed at G7.
- **Bond ties to Receipts.** Each Receipt implicitly reserves a
  fraction `f_reserve` of the Attester's current bond as claimable
  stake for the challenge window `T_window` (default 30 days).
  Reserved stake cannot be unbonded during the window.
- **Unbonding cooldown.** `T_cooldown` (default 90 days) between
  unbond request and withdrawal. Any challenge opened during cooldown
  extends it to at least `T_window`.
- **Refills.** Attester may top up bond at any time. Top-ups do not
  reset reputation or age weight.

## 5. Challenge lifecycle

1. **Open.** Challenger posts `CHALLENGE_BOND = c_bond * bond_current`
   (default `c_bond = 0.02`) and cites the disputed receipt_digest
   plus a rationale in a bounded schema (integrity / provenance /
   substance).
2. **Response window.** Attester has `T_response` (default 7 days) to
   post a counter-record: either a corrected Receipt (voluntary
   withdrawal — partial refund, see §6), or a defence citing evidence.
3. **Adjudication.** M-of-N Adjudicators produce a decision. Their
   decision is itself a signed Receipt anchored via the standard
   anchor path (AA), so adjudicator dishonesty is publicly detectable.
4. **Settlement.** §6 payoff table applies. All movements are logged
   in the reputation record.
5. **Appeal.** One appeal per challenge, to a *different* Adjudicator
   quorum drawn from the same pool but excluding the original signers.
   Appeal filing requires a doubled challenge bond; loser forfeits it.

The entire lifecycle is bounded — no evergreen disputes.

## 6. Payoff structure

Notation: `S = slashed amount`, `B = challenge bond`, `T = treasury`.

| Outcome                                   | Attester       | Challenger      | Treasury    |
|-------------------------------------------|----------------|-----------------|-------------|
| Challenge upheld (Attester wrong)         | `-S`           | `+B` refund `+r*S` reward | `+(1-r)*S`  |
| Challenge dismissed                       | `+B` (from Challenger) | `-B` | `0`         |
| Attester withdraws early (voluntary)      | `-S_soft` (< S) | `+B` refund     | `+S_soft`   |
| Adjudicator ties / abstains               | `0`            | `+B` refund     | `0`         |
| Frivolous challenge (schema-invalid)      | `0`            | `-B`            | `+B`        |

Defaults (governance-tunable): `r = 0.30` (Challenger reward share),
`S_soft = 0.25 * S` (early withdrawal discount).

Design intent: Challenger reward is **non-zero** so honest challenges
are worth pursuing; Attester's soft-withdrawal discount is **material**
so voluntary correction is genuinely cheaper than fighting a lost case;
the **treasury** never captures a majority of the slash — it is a
distribution mechanism, not a rent extractor.

## 7. Anti-manipulation

- **Sybil resistance.** Reputation score gains scale sub-linearly with
  bond size (default: `age_weight` factor logarithmic in `bond_current`)
  so wealthy Attesters do not automatically dominate the reputation
  surface.
- **Wash-challenge resistance.** A Challenger cannot target the same
  Attester more than `k_target` times in `T_window` (default 3 in
  30 days) without the additional challenges being auto-classified
  frivolous.
- **Collusion resistance.** Adjudicator quorum membership rotates each
  epoch (default epoch = 30 days). Governance signers cannot be
  Adjudicators in the same epoch. Adjudicator votes are individually
  signed and public.
- **Bribe resistance.** Rewards flow via on-chain settlement; there is
  no off-chain payout that could conceal a side payment. Adjudicator
  compensation (if any post-G7) is a fixed epoch stipend, not
  per-decision, so a bribe does not scale with case volume.
- **Governance capture resistance.** §8 parameter changes require a
  time-locked delay `T_gov_delay` (default 14 days) between proposal
  and activation, giving Attesters time to unbond if they reject the
  change. Time-lock cannot be shortened except via a governance
  proposal that itself respects it (no bootstrap escape).

## 8. Governance boundary

Parameter matrix (all defaults above are placeholders — G7 review is
where real values are set):

| Parameter        | Owner          | Change path                     |
|------------------|----------------|---------------------------------|
| BOND_MIN, MAX    | Governance     | Time-locked proposal + M-of-N   |
| f_reserve        | Governance     | Time-locked proposal + M-of-N   |
| T_window         | Governance     | Time-locked proposal + M-of-N   |
| T_cooldown       | Governance     | Time-locked proposal + M-of-N   |
| T_response       | Governance     | Time-locked proposal + M-of-N   |
| c_bond           | Governance     | Time-locked proposal + M-of-N   |
| r, S_soft        | Governance     | Time-locked proposal + M-of-N   |
| Adjudicator pool | Governance     | Time-locked proposal + M-of-N   |
| Score formula k  | Governance     | Time-locked proposal + M-of-N   |
| Adjudicator quorum decisions | Adjudicators | Direct signed decision |
| Emergency pause  | Governance     | M-of-N signed, 72h auto-expire  |

Notable: **emergency pause auto-expires** so a paused system cannot be
held hostage indefinitely by silence.

## 9. Interaction with existing packs

- **AA** anchor stubs: adjudication decisions and bond movements are
  anchored via existing AA slots — no new contracts required. If the
  slashing contract in production diverges from the AA anchor schema,
  that is a governance issue, not an architectural one.
- **AB** API hardening: the reputation read surface (`/reputation/*`)
  inherits the AB middleware chain (rate limiting, quotas) and adds no
  new mutation endpoints. All mutation flows through on-chain
  transactions signed by bonded parties.
- **AC** VRF / range proofs: Adjudicator selection MAY use VRF for
  epoch quorum drawing; keeps selection publicly auditable without a
  trusted randomness beacon dependency.
- **AD** SLH-DSA: Governance and Adjudicator signatures MAY be
  post-quantum (SLH-DSA) — decision deferred to G3/G4.
- **AE** provenance vectors: reputation-record updates emit provenance
  records via the same primitive; `verify.sh` covers both surfaces.
- **AF** ceremony: Governance and Adjudicator key material generated
  via the same multi-party ceremony pattern; separate ceremony
  instance, publicly logged.
- **AG** observability: metrics families `reputation_score_update`,
  `challenge_open_total`, `challenge_upheld_total`, `challenge_dismissed_total`,
  `bond_movement_total`, `slash_total_stake_units`, all
  cardinality-bounded.
- **AH** SLO / runbooks: SEV-1 stubs for "governance pause activated",
  "adjudicator quorum drift", "challenge queue backlog"; runbook
  forward-refs prepared.
- **AI** LEGAL: Reputation record is public — TOS.md constrains what
  fields may be included in `attestations_total` provenance; AUP.md
  prohibits reputation-farming (i.e. self-challenge attacks). No
  personal data lands in the reputation record.
- **AJ** HSM / ceremony: Governance and Adjudicator keys sit on same
  HSM plan (G3).
- **AK** mainnet launch: **G7 (financial resilience) explicitly
  gates activation** of this model — real economics do not turn on
  until bond band and payoff parameters have been reviewed against
  cyber-liability and E&O policy limits.
- **AL Confidential Storage**: reputation record is deliberately in
  the *hash-only* surface — no CSL dependency. A Challenger cites
  receipt_digest; if the underlying dispute requires payload
  reproduction, that is a CSL disclosure workflow separate from the
  challenge itself.

## 10. Honesty ladder

- Today: on-chain `stake-slashing` contract is a **structural stub**.
  This document is `SIMULATION`. No stake has real economic
  interpretation on testnet.
- Post-hackathon, pre-mainnet: implement contract-level parameter
  storage and lifecycle transitions with **zero payoff activation**.
  Label `REAL / ON-CHAIN / SIMULATION-ECONOMICS` — the transitions
  are real, the numbers are not.
- After G7 review: activate payoffs. Label `REAL / ON-CHAIN`.
- After first six months of live operation with clean incident record:
  Governance may propose relaxing the initial conservative bands.

## 11. What this plan does NOT do

- Does not launch a token.
- Does not authorise procurement or spend.
- Does not commit to a schedule.
- Does not give the Service operator special authority — Governance
  and Adjudicator quorums are explicitly external to the Service team.
- Does not preempt regulator scrutiny — an Operator running a
  jurisdiction where this model constitutes financial activity must
  clear that themselves (see `LEGAL/TOS.md` operator responsibilities).

## 12. Open questions

1. Should the Challenger reward share `r` be a fixed constant or a
   sliding scale that decreases with challenge volume against the
   same Attester? (Bias: sliding, to reward diverse challenge
   coverage.)
2. Should Adjudicator compensation be zero, a fixed stipend, or a
   share of `T`? (Bias: fixed stipend from Treasury; share-of-T
   creates conflict of interest.)
3. Should Governance and Adjudicator pools be entirely disjoint, or
   overlap with a cooldown? (Bias: disjoint per epoch, cooldown ≥ 2
   epochs to re-enter the other pool.)
4. Should reputation score be usable as an input to `defi-mock` KYC
   gating? (Bias: no — reputation is advisory; KYC gating uses the
   `verifier-gate` primitives directly.)
5. Should we publish an anti-collusion audit alongside the first
   real-payoff epoch? (Bias: yes — publish quorum vote logs, drift
   metrics, and a written attestation from the Adjudicator quorum.)

Answers land here or in a follow-up pack, before any economic activation.

## References

- On-chain `stake-slashing` contract — current structural stub
- `contracts/verifier-gate` — cross-contract verification surface
- `docs/HSM_PLAN.md` (AJ) — key custody for Governance / Adjudicator
- `docs/KEY_CEREMONY_PLAN.md` (AJ) — quorum ceremony
- `docs/OPS_RUNBOOKS.md` (AH) — incident response for governance events
- `docs/MAINNET_LAUNCH_PLAN.md` (AK) — G7 gates financial activation
- `docs/CONFIDENTIAL_STORAGE.md` (AL) — payload disclosure orthogonal
- `LEGAL/TOS.md`, `LEGAL/AUP.md` (AI) — operator obligations, farming ban
- `docs/KNOWN_LIMITATIONS.md` — honesty ladder
