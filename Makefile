.PHONY: test test-unit test-integration test-coverage test-mainnet build clean

# ── Test targets ──────────────────────────────────────────────────────────────

## Run all unit tests (default)
test: test-unit

## Unit tests only — no network, no Postgres required (DB tests skipped automatically)
test-unit:
	go test -mod=vendor -count=1 ./...

## Integration tests — requires a running Dashi instance (docker compose up -d)
## Set API_KEY and optionally DASHI_URL before running.
test-integration:
	go test -mod=vendor -count=1 -tags integration -run TestIntegration ./...

## Unit tests with HTML coverage report (opens coverage.html)
test-coverage:
	go test -mod=vendor -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | tail -1
	@echo "Coverage report written to coverage.html"

## Integration tests against mainnet — set DASHI_URL to your mainnet instance
test-mainnet:
	DASHI_URL=$${DASHI_MAINNET_URL:-http://localhost:8080} \
	go test -mod=vendor -count=1 -tags integration -run TestIntegration -v ./...

# ── Build ─────────────────────────────────────────────────────────────────────

build:
	go build -mod=vendor -ldflags="-s -w" -o dashi .

clean:
	rm -f dashi coverage.out coverage.html
