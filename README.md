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
cp .env.example .env                      # fill in API_KEY, GASPOOL_AUTH_TOKEN, SPONSOR_ADDRESS
cp config/gas-pool.yaml.example config/gas-pool.yaml
./scripts/setup-sponsor-wallet.sh         # generates keypair, writes config/gas-pool.yaml
docker compose up -d
```

Your Gas Station is live in under 5 minutes.

```bash
# Health check
curl http://localhost:8080/health
# → {"status":"ok","network":"mainnet","version":"1.0.0"}

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
 │   ├── Logging (Postgres)
 │   └── Fee Info
 │
 │  internal
 ▼
sui-gas-pool (Mysten Labs)    ← coin management backend
 │
 ▼
Sui Blockchain (Mainnet)
```

- **Current:** `sui-gas-pool` manages gas coins — fully self-contained, no third-party dependency.
- **Roadmap:** On-chain fee collection via Move smart contract.

---

## Tech Stack

| Component       | Technology               |
|-----------------|--------------------------|
| API Server      | Go + Gin                 |
| Database        | PostgreSQL 16            |
| Cache / Queue   | Redis 7                  |
| Reverse Proxy   | Nginx                    |
| Deployment      | Docker Compose           |
| Blockchain      | Sui Mainnet              |

---

## Configuration

See `.env.example` for all available options. Key settings:

```env
SUI_NETWORK=mainnet
SUI_RPC_URL=https://fullnode.mainnet.sui.io:443

# Generate: openssl rand -hex 32
API_KEY=your_secure_random_key
GASPOOL_AUTH_TOKEN=your_secure_random_token

SPONSOR_ADDRESS=0xYOUR_SPONSOR_WALLET
```

---

## Setup

### Step 1: Generate sponsor wallet

```bash
./scripts/setup-sponsor-wallet.sh
```

Generates an Ed25519 keypair, writes it into `config/gas-pool.yaml`, and prints the sponsor address. **`config/gas-pool.yaml` is gitignored — never commit it.**

### Step 2: Fund the sponsor wallet

Send at least **10 SUI** to your sponsor address on Mainnet:

```bash
curl -H "X-API-Key: $API_KEY" http://localhost:8080/v1/balance
```

### Step 3: Configure and start

```bash
cp .env.example .env   # fill in API_KEY, GASPOOL_AUTH_TOKEN, SPONSOR_ADDRESS

# First build takes 20-40 min (compiles sui-gas-station from source)
docker compose build gaspool

docker compose up -d
```

### Step 4: Verify

```bash
curl http://localhost:8080/health
# → {"status":"ok","network":"mainnet","version":"1.0.0"}

./scripts/mainnet-check.sh
# Checks: health, network=mainnet, balance > 0, Postgres, Redis, gas-pool
```

---

## Testing

### Unit Tests

```bash
# No network, no external dependencies
make test

# Coverage report
make test-coverage
```

Coverage target: **≥ 80%**

### Manual End-to-End Test (Mainnet)

Add `SENDER_PRIVKEY=suiprivkey1...` to your `.env`, then:

```bash
node test.mjs
```

For a pipeline smoke-check without a real sender wallet:

```bash
./scripts/manual-mainnet-test.sh
```

This checks health, shows the sponsor balance, asks for confirmation, then verifies the sponsor pipeline.

**Never run mainnet tests in CI/CD.**

---

## Roadmap

- [x] API Server with sui-gas-pool backend
- [x] Docker Compose (one command setup)
- [x] PostgreSQL transaction logging
- [x] API Key authentication
- [x] Async execute with polling (`POST /v1/execute` → `GET /v1/execute/:id`)
- [ ] Multi-tenant API keys from database
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
