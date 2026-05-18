//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

// Integration tests hit a running Dashi instance (default: localhost:8080).
// Run with:
//
//	go test ./... -tags integration -run TestIntegration
//
// Required env vars:
//
//	API_KEY    — Dashi API key (same as in .env)
//	DASHI_URL  — optional base URL (default http://localhost:8080)

func integrationBaseURL() string {
	if u := os.Getenv("DASHI_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

func integrationAPIKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("API_KEY")
	if key == "" {
		t.Skip("API_KEY not set — skipping integration test")
	}
	return key
}

func TestIntegration_Health(t *testing.T) {
	resp, err := http.Get(integrationBaseURL() + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
}

func TestIntegration_Balance(t *testing.T) {
	apiKey := integrationAPIKey(t)

	req, _ := http.NewRequest("GET", integrationBaseURL()+"/v1/balance", nil)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("balance request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if _, ok := body["balance"]; !ok {
		t.Error("expected balance field in response")
	}
}

func TestIntegration_SponsorTransaction(t *testing.T) {
	apiKey := integrationAPIKey(t)

	// The transaction kind bytes below are intentionally minimal / likely invalid
	// for the gas pool, so this test verifies the request pipeline, not on-chain success.
	// For a full end-to-end test use a real transaction built via the Sui SDK (test.mjs).
	payload := map[string]string{
		"transactionKindBytes": "AQIDBA==",
		"sender":               "0x5757176f7fd65aa19893ec3dd368d88e25e032956af29843bdcbb03ca60f86f6",
	}
	b, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", integrationBaseURL()+"/v1/sponsor", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sponsor request failed: %v", err)
	}
	defer resp.Body.Close()

	// Accept 200 (success) or 502 (gas pool rejected the dummy tx) — both indicate
	// that Dashi's request pipeline is working correctly.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 200 or 502, got %d", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusOK {
		var body map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&body)
		for _, field := range []string{"sponsoredTransaction", "sponsorshipId", "feeInfo"} {
			if _, ok := body[field]; !ok {
				t.Errorf("expected %q in response", field)
			}
		}
		feeInfo, ok := body["feeInfo"].(map[string]interface{})
		if !ok {
			t.Fatal("feeInfo is not an object")
		}
		for _, fee := range []string{"networkFee", "serviceFee", "totalFee"} {
			if _, ok := feeInfo[fee]; !ok {
				t.Errorf("expected feeInfo.%s", fee)
			}
		}
	}
}

func TestIntegration_AuthRejectsNoKey(t *testing.T) {
	resp, err := http.Post(integrationBaseURL()+"/v1/sponsor", "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without API key, got %d", resp.StatusCode)
	}
}
