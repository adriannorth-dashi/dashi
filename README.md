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

## Why Dashi?

Most Gas Station solutions are proprietary, custodial, and region-locked.
Dashi is different:

- **Self-hosted** — runs on your own infrastructure, your rules
- **Non-custodial** — your SUI never leaves your wallet
- **Open Source** — transparent, auditable, no black boxes
- **No vendor lock-in** — deploy anywhere, migrate anytime
- **Fee transparency** — every cost is visible, nothing hidden

---

## Quickstart

### Option A — Docker Hub (recommended)

No build step. Two files, images pulled from Docker Hub automatically.

```bash
curl -O https://codeberg.org/adrian_north/dashi/raw/branch/main/docker-compose.hub.yml
curl -O https://codeberg.org/adrian_north/dashi/raw/branch/main/.env.example
mv .env.example .env
# Fill in all values in .env (API_KEY, GASPOOL_AUTH_TOKEN, SPONSOR_ADDRESS, GASPOOL_KEYPAIR)
docker compose -f docker-compose.hub.yml up -d
```

API is available on port 80.

### Option B — Build from source

```bash
git clone https://codeberg.org/adrian_north/dashi
cd dashi
cp .env.example .env
cp config/gas-pool.yaml.example config/gas-pool.yaml
# Fill in all required values in .env and config/gas-pool.yaml
docker compose up -d
```

First build takes 20–40 min (compiles sui-gas-station from Rust source).

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

## Roadmap

- [x] API Server with sui-gas-pool backend
- [x] Docker Compose (one command setup)
- [x] PostgreSQL transaction logging
- [x] API Key authentication
- [x] Async execute with polling (`POST /v1/execute` → `GET /v1/execute/:id`)
- [ ] Phase 2 — Rate limiting per customer
- [ ] Phase 3 — On-chain fee collection to operator wallet
- [ ] Phase 3 — Web dashboard for monitoring
- [ ] Phase 3 — Fraud detection

---

## Credits

Dashi uses [sui-gas-pool](https://github.com/MystenLabs/sui-gas-pool)
by Mysten Labs as its internal coin management backend (Phase 2+).
Licensed under Apache 2.0.

---

## Disclaimer

Dashi is experimental software. Use at your own risk.

- This software is provided "as is" without warranty of any kind
- The authors are not responsible for any loss of funds
- Always test on a small amount before production use
- You are responsible for securing your sponsor wallet private key
- Never share your private key or API key with anyone

This software is not affiliated with Mysten Labs or the Sui Foundation.
sui-gas-pool is developed by Mysten Labs and licensed under Apache 2.0.

## License

Copyright (C) 2025 Dashi Project  
Licensed under [AGPL-3.0](LICENSE)

---

*Dashi (出汁) — the invisible base that makes everything taste better.*
