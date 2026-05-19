#!/usr/bin/env bash
# setup-sponsor-wallet.sh — generate an Ed25519 sponsor keypair for sui-gas-pool
#
# Writes the keypair to config/gas-pool.yaml and prints the Sui address
# so you can fund the wallet with SUI on Mainnet before starting the service.
#
# Usage: ./scripts/setup-sponsor-wallet.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
CONFIG_DIR="$ROOT_DIR/config"
CONFIG_FILE="$CONFIG_DIR/gas-pool.yaml"

mkdir -p "$CONFIG_DIR"

# ── copy example config if gas-pool.yaml doesn't exist yet ────────────────

EXAMPLE_FILE="$CONFIG_DIR/gas-pool.yaml.example"
if [[ ! -f "$CONFIG_FILE" ]]; then
    if [[ -f "$EXAMPLE_FILE" ]]; then
        cp "$EXAMPLE_FILE" "$CONFIG_FILE"
        echo "Copied $EXAMPLE_FILE → $CONFIG_FILE"
    else
        echo "ERROR: $EXAMPLE_FILE not found. Cannot create $CONFIG_FILE."
        exit 1
    fi
fi

# ── key generation (Python3 + cryptography package) ───────────────────────

KEYGEN_RESULT=$(python3 - <<'PYEOF' 2>/dev/null
import base64, hashlib, sys

try:
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    from cryptography.hazmat.primitives.serialization import (
        Encoding, PrivateFormat, PublicFormat, NoEncryption
    )
except ImportError:
    sys.exit(1)

key = Ed25519PrivateKey.generate()

# Raw bytes — compatible with cryptography >= 2.6 (private_bytes_raw() needs >= 40)
priv = key.private_bytes(Encoding.Raw, PrivateFormat.Raw, NoEncryption())   # 32 bytes
pub  = key.public_key().public_bytes(Encoding.Raw, PublicFormat.Raw)        # 32 bytes

# Sui keypair format: flag(0x00=Ed25519) || private_key
keypair_b64 = base64.b64encode(bytes([0x00]) + priv).decode()

# Sui address: BLAKE2b-256(flag || public_key) → 32 bytes → 64 hex chars
digest  = hashlib.blake2b(bytes([0x00]) + pub, digest_size=32).digest()
address = "0x" + digest.hex()

print(keypair_b64 + "|" + address)
PYEOF
) || true

if [[ -z "$KEYGEN_RESULT" ]]; then
    echo ""
    echo "ERROR: Could not generate keypair automatically."
    echo ""
    echo "Option A — install the Python cryptography library:"
    echo "  pip3 install cryptography"
    echo "  then re-run this script."
    echo ""
    echo "Option B — use the Sui CLI (if installed):"
    echo "  sui keytool generate ed25519"
    echo "  Then copy the 'privateKeyBase64' output into config/gas-pool.yaml"
    echo "  under signer-config.Local.keypair"
    echo ""
    exit 1
fi

KEYPAIR_B64="${KEYGEN_RESULT%%|*}"
SUI_ADDRESS="${KEYGEN_RESULT##*|}"

# ── check if keypair already exists ───────────────────────────────────────

if grep -q "REPLACE_WITH_BASE64_KEYPAIR" "$CONFIG_FILE" 2>/dev/null; then
    # Replace the placeholder in gas-pool.yaml
    sed -i "s|REPLACE_WITH_BASE64_KEYPAIR|${KEYPAIR_B64}|g" "$CONFIG_FILE"
    echo "Keypair written to $CONFIG_FILE"
else
    echo "WARNING: gas-pool.yaml already contains a keypair. No changes made."
    echo "Delete config/gas-pool.yaml and re-run this script to generate a new one."
fi

# ── print sponsor address ─────────────────────────────────────────────────

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "  Sponsor wallet address:"
echo "  $SUI_ADDRESS"
echo "═══════════════════════════════════════════════════════════════"
echo ""
echo "Next steps:"
echo ""
echo "  1. Fund this address with at least 10 SUI on Mainnet."
echo ""
echo "  2. Set SPONSOR_ADDRESS in .env:"
echo "     SPONSOR_ADDRESS=$SUI_ADDRESS"
echo ""
echo "  3. Build and start:"
echo "     docker compose build gaspool   # takes 20-40 min first time"
echo "     docker compose up -d"
echo ""
echo "  4. Verify:"
echo "     curl http://localhost:8080/health"
echo "     ./scripts/mainnet-check.sh"
echo ""

