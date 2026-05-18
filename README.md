# Dashi — Sui Gas Station

**The invisible base layer for Sui dApps.**

Dashi is a self-hosted, non-custodial Gas Station for the Sui blockchain.
It enables dApp developers to sponsor Gas Fees for their users —
so users can interact with your dApp without ever buying SUI.

---

## The Problem

Every new user who wants to use a Sui dApp today has to:

1. Sign up on an exchange
2. Complete KYC (upload ID, wait days)
3. Buy SUI
4. Transfer SUI to a wallet
5. Connect wallet to the dApp

**Result: 90% of users drop off before they ever touch your app.**

---

## The Solution

With Dashi, your users just click and go. No exchange. No KYC. No SUI required.

Dashi sits between your dApp and the Sui blockchain and sponsors the tiny
Gas Fee (~0.003 SUI ≈ fraction of a cent) on behalf of your user.
The user never knows it happened.

```
User clicks "Swap"
    ↓
Your dApp sends transaction to Dashi
    ↓
Dashi sponsors the Gas Fee from your fund
    ↓
Transaction goes through
    ↓
User sees: "Done." — never bought SUI, never touched a wallet
```

---

## Why Dashi, not Shinami or Enoki?

|                    | Shinami                  | Enoki                     | Dashi                        |
|--------------------|--------------------------|---------------------------|------------------------------|
| **Custodial**      | ✅ They hold your SUI    | ✅ They hold your SUI     | ❌ You hold your SUI         |
| **Self-hosted**    | ❌                       | ❌                        | ✅ Your server, your rules   |
| **Region**         | 🇺🇸 US East only        | ❌ unclear                | 🌍 Anywhere                  |
| **Open Source**    | ❌                       | ❌                        | ✅                           |
| **Vendor Lock-in** | Medium                   | High (Mysten Labs)        | ❌ None                      |
| **Fee transparency** | ❌ Black box           | ❌ Black box              | ✅ On-chain, always visible  |

Dashi never holds your money. Your SUI stays in your own fund.
If Dashi goes down, your funds are safe. No custodial risk. No regulatory gray area.

---

## Quickstart

```bash
git clone https://codeberg.org/adrian_north/dashi
cd dashi
cp .env.example .env
# Fill in your Shinami API Key and a secure API Key
docker compose up -d
```

Your Gas Station is live in under 5 minutes.

```bash
# Health check
curl http://localhost:8080/health
# → { "status": "ok", "network": "testnet" }

# Sponsor a transaction
curl -X POST http://localhost:8080/v1/sponsor \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "transactionKindBytes": "BASE64_TX_BYTES",
    "sender": "0xYOUR_USER_WALLET"
  }'
```

---

## API Reference

### `POST /v1/sponsor`

Sponsor a transaction for a user.

**Request:**
```json
{
  "transactionKindBytes": "string (Base64)",
  "sender": "string (0x + 64 hex chars)"
}
```

**Response:**
```json
{
  "sponsoredTransaction": "string (Base64)",
  "sponsorshipId": "sp_1234567890",
  "feeInfo": {
    "networkFee": "0.003 SUI",
    "serviceFee": "0.001 SUI",
    "totalFee":   "0.004 SUI"
  }
}
```

### `GET /v1/sponsor/:digest`

Get the status of a sponsored transaction.

```json
{
  "digest": "ABC123...",
  "status": "success"
}
```

### `GET /v1/balance`

Get the current SUI balance of your gas fund.

```json
{
  "balance": "47.32",
  "currency": "SUI",
  "network": "mainnet"
}
```

### `GET /health`

No API key required. Returns service status.

---

## Architecture

```
dApp
 │
 │  POST /v1/sponsor
 ▼
Dashi API (Go + Gin)          ← your Docker Compose
 │   ├── Auth (API Key)
 │   ├── Rate Limiting (Redis)
 │   ├── Logging (Postgres)
 │   └── Fee Info
 │
 │  internal
 ▼
Shinami Gas Station API       ← Phase 1 backend
 │
 ▼
Sui Blockchain
```

- **Phase 1 (current):** Shinami handles coin management internally.
- **Phase 2 (roadmap):** `sui-gas-pool` (Mysten Labs, Apache 2.0) replaces Shinami — fully self-contained, no third-party dependency.
- **Phase 3 (roadmap):** On-chain fee collection via Move smart contract.

The API surface never changes between phases. Your integration stays the same.

---

## Tech Stack

| Component       | Technology               |
|-----------------|--------------------------|
| API Server      | Go + Gin                 |
| Database        | PostgreSQL 16            |
| Cache / Queue   | Redis 7                  |
| Reverse Proxy   | Nginx                    |
| Deployment      | Docker Compose           |
| Blockchain      | Sui (Testnet + Mainnet)  |

---

## Configuration

```env
# Network
SUI_NETWORK=testnet
SUI_RPC_URL=https://fullnode.testnet.sui.io:443

# Shinami (get your free key at app.shinami.com)
SHINAMI_GAS_STATION_KEY=us1_sui_testnet_YOUR_KEY

# Your API Key (generate: openssl rand -hex 32)
API_KEY=your_secure_random_key

# Server
PORT=8080
```

---

## Phase 2 Setup (sui-gas-pool)

Phase 2 replaces Shinami with [sui-gas-pool](https://github.com/MystenLabs/sui-gas-pool)
by Mysten Labs — fully self-contained, no third-party dependency.

### Step 1: Generate sponsor wallet

```bash
./scripts/setup-sponsor-wallet.sh
```

The script generates an Ed25519 keypair, writes it into `config/gas-pool.yaml`,
and prints the sponsor address. **Never commit `config/gas-pool.yaml` after this step.**

### Step 2: Fund the sponsor wallet

```bash
curl -X POST https://faucet.testnet.sui.io/v1/gas \
  -H "Content-Type: application/json" \
  -d '{"FixedAmountRequest":{"recipient":"0xYOUR_SPONSOR_ADDRESS"}}'
```

The gas pool needs at least **1 SUI** to start splitting coins and sponsoring transactions.

### Step 3: Set the auth token

In `.env`:
```env
GASPOOL_AUTH_TOKEN=your_secure_random_token   # openssl rand -hex 32
```

The same token is used by `docker-compose.yml` as `GAS_STATION_AUTH` for the gas pool container.

### Step 4: Build and start

```bash
# First build takes 20-40 min (compiles sui-gas-station from source)
docker compose build gaspool

docker compose up -d
```

### Step 5: Test

```bash
curl http://localhost:8080/health
# → {"status":"ok","network":"testnet","version":"1.0.0"}

node test.mjs
# → {"sponsoredTransaction":"...","sponsorshipId":"<txDigest>","feeInfo":{...}}
```

---

## Going to Mainnet

### Step 1 — Fund your gas pool

Send at least **10 SUI** to your sponsor wallet address (generated in Phase 2 setup).
Check balance after funding:

```bash
curl -H "X-API-Key: $API_KEY" http://localhost:8080/v1/balance
```

### Step 2 — Switch to Mainnet

Edit `.env`:

```env
SUI_NETWORK=mainnet
SUI_RPC_URL=https://fullnode.mainnet.sui.io:443
```

For production traffic, consider a dedicated RPC provider:

```env
# Dwellir (EU, low latency)
SUI_RPC_URL=https://sui-mainnet.dwellir.com
# QuickNode
SUI_RPC_URL=https://your-endpoint.quiknode.pro/your-key/
```

### Step 3 — Run pre-flight check

```bash
./scripts/mainnet-check.sh
# Checks: API health, network=mainnet, balance > 0, Postgres, Redis, gas-pool
```

### Step 4 — Start in production mode

```bash
docker compose -f docker-compose.yml -f docker-compose.mainnet.yml up -d
```

The production override enables:
- `GIN_MODE=release` (no debug output)
- Redis AOF persistence (no data loss on restart)
- Postgres tuning (`shared_buffers=256MB`, slow query logging)
- `restart: always` on all services

---

## Testing

```bash
# All unit tests (no external dependencies)
make test

# With coverage report (opens coverage.html)
make test-coverage

# Integration tests against a running local instance
make test-integration
```

Coverage target: **≥ 80%**

---

## Roadmap

- [x] Phase 1 — API Server with Shinami backend
- [x] Phase 1 — Docker Compose (one command setup)
- [x] Phase 1 — PostgreSQL transaction logging
- [x] Phase 1 — API Key authentication
- [x] Phase 2 — `sui-gas-pool` integration (remove Shinami dependency)
- [ ] Phase 2 — Multi-tenant API keys from database
- [ ] Phase 2 — Rate limiting per customer
- [ ] Phase 3 — On-chain fee collection to operator wallet
- [ ] Phase 3 — Web dashboard for monitoring
- [ ] Phase 3 — Fraud detection

---

## Contributing

Pull requests are welcome. For major changes, please open an issue first.

---

## Credits

Dashi uses [sui-gas-pool](https://github.com/MystenLabs/sui-gas-pool)
by Mysten Labs as its internal coin management backend (Phase 2+).
Licensed under Apache 2.0.

---

## License

MIT — do whatever you want, just don't blame us if something breaks.

---

*Dashi (出汁) — the invisible base that makes everything taste better.*
