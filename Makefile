.PHONY: build test lint clean contracts contracts-test sdk-test \
        judge-demo judge-repro judge-verify judge-all gate3-demo bench bench-baseline sync-onchain

build:
	cd engine && go build -o ../bin/casperprover ./cmd/casperprover

# One-command Gate 3 agentic vertical slice — approve + malicious + conflict
# + abstain paths, no network I/O. Prints a JSON receipt to stdout and a
# human summary to stderr. Exit code non-zero on any scenario failure.
gate3-demo: build
	./bin/casperprover gate3

test:
	cd engine && go test -v -race ./...

bench:
	./scripts/run_benchmarks.sh

bench-baseline:
	./scripts/run_benchmarks.sh --baseline

lint:
	cd engine && golangci-lint run

contracts:
	cd contracts/proof-registry && cargo build --release --target wasm32-unknown-unknown --no-default-features
	cd contracts/verifier-gate && cargo build --release --target wasm32-unknown-unknown --no-default-features
	cd contracts/defi-mock && cargo build --release --target wasm32-unknown-unknown --no-default-features
	cd contracts/stake-slashing && cargo build --release --target wasm32-unknown-unknown --no-default-features
	cd contracts/stake-slashing-session && cargo build --release --target wasm32-unknown-unknown --no-default-features

sdk-test:
	cd sdk && go build ./...
	cd sdk && go vet ./...
	cd sdk && go test -race -v ./...

contracts-test:
	cd contracts/tests && cargo test --release

clean:
	rm -rf bin/ target/

# ---------------------------------------------------------------------------
# Judge-facing one-command targets — used by docs/JUDGE_GUIDE.md.
# From a clean clone, `make judge-all` should exit 0 with:
#   * engine + SDK builds
#   * 5 deployed-contract WASMs (parity with 3 undeployed)
#   * 47+ contract semantic tests
#   * 8/8 verify.sh checks
#   * cp-repro drift-free
# ---------------------------------------------------------------------------

## Run the pinned decision-layer reproducibility scenarios.
## Exits non-zero if any golden hash drifts.
judge-repro:
	cd engine && go run ./cmd/cp-repro

## Re-derive chain roots against onchain.json — the 8-check gate.
judge-verify:
	./verify.sh

## Run the everything-must-be-green judge gate. Order matters —
## build first (fail fast on toolchain), then tests, then verify,
## then the golden reproducibility CLI.
judge-all: build sdk-test test contracts-test judge-verify judge-repro
	@echo
	@echo "  ✅  judge-all OK."
	@echo

## Alias so a first-time judge can just type `make judge-demo`.
judge-demo: judge-all

# Sync canonical on-chain manifest from deploy-out/onchain.json into consumer
# surfaces (frontend/public/onchain.json). Run after every redeploy.
sync-onchain:
	./scripts/sync-onchain-manifest.sh
