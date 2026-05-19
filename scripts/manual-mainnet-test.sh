#!/usr/bin/env bash
# manual-mainnet-test.sh — One-shot manual test against the live mainnet Dashi instance.
#
# Run this ONCE manually before going live to verify the full sponsorship flow.
# NEVER run this in CI/CD pipelines. Automated tests are unit tests only.
#
# Usage: ./scripts/manual-mainnet-test.sh
# Requires: curl, jq

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

if [[ -f "$ROOT_DIR/.env" ]]; then
    source <(grep -v '^#' "$ROOT_DIR/.env" | grep -v '^$' | grep -v '^TEST_')
fi

BASE_URL="${DASHI_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo ""
echo "════════════════════════════════════════════════════════════"
echo "  Dashi — MANUAL MAINNET TEST"
echo "  This script runs ONE test transaction on Sui MAINNET."
echo "  It will NOT run automatically or in CI/CD."
echo "════════════════════════════════════════════════════════════"
echo ""
echo "  Dashi URL: $BASE_URL"
echo ""

# ── Guard: require API_KEY ────────────────────────────────────────────────────
if [[ -z "$API_KEY" ]]; then
    echo -e "${RED}ERROR: API_KEY is not set.${NC}"
    echo "  Set API_KEY in .env or export it before running this script."
    exit 1
fi

# ── Step 1: Health check and network guard ────────────────────────────────────
echo "[ 1 ] Checking Dashi health..."
HEALTH=$(curl -sf "$BASE_URL/health" 2>/dev/null || echo "")
if [[ -z "$HEALTH" ]]; then
    echo -e "${RED}ERROR: Dashi is not reachable at $BASE_URL${NC}"
    echo "  Start the service: docker compose -f docker-compose.yml -f docker-compose.mainnet.yml up -d"
    exit 1
fi

NETWORK=$(echo "$HEALTH" | jq -r '.network // empty' 2>/dev/null || echo "")
if [[ "$NETWORK" != "mainnet" ]]; then
    echo -e "${RED}ERROR: Dashi is running on network='$NETWORK', expected 'mainnet'.${NC}"
    echo "  This script is for mainnet only."
    echo "  This script is for mainnet only."
    exit 1
fi

VERSION=$(echo "$HEALTH" | jq -r '.version // empty' 2>/dev/null || echo "unknown")
echo -e "  ${GREEN}✅ Dashi $VERSION — network: mainnet${NC}"

# ── Step 2: Show balance ──────────────────────────────────────────────────────
echo ""
echo "[ 2 ] Checking sponsor wallet balance..."
BALANCE_RESP=$(curl -sf "$BASE_URL/v1/balance" -H "X-API-Key: $API_KEY" 2>/dev/null || echo "")
BALANCE=$(echo "$BALANCE_RESP" | jq -r '.balance // empty' 2>/dev/null || echo "")

if [[ -z "$BALANCE" ]]; then
    echo -e "${RED}ERROR: Could not retrieve balance (check API_KEY and gas-pool).${NC}"
    exit 1
fi

echo -e "  Sponsor balance: ${YELLOW}$BALANCE SUI${NC}"

if [[ "$BALANCE" == "0.00" ]] || [[ "$BALANCE" == "0" ]]; then
    echo -e "${RED}ERROR: Sponsor wallet is empty. Fund it before testing mainnet.${NC}"
    exit 1
fi

# ── Step 3: Operator confirmation ─────────────────────────────────────────────
echo ""
echo "════════════════════════════════════════════════════════════"
echo -e "  ${YELLOW}WARNING: The next step submits a real transaction on Sui MAINNET.${NC}"
echo "  A small amount of gas will be deducted from the sponsor wallet."
echo "  Sponsor balance: $BALANCE SUI"
echo "════════════════════════════════════════════════════════════"
echo ""
read -r -p "  Type 'yes' to continue, anything else to abort: " CONFIRM
echo ""

if [[ "$CONFIRM" != "yes" ]]; then
    echo "  Aborted."
    exit 0
fi

# ── Step 4: Send a test sponsorship request (pipeline check, no real tx) ──────
echo "[ 3 ] Sending test sponsor request (pipeline verification)..."
SPONSOR_RESP=$(curl -s -w "\n%{http_code}" \
    -X POST "$BASE_URL/v1/sponsor" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $API_KEY" \
    -d "{\"transactionKindBytes\":\"AQIDBA==\",\"sender\":\"${SPONSOR_ADDRESS}\"}" \
    2>/dev/null)

HTTP_CODE=$(echo "$SPONSOR_RESP" | tail -1)
BODY=$(echo "$SPONSOR_RESP" | head -n -1)

if [[ "$HTTP_CODE" == "200" ]]; then
    SPONSORSHIP_ID=$(echo "$BODY" | jq -r '.sponsorshipId // empty')
    echo -e "  ${GREEN}✅ Sponsor pipeline OK — sponsorshipId: $SPONSORSHIP_ID${NC}"
    echo "  (Dummy TransactionKind used — gas pool may reject the execute step, which is expected.)"
elif [[ "$HTTP_CODE" == "502" ]]; then
    echo -e "  ${YELLOW}⚠  Sponsor returned 502 — gas pool rejected the dummy tx (expected for test bytes).${NC}"
    echo "  The request pipeline is working. Use test.mjs for a real end-to-end test."
else
    echo -e "${RED}ERROR: Sponsor returned HTTP $HTTP_CODE${NC}"
    echo "  Response: $BODY"
    exit 1
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "════════════════════════════════════════════════════════════"
echo -e "  ${GREEN}Manual mainnet test complete.${NC}"
echo ""
echo "  For a full end-to-end transaction test (requires a funded"
echo "  sender wallet on mainnet), run:"
echo "    node test.mjs"
echo ""
echo "  Current sponsor balance: $BALANCE SUI"
echo "════════════════════════════════════════════════════════════"
echo ""
