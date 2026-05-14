package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// ShinamiClient wraps the sui-gas-pool backend.
//
// The type name is kept from Phase 1 so handlers.go needs zero changes.
// Phase 3 replacement plan: delete this file and create a Move smart contract
// client with the same SponsorTransaction signature.
//
// sui-gas-pool docs: https://github.com/MystenLabs/sui-gas-pool
type ShinamiClient struct {
	endpoint   string
	authToken  string
	httpClient *http.Client
}

// SponsorshipResult is the normalized response from the gas backend.
// The field names define the Dashi API contract — not any backend's internals.
type SponsorshipResult struct {
	// SponsoredTransaction is the Base64-encoded tx reference returned by the backend.
	SponsoredTransaction string
	// SponsorshipID is the on-chain transaction digest after execution.
	SponsorshipID string
}

// NewShinamiClient creates a gas-pool client.
// endpoint is the gas-pool base URL (e.g. "http://127.0.0.1:9527").
// authToken is the GAS_STATION_AUTH Bearer token.
func NewShinamiClient(endpoint, authToken string) *ShinamiClient {
	return &ShinamiClient{
		endpoint:  endpoint,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// gasPoolRequest is a generic wrapper for all gas-pool HTTP requests.
func (c *ShinamiClient) post(ctx context.Context, path string, reqBody, respBody interface{}) error {
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

// ── Step 1: reserve_gas ────────────────────────────────────────────────────

type reserveGasReq struct {
	GasBudget           int64 `json:"gas_budget"`
	ReserveDurationSecs int64 `json:"reserve_duration_secs"`
}

type reserveGasResp struct {
	Result *struct {
		SponsorAddress string `json:"sponsor_address"`
		ReservationID  string `json:"reservation_id"`
	} `json:"result"`
	Error *string `json:"error"`
}

func (c *ShinamiClient) reserveGas(ctx context.Context) (*reserveGasResp, error) {
	req := reserveGasReq{
		GasBudget:           5_000_000, // 0.005 SUI in MIST — ceiling for this transaction
		ReserveDurationSecs: 60,        // release reservation after 60 s if execute_tx never called
	}
	var resp reserveGasResp
	if err := c.post(ctx, "/v1/reserve_gas", req, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("reserve_gas error: %s", *resp.Error)
	}
	if resp.Result == nil || resp.Result.ReservationID == "" {
		return nil, fmt.Errorf("reserve_gas returned empty result")
	}
	return &resp, nil
}

// ── Step 2: execute_tx ─────────────────────────────────────────────────────

type executeTxReq struct {
	ReservationID string `json:"reservation_id"`
	TxBytes       string `json:"tx_bytes"`
	UserSig       string `json:"user_sig"`
}

type executeTxResp struct {
	TxBlockResponse *struct {
		Digest string `json:"digest"`
	} `json:"tx_block_response"`
	Error *string `json:"error"`
}

func (c *ShinamiClient) executeTx(ctx context.Context, reservationID, txKindBytes string) (*executeTxResp, error) {
	req := executeTxReq{
		ReservationID: reservationID,
		TxBytes:       txKindBytes,
		UserSig:       "", // gas-pool signs on behalf of the operator
	}
	var resp executeTxResp
	if err := c.post(ctx, "/v1/execute_tx", req, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("execute_tx error: %s", *resp.Error)
	}
	if resp.TxBlockResponse == nil || resp.TxBlockResponse.Digest == "" {
		return nil, fmt.Errorf("execute_tx returned empty digest")
	}
	return &resp, nil
}

// ── Public interface (matches the Phase 1 ShinamiClient signature) ─────────

// SponsorTransaction reserves gas coins from sui-gas-pool and executes the
// sponsored transaction. Returns the on-chain tx digest as SponsorshipID.
func (c *ShinamiClient) SponsorTransaction(ctx context.Context, txKindBytes, sender string) (*SponsorshipResult, error) {
	reservation, err := c.reserveGas(ctx)
	if err != nil {
		return nil, fmt.Errorf("reserve gas: %w", err)
	}

	result, err := c.executeTx(ctx, reservation.Result.ReservationID, txKindBytes)
	if err != nil {
		return nil, fmt.Errorf("execute tx: %w", err)
	}

	digest := result.TxBlockResponse.Digest
	return &SponsorshipResult{
		SponsoredTransaction: digest, // tx is already executed; digest is the reference
		SponsorshipID:        digest,
	}, nil
}
