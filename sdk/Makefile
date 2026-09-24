# CasperProver SDK smoke targets.
#
# `make smoke` runs the full offline unit-test suite across Go / Python /
# TypeScript. `make smoke-live` additionally shells out to the quickstart
# examples against a real CP_API_URL (opt-in; not run in CI).

.PHONY: smoke smoke-go smoke-python smoke-typescript smoke-live

smoke: smoke-go smoke-python smoke-typescript

smoke-go:
	@echo "-- go sdk"
	@go test -race -count=1 ./...

smoke-python:
	@echo "-- python sdk"
	@cd python && python3 -m unittest casperprover.tests.test_client -v

smoke-typescript:
	@echo "-- typescript sdk"
	@cd typescript && node --test --experimental-strip-types src/client.test.ts

smoke-live:
	@echo "-- live quickstart examples (requires CP_API_URL)"
	@go run ./examples/go/quickstart
	@cd python && CP_API_URL=$${CP_API_URL:-http://localhost:9090} \
	    python3 ../examples/python/quickstart.py
	@node --experimental-strip-types examples/typescript/quickstart.ts
