.PHONY: test test-unit test-integration test-coverage test-mainnet build clean

# ── Test targets ──────────────────────────────────────────────────────────────

## Unit tests only — no network, no Postgres required (default)
test: test-unit

test-unit:
	go test -mod=vendor -count=1 ./...

## Integration tests require a separate Dashi instance on TESTNET.
## The Docker stack runs Mainnet-only — running this against it will fail immediately
## at the requireTestnet() guard. To use: stand up a local testnet Dashi separately.
test-integration:
	@echo ""
	@echo "  Integration tests require a TESTNET Dashi instance."
	@echo "  The Docker stack runs Mainnet-only and cannot be used here."
	@echo ""
	@echo "  To run integration tests, start a separate testnet instance and set:"
	@echo "    DASHI_URL=http://localhost:<testnet-port> API_KEY=<key> make test-integration-run"
	@echo ""

## Unit tests with HTML coverage report
test-coverage:
	go test -mod=vendor -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | tail -1
	@echo "Coverage report written to coverage.html"

## Mainnet testing is MANUAL ONLY — this target prints instructions and exits.
## Run ./scripts/manual-mainnet-test.sh directly. Never run mainnet tests in CI/CD.
test-mainnet:
	@echo ""
	@echo "  Mainnet testing is MANUAL ONLY. This target does nothing."
	@echo ""
	@echo "  To test against mainnet, run the script directly:"
	@echo "    ./scripts/manual-mainnet-test.sh"
	@echo ""
	@echo "  Never run mainnet tests in CI/CD pipelines."
	@echo ""

# ── Build ─────────────────────────────────────────────────────────────────────

build:
	go build -mod=vendor -ldflags="-s -w" -o dashi .

clean:
	rm -f dashi coverage.out coverage.html
