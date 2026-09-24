# CasperProver Casper Testnet Deployment — Lessons Learned

**Compiled**: 2026-07-19 after the stake-slashing redeploy at `1ad1b3d9…983d52`.

**Audience**: whoever runs the next contract deploy against `casper-test` — either from `anna-stolbovskaja`, `defi_mock_owner`, or a fresh wallet.

**Source**: real-world diagnosis of failed and successful deploys on 2026-07-18, cross-checked against the `casper-contract` 5.1.1 crate source (not the public docs, which lag).

---

## TL;DR — before you deploy

1. **Toolchain must be `nightly-2025-01-01`** — see `contracts/rust-toolchain.toml`. Bumping this without a live-testnet verification will burn CSPR.
2. **Every `storage::new_contract(...)` deploy needs the `install_or_upgrade` flag on the transaction.** Without it, the chain returns `NotAllowedToAddContractVersion [48]`. This applies **even to fresh packages**, contrary to older internal notes.
3. **`storage::new_dictionary(name)` fails with `InvalidArgument [3]` if the deployer account already holds a named key with that same `name`.** Detect this by calling `state_get_account_info` before submission. If any planned dictionary name collides, deploy from a clean wallet — for CP, that was `defi_mock_owner`, verified clean before use.
4. **Never blind-retry a full deploy.** Every failed installer still costs CSPR. Isolate the failure in a minimal harness (10–20 CSPR) before spending 100+ CSPR on another full attempt.

---

## Lesson 1 — Rust nightly compatibility

### What broke

- `nightly-2025-03-01` produces WASM that Casper testnet preprocessing rejects outright with `Bulk memory operations are not supported`.
- The failure happens **before** the contract executes, so no useful chain-side error is emitted. Testnet payment is still consumed for the deploy attempt.

### Why

Recent rustc/LLVM nightlies emit bulk-memory instructions (`memory.copy`, `memory.fill`, etc.) that Casper's WASM preprocessor doesn't accept. This has nothing to do with the Casper SDK version — it's the codegen path.

### Fix

Pin to `nightly-2025-01-01`. This has been confirmed deploy-compatible by the successful stake-slashing redeploy at `1ad1b3d9…983d52`.

### How to safely bump

1. Change `contracts/rust-toolchain.toml`.
2. Run the full contract build.
3. Deploy the smallest contract in the repo to `casper-test` from a **disposable** wallet with ~200 CSPR balance.
4. Query the deploy result until it succeeds or fails.
5. **Only then** update the pin for the rest of the team.

Do **not** bump based on rustc release notes alone.

---

## Lesson 2 — `install_or_upgrade` is required for every fresh contract

### What broke

The stake-slashing redeploy failed on the first `storage::new_contract(...)` call with `NotAllowedToAddContractVersion [48]` — even though the package was fresh.

### The wrong hypothesis

An earlier internal note in the shared skill said "`install_or_upgrade` rejects WASM > 64KB". That was **verified false** during this redeploy — the SDK accepts hundreds of KB with the flag set. The 64KB limit belongs to a different code path (session-code payload size), not to the install-or-upgrade flag.

### Why `install_or_upgrade` is actually required

Reading the `casper-contract` 5.1.1 crate source directly (not the docs):

- `storage::new_contract(...)` unconditionally attempts to associate the new contract with the caller's account permissions.
- Without `install_or_upgrade` set on the transaction envelope, the chain treats the call as a **version upgrade** of an existing (nonexistent) contract → `NotAllowedToAddContractVersion`.
- The flag tells the chain: "This may be a fresh install OR an upgrade — handle both."

### Fix

Set the flag when constructing the transaction:

```rust
// pseudo — actual SDK call depends on which Casper SDK you use
transaction_builder
    .with_install_or_upgrade(true)
    .with_session_wasm(wasm_bytes)
    .build()?;
```

Verify by building the WASM (any reasonable size — the 64KB story is wrong), then deploying with the flag set.

---

## Lesson 3 — `storage::new_dictionary` and named-key collisions (the CP-specific one)

### What broke

Several stake-slashing redeploy attempts from `anna-stolbovskaja` failed with `ApiError::InvalidArgument [3]` at the `storage::new_dictionary("stakes")` call — even though the args looked correct on paper.

The error was silent about the cause; from the outside it looked like a corrupt argument.

### Why (from `casper-contract` crate source)

`storage::new_dictionary(name)` requires that the deployer's account **does not already own a named key with that exact name**. It doesn't merge, it doesn't reuse — it rejects.

The `anna-stolbovskaja` account, from the initial CP deploy months earlier, still held named keys `stakes` and `slashed_proofs`. So any re-`new_dictionary` call from that wallet was doomed. The named keys aren't automatically cleaned up between contract versions.

### The successful path this time

**Deploy from `defi_mock_owner` instead of `anna-stolbovskaja`.**

- `defi_mock_owner` is the same-project wallet used to fund CP-side experiments and had not been used for a contract deploy before.
- We verified it was clean via `state_get_account_info` before submitting.
- Result: stake-slashing redeploy succeeded at `1ad1b3d9…983d52` (deploy `ac4712a3…2532`), superseding the old `cf70e1fe…d9bd1`.

### For the next CP deploy

- Any future stake-slashing or proof-of-inference redeploy from `anna-stolbovskaja` will hit the same wall for any dictionary that already exists on that account.
- Options, in order of preference:
  1. Deploy from a still-clean same-project wallet (`defi_mock_owner` is now dirty — a fresh project wallet is the cleanest path).
  2. Rename the dictionaries in the new contract version (breaks existing state readers — not recommended).
  3. Add cleanup logic in the contract itself before `new_dictionary` — non-trivial and error-prone.

Option 1 is the proven path.

---

## Lesson 4 — Economic discipline

### What broke

Multiple blind full-deploy attempts of stake-slashing burned testnet CSPR before we understood the root cause. Retrying without diagnosis is expensive.

### The rule

**Every failed installer costs real CSPR. Diagnose in isolation before retrying.**

For CP in particular, an **installer-isolation harness** looks like:

1. Take the failing constructor call sequence (the specific `storage::new_dictionary` chain).
2. Wrap it in a minimal contract that does *only* that sequence and stops.
3. Deploy the minimal harness (10–20 CSPR, not 100+ for the full contract).
4. Read the exact error on the harness alone.
5. Fix root cause (dictionary rename or wallet swap).
6. Only then re-run the full deploy.

For CP this caught the named-key collision at ~30 CSPR total instead of another 200+.

### For the next agent

If you see a testnet failure and your instinct is "let me just try again with slightly different args" — **stop**. Isolate first. The chain will not tell you what's wrong beyond a numeric error code, and the numeric codes are ambiguous. The crate source is the ground truth.

---

## Open items (not yet closed, need production access)

These require credentials no automated agent currently holds. Track them here so they don't get lost:

- [ ] **Prod env vars on Render** — CP backend's `STAKE_SLASHING_CONTRACT_HASH` / `STAKE_SLASHING_PACKAGE_HASH` (and any equivalents in the frontend deploy target) still point to the old `cf70e1fe…d9bd1` address. Must be updated to `1ad1b3d9…983d52` / `e33812f9…e2947`.
- [ ] **Live smoke test** — one on-chain `stake(...)` transaction followed by one deliberate `slash(...)` on the new contract, then a `slash(...)` with amount=0 (must be rejected). This is the final acceptance check that the redeploy actually delivers the invariants it claimed (zero-value slash rejection, checked arithmetic).
- [ ] **`judge_demo.py` real-testnet run** — the script exists (`scripts/judge_demo.py`, committed in 9f392dd), but has it been end-to-end against the new `1ad1b3d9…` contract? Judges following `docs/JUDGE_GUIDE.md` will expect green.
- [ ] **Fresh project wallet** — `defi_mock_owner` is now dirty (holds CP `stakes`/`slashed_proofs` named keys from this redeploy). A next-redeploy plan should provision a new clean wallet ahead of time, not scramble for one at deploy time.

---

## Change log

- **2026-07-19** — File created after stake-slashing redeploy at `1ad1b3d9…983d52`. Root causes for previous failed attempts documented from `casper-contract` 5.1.1 crate source.
