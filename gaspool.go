// Copyright (C) 2025 Dashi Project
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DashiClient wraps the sui-gas-pool backend.
//
// Phase 3 replacement plan: delete this file and create a Move smart contract
// client with the same Reserve/Execute signatures.
//
// sui-gas-pool docs: https://github.com/MystenLabs/sui-gas-pool
type DashiClient struct {
	endpoint   string
	authToken  string
	rpcURL     string // Sui fullnode RPC for gas price queries
	httpClient *http.Client
}

// SponsorshipReservation is returned by Reserve.
// TxBytes is base64 BCS TransactionData the sender must sign before calling Execute.
// ReservationID ties this reservation to the gas coins held by sui-gas-pool.
type SponsorshipReservation struct {
	TxBytes       string
	ReservationID int64
}

// NewDashiClient creates a gas-pool client.
// endpoint is the gas-pool base URL (e.g. "http://127.0.0.1:9527").
// authToken is the GAS_STATION_AUTH Bearer token.
// rpcURL is the Sui fullnode RPC used to query the reference gas price.
func NewDashiClient(endpoint, authToken, rpcURL string) *DashiClient {
	return &DashiClient{
		endpoint:  endpoint,
		authToken: authToken,
		rpcURL:    rpcURL,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// post is a generic wrapper for all gas-pool HTTP requests.
func (c *DashiClient) post(ctx context.Context, path string, reqBody, respBody interface{}) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	log.Printf("→ gas-pool %s: %s", path, payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gas-pool request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	log.Printf("← gas-pool %s (HTTP %d): %s", path, resp.StatusCode, raw)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gas-pool returned HTTP %d: %s", resp.StatusCode, raw)
	}
	return json.Unmarshal(raw, respBody)
}

// ── Step 1: reserve_gas ──────────────────────────────────────────────────────

type reserveGasReq struct {
	GasBudget           int64 `json:"gas_budget"`
	ReserveDurationSecs int64 `json:"reserve_duration_secs"`
}

type gasCoin struct {
	ObjectID string `json:"objectId"`
	Version  uint64 `json:"version"`
	Digest   string `json:"digest"` // base58-encoded 32-byte hash
}

type reserveGasResp struct {
	Result *struct {
		SponsorAddress string    `json:"sponsor_address"`
		ReservationID  int64     `json:"reservation_id"`
		GasCoins       []gasCoin `json:"gas_coins"`
	} `json:"result"`
	Error *string `json:"error"`
}

func (c *DashiClient) reserveGas(ctx context.Context) (*reserveGasResp, error) {
	req := reserveGasReq{
		GasBudget:           5_000_000, // 0.005 SUI in MIST — ceiling for this transaction
		ReserveDurationSecs: 60,        // release reservation after 60 s if execute never called
	}
	var resp reserveGasResp
	if err := c.post(ctx, "/v1/reserve_gas", req, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("reserve_gas error: %s", *resp.Error)
	}
	if resp.Result == nil || resp.Result.ReservationID == 0 || len(resp.Result.GasCoins) == 0 {
		return nil, fmt.Errorf("reserve_gas returned empty result")
	}
	return &resp, nil
}

// ── Step 2: execute_tx ───────────────────────────────────────────────────────

type executeTxReq struct {
	ReservationID int64  `json:"reservation_id"`
	TxBytes       string `json:"tx_bytes"`
	UserSig       string `json:"user_sig"`
}

type executeTxResp struct {
	Effects *struct {
		TransactionDigest string `json:"transactionDigest"`
	} `json:"effects"`
	TxBlockResponse *struct {
		Digest string `json:"digest"`
	} `json:"tx_block_response"`
	Error *string `json:"error"`
}

func (c *DashiClient) executeTx(ctx context.Context, reservationID int64, txBytes, userSig string) (*executeTxResp, error) {
	req := executeTxReq{
		ReservationID: reservationID,
		TxBytes:       txBytes,
		UserSig:       userSig,
	}
	var resp executeTxResp
	if err := c.post(ctx, "/v1/execute_tx", req, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("execute_tx error: %s", *resp.Error)
	}
	if resp.digest() == "" {
		return nil, fmt.Errorf("execute_tx returned empty digest")
	}
	return &resp, nil
}

// digest returns the transaction digest from whichever field the gas pool populated.
// sui-gas-pool places it in effects.transactionDigest; tx_block_response is a fallback.
func (r *executeTxResp) digest() string {
	if r.Effects != nil && r.Effects.TransactionDigest != "" {
		return r.Effects.TransactionDigest
	}
	if r.TxBlockResponse != nil && r.TxBlockResponse.Digest != "" {
		return r.TxBlockResponse.Digest
	}
	return ""
}

// ── Gas price ────────────────────────────────────────────────────────────────

// getReferenceGasPrice queries suix_getReferenceGasPrice from the Sui RPC.
// Falls back to 750 MIST on any error or when rpcURL is empty.
func (c *DashiClient) getReferenceGasPrice(ctx context.Context) uint64 {
	const fallback = uint64(750)
	if c.rpcURL == "" {
		return fallback
	}

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "suix_getReferenceGasPrice",
		"params":  []interface{}{},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fallback
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return fallback
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()

	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fallback
	}

	// suix_getReferenceGasPrice returns u64 as a JSON string (e.g. "750").
	var priceStr string
	if err := json.Unmarshal(envelope.Result, &priceStr); err != nil {
		return fallback
	}
	price, err := strconv.ParseUint(priceStr, 10, 64)
	if err != nil {
		return fallback
	}
	return price
}

// ── BCS TransactionData builder ──────────────────────────────────────────────

// hexToAddress converts a "0x…" hex string to a 32-byte Sui address.
func hexToAddress(addr string) ([32]byte, error) {
	b, err := hex.DecodeString(strings.TrimPrefix(addr, "0x"))
	if err != nil || len(b) != 32 {
		return [32]byte{}, fmt.Errorf("invalid Sui address %q", addr)
	}
	var out [32]byte
	copy(out[:], b)
	return out, nil
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// base58Decode decodes a standard (Bitcoin-alphabet) base58 string to 32 bytes.
// Used for Sui ObjectDigest values returned by the gas pool.
func base58Decode(s string) ([32]byte, error) {
	result := new(big.Int)
	base := big.NewInt(58)
	for _, c := range s {
		idx := strings.IndexRune(base58Alphabet, c)
		if idx < 0 {
			return [32]byte{}, fmt.Errorf("invalid base58 character %q", c)
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(idx)))
	}
	decoded := result.Bytes()
	if len(decoded) > 32 {
		return [32]byte{}, fmt.Errorf("base58 decoded to %d bytes, expected ≤32", len(decoded))
	}
	var out [32]byte
	copy(out[32-len(decoded):], decoded)
	return out, nil
}

// writeBCSULEB128 writes v as a BCS/ULEB128 variable-length integer.
func writeBCSULEB128(buf *bytes.Buffer, v uint64) {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		buf.WriteByte(b)
		if v == 0 {
			break
		}
	}
}

// buildTransactionData constructs BCS-encoded Sui TransactionData (V1) from a
// TransactionKind byte slice, sponsorship parameters, and gas coins.
//
// BCS layout (TransactionData::V1 → TransactionDataV1):
//
//	[0x00]                TransactionData::V1 discriminant
//	[kindBytes]           BCS TransactionKind
//	[32 bytes]            sender SuiAddress
//	ULEB128(len(coins))   GasData.payment Vec<ObjectRef>
//	  for each coin:
//	    [32 bytes]         ObjectID
//	    [8 bytes LE]       SequenceNumber (version, u64)
//	    [0x20, 32 bytes]   ObjectDigest (serde_bytes: ULEB128 len + raw bytes)
//	[32 bytes]            GasData.owner (sponsor address)
//	[8 bytes LE]          GasData.price (u64)
//	[8 bytes LE]          GasData.budget (u64)
//	[0x00]                TransactionExpiration::None
func buildTransactionData(kindBytes []byte, sender, sponsorAddr string, coins []gasCoin, gasPrice, gasBudget uint64) ([]byte, error) {
	senderAddr, err := hexToAddress(sender)
	if err != nil {
		return nil, fmt.Errorf("parse sender: %w", err)
	}
	sponsor, err := hexToAddress(sponsorAddr)
	if err != nil {
		return nil, fmt.Errorf("parse sponsor: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteByte(0x00)      // TransactionData::V1
	buf.Write(kindBytes)     // TransactionKind (BCS-encoded by the Sui SDK)
	buf.Write(senderAddr[:]) // sender

	// GasData.payment: Vec<ObjectRef>
	writeBCSULEB128(&buf, uint64(len(coins)))
	for _, coin := range coins {
		objID, err := hexToAddress(coin.ObjectID)
		if err != nil {
			return nil, fmt.Errorf("parse gas coin objectId: %w", err)
		}
		buf.Write(objID[:]) // ObjectID (32 bytes)

		var verBytes [8]byte
		binary.LittleEndian.PutUint64(verBytes[:], coin.Version)
		buf.Write(verBytes[:]) // SequenceNumber (u64 LE)

		digest, err := base58Decode(coin.Digest)
		if err != nil {
			return nil, fmt.Errorf("decode gas coin digest: %w", err)
		}
		buf.WriteByte(0x20) // ObjectDigest: serde_bytes → ULEB128(32) then raw bytes
		buf.Write(digest[:])
	}

	buf.Write(sponsor[:]) // GasData.owner
	var u64 [8]byte
	binary.LittleEndian.PutUint64(u64[:], gasPrice)
	buf.Write(u64[:]) // GasData.price
	binary.LittleEndian.PutUint64(u64[:], gasBudget)
	buf.Write(u64[:]) // GasData.budget

	buf.WriteByte(0x00) // TransactionExpiration::None

	return buf.Bytes(), nil
}

// ── Public interface ─────────────────────────────────────────────────────────

// Reserve reserves gas coins and returns TransactionData bytes for the sender to sign.
// The returned TxBytes and ReservationID must both be passed to Execute.
func (c *DashiClient) Reserve(ctx context.Context, txKindBytes, sender string) (*SponsorshipReservation, error) {
	kindBytes, err := base64.StdEncoding.DecodeString(txKindBytes)
	if err != nil {
		return nil, fmt.Errorf("decode txKindBytes: %w", err)
	}

	reservation, err := c.reserveGas(ctx)
	if err != nil {
		return nil, fmt.Errorf("reserve gas: %w", err)
	}

	gasPrice := c.getReferenceGasPrice(ctx)

	txData, err := buildTransactionData(kindBytes, sender, reservation.Result.SponsorAddress, reservation.Result.GasCoins, gasPrice, 5_000_000)
	if err != nil {
		return nil, fmt.Errorf("build tx data: %w", err)
	}

	return &SponsorshipReservation{
		TxBytes:       base64.StdEncoding.EncodeToString(txData),
		ReservationID: reservation.Result.ReservationID,
	}, nil
}

// Execute submits a reserved sponsored transaction with the sender's signature.
// txBytes is the base64 TransactionData returned by Reserve (unmodified).
// userSig is the sender's Sui signature over the intent message of txBytes.
// Returns the on-chain transaction digest.
func (c *DashiClient) Execute(ctx context.Context, reservationID int64, txBytes, userSig string) (string, error) {
	result, err := c.executeTx(ctx, reservationID, txBytes, userSig)
	if err != nil {
		return "", err
	}
	return result.digest(), nil
}
