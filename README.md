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

## Roadmap

- [x] Phase 1 — API Server with Shinami backend
- [x] Phase 1 — Docker Compose (one command setup)
- [x] Phase 1 — PostgreSQL transaction logging
- [x] Phase 1 — API Key authentication
- [ ] Phase 2 — `sui-gas-pool` integration (remove Shinami dependency)
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
