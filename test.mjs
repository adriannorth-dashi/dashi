import { SuiJsonRpcClient, getJsonRpcFullnodeUrl } from '@mysten/sui/jsonRpc';
import { Transaction } from '@mysten/sui/transactions';

const API_KEY    = '64c51e0378cba47087f150f470610fe3fcab3d3dbd020f9b35f34de9879b4ab5';
const SENDER     = '0x5757176f7fd65aa19893ec3dd368d88e25e032956af29843bdcbb03ca60f86f6';
const API_URL    = 'http://localhost:8080/v1/sponsor';
const ZERO_ADDR  = '0x0000000000000000000000000000000000000000000000000000000000000000';
const RPC_URL    = getJsonRpcFullnodeUrl('testnet');
const FAUCET_URL = 'https://faucet.testnet.sui.io/v1/gas';

// ── helpers ────────────────────────────────────────────────────────────────

async function rpc(method, params = []) {
  const res = await fetch(RPC_URL, {
    method:  'POST',
    headers: { 'Content-Type': 'application/json' },
    body:    JSON.stringify({ jsonrpc: '2.0', id: 1, method, params }),
  });
  const json = await res.json();
  if (json.error) throw new Error(`RPC ${method}: ${json.error.message}`);
  return json.result;
}

async function getCoins(owner) {
  const result = await rpc('suix_getCoins', [owner, '0x2::sui::SUI', null, 5]);
  return result.data; // array of { coinObjectId, balance, ... }
}

async function requestFaucet(address) {
  console.log('  Requesting testnet SUI from faucet...');
  const res = await fetch(FAUCET_URL, {
    method:  'POST',
    headers: { 'Content-Type': 'application/json' },
    body:    JSON.stringify({ FixedAmountRequest: { recipient: address } }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Faucet returned ${res.status}: ${text}`);
  }
  const json = await res.json();
  console.log(`  Faucet response: ${JSON.stringify(json)}`);
}

// ── 1. Connect ─────────────────────────────────────────────────────────────
const client = new SuiJsonRpcClient({ url: RPC_URL });
console.log('Connected to Sui Testnet');
console.log(`Sender: ${SENDER}\n`);

// ── 2. Make sure sender has SUI coins ──────────────────────────────────────
// Sponsored transactions: the sponsor (Shinami) provides gas.
// But the sender's coin objects must be used in the TX body — tx.gas is
// owned by Shinami and is NOT available as a transaction argument.
let coins = await getCoins(SENDER);
console.log(`Sender has ${coins.length} SUI coin(s).`);

if (coins.length === 0) {
  await requestFaucet(SENDER);
  console.log('  Waiting 5 s for coin to appear on-chain...');
  await new Promise(r => setTimeout(r, 5000));
  coins = await getCoins(SENDER);
  console.log(`  After faucet: ${coins.length} coin(s).`);
}

if (coins.length === 0) {
  console.error('Still no coins after faucet — try manually funding the sender on testnet.');
  process.exit(1);
}

const coinObjectId = coins[0].coinObjectId;
const coinBalance  = BigInt(coins[0].balance);
console.log(`Using coin ${coinObjectId} (balance: ${coinBalance} MIST)\n`);

// ── 3. Build transaction using sender's own coin (NOT tx.gas) ──────────────
// This is the correct pattern for sponsored transactions:
// - gas coin belongs to the sponsor → cannot appear in the TX body
// - sender's coin is used for the value transfer
const tx = new Transaction();
const [zeroCoin] = tx.splitCoins(coinObjectId, [0]);
tx.transferObjects([zeroCoin], ZERO_ADDR);

const kindBytes  = await tx.build({ client, onlyTransactionKind: true });
const kindBase64 = Buffer.from(kindBytes).toString('base64');

console.log(`TransactionKind bytes: ${kindBytes.length} bytes`);
console.log(`Base64 (first 60): ${kindBase64.slice(0, 60)}...\n`);

// ── 4. Send to Dashi ───────────────────────────────────────────────────────
const payload = { transactionKindBytes: kindBase64, sender: SENDER };
console.log(`POST ${API_URL}`);
console.log('Request body:', JSON.stringify(payload, null, 2).replace(kindBase64, kindBase64.slice(0, 40) + '...'));
console.log();

const res  = await fetch(API_URL, {
  method:  'POST',
  headers: { 'Content-Type': 'application/json', 'X-API-Key': API_KEY },
  body:    JSON.stringify(payload),
});

// ── 5. Print full response ─────────────────────────────────────────────────
const body = await res.json();
console.log(`HTTP ${res.status} ${res.statusText}\n`);
console.log(JSON.stringify(body, null, 2));
