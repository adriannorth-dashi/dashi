import { SuiJsonRpcClient, getJsonRpcFullnodeUrl } from '@mysten/sui/jsonRpc';
import { Transaction } from '@mysten/sui/transactions';

const API_KEY   = '64c51e0378cba47087f150f470610fe3fcab3d3dbd020f9b35f34de9879b4ab5';
const SENDER    = '0x5757176f7fd65aa19893ec3dd368d88e25e032956af29843bdcbb03ca60f86f6';
const API_URL   = 'http://localhost:8080/v1/sponsor';
const ZERO_ADDR = '0x0000000000000000000000000000000000000000000000000000000000000000';

// ── 1. Connect to Sui Testnet ──────────────────────────────────────────────
const client = new SuiJsonRpcClient({ url: getJsonRpcFullnodeUrl('testnet') });
console.log('Connected to Sui Testnet');

// ── 2. Build transaction: split 0 SUI from gas, transfer to zero address ───
const tx = new Transaction();
const [zeroCoin] = tx.splitCoins(tx.gas, [0]);
tx.transferObjects([zeroCoin], ZERO_ADDR);

// ── 3. Serialize as TransactionKind bytes (no gas/sender info) ─────────────
const kindBytes = await tx.build({ client, onlyTransactionKind: true });
const base64    = Buffer.from(kindBytes).toString('base64');
console.log(`TransactionKind bytes: ${kindBytes.length} bytes → Base64 length: ${base64.length}`);

// ── 4. Send to Dashi ───────────────────────────────────────────────────────
console.log(`\nPOST ${API_URL}`);
const res = await fetch(API_URL, {
  method:  'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-API-Key':    API_KEY,
  },
  body: JSON.stringify({
    transactionKindBytes: base64,
    sender:               SENDER,
  }),
});

// ── 5. Print full response ─────────────────────────────────────────────────
const body = await res.json();
console.log(`HTTP ${res.status} ${res.statusText}\n`);
console.log(JSON.stringify(body, null, 2));
