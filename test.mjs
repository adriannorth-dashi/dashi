import { SuiJsonRpcClient } from '@mysten/sui/jsonRpc';
import { Transaction } from '@mysten/sui/transactions';
import { Ed25519Keypair } from '@mysten/sui/keypairs/ed25519';
import { decodeSuiPrivateKey } from '@mysten/sui/cryptography';
import { readFileSync } from 'fs';

// ── Load .env ───────────────────────────────────────────────────────────────
// Shell env vars take precedence; .env fills in anything not already set.
try {
  readFileSync('.env', 'utf8').split('\n').forEach(line => {
    const m = line.match(/^([A-Z_][A-Z0-9_]*)=(.*)$/);
    if (m && !process.env[m[1]]) process.env[m[1]] = m[2].trim();
  });
} catch { /* .env is optional */ }

// ── Config ──────────────────────────────────────────────────────────────────
// Usage: node test.mjs   (all values read from .env)
const NETWORK      = process.env.SUI_NETWORK  || 'mainnet';
const RPC_URL      = process.env.SUI_RPC_URL  || 'https://fullnode.mainnet.sui.io:443';
const API_KEY      = process.env.API_KEY;
const DASHI_URL    = process.env.DASHI_URL    || 'http://localhost:8080';
const SENDER_PRIVKEY = process.env.SENDER_PRIVKEY;

if (!API_KEY) {
  console.error('Error: API_KEY is not set. Add it to .env or export it.');
  process.exit(1);
}
if (!SENDER_PRIVKEY) {
  console.error('Error: SENDER_PRIVKEY is not set. Add it to .env or export it.');
  process.exit(1);
}
const ZERO_ADDR = '0x0000000000000000000000000000000000000000000000000000000000000000';

// ── helpers ─────────────────────────────────────────────────────────────────

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
  return result.data;
}

// ── 1. Setup ────────────────────────────────────────────────────────────────
const { secretKey } = decodeSuiPrivateKey(SENDER_PRIVKEY);
const keypair = Ed25519Keypair.fromSecretKey(secretKey);
const SENDER = keypair.getPublicKey().toSuiAddress();
const suiClient = new SuiJsonRpcClient({ url: RPC_URL });

console.log('Dashi 2-step sponsored transaction test');
console.log(`Network: ${NETWORK}`);
console.log(`Sender:  ${SENDER}\n`);

// ── 2. Get sender coins ──────────────────────────────────────────────────────
const coins = await getCoins(SENDER);
console.log(`Sender has ${coins.length} SUI coin(s).`);
if (coins.length === 0) {
  console.error('No coins found — fund the sender wallet first.');
  process.exit(1);
}

// Pick the coin with the largest balance to avoid using gas-pool managed coins.
const coin = coins.reduce((a, b) => BigInt(a.balance) > BigInt(b.balance) ? a : b);
console.log(`Using coin ${coin.coinObjectId} (balance: ${coin.balance} MIST)\n`);

// ── 3. Build TransactionKind bytes ───────────────────────────────────────────
const tx = new Transaction();
const [zeroCoin] = tx.splitCoins(coin.coinObjectId, [0]);
tx.transferObjects([zeroCoin], ZERO_ADDR);

const kindBytes  = await tx.build({ client: suiClient, onlyTransactionKind: true });
const kindBase64 = Buffer.from(kindBytes).toString('base64');

console.log(`TransactionKind: ${kindBytes.length} bytes`);
console.log(`Base64 (first 60): ${kindBase64.slice(0, 60)}...\n`);

// ── Step 1: POST /v1/sponsor — reserve gas + get TransactionData ─────────────
console.log(`── Step 1: POST ${DASHI_URL}/v1/sponsor`);
const sponsorRes = await fetch(`${DASHI_URL}/v1/sponsor`, {
  method:  'POST',
  headers: { 'Content-Type': 'application/json', 'X-API-Key': API_KEY },
  body:    JSON.stringify({ transactionKindBytes: kindBase64, sender: SENDER }),
});
const sponsorBody = await sponsorRes.json();
console.log(`HTTP ${sponsorRes.status}\n`, JSON.stringify(sponsorBody, null, 2), '\n');

if (!sponsorRes.ok) {
  console.error('Step 1 failed — cannot continue.');
  process.exit(1);
}

const { sponsoredTransaction: txBytesB64, sponsorshipId } = sponsorBody;

// ── Step 2: Sign the TransactionData ────────────────────────────────────────
// In a real dApp the user signs this in their browser wallet (e.g. Suiet, Slush).
console.log(`── Step 2: Signing TransactionData (sponsorshipId: ${sponsorshipId})`);
const txBytes = Uint8Array.from(Buffer.from(txBytesB64, 'base64'));
const { signature } = await keypair.signTransaction(txBytes);
console.log(`Signature (first 40): ${signature.slice(0, 40)}...\n`);

// ── Step 3: POST /v1/execute — submit with user signature (async) ────────────
console.log(`── Step 3: POST ${DASHI_URL}/v1/execute`);
const executeRes = await fetch(`${DASHI_URL}/v1/execute`, {
  method:  'POST',
  headers: { 'Content-Type': 'application/json', 'X-API-Key': API_KEY },
  body:    JSON.stringify({ sponsorshipId, txBytes: txBytesB64, userSig: signature }),
});
const executeBody = await executeRes.json();
console.log(`HTTP ${executeRes.status}\n`, JSON.stringify(executeBody, null, 2), '\n');

if (!executeRes.ok) {
  console.error('Step 3 failed — cannot continue.');
  process.exit(1);
}

// ── Step 4: Poll GET /v1/execute/:id until completed or failed ───────────────
console.log(`── Step 4: Polling ${DASHI_URL}/v1/execute/${sponsorshipId}`);
let status = executeBody.status;
let digest = null;
let attempts = 0;
while (status === 'submitted' || status === 'reserved' || status === 'pending') {
  await new Promise(r => setTimeout(r, 3000));
  attempts++;
  const pollRes  = await fetch(`${DASHI_URL}/v1/execute/${sponsorshipId}`, {
    headers: { 'X-API-Key': API_KEY },
  });
  const pollBody = await pollRes.json();
  status = pollBody.status;
  digest = pollBody.digest ?? null;
  console.log(`  [${attempts}] status=${status}${digest ? '  digest='+digest : ''}`);
  if (attempts >= 120) { console.error('Polling timeout (6 min)'); break; }
}

if (status === 'completed' && digest) {
  console.log(`\n✅ Transaction confirmed!`);
  console.log(`   Digest: ${digest}`);
  console.log(`   View:   https://suiscan.xyz/${NETWORK}/tx/${digest}`);
} else {
  console.error(`\n❌ Execution ended with status: ${status}`);
}
