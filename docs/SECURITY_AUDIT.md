# CasperProver — Security Audit: Owner/Admin Lifecycle & Cross-Contract Invariants

**Scope:** all 9 contract crates in `contracts/` (all deployed as of 2026-07-27).  
**Date:** 2026-07-20 (Gate 1, item 4 of `CP_FINAL_TASKS_V2_new.md`); extended 2026-07-27 to cover `governance` and `zk-verifier` after they went live, and to close a critical finding in `zk-verifier`.  
**Reviewer:** in-repo static audit (no on-chain formal verification tool).  
**Purpose:** document the actual ownership model per contract, flag *irreversible* privilege paths, and record cross-contract reentrancy/invariant reasoning **before** any redeploy of Gate 2.

Nothing in this document changes contract source. It is the reference security review that must be re-read whenever a new privileged entry point is added.

---

## 1. Ownership model — cheat sheet

| Contract | Admin key | Set at | Mutable? | Renounce? | Entry points gated | Verdict |
|---|---|---|---|---|---|---|
| `defi-mock` | `ADMIN_KEY` → `Key::Account(deployer)` | `call()` (install) | No | No | `grant_access`, `revoke_access` | **Safe — no privilege escalation, no lost-key risk beyond deployer wallet.** |
| `verifier-gate` | none | — | — | — | none (rate-limit is per-caller, not admin-gated) | **Safe — read-only proxy over proof-registry.** |
| `stake-slashing` | none | — | — | — | none (all state gated by caller identity or proof-registry state) | **Safe — permissionless-by-design.** |
| `proof-registry` | none (agent = record.owner) | per-record | Per-record `agent` string | No | `revoke_proof` (agent-only) | **Safe — no global admin.** |
| `proof-of-inference` | `INSTALLER_KEY` → `Key::Account(installer)` | `call()` | No | No | `resolve_challenge`, `register_verifier`, `revoke_verifier` | **Advisory: hot deployer key. Add timelock roadmap.** |
| `model-registry` | `INSTALLER_KEY` (for `set_price_bps` only) + per-model `owner` string | `call()` / `register_model` | Per-model transferable | **No renounce** (transfer only) | `set_price_bps` (installer), `deprecate_model` / `update_model_metadata` / `transfer_ownership` (per-model owner) | **Safe — dual-tier, no burn address.** |
| `proof-aggregation` | `INSTALLER_KEY` → `Key::Account(installer)` | `call()` | No | No | `finalize_batch` | **Advisory: hot deployer key. Same roadmap as proof-of-inference.** |
| `stake-slashing-session` | n/a (session code, not a contract) | — | — | — | — | Session runs under caller's own account context. |
| `governance` | `owner` → `Key::Account(caller)` at install | `call()` | Yes — 48h-timelocked transfer, or 2-of-3 guardian recovery | No | `propose`/`execute`/`cancel`, `emergency_pause`, `propose_unpause`/`execute_unpause`, `propose_owner_transfer`/`execute_owner_transfer` (owner-only); `sign_recovery`/`execute_recovery` (guardian-only) | **Safe — see 2.9 for guardian/pause caveats.** |
| `zk-verifier` | `owner` → `Key::Account(caller)` at install (redeployed 2026-07-28 to a new dedicated wallet, not `anna-stolbovskaja` — see 2.10) | `call()` | No direct rotation entry point (relies on `governance` externally) | No | `add_verifier`/`remove_verifier`/`revoke_verdict`/`pause`/`unpause` (owner-only); `register_vk`/`disable_vk` (owner-only — bypass fixed and live, see 2.10) | **🟢 Fixed and verified live on-chain, see 2.10.** |

**Global observation.** No contract exposes a `renounce_ownership()` entry point today. This is intentional: an irreversible renounce on a testnet demo with no recovery/timelock would be a footgun. Whenever renounce is added later (roadmap item), it must ship together with:

1. A timelocked propose → confirm → activate flow (min 48h).
2. An emergency-pause switch that survives renounce (multi-sig or on-chain governance module).
3. A documented recovery path if the successor key is lost.

Without those three, **do not merge a renounce entry point in any contract**.

---

## 2. Per-contract audit notes

### 2.1 `defi-mock` (deployed)

- **Admin:** `Key::Account(deployer)` captured in `call()` as `ADMIN_KEY`.
- **Gated entry points:**
  - `grant_access(user, proof_id)` — refuses to overwrite an active whitelist entry (post-2026-07-18 hardening — must go through `revoke_access` first, keeps on-chain history explicit).
  - `revoke_access(user)` — writes tombstone `(user, "", 0)` (post-fix: `is_whitelisted` returns `false` on `ts == 0`).
- **Cross-contract:** calls `verifier-gate::is_valid(proof_id)` **before** whitelist mutation (checks-effects-interactions ordering respected — external call cannot re-enter to force a spurious `true`).
- **Reentrancy analysis:** `call_is_valid` is a *read-only* proxy that ends in `runtime::ret`. It cannot mutate this contract's state, and if it did (via a malicious swapped verifier), the current implementation still checks the returned `bool` and reverts on `false`. Only worry vector: admin replaces `VERIFIER_HASH` — but there is **no entry point to update `VERIFIER_HASH` post-install**, so the trust anchor is immutable-at-install.
- **Renounce risk:** admin key lost = whitelist is frozen (no new grants/revokes). Read paths (`check_kyc`, `is_whitelisted`) continue to work. **Not catastrophic.**
- **Verdict:** ✅ Safe as-is. No changes recommended before redeploy.

### 2.2 `verifier-gate` (deployed)

- **Admin:** none. Contract has no privileged operations.
- **Gated entry points:** none. `is_valid`, `is_valid_batch`, and `get_verify_count` are all public reads.
- **Cross-contract:** calls `proof-registry::get_proof`. Read-only, cannot cause state-drift.
- **Rate limit:** per-caller `VERIFY_COUNTS` dictionary + `ERR_RATE_LIMIT` revert. This is bookkeeping, not a privilege check.
- **Reentrancy analysis:** `runtime::call_contract` to proof-registry returns typed data; the contract does not modify state after the call (only `runtime::ret`). No CEI concern.
- **Renounce risk:** none — no admin exists.
- **Verdict:** ✅ Safe as-is.

### 2.3 `stake-slashing` (deployed, hardened 2026-07-18/19)

- **Admin:** none. Deliberately permissionless — anyone can call `report_and_slash` because the only trigger is that proof-registry already recorded a revocation.
- **Gated by caller identity (not admin):**
  - `record_stake` — credits *the caller* only. Cannot be used to inflate a third party's stake.
  - `unstake` — withdraws from *the caller's* recorded stake only, using `checked_sub` (no underflow).
- **Cross-contract:** calls `proof-registry::get_proof` (read-only) before slashing.
- **Reentrancy analysis:** the only external interaction after state mutation is `system::transfer_from_purse_to_account` in `unstake` and the `report_and_slash` payout. Both use Casper's native transfer primitives, not contract calls back into this crate, so there is no callback surface to re-enter. State (`stakes`, `slashed_proofs`, `total_recorded`) is updated *before* the transfer in both paths (checks-effects-interactions respected).
- **Bookkeeping invariant:** `sum(stakes) + slash_reserve ≤ contract_purse_balance` — enforced by:
  - `record_stake` capping credit to `actual_balance − total_recorded` (post-2026-07-18 hardening — prevents unbacked credit).
  - `decrease_total_recorded()` on every `unstake` / `report_and_slash` payout.
- **Zero-slash invariant:** `ERR_NO_SLASHABLE_STAKE` (post-2026-07-19 fix) prevents burning the one-shot slash tombstone with a zero payout — closes the "consume tombstone, pay nothing" attack.
- **Renounce risk:** none — no admin exists.
- **Verdict:** ✅ Safe as-is. Prior hardening (record_stake self-verification, zero-slash guard, checked arithmetic) is preserved.

### 2.4 `proof-registry` (deployed)

- **Admin:** none globally. Per-record: `agent` field stored at registration (`(agent_id, owner, model_hash)` tuple).
- **Gated entry points:** `revoke_proof` — only the recorded `agent` may revoke their own proof.
- **Cross-contract:** none — `proof-registry` is a leaf contract.
- **Reentrancy analysis:** no external calls; only local dictionary updates.
- **Renounce risk:** none globally. A specific agent losing their key freezes only that agent's own proofs (they cannot self-revoke), which is a per-user operational issue, not a systemic one.
- **Verdict:** ✅ Safe as-is.

### 2.5 `proof-of-inference` (deployed 2026-07-25)

- **Admin:** `INSTALLER_KEY` → `Key::Account(installer)`, set in `call()`.
- **Gated entry points:**
  - `resolve_challenge(proof_id, valid)` — installer-only; can decide rejected/resolved verdict.
  - `register_verifier(verifier_id, pub_key)` — installer-only.
  - `revoke_verifier(verifier_id)` — installer-only.
- **Cross-contract:** none.
- **Reentrancy analysis:** no external calls. All state transitions are local dictionary updates with prior status checks (`ERR_INVALID_STATUS`, `ERR_ALREADY_VERIFIED`, `ERR_NOT_CHALLENGED`).
- **Renounce risk:** if the installer key is lost, the verifier roster is frozen (no add/revoke) and **challenges cannot be resolved** (permanently stuck at status 2). This is a **hot-key concentration risk**. Advisory below.
- **Recommended before deploy (non-blocking for Gate 2):**
  - Add a `pending_installer` + `accept_installer` two-step transfer entry point, gated by the current installer, so the key can rotate before it is lost.
  - Do **not** add `renounce_installer` — irreversible on a demo with no timelock.
- **Verdict:** ⚠️ Deploy as-is is acceptable for the hackathon (single-owner, testnet). Timelocked two-step transfer is a 30-day roadmap item.

### 2.6 `model-registry` (deployed 2026-07-25)

- **Admin (dual-tier):**
  - `INSTALLER_KEY` → global, for `set_price_bps` only.
  - Per-model `owner` string (stored inside each `ModelRecord`) → for `deprecate_model`, `update_model_metadata`, `transfer_ownership`.
- **Ownership transfer:** `transfer_ownership(model_hash, new_owner)` — current owner only. **No renounce entry point** (cannot transfer to `AccountHash::default()` because the record still requires *someone* to own it for downstream deprecate/update calls). This is the correct posture.
- **Cross-contract:** none.
- **Reentrancy analysis:** no external calls. Status transitions (`ERR_ALREADY_DEPRECATED`, `ERR_ALREADY_REGISTERED`) prevent double-updates.
- **Renounce risk:** installer-key loss freezes `set_price_bps` (pricing knob). Per-model owner-key loss freezes that model's lifecycle. Both are per-record blast radius, not systemic.
- **Owner string format caveat:** `owner` is stored as `format!("{:?}", caller)` (Rust `Debug` output of `AccountHash`). This is *stable within a single Rust toolchain build* but has bitten other Casper projects on version bumps. **Follow-up:** normalize to `caller.to_string()` (hex, no prefix) in the next non-breaking migration; currently a comparison bug not a security bug because writer and comparator both use the same `Debug` format.
- **Verdict:** ⚠️ Deploy as-is is acceptable. The `owner` string format normalization is a P2 hygiene item — file as follow-up.

### 2.7 `proof-aggregation` (deployed 2026-07-25)

- **Admin:** `INSTALLER_KEY` → `Key::Account(installer)`, set in `call()`.
- **Gated entry points:** `finalize_batch(batch_id)` — installer-only via `ApiError::PermissionDenied`.
- **Ungated writes:** `create_batch`, `add_proof` — public. **This is a design choice** (permissionless Merkle-batch construction; only finalization is trusted), but there is no per-batch access control:
  - Anyone can call `add_proof` against an existing `batch_id`.
  - `add_proof` writes to key `format!("{}:{}", batch_id, proof_hash)` — duplicates by different callers overwrite each other **only if `proof_hash` is identical**, which is fine.
  - **Fixed (2026-07-25, verified in code 2026-07-27):** `create_batch` now checks `storage::dictionary_get(batches_uref, &batch_id)` and reverts with `ApiError::User(22)` if the batch already exists, before writing — the silent-overwrite path this audit originally flagged as P1 no longer exists. The fix shipped in the same deploy that made this contract live, so the live testnet instance was never exposed to the original bug.
- **Cross-contract:** none.
- **Reentrancy analysis:** no external calls.
- **Renounce risk:** installer-key loss freezes finalization → batches never close. Per-batch blast radius.
- **Verdict:** ✅ Safe as deployed. Original P1 (`create_batch` overwrite) closed before this crate went live; item 1 in section 4 below is resolved.

### 2.8 `stake-slashing-session`

- **Session code, not a contract** — runs with the caller's own account context, no installer, no cross-account privilege.
- **Purpose:** atomically transfer CSPR from caller's main purse to `stake-slashing`'s contract purse **and** call `record_stake` in the same deploy so they cannot be split/front-run.
- **Verdict:** ✅ Safe by construction — session code has no persistent state to attack.

### 2.9 `governance` (deployed 2026-07-26)

- **Admin:** `owner` → `Key::Account(caller)`, bootstrapped in `call()`. Up to 3 guardians (account-hash strings) set at install (`guardian_1/2/3`); the live deploy uses 2 real guardians (`anna`, `defi_mock_owner`) and a reserved zero-hex third slot for mainnet.
- **Gated entry points:**
  - `propose` / `execute` / `cancel`, `propose_unpause` / `execute_unpause`, `propose_owner_transfer` / `execute_owner_transfer` — owner-only.
  - `emergency_pause` — owner **or** any guardian (fast lane, no timelock).
  - `sign_recovery` / `execute_recovery` — guardian-only, 2-of-3 threshold.
- **Timelock:** all owner-gated state changes (except the pause fast-lane) require a `propose` now → `execute` ≥48h later. `next_proposal_id` uses `checked_add` (no overflow panic path), and `executable_at` uses `saturating_add`/`saturating_mul` — no arithmetic panics on adversarial input.
- **Reentrancy analysis:** no cross-contract calls at all — every entry point is local dictionary/uref reads and writes. No callback surface.
- **Design caveat found in this audit — recovery-vs-pause ordering:** `propose_unpause` / `execute_unpause` are owner-only, not guardian-callable. If the owner key is the one that's lost or compromised (the scenario guardian recovery exists to solve) **and** the system is paused at that moment, guardians must first run `sign_recovery` + `execute_recovery` to become the new owner *before* they can unpause — there is no direct guardian unpause path. This is a two-step recovery, not a broken one, but it should be documented in the runbook so guardians don't expect a one-call unpause during an incident.
- **Design caveat — no guardian rotation:** guardians are fixed at install time; there is no entry point to replace a guardian whose key is lost or compromised. Same hot-key-concentration pattern already flagged for `proof-of-inference`/`proof-aggregation`/`model-registry` installer keys — same recommendation applies (roadmap item, non-blocking for the hackathon).
- **Process note:** `execute_recovery` rotates `owner` but does not touch `PROPOSALS_DICT`. If an owner key is compromised, an attacker-as-owner could `propose_owner_transfer` (or any other proposal) before losing control; after guardians recover ownership, the new owner should audit and `cancel` any pending proposals from before the recovery rather than assume the slate is clean.
- **Verdict:** ✅ Safe. Two caveats above are process/roadmap items, not exploitable bugs — no external caller can act without owner or guardian keys.

### 2.10 `zk-verifier` (deployed 2026-07-27) — 🔴 CRITICAL finding

- **Admin:** `owner` → `Key::Account(caller)` at install (`anna-stolbovskaja`, same account as the other 8 canonical contracts).
- **Gated entry points:** `add_verifier` / `remove_verifier` / `revoke_verdict` / `pause` / `unpause` — owner-only, no issues found.
- **`register_vk` / `disable_vk` — 🔴 CRITICAL access-control bypass.** Both entry points call `require_owner_or_gov(governance_approved)`, where `governance_approved: u64` is a **plain caller-supplied named argument on the deploy**, not a value read from the `governance` contract on-chain:
  ```rust
  fn require_owner_or_gov(governance_approved: u64) {
      let caller = runtime::get_caller();
      if caller == get_owner() { return; }
      if governance_approved != 1 {
          runtime::revert(ApiError::User(ERR_NOT_OWNER));
      }
      // Session-based gov approval - deployer's session code must have called
      // governance.is_executed before us and gated their runtime path on it.
      // No cross-contract call needed here.
  }
  ```
  There is **no cross-contract call to `governance.is_executed`, no signature, no proof of any kind** — the function trusts the caller's own claim. **Any account that can submit a deploy to this contract can call `register_vk` with `governance_approved: 1` and register an arbitrary verifying key for any `circuit_id`, or `disable_vk` to kill an existing one — completely bypassing both the owner check and the governance timelock.** The in-code comment claims enforcement happens in "the deployer session (see `scripts/register-vk.mjs`)" — **that script does not exist anywhere in this repository** (confirmed via full-repo search), and even if it did, a client-side script cannot constrain what a different, adversarial deploy submits directly to the chain. This is not a theoretical gap: it is a live, unauthenticated privilege escalation on a contract deployed to testnet two days ago.
  - **Impact:** since `record_verdict` requires the vk for a `circuit_id` to be `active` (checked via this same dictionary), an attacker who swaps a circuit's `vk_hash` can make `record_verdict` accept verdicts that should be tied to a different (attacker-controlled) verifying key — undermining the "real ZK verification anchored on-chain" claim this whole contract exists to back. Blast radius is scoped to vk metadata and verdict anchoring (no fund custody in this contract), but it directly contradicts the project's core "trustless, not a mockup" pitch.
  - **Fix status (2026-07-28): FIXED AND LIVE ON TESTNET.** The bypass is fixed in source (`require_owner_or_gov` now delegates to `require_owner()` unconditionally), covered by a new regression test (`gov_approved_flag_no_longer_bypasses_owner_check`), 105/105 contract tests pass, MVP-clean wasm. Redeployed successfully at `contract_hash 4500da5d…dc96a1` / `package_hash 1b4faa04…7564f1a` (deploy `0c5f87ec…9eaacc`, block 8645946, `error_message: null`) — see `docs/roadmap/ZK_VERIFIER_REDEPLOY_2026-07-27.md` for the full root-cause writeup and fingerprint. **Live regression-tested, not just deployed:** calling `register_vk(governance_approved=1, ...)` from a non-owner account now reverts `User error: 1` (`ERR_NOT_OWNER`); the same call from the contract's owner succeeds. The bypass is closed on-chain, verified, not just in source.
  - The three initial redeploy attempts that failed were misdiagnosed at the time as a `CSPR_CLOUD_API_KEY`/auth blocker. The real cause (see the redeploy doc): `storage::new_dictionary`/`new_uref` write NamedKeys onto the *calling account*, and `anna-stolbovskaja` already held `vks`/`verdicts`/`verifiers` from the original install two days earlier — every attempt reverted with `ApiError::InvalidArgument` before reaching any contract logic, independent of wasm content or API key validity. Fixed by deploying from a fresh, never-used wallet (consistent with `docs/DEPLOYMENT_LESSONS.md` Lesson 3, previously documented for a different contract but not connected to this failure until now).
  - **⚠️ Owner-account inconsistency introduced by the fix:** the redeploy's owner/admin key is a **new dedicated wallet, not `anna-stolbovskaja`** — the only one of CP's 9 contracts where this differs. No `transfer_owner` entry point exists to change this later. See the redeploy doc for the new deployer key fingerprint.
  - **Still open:** the live CP API's `CONTRACT_ZK_VERIFIER` env var (Render) has not yet been updated to the new contract_hash — until it is, `register_vk`/`disable_vk`/`get_vk`/`is_active_vk` calls routed through the backend (not direct chain calls) will resolve to the stale/empty value.
  - **Verdict:** 🟢 **Fixed and verified on-chain.** Backend env var propagation is the one remaining follow-up (tracked above), not a contract-level risk.
- **Other entry points:** `get_vk` / `is_active_vk` / `get_verdict` are public reads, no issue. `record_verdict` requires an active registered verifier (owner-managed allowlist) and an active vk — correctly gated, but inherits the vk-integrity problem above since the vk itself can be swapped by anyone.
- **Reentrancy analysis:** no cross-contract calls at all (by design — see the file's own header comment on wasm-size constraints). No callback surface.

---

## 3. Cross-contract invariants (system-level)

### 3.1 Call graph

```
defi-mock.grant_access
  └─ verifier-gate.is_valid
       └─ proof-registry.get_proof                   (read)

stake-slashing.report_and_slash
  └─ proof-registry.get_proof                        (read)

stake-slashing.record_stake
  └─ system::get_purse_balance(contract_purse)       (read, native)

stake-slashing.unstake / .report_and_slash payout
  └─ system::transfer_from_purse_to_account          (native transfer)
```

Every cross-contract edge is either **read-only** (returns via `runtime::ret`) or a **native system transfer** (not a contract-to-contract call that could re-enter). There is no callback path that lets a downstream contract re-enter an upstream contract in mid-write. **Casper's `call_contract` is synchronous but the contract being called cannot re-enter the caller unless the caller passes its own contract hash as an argument** — and none of our contracts do this.

### 3.2 CEI (checks-effects-interactions) posture

| Contract & entry point | Order | Notes |
|---|---|---|
| `defi-mock.grant_access` | Check (admin), Check (`ALREADY_WHITELISTED`), Effect-less external call (`is_valid`), Check (result), Effect (write whitelist) | External call is read-only; even if reordered, no state-drift. |
| `stake-slashing.record_stake` | Check (`available` vs claimed), Effect (write stakes + total_recorded) | No external call after state write. |
| `stake-slashing.unstake` | Check (`checked_sub`), Effect (dict write), Interaction (`transfer_from_purse_to_account`), Effect (`decrease_total_recorded`) | Slight CEI drift: `total_recorded` decreases *after* transfer. Native transfer cannot reenter, so acceptable. Documented here so future reviewers don't refactor blind. |
| `stake-slashing.report_and_slash` | Check (`ALREADY_SLASHED`), Interaction (`get_proof` — read), Check (`AGENT_MISMATCH`, `NOT_REVOKED`, `NO_SLASHABLE_STAKE`), Effect (stakes + slashed dict), Interaction (transfer), Effect (`decrease_total_recorded`) | Same "native transfer at end" pattern. Safe. |
| `proof-of-inference.*` | Check → Effect only | No external calls. |
| `model-registry.*` | Check → Effect only | No external calls. |
| `proof-aggregation.create_batch/add_proof/finalize_batch` | Check → Effect only | No external calls. |

**No entry point in the system performs an external contract call after writing state and before finishing.** The two `transfer_from_purse_to_account` cases in `stake-slashing` are the only interactions after state effects, and they are native system transfers with no callback surface. Reentrancy risk is minimal.

### 3.3 Trust boundaries at install time

Every cross-contract reference is written to `NamedKeys` inside `call()` (install-time only). There is **no entry point in any contract to update `VERIFIER_HASH` / `PROOF_REGISTRY_HASH` / etc. post-install.** This is intentional: it removes an entire class of "swap a trusted dependency" attacks. Cost: to upgrade, we redeploy and re-wire — which is what we already do.

---

## 4. Follow-up items (opened as GitHub issues after this audit lands)

0. ~~**[P0, blocking, added 2026-07-27]** `zk-verifier.register_vk` / `disable_vk` trust a caller-supplied `governance_approved` flag with zero on-chain verification.~~ **Done** — fixed, redeployed, and live-regression-tested on-chain 2026-07-28 (see 2.10). The Render `CONTRACT_ZK_VERIFIER` env var was updated the same day and reconfirmed live via `/health`; fully closed, nothing outstanding.
1. ~~**[P1, pre-Gate 2 blocking for `proof-aggregation`]** Fix `create_batch` silent overwrite — 10 LOC, add `ERR_BATCH_EXISTS` guard.~~ **Done** — fixed before the 2026-07-25 deploy, verified in code 2026-07-27 (see 2.7).
2. **[P2, roadmap 30-day, NOT recommended pre-submission]** Two-step installer transfer (`pending_installer` + `accept_installer`) in `proof-of-inference`, `proof-aggregation`, `model-registry`. Same pattern applies to `governance`'s fixed guardian set (no rotation entry point today). **No** `renounce_installer` without a timelock module. Requires redeploying 3 contracts — the zk-verifier redeploy on 2026-07-28 already showed this class of change can fragment the owner-account set (see 2.10); doing it again this close to submission is not worth the risk for a cosmetic-today improvement.
3. **[P2, hygiene, NOT recommended pre-submission]** Normalize `model-registry`'s per-model `owner` from `format!("{:?}", caller)` to `caller.to_string()` — non-breaking behavior today but toolchain-fragile. Also requires a redeploy; same reasoning as item 2 applies.
4. ~~**[P3, doc-only]** Add this file's summary table to `docs/ARCHITECTURE.md`'s security section so it's linked from the top-level architecture doc, not only referenced here.~~ **Done 2026-07-28** — ownership cheat sheet embedded in `docs/ARCHITECTURE.md`'s Security posture section.
5. ~~**[P3, runbook]** Document in the ops runbook that `governance` guardians cannot directly unpause — they must complete `sign_recovery`/`execute_recovery` to become owner first if the owner key itself is the one lost during an incident (see 2.9).~~ **Done 2026-07-28** — added as `docs/OPS_RUNBOOKS.md` §4.3.

---

## 5. Sign-off

- Static audit complete on the pinned tree, extended 2026-07-27 to cover all 9 deployed contracts (`governance` and `zk-verifier` added).
- **One P0 finding, now fully closed: `zk-verifier`'s `governance_approved` flag was an unauthenticated self-assertion with no on-chain enforcement — fixed, redeployed, live-regression-tested, and the Render `CONTRACT_ZK_VERIFIER` env var updated + reconfirmed via `/health`, all 2026-07-28 (2.10). Nothing outstanding.**
- All P3 doc-only follow-ups (items 4 and 5 above) closed 2026-07-28. Remaining P2 items (2 and 3) are deliberately deferred pre-submission — both require a contract redeploy, which risks repeating the owner-account fragmentation seen with `zk-verifier` in 2.10, for cosmetic-only gain today.
- The previously-flagged P1 (`proof-aggregation.create_batch` overwrite) was fixed before its contract went live — confirmed resolved.
- Reentrancy: analyzed across all 9 contracts, low risk — every cross-contract call is read-only or a native transfer, and no callback surface exists; `governance` and `zk-verifier` make no cross-contract calls at all.
- Ownership: single-owner model per contract with clear renounce policy = **no irreversible renounce anywhere**. `governance` adds 48h-timelocked owner rotation and 2-of-3 guardian recovery on top of that baseline.
