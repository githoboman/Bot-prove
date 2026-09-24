# SLH-DSA (FIPS 205) — engine/internal/crypto/slhdsa.go

Backlog item **2.10 (partial finish)**.

Real, standardised, post-quantum, stateless hash-based signatures via
`github.com/cloudflare/circl@v1.6.4/sign/slhdsa`. This is what fills the
"SPHINCS+ slot" the earlier Lamport-OTS placeholder occupied — an audited
FIPS 205 implementation from Cloudflare Research, not a hand-rolled scheme.

## Why it replaces Lamport (in new code)

| | Lamport OTS | SLH-DSA (FIPS 205) |
|---|---|---|
| Standardised | ❌ | ✅ NIST FIPS 205 (Aug 2024) |
| Post-quantum | ✅ (hash-based) | ✅ (hash-based) |
| Reuse of one key | ❌ single-use | ✅ safe for many messages |
| Signature size | small | ~7.9 KB (128s) … ~49 KB (256f) |
| Audited implementation | rolled by us | circl (Cloudflare) |
| Backlog slot 2.10 | placeholder | **honest finish** |

Lamport stays in `pq.go` because it is a real, correct OTS and some
narrow protocols specifically want its footprint. New code should reach
for the SLH-DSA API.

## Parameter sets exposed

| Constant | Name | Security cat | Sig size | Speed |
|---|---|---|---|---|
| `SLHDSA128s` | SLH-DSA-SHA2-128s | 1 | ≈ 7.9 KB | slow (small) |
| `SLHDSA128f` | SLH-DSA-SHA2-128f | 1 | ≈ 17 KB | fast |
| `SLHDSA192s` | SLH-DSA-SHA2-192s | 3 | ≈ 16 KB | slow |
| `SLHDSA256s` | SLH-DSA-SHA2-256s | 5 | ≈ 29 KB | slow |

The four SHA-2 variants above are exposed by name. Add SHAKE variants
(`SHAKE_128s` etc.) when a caller needs them.

## API

```go
kp, err := crypto.SLHDSAKeygen(crypto.SLHDSA128s)

sig, err := crypto.SLHDSASign(crypto.SLHDSA128s, kp.Private, msg, nil /* context */)

err  = crypto.SLHDSAVerify(crypto.SLHDSA128s, kp.Public, msg, nil, sig)
```

Signatures are **deterministic** (FIPS 205 `SignDeterministic`) — same
`(priv, msg, ctx)` → byte-identical signature.

## Tests

`slhdsa_test.go`:

- `TestSLHDSA_Roundtrip_All128s`
- `TestSLHDSA_TamperedSig_Rejected`
- `TestSLHDSA_TamperedMessage_Rejected`
- `TestSLHDSA_WrongKey_Rejected`
- `TestSLHDSA_Determinism`
- `TestSLHDSA_UnknownParamSet_Errors`

All 6 pass in <2s.

## Honesty

- **REAL POST-QUANTUM CRYPTOGRAPHY.** Standardised, audited, third-party
  implementation. This is not a simulation.
- **NOT ON-CHAIN.** No Casper Rust contract verifies a FIPS 205 signature
  today. On-chain verification would need either a precompile (Condor
  2.x, not yet available) or an in-contract implementation (large — a
  follow-up ticket).
- **NOT USED IN A HOT PATH YET.** This commit adds the primitive and its
  tests; wiring it into the PQ signature slot in the API layer is a
  follow-up commit (kept small on purpose so review is straightforward).
