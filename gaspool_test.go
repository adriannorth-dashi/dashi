package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/adrian_north/dashi/testutils"
)

// ── NewDashiClient ────────────────────────────────────────────────────────────

func TestNewDashiClient_StoresConfig(t *testing.T) {
	c := NewDashiClient("http://127.0.0.1:9527", "my-token")
	if c.endpoint != "http://127.0.0.1:9527" {
		t.Errorf("endpoint = %q", c.endpoint)
	}
	if c.authToken != "my-token" {
		t.Errorf("authToken = %q", c.authToken)
	}
	if c.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

// ── SponsorTransaction success path ──────────────────────────────────────────

func TestSponsorTransaction_ReturnsDigestOnSuccess(t *testing.T) {
	srv := testutils.MockGasPoolServer(t)
	client := NewDashiClient(srv.URL, "tok")

	res, err := client.SponsorTransaction(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SponsoredTransaction != testutils.TestTxDigest {
		t.Errorf("SponsoredTransaction = %q, want %q", res.SponsoredTransaction, testutils.TestTxDigest)
	}
	if res.SponsorshipID != testutils.TestTxDigest {
		t.Errorf("SponsorshipID = %q, want %q", res.SponsorshipID, testutils.TestTxDigest)
	}
}

// ── reserveGas error paths ────────────────────────────────────────────────────

func TestSponsorTransaction_ReserveGasHTTPError(t *testing.T) {
	srv := testutils.MockGasPoolServerError(t)
	client := NewDashiClient(srv.URL, "tok")

	_, err := client.SponsorTransaction(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err == nil {
		t.Fatal("expected error from gas-pool 500 response, got nil")
	}
}

func TestSponsorTransaction_ReserveGasApplicationError(t *testing.T) {
	// Server returns HTTP 200 but with an application-level error field.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		msg := "insufficient funds"
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": &msg,
		})
	}))
	t.Cleanup(srv.Close)
	client := NewDashiClient(srv.URL, "tok")

	_, err := client.SponsorTransaction(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err == nil {
		t.Fatal("expected application-level error, got nil")
	}
}

func TestSponsorTransaction_ReserveGasEmptyResult(t *testing.T) {
	// Server returns HTTP 200 with no result and no error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	t.Cleanup(srv.Close)
	client := NewDashiClient(srv.URL, "tok")

	_, err := client.SponsorTransaction(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err == nil {
		t.Fatal("expected error for empty result, got nil")
	}
}

// ── executeTx error paths ─────────────────────────────────────────────────────

func TestSponsorTransaction_ExecuteTxApplicationError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if r.URL.Path == "/v1/reserve_gas" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{
					"sponsor_address": testutils.ValidSuiAddress(),
					"reservation_id":  "rsv-123",
				},
			})
			return
		}
		// execute_tx returns application-level error
		msg := "transaction rejected"
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": &msg,
		})
	}))
	t.Cleanup(srv.Close)
	client := NewDashiClient(srv.URL, "tok")

	_, err := client.SponsorTransaction(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err == nil {
		t.Fatal("expected error from execute_tx failure, got nil")
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 calls (reserve + execute), got %d", callCount)
	}
}

func TestSponsorTransaction_ExecuteTxEmptyDigest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/reserve_gas" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{
					"sponsor_address": testutils.ValidSuiAddress(),
					"reservation_id":  "rsv-123",
				},
			})
			return
		}
		// execute_tx returns a response with an empty digest
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tx_block_response": map[string]interface{}{
				"digest": "",
			},
		})
	}))
	t.Cleanup(srv.Close)
	client := NewDashiClient(srv.URL, "tok")

	_, err := client.SponsorTransaction(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err == nil {
		t.Fatal("expected error for empty digest, got nil")
	}
}

func TestSponsorTransaction_UnreachableEndpoint(t *testing.T) {
	client := NewDashiClient("http://127.0.0.1:19997", "tok")

	_, err := client.SponsorTransaction(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err == nil {
		t.Fatal("expected error for unreachable endpoint, got nil")
	}
}
