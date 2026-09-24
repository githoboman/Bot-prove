# Governance, Emergency Pause, Ownership Transfer/Renounce — Design

Ref: `handoff/CP_FINAL_TASKS_V2.md` §D.

## Problem

The current contracts (`proof-registry`, `verifier-gate`, `defi-mock`,
`stake-slashing`) use single-key admin controls. That is acceptable for a
hackathon demo but not for a production surface. We need:

- Timelocked governance for parameter changes.
- Emergency pause for the whole contract set.
- Ownership transfer with a recovery path.
- Renounce that does not brick the contract if a recovery event is needed
  later.

## Design overview

Add a `governance` contract:

```
governance {
  proposal(id, target_contract, entrypoint, args, delay_seconds)
  vote(id, weight)
  execute(id)              // only after delay expires and quorum passes
  cancel(id)               // proposer or emergency-pause admin
  emergency_pause(target)  // multi-sig admin
  emergency_unpause(target)
  transfer_owner(target, new_owner, cooldown_seconds)
  recover_owner(target, evidence)  // multi-sig admin after transfer failure
}
```

## Timelock model

- Parameter changes (fees, thresholds, addresses) go through `proposal → vote → execute` with `delay_seconds ≥ 24h` by default.
- Emergency actions (pause, recover_owner) bypass the timelock but require
  m-of-n multi-sig where m ≥ 2 and n ≥ 3.
- Every governance event emits a signed evidence blob and anchors the
  hash on-chain so external monitors can subscribe.

## Ownership transfer with recovery

Naïve `renounce` bricks a contract. Instead:

1. `transfer_owner(new_owner, cooldown)` sets a *pending* owner. The
   current owner remains active during `cooldown`.
2. After `cooldown`, the new owner activates by calling
   `accept_owner(proof_of_control)`.
3. If the pending owner does not accept within `cooldown + grace`, the
   transfer expires and the current owner remains.
4. `recover_owner` is available to the multi-sig admin if the new owner
   accepts but is later found compromised (evidence hash bound on-chain).
5. `renounce_owner` is available only after a full audit + governance
   proposal + m-of-n approval; it sets the owner to a burn address AND
   preserves a `recovery_multisig` fallback with a 30-day emergency
   window.

## Emergency pause

- Each protected contract exposes `set_paused(bool)`.
- `governance.emergency_pause(target)` requires m-of-n multi-sig
  signatures and writes the pause state atomically.
- Paused contracts reject all mutating entrypoints but keep read paths
  live so external verifiers can still audit the on-chain state.
- Auto-unpause after N days unless the multi-sig re-signs a "keep paused"
  proposal.

## Slashing interlock

`stake-slashing` becomes a `governance`-consumer:

- Slashing parameters (severity levels, bond thresholds) live in
  `governance`.
- `stake-slashing` reads them via cross-contract call; on read failure,
  falls back to conservative hard-coded defaults.

## Milestones

1. **Contract skeleton (5 days).** Rust crate `contracts/governance/`
   with `checked_*` arithmetic; unit tests.
2. **Cross-contract wiring (5 days).** Modify each protected contract to
   accept a `governance` address at install time and read parameters
   through it.
3. **Multi-sig verifier (5 days).** Off-chain aggregation of m-of-n
   signatures; on-chain verification via the same off-chain-verify /
   on-chain-commit pattern as `docs/roadmap/BLS_QUORUM.md`.
4. **Testnet dry-run (5 days).** Full parameter change + emergency
   pause + recovery flow, reconciled.

## Non-goals

- On-chain voting weight based on token holdings. Not in scope — governance
  is admin-multi-sig, not tokenised DAO.
- Full cross-contract atomic transactions. Casper does not natively
  support them; each cross-call is checked at the boundary.

## Acceptance criteria

- [ ] `contracts/governance/` Rust crate compiles under 65 KiB.
- [ ] Protected contracts read parameters via `governance` with a hard
      fallback.
- [ ] `scripts/mass-runner-governance.mjs` proves the full
      proposal → vote → execute path on testnet.
- [ ] `docs/roadmap/GOVERNANCE.md` cross-linked from `30-DAY.md`.
