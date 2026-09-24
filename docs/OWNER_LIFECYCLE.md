# Owner Lifecycle

How ownership of the four deployed CasperProver contracts is granted,
transferred, and (deliberately) not renounced. This document is the design
target: not everything below is implemented in the hackathon build. Items
marked **[shipped]** are enforced in source; items marked **[design]** are
what a production version would add and are documented here so reviewers can
see the intent — see the `Roadmap` at the bottom for the concrete tasks.

## Principals

- **`owner`** — the Casper account that deployed the contract. Recorded at
  install time in the `owner` named key of every contract. There is one
  live owner per contract.
- **`pending_owner`** — an account that has been proposed as the next
  owner but has not yet accepted or has not yet cleared the timelock.
  Two-phase acceptance prevents a "typo" transfer from bricking a contract.
  **[design]**
- **`recovery_account`** — an account configured at install time that can
  reset the owner slot if the primary key is lost. Rate-limited (see
  below). **[design]**

## Guarantees

**O-1 · Ownership is always defined.** Every deployed contract has
exactly one `owner` at every point in time. There is no state in which
`owner` is null, absent, or set to the burn address. This holds by
construction: the `owner` named key is written once during `call` and
never deleted. **[shipped]**

**O-2 · Transfer is two-phase and timelocked.** The current owner
proposes a `pending_owner`. The proposed account calls `accept_ownership`
to accept, and only after a timelock delay (default 48h) does the swap
land. Either side can cancel before the timelock expires — the current
owner via `cancel_transfer`, the pending owner by simply not calling
accept. **[design]**

**O-3 · Recovery is bounded.** `recovery_account` can, at any time,
initiate an emergency ownership reset. That reset carries a longer
timelock (default 7d) and is announced on-chain via an event. Within one
timelock window at most one recovery can be pending, and any successful
recovery invalidates the previous `recovery_account` — the new owner must
configure a fresh one. **[design]**

**O-4 · `renounce` is not shipped.** No entry point sets `owner` to a
burn address, a zero key, or the caller-cannot-recover state. The
hackathon build deliberately does not offer irrecoverable renounce
because the failure mode ("congrats, you locked yourself out of the
recovery/timelock path forever") is asymmetric with the benefit
("appear more decentralised"). Renounce, if we ever ship it, must land
alongside a recovery/timelock design; today's contracts don't have one,
so today's contracts don't have a renounce. **[shipped: absence enforced]**

**O-5 · Emergency pause is orthogonal to ownership.** `emergency_pause`
freezes state-changing entry points but does not change `owner` and does
not consume the transfer or recovery timers. Pause is a coarse-grained
kill switch; transfer/recovery is a fine-grained key rotation. **[design]**

## Entry points

| Entry point | Caller | Effect | Timelock | Status |
|-------------|--------|--------|----------|--------|
| `propose_transfer(next)` | `owner` | Sets `pending_owner = next`, records `proposed_at`. Overwrites any existing pending proposal. | none (proposal-only) | [design] |
| `accept_ownership()` | `pending_owner` | If `now - proposed_at >= TRANSFER_TIMELOCK`, swaps `owner ← pending_owner`, clears pending slot. Else reverts. | 48h | [design] |
| `cancel_transfer()` | current `owner` | Clears `pending_owner`. | none | [design] |
| `initiate_recovery(new_owner)` | `recovery_account` | Sets `pending_recovery = (new_owner, now)`. Overwrites any prior pending recovery. Emits `RecoveryInitiated`. | none (initiation only) | [design] |
| `complete_recovery()` | anyone | If `now - pending_recovery.proposed_at >= RECOVERY_TIMELOCK`, swaps `owner`. Emits `RecoveryCompleted`. Invalidates old `recovery_account`. | 7d | [design] |
| `cancel_recovery()` | current `owner` OR `recovery_account` | Clears `pending_recovery`. | none | [design] |
| `emergency_pause()` | `owner` OR `recovery_account` | Sets `paused_at`, all state-mutating entry points early-return with `Error::Paused`. | none (immediate) | [design] |
| `unpause()` | `owner` | Clears `paused_at`. | none | [design] |
| `renounce()` | — | Does not exist. Present neither as entry point nor as reachable code path. | — | [shipped: absence] |

## Threat model

**T-1 · Lost primary key.** Recovery path exists (O-3). Without a
`recovery_account`, the contract is bricked. Every deploy MUST configure
a `recovery_account`; the deploy script (`scripts/deploy.sh`) refuses to
proceed if it's unset. **[design: deploy-script check]**

**T-2 · Compromised primary key.** The attacker calls
`propose_transfer(attacker_key)`. The 48h transfer timelock gives the
`recovery_account` time to observe the on-chain event and invoke
`initiate_recovery` — which starts a 7d clock but also flips
`emergency_pause` implicitly (see O-5, which decouples them, so pause
must be called explicitly). Detection is on the operator via off-chain
monitoring of the `TransferProposed` event.

**T-3 · Compromised recovery key.** The attacker calls
`initiate_recovery(attacker_key)`. The 7d timelock and the on-chain
`RecoveryInitiated` event give the current `owner` time to call
`cancel_recovery`. If both `owner` and `recovery` are compromised
simultaneously — game over; that's a two-key custody failure and no
contract-level design saves you.

**T-4 · Front-running transfer acceptance.** Not applicable: `accept_ownership`
is called by `pending_owner`, whose identity is set by the owner in
`propose_transfer`. An attacker cannot substitute their own key.

**T-5 · Timelock elision via reinstall.** Would require the `owner` to
call `install_contract` with a fresh package hash and migrate state — a
strictly manual operation that would show up as a new `contract_package_hash`
in `onchain.json`. The FE + SDK cache against `contract_package_hash`, so
callers see the swap and can validate the transition.

## Roadmap

- **[design → shipped]** Add `propose_transfer` / `accept_ownership` /
  `cancel_transfer` to all four deployed contracts via a shared
  `Ownable` trait module. Backlog: 1.9.
- **[design → shipped]** Add `emergency_pause` / `unpause` hooks
  wired into all state-mutating entry points. Backlog: 1.10, 1.11.
- **[design → shipped]** Add recovery account configuration to the
  install macro; deploy script refuses if unset. Backlog: 1.9.
- **[shipped]** `renounce` absence is preserved. Any PR that adds a
  `renounce` entry point without shipping a recovery / timelock design
  MUST be rejected during review.

## Related

- `docs/CONTRACT_INVARIANTS.md` — I-1 (Owner isolation), X-3 (cross-contract
  writes on slash), F-2 (`CP_STRICT` fail-closed on anchor).
- `docs/JUDGE_GUIDE.md` — the hardened `stake-slashing v2` redeploy that
  changed the deployer to `defi_mock_owner` to avoid an
  `anna-stolbovskaja` storage collision. That deploy exercised the
  informal recovery path documented above.
- `contracts/*/src/main.rs` — where `owner` is written and read today.
