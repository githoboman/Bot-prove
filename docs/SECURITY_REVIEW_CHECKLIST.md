# CasperProver — Security Review Checklist

*Backlog 10.7.* Every PR that changes surface (network, contract,
auth, crypto, secrets, deploy pipeline) is reviewed against this
list. Item counts are held small on purpose — checklists over
20 items get skimmed.

## A. Authentication & authorization

- [ ] Every new mutating endpoint sits behind `authMiddleware` or has an explicit "public by design" comment linking to a threat-model row in `docs/SECURITY.md`.
- [ ] `CP_STRICT=1` still fails-loud when API_KEY is unset (tested in `feat/cp-api-key-fail-closed`).
- [ ] No route added under `/admin/*` without matching entry in `docs/ADMIN_SURFACE.md` (planned).
- [ ] Rate-limit budget adjusted only when a threat-model row justifies it.

## B. Cryptography

- [ ] Any new proof/verify path is documented as REAL / SIMULATION per the honest-claims contract (see `docs/JUDGE_GUIDE.md`).
- [ ] Random material sourced from `crypto/rand`, never `math/rand`.
- [ ] Hash functions consistent with the on-chain expectation (SHA-256 or Poseidon per manifest).
- [ ] No hand-rolled pairing or curve arithmetic; only via `gnark` or vetted upstreams.
- [ ] Key material path environment-driven; no on-disk generation without a corresponding `docs/rotation-log.md` entry (planned).

## C. Contracts

- [ ] WASM artifact size delta ≤ 5 KB or justified.
- [ ] New entry-points listed in the contract's `README.md` + `docs/CONTRACT_INVARIANTS.md`.
- [ ] Reentrancy — for anything that calls out to another contract, invariant re-checked at return.
- [ ] Owner-only entry-points guarded by `only_owner!` (or the trait equivalent) plus a matching test.
- [ ] No `panic!` on user input where an `Error::…` return exists.

## D. Data & secrets

- [ ] No new hard-coded credential, endpoint, or hash. Env-driven with `.env.example` update.
- [ ] gitleaks + trufflehog clean on the branch head.
- [ ] `redactMetadata` covers any new metadata key that could plausibly carry PII.
- [ ] `SBOM.json` regenerated if any dependency version changed.

## E. Observability

- [ ] Every new endpoint emits a slog line with request id, latency, status — no PII.
- [ ] Errors have distinct codes so dashboards can page on them (planned Grafana).
- [ ] `X-CP-Deprecation` header preserved on the legacy path if a v1 alias was added.

## F. Docs

- [ ] `CHANGELOG.md` § Unreleased updated.
- [ ] `docs/JUDGE_GUIDE.md` mentions the new surface if it's judge-facing.
- [ ] Threat-model row appended to `docs/SECURITY.md` if the change opens/reduces surface area.

## Sign-off

- Reviewer: `<GitHub handle>`
- Date: `<YYYY-MM-DD>`
- All boxes above are either checked or explicitly waived in the PR description.
- Merge only if the reviewer AND author agree waivers are safe.
