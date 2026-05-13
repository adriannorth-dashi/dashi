package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SuiClient queries the Sui RPC for transaction status and balance information.
// RPC spec: https://docs.sui.io/sui-api-ref
type SuiClient struct {
	rpcURL     string
	httpClient *http.Client
}

// NewSuiClient creates a client for the given Sui RPC endpoint.
func NewSuiClient(rpcURL string) *SuiClient {
	return &SuiClient{
		rpcURL: rpcURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// suiRPCCall sends a JSON-RPC 2.0 request to the Sui fullnode and decodes the response.
func (s *SuiClient) suiRPCCall(ctx context.Context, method string, params []interface{}, result interface{}) error {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.rpcURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "dashi/"+version)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("rpc call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sui rpc returned HTTP %d", resp.StatusCode)
	}

	// Decode into a generic envelope to separate RPC errors from results.
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if envelope.Error != nil {
		return fmt.Errorf("rpc error %d: %s", envelope.Error.Code, envelope.Error.Message)
	}

	return json.Unmarshal(envelope.Result, result)
}

// GetTransactionStatus queries the Sui RPC for a transaction's execution status.
// Returns "pending", "success", or "failed".
func (s *SuiClient) GetTransactionStatus(ctx context.Context, digest string) (string, error) {
	var txBlock struct {
		Effects struct {
			Status struct {
				Status string `json:"status"`
			} `json:"status"`
		} `json:"effects"`
	}

	params := []interface{}{
		digest,
		map[string]bool{"showEffects": true},
	}

	err := s.suiRPCCall(ctx, "sui_getTransactionBlock", params, &txBlock)
	if err != nil {
		// RPC error typically means the transaction is not yet finalized.
		return "pending", nil
	}

	switch txBlock.Effects.Status.Status {
	case "success":
		return "success", nil
	case "failure":
		return "failed", nil
	default:
		return "pending", nil
	}
}

// GetBalance returns the SUI balance of the gas fund.
// Phase 1: placeholder — the fund is managed inside Shinami's custody.
// Phase 2: will query sui-gas-pool for the live available balance.
func (s *SuiClient) GetBalance(_ context.Context) (string, error) {
	return "0.00", nil
}
