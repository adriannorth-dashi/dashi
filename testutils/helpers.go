// Package testutils provides shared helpers for Dashi unit tests.
// It contains only standard-library dependencies so it never imports
// the main package (which would create a circular dependency).
package testutils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAPIKey is the API key used across all unit tests.
const TestAPIKey = "test-api-key-32chars-1234567890ab"

// TestTxDigest is a realistic-looking Sui transaction digest used as a mock response.
const TestTxDigest = "EXc7TJpYGEuhXBnRJBxsMgbFAMkWqRCNhSMaCKbTnpyQ"

// ValidSuiAddress returns a well-formed Sui address (0x + 64 lowercase hex chars).
func ValidSuiAddress() string {
	return "0x5757176f7fd65aa19893ec3dd368d88e25e032956af29843bdcbb03ca60f86f6"
}

// InvalidSuiAddresses returns named test cases for malformed Sui addresses.
func InvalidSuiAddresses() []struct {
	Name    string
	Address string
} {
	return []struct {
		Name    string
		Address string
	}{
		{"empty string", ""},
		{"only 0x prefix", "0x"},
		{"4 hex chars", "0x1234"},
		{"63 hex chars (too short)", "0x" + strings.Repeat("a", 63)},
		{"65 hex chars (too long)", "0x" + strings.Repeat("a", 65)},
		{"no 0x prefix", strings.Repeat("a", 64)},
		{"invalid hex z chars", "0x" + strings.Repeat("z", 64)},
		{"uppercase 0X prefix", "0X" + strings.Repeat("a", 64)},
	}
}

// MockGasPoolServer returns a test HTTP server that mimics the sui-gas-pool REST API.
// Responds successfully to POST /v1/reserve_gas and POST /v1/execute_tx.
func MockGasPoolServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/reserve_gas", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"sponsor_address": ValidSuiAddress(),
				"reservation_id":  "test-reservation-abc123",
			},
		})
	})

	mux.HandleFunc("/v1/execute_tx", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tx_block_response": map[string]interface{}{
				"digest": TestTxDigest,
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// MockGasPoolServerError returns a test HTTP server that always responds HTTP 500.
// Use this to test how Dashi handles gas-pool backend failures.
func MockGasPoolServerError(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"pool exhausted"}`, http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// MockShinamiServer is an alias for MockGasPoolServer — same wire format.
func MockShinamiServer(t *testing.T) *httptest.Server {
	return MockGasPoolServer(t)
}

// MockSuiRPC returns a test HTTP server that mimics the Sui JSON-RPC API,
// returning a successful transaction status for any request.
func MockSuiRPC(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"effects": map[string]interface{}{
					"status": map[string]string{"status": "success"},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// MockSuiRPCError returns a test server that responds with a JSON-RPC error.
func MockSuiRPCError(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "transaction not found",
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}
