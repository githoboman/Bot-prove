# Contributing

## Requirements

- Go **1.24+**
- Rust **nightly-2025-01-15** (or newer 2024-edition nightly), with
  `wasm32-unknown-unknown` target installed.
- Node **20+** (frontend / SDK-TS).
- Python **3.11+** (SDK-Py + `judge_demo.py`).

## Build

```bash
# API + CLI
cd engine && go build ./...

# Contracts (deployed 4)
cd contracts/proof-registry && cargo +nightly build --release --target wasm32-unknown-unknown --no-default-features

# Contracts (undeployed 3 — parity)
for c in proof-of-inference model-registry proof-aggregation; do
  cd contracts/$c && cargo +nightly build --release --target wasm32-unknown-unknown --no-default-features && cd -
done

# Frontend
cd frontend && npm ci && npm run build
```

## Test

```bash
# Go
cd engine && go test -race ./...
cd engine && go vet ./...

# Rust
cd contracts/tests && cargo test --release   # 47+ semantic-boundary tests

# Reproducibility CLI (goldens must not drift)
cd engine && go run ./cmd/cp-repro

# End-to-end
./verify.sh                                   # must exit 8/8
```

## Style

- Go: `gofmt` + `golangci-lint run`.
- Rust: `cargo fmt` + `cargo clippy`.
- TS/JS: `prettier` + `eslint`.
- Python: `ruff format` + `ruff check`.

## Commits

Conventional commits: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`,
`ci:`, `chore:`. Sign every commit with a resolvable identity —
CI enforces `git log --format=%aE HEAD | grep '@'`.

## Pull Requests

`fork → branch → test → PR against main`. Keep PRs focused: **one
concern per PR**. Multi-concern PRs will be asked to split.

## Code-Review Checklist

Reviewer confirms **every** item before approving.

### Correctness

- [ ] Unit tests pass locally on the branch (`go test`, `cargo test`).
- [ ] `verify.sh` still exits 8/8.
- [ ] `cp-repro` produces no drift for any pinned scenario.
- [ ] For contract changes: WASM builds under nightly and the size is
      within ±10% of the previous artefact (check `Report WASM sizes`
      step in CI).

### Trust & labelling

- [ ] Every user-facing string that mentions a crypto or on-chain
      claim carries the correct **TrustBadge** (REAL / ON-CHAIN /
      SIMULATION).
- [ ] No new secret appears in the diff (grep for `ghp_`, `sk_`,
      `-----BEGIN`, `casper_`, `AKIA`; run `gitleaks detect --no-git`).
- [ ] `KNOWN_LIMITATIONS.md` is updated when a mock is added, changed,
      or lifted to a real primitive.

### API / SDK

- [ ] A route that mutates state is behind `authMiddleware` **and**
      `scopeGate` (or explicitly documented as public write).
- [ ] Any new mutating endpoint supports `X-Idempotency-Key` (or
      documents why it doesn't need to).
- [ ] Backwards-incompatible API changes bump `X-CP-API-Version` and
      appear in `docs/API_CHANGELOG.md`.

### Data / on-chain

- [ ] `deploy-out/onchain.json` is regenerated when a contract hash
      changes; **no** file elsewhere hard-codes a contract hash.
- [ ] Chain root reproducibility: for a decision change, add a
      `testdata/repro/*.json` scenario with pinned goldens.

### Docs

- [ ] `README.md` / `docs/JUDGE_GUIDE.md` still match the code paths.
- [ ] For architectural changes, `docs/ARCHITECTURE_EXTENSIONS.md` is
      updated (component map + trust boundary table).
- [ ] Every new externally-facing endpoint is listed in
      `docs/openapi.yaml`.

### Security

- [ ] Owner-only / installer-only paths call `assert_installer` or
      equivalent.
- [ ] No `unwrap()` without a `revert(ApiError::…)` behind it in Rust
      hot paths.
- [ ] New goroutines / async workers respect `context.Context`
      cancellation.

## Reviewers

At least one of the maintainers must approve before merge.
Maintainer list lives in `MAINTAINERS.md` (once populated); until then
the repo owner reviews.
