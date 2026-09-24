# Mass runner final report — 2026-07-26T00:44:12.924Z

Reconciled 492 transactions against testnet.cspr.live.

## Totals

- **Total sent**: 492
- **Succeeded on-chain**: 251
- **Errored on-chain**: 241
- **Not found in explorer (finality pending or rejected)**: 0
- **Total gas billed**: 1476.0000 CSPR

## Per-contract

| Contract | Total | Ok | Err | Missing | Gas (CSPR) | Avg (CSPR) |
|---|---|---|---|---|---|---|
| proof_registry | 112 | 73 | 39 | 0 | 336.000 | 4.6027 |
| verifier_gate | 80 | 0 | 80 | 0 | 240.000 | 240.0000 |
| model_registry | 90 | 40 | 50 | 0 | 270.000 | 6.7500 |
| proof_of_inference | 50 | 33 | 17 | 0 | 150.000 | 4.5455 |
| defi_mock | 60 | 5 | 55 | 0 | 180.000 | 36.0000 |
| stake_slashing | 50 | 50 | 0 | 0 | 150.000 | 3.0000 |
| proof_aggregation | 50 | 50 | 0 | 0 | 150.000 | 3.0000 |

## Per-entrypoint

| Contract | Entry point | Ok | Err | Gas (CSPR) | Avg (CSPR) |
|---|---|---|---|---|---|
| proof_registry | submit_proof | 72 | 8 | 240.000 | 3.0000 |
| proof_registry | register_agent | 1 | 11 | 36.000 | 3.0000 |
| proof_registry | revoke_proof | 0 | 20 | 60.000 | 3.0000 |
| verifier_gate | verify | 0 | 60 | 180.000 | 3.0000 |
| verifier_gate | batch_check | 0 | 20 | 60.000 | 3.0000 |
| model_registry | register_model | 20 | 30 | 150.000 | 3.0000 |
| model_registry | update_model | 10 | 10 | 60.000 | 3.0000 |
| model_registry | verify_model | 10 | 10 | 60.000 | 3.0000 |
| proof_of_inference | register_proof | 33 | 7 | 120.000 | 3.0000 |
| proof_of_inference | verify_proof | 0 | 10 | 30.000 | 3.0000 |
| defi_mock | check_kyc | 0 | 40 | 120.000 | 3.0000 |
| defi_mock | grant_access | 0 | 15 | 45.000 | 3.0000 |
| defi_mock | revoke_access | 5 | 0 | 15.000 | 3.0000 |
| stake_slashing | get_purse | 20 | 0 | 60.000 | 3.0000 |
| stake_slashing | get_stake | 30 | 0 | 90.000 | 3.0000 |
| proof_aggregation | add_proof | 40 | 0 | 120.000 | 3.0000 |
| proof_aggregation | create_batch | 5 | 0 | 15.000 | 3.0000 |
| proof_aggregation | finalize_batch | 5 | 0 | 15.000 | 3.0000 |

## Signers

| Signer | Total | Ok | Err | Missing | Gas (CSPR) |
|---|---|---|---|---|---|
| anna | 246 | 130 | 116 | 0 | 738.0000 |
| dmo | 246 | 121 | 125 | 0 | 738.0000 |

## Raw data

- Reconciled per-tx: /data/cp/repo/reports/reconciled-2026-07-26T00-44-12-922Z.jsonl
- Source logs: /data/cp/repo/reports/mass-runner-2026-07-26T00-20-30-590Z.jsonl, /data/cp/repo/reports/mass-runner-2026-07-26T00-22-34-578Z.jsonl, /data/cp/repo/reports/mass-runner-dmo-recover-2026-07-26T00-29-57-517Z.jsonl, /data/cp/repo/reports/mass-runner-dmo-recover-2026-07-26T00-30-52-800Z.jsonl, /data/cp/repo/reports/mass-runner-fix-2026-07-26T00-39-38-168Z.jsonl
