#!/usr/bin/env bash
# mainnet-check.sh — Pre-flight checklist before going live on Mainnet.
#
# Usage: ./scripts/mainnet-check.sh
# Requires: curl, jq, docker compose
#
# Set API_KEY in your environment or .env before running.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

# Load .env if present (so API_KEY is available)
if [[ -f "$ROOT_DIR/.env" ]]; then
    # shellcheck source=/dev/null
    source <(grep -v '^#' "$ROOT_DIR/.env" | grep -v '^$')
fi

BASE_URL="${DASHI_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-}"

PASS=0
FAIL=0

ok()   { echo "  ✅ $*"; ((PASS++)) || true; }
fail() { echo "  ❌ $*"; ((FAIL++)) || true; }
info() { echo "  ℹ  $*"; }

echo ""
echo "Dashi Mainnet Pre-flight Check"
echo "================================"
echo "  URL: $BASE_URL"
echo ""

# ── Check 1: Health endpoint ──────────────────────────────────────────────────
echo "[ 1 ] API health"
HEALTH=$(curl -sf "$BASE_URL/health" 2>/dev/null || echo "")
if echo "$HEALTH" | grep -q '"status":"ok"'; then
    NETWORK=$(echo "$HEALTH" | grep -o '"network":"[^"]*"' | cut -d'"' -f4)
    ok "Health OK — network: $NETWORK"
    if [[ "$NETWORK" == "testnet" ]]; then
        fail "Network is testnet — set SUI_NETWORK=mainnet in .env before going live"
    fi
else
    fail "Health check failed — is the API running? (docker compose up -d)"
fi

# ── Check 2: API key is set ───────────────────────────────────────────────────
echo ""
echo "[ 2 ] API key"
if [[ -z "$API_KEY" ]]; then
    fail "API_KEY is not set in environment or .env"
else
    ok "API_KEY is set (${#API_KEY} chars)"
    if [[ ${#API_KEY} -lt 32 ]]; then
        fail "API_KEY is shorter than 32 chars — generate with: openssl rand -hex 32"
    fi
fi

# ── Check 3: Gas fund balance ─────────────────────────────────────────────────
echo ""
echo "[ 3 ] Gas fund balance"
if [[ -n "$API_KEY" ]]; then
    BALANCE_RESP=$(curl -sf "$BASE_URL/v1/balance" \
        -H "X-API-Key: $API_KEY" 2>/dev/null || echo "")
    BALANCE=$(echo "$BALANCE_RESP" | grep -o '"balance":"[^"]*"' | cut -d'"' -f4 || echo "")
    if [[ -z "$BALANCE" ]]; then
        fail "Could not retrieve balance (API may be down or key invalid)"
    elif [[ "$BALANCE" == "0.00" ]] || [[ "$BALANCE" == "0" ]]; then
        fail "Gas fund is empty — fund the sponsor wallet with SUI first"
        info "Sponsor wallet: check config/gas-pool.yaml for the address"
    else
        ok "Gas fund balance: $BALANCE SUI"
        info "Recommended minimum for production: 10 SUI"
    fi
else
    info "Skipping balance check (no API_KEY)"
fi

# ── Check 4: Postgres is healthy ──────────────────────────────────────────────
echo ""
echo "[ 4 ] Postgres"
if docker compose -f "$ROOT_DIR/docker-compose.yml" exec -T postgres \
    pg_isready -U gasstation >/dev/null 2>&1; then
    ROW_COUNT=$(docker compose -f "$ROOT_DIR/docker-compose.yml" exec -T postgres \
        psql -U gasstation -d gasstation -t -c "SELECT COUNT(*) FROM sponsorships;" \
        2>/dev/null | tr -d ' \n' || echo "?")
    ok "Postgres healthy — $ROW_COUNT sponsorship(s) logged"
else
    fail "Postgres is not healthy — check docker compose ps"
fi

# ── Check 5: Redis is healthy ─────────────────────────────────────────────────
echo ""
echo "[ 5 ] Redis"
if docker compose -f "$ROOT_DIR/docker-compose.yml" exec -T redis \
    redis-cli ping 2>/dev/null | grep -q PONG; then
    ok "Redis healthy"
else
    fail "Redis is not responding — check docker compose ps"
fi

# ── Check 6: gaspool is running ───────────────────────────────────────────────
echo ""
echo "[ 6 ] sui-gas-pool"
GASPOOL_URL="${GASPOOL_URL:-http://127.0.0.1:9527}"
# gas-pool exposes /health or can be checked by the api container reachability
if curl -sf --max-time 3 "$GASPOOL_URL/v1/reserve_gas" \
    -X POST -H "Content-Type: application/json" \
    -d '{"gas_budget":1,"reserve_duration_secs":1}' >/dev/null 2>&1; then
    ok "sui-gas-pool is reachable at $GASPOOL_URL"
else
    info "sui-gas-pool at $GASPOOL_URL did not respond to a test probe"
    info "This is expected if auth is required — check docker compose logs gaspool"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "================================"
if [[ $FAIL -eq 0 ]]; then
    echo "✅ All $PASS checks passed — Dashi is ready for Mainnet"
    echo ""
    echo "Start in production mode:"
    echo "  docker compose -f docker-compose.yml -f docker-compose.mainnet.yml up -d"
else
    echo "❌ $FAIL check(s) failed, $PASS passed"
    echo ""
    echo "Fix the issues above before going live."
    exit 1
fi
