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
	c := NewDashiClient("http://127.0.0.1:9527", "my-token", "http://rpc.example.com")
	if c.endpoint != "http://127.0.0.1:9527" {
		t.Errorf("endpoint = %q", c.endpoint)
	}
	if c.authToken != "my-token" {
		t.Errorf("authToken = %q", c.authToken)
	}
	if c.rpcURL != "http://rpc.example.com" {
		t.Errorf("rpcURL = %q", c.rpcURL)
	}
	if c.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

// ── Reserve success path ──────────────────────────────────────────────────────

func TestReserve_ReturnsTxBytesOnSuccess(t *testing.T) {
	srv := testutils.MockGasPoolServer(t)
	client := NewDashiClient(srv.URL, "tok", "")

	res, err := client.Reserve(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TxBytes == "" {
		t.Error("expected non-empty TxBytes")
	}
	if res.ReservationID != 12345 {
		t.Errorf("ReservationID = %d, want 12345", res.ReservationID)
	}
}

// ── Reserve error paths ───────────────────────────────────────────────────────

func TestReserve_ReserveGasHTTPError(t *testing.T) {
	srv := testutils.MockGasPoolServerError(t)
	client := NewDashiClient(srv.URL, "tok", "")

	_, err := client.Reserve(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err == nil {
		t.Fatal("expected error from gas-pool 500 response, got nil")
	}
}

func TestReserve_ReserveGasApplicationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		msg := "insufficient funds"
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": &msg})
	}))
	t.Cleanup(srv.Close)
	client := NewDashiClient(srv.URL, "tok", "")

	_, err := client.Reserve(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err == nil {
		t.Fatal("expected application-level error, got nil")
	}
}

func TestReserve_ReserveGasEmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	t.Cleanup(srv.Close)
	client := NewDashiClient(srv.URL, "tok", "")

	_, err := client.Reserve(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err == nil {
		t.Fatal("expected error for empty result, got nil")
	}
}

func TestReserve_InvalidBase64KindBytes(t *testing.T) {
	client := NewDashiClient("http://127.0.0.1:19997", "tok", "")

	_, err := client.Reserve(context.Background(), "not-valid-base64!!!", testutils.ValidSuiAddress())
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestReserve_UnreachableEndpoint(t *testing.T) {
	client := NewDashiClient("http://127.0.0.1:19997", "tok", "")

	_, err := client.Reserve(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err == nil {
		t.Fatal("expected error for unreachable endpoint, got nil")
	}
}

// ── Execute success path ──────────────────────────────────────────────────────

func TestExecute_ReturnsDigestOnSuccess(t *testing.T) {
	srv := testutils.MockGasPoolServer(t)
	client := NewDashiClient(srv.URL, "tok", "")

	digest, err := client.Execute(context.Background(), 12345, "AQIDBA==", "dGVzdA==")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digest != testutils.TestTxDigest {
		t.Errorf("digest = %q, want %q", digest, testutils.TestTxDigest)
	}
}

// ── Execute error paths ───────────────────────────────────────────────────────

func TestExecute_ApplicationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		msg := "transaction rejected"
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": &msg})
	}))
	t.Cleanup(srv.Close)
	client := NewDashiClient(srv.URL, "tok", "")

	_, err := client.Execute(context.Background(), 123, "AQIDBA==", "dGVzdA==")
	if err == nil {
		t.Fatal("expected error from execute_tx failure, got nil")
	}
}

func TestExecute_EmptyDigest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tx_block_response": map[string]interface{}{"digest": ""},
		})
	}))
	t.Cleanup(srv.Close)
	client := NewDashiClient(srv.URL, "tok", "")

	_, err := client.Execute(context.Background(), 123, "AQIDBA==", "dGVzdA==")
	if err == nil {
		t.Fatal("expected error for empty digest, got nil")
	}
}

func TestExecute_HTTPError(t *testing.T) {
	srv := testutils.MockGasPoolServerError(t)
	client := NewDashiClient(srv.URL, "tok", "")

	_, err := client.Execute(context.Background(), 123, "AQIDBA==", "dGVzdA==")
	if err == nil {
		t.Fatal("expected error from HTTP 500, got nil")
	}
}

func TestExecute_UnreachableEndpoint(t *testing.T) {
	client := NewDashiClient("http://127.0.0.1:19997", "tok", "")

	_, err := client.Execute(context.Background(), 123, "AQIDBA==", "dGVzdA==")
	if err == nil {
		t.Fatal("expected error for unreachable endpoint, got nil")
	}
}

// ── Reserve → Execute full flow ───────────────────────────────────────────────

func TestReserveAndExecute_FullFlow(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if r.URL.Path == "/v1/reserve_gas" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{
					"sponsor_address": testutils.ValidSuiAddress(),
					"reservation_id":  123,
					"gas_coins": []map[string]interface{}{
						{
							"objectId": testutils.ValidSuiAddress(),
							"version":  uint64(12345),
							"digest":   testutils.TestTxDigest,
						},
					},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tx_block_response": map[string]interface{}{"digest": testutils.TestTxDigest},
		})
	}))
	t.Cleanup(srv.Close)
	client := NewDashiClient(srv.URL, "tok", "")

	res, err := client.Reserve(context.Background(), "AQIDBA==", testutils.ValidSuiAddress())
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	digest, err := client.Execute(context.Background(), res.ReservationID, res.TxBytes, "dGVzdA==")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if digest != testutils.TestTxDigest {
		t.Errorf("digest = %q, want %q", digest, testutils.TestTxDigest)
	}
	if callCount != 2 {
		t.Errorf("expected 2 gas-pool calls, got %d", callCount)
	}
}
