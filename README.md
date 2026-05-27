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
# Fill in all required values in .env
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
  "transactionKindBytes": "AQIDBA==",
  "sender": "0xabcd...1234"
}
```

**`sender`** — The user's Sui wallet address. You get this from the connected wallet:
```ts
const sender = wallet.address; // e.g. "0xabc...123"
```

**`transactionKindBytes`** — The transaction the user wants to execute, serialized to Base64.
Build it with the Sui TypeScript SDK and set `onlyTransactionKind: true` — this tells the SDK
to serialize only what the transaction *does*, without gas or sender fields (Dashi fills those in):

```ts
import { Transaction } from "@mysten/sui/transactions";
import { toBase64 } from "@mysten/sui/utils";

const tx = new Transaction();
tx.moveCall({
  target: "0xPACKAGE::MODULE::FUNCTION",
  arguments: [tx.pure.u64(42)],
});

const kindBytes = await tx.build({ onlyTransactionKind: true });
const transactionKindBytes = toBase64(kindBytes); // send this to Dashi
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

## Logging

Dashi emits structured JSON logs compatible with any log management platform:

- Grafana Loki
- Graylog / GELF
- Datadog
- AWS CloudWatch
- Azure Application Insights
- Elastic Stack

Set `LOG_LEVEL` in `.env` to control verbosity:

| Level   | What you see                                            |
|---------|---------------------------------------------------------|
| `error` | Fatal errors only                                       |
| `warn`  | Auth failures, rejected requests (default for alerts)   |
| `info`  | Successful sponsorships, server start/stop **(default)**|
| `debug` | Full gas-pool request/response bodies                   |

**Example — info level (default):**
```json
{"time":"...","level":"INFO","msg":"dashi starting","version":"1.0.0","port":"8080","network":"mainnet"}
{"time":"...","level":"INFO","msg":"sponsorship reserved","sponsorship_id":2,"sender":"0xabcd...","duration_ms":463}
{"time":"...","level":"WARN","msg":"auth rejected: invalid api key","path":"/v1/sponsor"}
{"time":"...","level":"WARN","msg":"api error","status":400,"error":"Invalid Sui address format","path":"/v1/sponsor","method":"POST"}
```

**Example — debug level** (`LOG_LEVEL=debug` in `.env`):
```json
{"time":"...","level":"DEBUG","msg":"gas-pool request","path":"/v1/reserve_gas","body":"{\"gas_budget\":5000000,...}"}
{"time":"...","level":"DEBUG","msg":"gas-pool response","path":"/v1/reserve_gas","status":200,"body":"{\"result\":{...}}"}
```

> Error responses sent to clients always contain only `error` and `hint`.
> The internal `detail` (stack trace / upstream message) stays in the log — never exposed to the caller.

---

## Rate Limiting

Dashi enforces two independent limits, both backed by Redis so they hold across multiple API replicas.

| Limit | Default | Applies to |
|---|---|---|
| Limit | Default | Env var | Applies to |
|---|---|---|---|
| Per API key — GET | 60 req/min | `RATE_LIMIT_PER_MINUTE` | `GET /v1/execute/:id`, `GET /v1/sponsor/:digest`, `GET /v1/balance` |
| Per API key — POST | 30 req/min | `RATE_LIMIT_POST_PER_MINUTE` | `POST /v1/sponsor`, `POST /v1/execute` |
| Global | 500 req/min | `RATE_LIMIT_GLOBAL_PER_MINUTE` | All requests combined |

Configure in `.env`:

```env
RATE_LIMIT_PER_MINUTE=60           # per-key limit for GET endpoints
RATE_LIMIT_POST_PER_MINUTE=30      # per-key limit for POST endpoints
RATE_LIMIT_GLOBAL_PER_MINUTE=500   # global cap across all callers
```

When a limit is exceeded the API returns HTTP **429**:

```json
{
  "error": "Rate limit exceeded",
  "hint": "Maximum 60 requests per minute per API key"
}
```

Every response also includes the standard rate limit headers:

```
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 42
X-RateLimit-Reset: 1748001120
```

> If Redis is unreachable on startup, Dashi logs a warning and continues without rate limiting (fail-open).

---

## Roadmap

- [x] API Server with sui-gas-pool backend
- [x] Docker Compose (one command setup)
- [x] PostgreSQL transaction logging
- [x] API Key authentication
- [x] Async execute with polling (`POST /v1/execute` → `GET /v1/execute/:id`)
- [x] Phase 2 — Rate limiting per customer (Redis-backed, per API key + global)
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
