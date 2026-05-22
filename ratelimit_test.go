// Copyright (C) 2025 Dashi Project
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"net/http"
	"testing"

	"codeberg.org/adrian_north/dashi/testutils"
)

// newTestHandlersWithRL creates a Handlers instance with a real RateLimiter.
// Skips the test if Redis is not reachable (mirrors the DB test pattern).
func newTestHandlersWithRL(t *testing.T) (*Handlers, *RateLimiter) {
	t.Helper()
	rl, err := NewRateLimiter("redis://127.0.0.1:6379")
	if err != nil {
		t.Skipf("Redis not reachable, skipping rate limit test: %v", err)
	}
	t.Cleanup(func() { rl.Close() })

	h := &Handlers{
		db:    nullDB(t),
		dashi: NewDashiClient("", "test-token", ""),
		sui:   NewSuiClient("http://127.0.0.1:19998", ""),
		cfg: Config{
			Network:            "mainnet",
			APIKey:             testutils.TestAPIKey,
			RateLimitPerMinute: 3, // tiny limit so we can trigger 429 quickly
			GlobalRateLimit:    500,
		},
		rl: rl,
	}
	return h, rl
}

// flushRateLimitKeys removes all rl:* keys created during a test run.
// Uses SCAN to avoid blocking the server with FLUSHDB.
func flushRateLimitKeys(t *testing.T, rl *RateLimiter) {
	t.Helper()
	ctx := t.Context()
	var cursor uint64
	for {
		keys, next, err := rl.rdb.Scan(ctx, cursor, "rl:*", 100).Result()
		if err != nil {
			return
		}
		if len(keys) > 0 {
			rl.rdb.Del(ctx, keys...)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
}

// TestPerKeyRateLimit_Returns429AfterLimit verifies that the n+1-th request
// from the same API key within a single minute window is rejected with 429.
func TestPerKeyRateLimit_Returns429AfterLimit(t *testing.T) {
	h, rl := newTestHandlersWithRL(t)
	flushRateLimitKeys(t, rl)
	t.Cleanup(func() { flushRateLimitKeys(t, rl) })

	r := newRouter(h)

	// The first RateLimitPerMinute requests must succeed (200 or 401 from DB, never 429).
	for i := range h.cfg.RateLimitPerMinute {
		w := do(t, r, "GET", "/v1/balance", nil, map[string]string{"X-API-Key": testutils.TestAPIKey})
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d/%d was rate-limited too early", i+1, h.cfg.RateLimitPerMinute)
		}
	}

	// The next request must be rejected.
	w := do(t, r, "GET", "/v1/balance", nil, map[string]string{"X-API-Key": testutils.TestAPIKey})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after %d requests, got %d: %s",
			h.cfg.RateLimitPerMinute, w.Code, w.Body.String())
	}

	body := parseBody(t, w)
	if body["error"] == nil {
		t.Error("expected error field in 429 response")
	}
	if body["hint"] == nil {
		t.Error("expected hint field in 429 response")
	}
}

// TestRateLimitHeaders_PresentOnOKResponse checks that X-RateLimit-* headers
// are included in successful responses when rate limiting is active.
func TestRateLimitHeaders_PresentOnOKResponse(t *testing.T) {
	h, rl := newTestHandlersWithRL(t)
	flushRateLimitKeys(t, rl)
	t.Cleanup(func() { flushRateLimitKeys(t, rl) })

	r := newRouter(h)

	w := do(t, r, "GET", "/v1/balance", nil, map[string]string{"X-API-Key": testutils.TestAPIKey})
	if w.Code == http.StatusTooManyRequests {
		t.Fatalf("first request should not be rate-limited")
	}

	for _, header := range []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"} {
		if w.Header().Get(header) == "" {
			t.Errorf("expected response header %s to be set", header)
		}
	}
}

// TestGlobalRateLimit_Returns429 verifies the global limiter triggers 429
// when the global cap is exceeded, even with a valid API key.
func TestGlobalRateLimit_Returns429(t *testing.T) {
	h, rl := newTestHandlersWithRL(t)
	flushRateLimitKeys(t, rl)
	t.Cleanup(func() { flushRateLimitKeys(t, rl) })

	// Override: tiny global cap, big per-key cap, so only the global fires.
	h.cfg.GlobalRateLimit = 2
	h.cfg.RateLimitPerMinute = 100

	r := newRouter(h)

	for i := range h.cfg.GlobalRateLimit {
		w := do(t, r, "GET", "/health", nil, nil) // /health also goes through global middleware
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d was rate-limited too early", i+1)
		}
	}

	w := do(t, r, "GET", "/health", nil, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after global limit, got %d", w.Code)
	}
}

// TestNoRateLimiter_RequestsAlwaysPass confirms that when rl is nil
// (the default in most handler tests) no rate limiting is applied.
func TestNoRateLimiter_RequestsAlwaysPass(t *testing.T) {
	h := &Handlers{
		db:    nullDB(t),
		dashi: NewDashiClient("", "test-token", ""),
		sui:   NewSuiClient("http://127.0.0.1:19998", ""),
		cfg: Config{
			Network: "mainnet",
			APIKey:  testutils.TestAPIKey,
			// rl is nil → no rate limiting
		},
	}
	r := newRouter(h)

	for i := range 10 {
		w := do(t, r, "GET", "/health", nil, nil)
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d was rate-limited with nil RateLimiter", i+1)
		}
	}
}

// TestRateLimitMiddleware_SkipsRequestsWithoutAPIKey ensures the per-key middleware
// does not rate-limit requests that have no API key — AuthMiddleware handles rejection.
// We test this end-to-end: without a key the router returns 401 (not 429).
func TestRateLimitMiddleware_SkipsRequestsWithoutAPIKey(t *testing.T) {
	h, rl := newTestHandlersWithRL(t)
	flushRateLimitKeys(t, rl)
	t.Cleanup(func() { flushRateLimitKeys(t, rl) })

	// Set a very tight per-key limit so it would fire immediately if the key
	// were being counted.
	h.cfg.RateLimitPerMinute = 1

	r := newRouter(h)

	// Send many requests without an API key — all should get 401, never 429.
	for i := range 5 {
		w := do(t, r, "GET", "/v1/balance", nil, nil) // no X-API-Key
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d without API key was rate-limited (expected 401, not 429)", i+1)
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unauthenticated request, got %d", w.Code)
		}
	}
}
