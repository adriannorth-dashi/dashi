import { getFullnodeUrl } from '@mysten/sui/client';
import { Transaction } from '@mysten/sui/transactions';
import { Ed25519Keypair } from '@mysten/sui/keypairs/ed25519';
import { decodeSuiPrivateKey } from '@mysten/sui/cryptography';

// ── Config ─────────────────────────────────────────────────────────────────
// Sender wallet: used for the transaction body (must own coins).
// In production this is the user's own wallet — they sign with their browser wallet.
// For this test we use the sponsor wallet since we have its private key.
const SENDER_PRIVKEY = 'suiprivkey1qpuqwnr6mvu95xrymvk7xwgrcngxuksndghks00ycjsq0lh5f7jf2u34znx';
const SENDER      = '0x7947caac8d728b10a960ac82fd6cd823f8ea7409245f2420465a71660f1e273c';
const API_KEY     = '64c51e0378cba47087f150f470610fe3fcab3d3dbd020f9b35f34de9879b4ab5';
const DASHI_URL   = 'http://localhost:8080';
const ZERO_ADDR   = '0x0000000000000000000000000000000000000000000000000000000000000000';
const RPC_URL     = getFullnodeUrl('mainnet');

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
  return result.data;
}

// ── 1. Setup ───────────────────────────────────────────────────────────────
const { secretKey } = decodeSuiPrivateKey(SENDER_PRIVKEY);
const keypair = Ed25519Keypair.fromSecretKey(secretKey);

console.log('Dashi 2-step sponsored transaction test');
console.log(`Network: mainnet`);
console.log(`Sender:  ${SENDER}\n`);

// ── 2. Get sender coins ────────────────────────────────────────────────────
const coins = await getCoins(SENDER);
console.log(`Sender has ${coins.length} SUI coin(s).`);
if (coins.length === 0) {
  console.error('No coins found — fund the sender wallet first.');
  process.exit(1);
}

// Pick a coin with the largest balance to avoid using gas-pool managed coins.
const coin = coins.reduce((a, b) => BigInt(a.balance) > BigInt(b.balance) ? a : b);
console.log(`Using coin ${coin.coinObjectId} (balance: ${coin.balance} MIST)\n`);

// ── 3. Build TransactionKind bytes ─────────────────────────────────────────
// The Sui client is used only to resolve object versions for the transaction.
import { SuiClient } from '@mysten/sui/client';
const suiClient = new SuiClient({ url: RPC_URL });

const tx = new Transaction();
const [zeroCoin] = tx.splitCoins(coin.coinObjectId, [0]);
tx.transferObjects([zeroCoin], ZERO_ADDR);

const kindBytes  = await tx.build({ client: suiClient, onlyTransactionKind: true });
const kindBase64 = Buffer.from(kindBytes).toString('base64');

console.log(`TransactionKind: ${kindBytes.length} bytes`);
console.log(`Base64 (first 60): ${kindBase64.slice(0, 60)}...\n`);

// ── Step 1: POST /v1/sponsor — reserve gas + get TransactionData ───────────
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

// ── Step 2: Sign the TransactionData ──────────────────────────────────────
// In a real dApp the user signs this in their browser wallet.
console.log(`── Step 2: Signing TransactionData (sponsorshipId: ${sponsorshipId})`);
const txBytes = Uint8Array.from(Buffer.from(txBytesB64, 'base64'));
const { signature } = await keypair.signTransaction(txBytes);
console.log(`Signature (first 40): ${signature.slice(0, 40)}...\n`);

// ── Step 3: POST /v1/execute — submit with user signature ─────────────────
console.log(`── Step 3: POST ${DASHI_URL}/v1/execute`);
const executeRes = await fetch(`${DASHI_URL}/v1/execute`, {
  method:  'POST',
  headers: { 'Content-Type': 'application/json', 'X-API-Key': API_KEY },
  body:    JSON.stringify({ sponsorshipId, txBytes: txBytesB64, userSig: signature }),
});
const executeBody = await executeRes.json();
console.log(`HTTP ${executeRes.status}\n`, JSON.stringify(executeBody, null, 2));

if (executeRes.ok) {
  console.log(`\n✅ Transaction submitted!`);
  console.log(`   Digest: ${executeBody.digest}`);
  console.log(`   View:   https://suiscan.xyz/mainnet/tx/${executeBody.digest}`);
}
