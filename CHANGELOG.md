# Changelog

All notable changes to Dashi are documented here.

---

## [0.3.0] — Phase 2 (current)

### Added
- **Test suite** — 81% code coverage across all packages
  - `handlers_test.go` — all HTTP endpoints (valid + error paths)
  - `middleware_test.go` — API key auth (X-API-Key and Bearer)
  - `sui_test.go` — address validation regex + RPC client tests
  - `gaspool_test.go` — sponsorship pipeline and all error paths
  - `db_test.go` — database operations (skips gracefully if Postgres is down)
  - `main_test.go` — config loading and env-var handling
  - `integration_test.go` — end-to-end tests (build tag: `integration`)
  - `testutils/helpers.go` — shared mock servers for gas-pool and Sui RPC
  - `Makefile` — `make test`, `make test-coverage`, `make test-integration`
- **Mainnet preparation**
  - `docker-compose.mainnet.yml` — production overrides (GIN_MODE=release, Redis AOF, Postgres tuning)
  - `scripts/mainnet-check.sh` — pre-flight checklist (health, balance, DB, Redis, gas-pool)
  - `.env.example` — mainnet RPC config with comments for dedicated providers

### Changed
- `newRouter()` extracted from `main()` — router setup is now testable and DRY
- `NewSuiClient` timeout raised from 10 s to 15 s for mainnet reliability
- `sui.go` timeout comment updated to explain mainnet rationale

---

## [0.2.0] — Phase 2

### Added
- **sui-gas-pool backend** (`gaspool.go`) — replaces Shinami with Mysten Labs'
  open-source [sui-gas-pool](https://github.com/MystenLabs/sui-gas-pool)
- `Dockerfile.gaspool` — multi-stage Rust build of `sui-gas-station` from source
- `config/gas-pool.yaml.example` — YAML config template (keypair placeholder)
- `scripts/setup-sponsor-wallet.sh` — Ed25519 keypair generation (Python 3 + cryptography)
- `docker-compose.yml` — `gaspool` service, all services on `network_mode: host`

### Removed
- `shinami.go` — replaced by `gaspool.go` (same `ShinamiClient` type name for zero handler changes)

### Changed
- `main.go` — reads `GASPOOL_URL` + `GASPOOL_AUTH_TOKEN` instead of `SHINAMI_GAS_STATION_KEY`
- `.gitignore` — `config/gas-pool.yaml` excluded (contains private key after setup)
- `README.md` — Phase 2 setup section added

---

## [0.1.0] — Phase 1

### Added
- Go + Gin API server with graceful shutdown
- `POST /v1/sponsor` — sponsors Sui transactions via Shinami Gas Station
- `GET /v1/sponsor/:digest` — transaction status from Sui RPC
- `GET /v1/balance` — gas fund balance
- `GET /health` — service health (no auth required)
- API key authentication via `X-API-Key` header or `Authorization: Bearer`
- PostgreSQL 16 transaction logging (`sponsorships` + `customers` tables)
- Redis 7 (prepared for rate limiting)
- Nginx reverse proxy
- Docker Compose one-command setup
- `test.mjs` — Node.js end-to-end test using `@mysten/sui` v2
