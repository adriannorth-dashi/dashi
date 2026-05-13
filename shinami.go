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

// ShinamiClient is the Phase 1 gas backend.
//
// Phase 2 replacement plan: delete this file and create a sui-gas-pool client
// that exposes the same SponsorTransaction method. No other file needs to change.
//
// Shinami Gas Station docs: https://docs.shinami.com/reference/gas-station-api
type ShinamiClient struct {
	endpoint   string
	httpClient *http.Client
}

// SponsorshipResult is the normalized response from the gas backend.
// The field names here define the Dashi API contract — not Shinami's internals.
type SponsorshipResult struct {
	// SponsoredTransaction is the Base64-encoded TransactionData ready for user signing.
	SponsoredTransaction string
	// SponsorshipID is a unique identifier for this sponsorship, used for tracking.
	SponsorshipID string
}

// NewShinamiClient creates a client for the Shinami Gas Station API.
// apiKey is embedded in the endpoint URL per Shinami's authentication scheme.
func NewShinamiClient(apiKey string) *ShinamiClient {
	return &ShinamiClient{
		endpoint: "https://api.shinami.com/gas/v1/" + apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// rpcRequest is a JSON-RPC 2.0 request envelope.
type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

// SponsorTransaction submits a TransactionKind to Shinami for gas sponsorship.
// txKindBytes is Base64-encoded. sender is a Sui address (0x + 64 hex chars).
// Returns the sponsored TransactionData bytes and a sponsorship ID.
func (s *ShinamiClient) SponsorTransaction(ctx context.Context, txKindBytes, sender string) (*SponsorshipResult, error) {
	// Shinami docs: params = [txKindBytes, senderAddress]
	// gasBudget omitted — Shinami derives it from the fund configuration.
	payload := rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "gas_sponsorTransactionBlock",
		Params:  []interface{}{txKindBytes, sender},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	log.Printf("→ Shinami request: %s", body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "dashi/"+version)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("shinami request: %w", err)
	}
	defer resp.Body.Close()

	// Read the full body so we can log it before decoding.
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read shinami response: %w", err)
	}
	log.Printf("← Shinami response (HTTP %d): %s", resp.StatusCode, rawBody)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("shinami returned HTTP %d: %s", resp.StatusCode, rawBody)
	}

	// Shinami JSON-RPC response fields:
	//   txBytes       — sponsored TransactionData, Base64-encoded
	//   txDigest      — Shinami's pre-computed digest (used as sponsorshipId)
	//   signature     — Shinami's signature over the transaction
	//   expireAtTime  — Unix ms after which this sponsorship expires
	var rpcResp struct {
		Result *struct {
			TxBytes      string `json:"txBytes"`
			TxDigest     string `json:"txDigest"`
			Signature    string `json:"signature"`
			ExpireAtTime int64  `json:"expireAtTime"`
		} `json:"result"`
		Error *struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		} `json:"error"`
	}

	if err := json.Unmarshal(rawBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("decode shinami response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("shinami error %d: %s (details: %s)",
			rpcResp.Error.Code, rpcResp.Error.Message, rpcResp.Error.Data)
	}

	if rpcResp.Result == nil {
		return nil, fmt.Errorf("shinami returned empty result")
	}

	return &SponsorshipResult{
		SponsoredTransaction: rpcResp.Result.TxBytes,
		// Shinami uses txDigest as the unique sponsorship identifier
		SponsorshipID: rpcResp.Result.TxDigest,
	}, nil
}
