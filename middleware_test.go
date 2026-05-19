// Copyright (C) 2025 Dashi Project
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newAuthRouter builds a minimal router where /protected is behind AuthMiddleware
// and /open is not — mirrors the production setup (/health vs /v1/*).
func newAuthRouter(apiKey string) *gin.Engine {
	r := gin.New()
	r.GET("/open", func(c *gin.Context) { c.Status(http.StatusOK) })
	protected := r.Group("/")
	protected.Use(AuthMiddleware(apiKey))
	protected.GET("/protected", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func authRequest(r *gin.Engine, xAPIKey, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/protected", nil)
	if xAPIKey != "" {
		req.Header.Set("X-API-Key", xAPIKey)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuthMiddleware_ValidXAPIKey(t *testing.T) {
	const key = "correct-api-key"
	r := newAuthRouter(key)

	w := authRequest(r, key, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid X-API-Key, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidBearerToken(t *testing.T) {
	const key = "correct-api-key"
	r := newAuthRouter(key)

	w := authRequest(r, "", key)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid Bearer token, got %d", w.Code)
	}
}

func TestAuthMiddleware_MissingKey_Returns401(t *testing.T) {
	r := newAuthRouter("correct-api-key")

	w := authRequest(r, "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no key, got %d", w.Code)
	}
}

func TestAuthMiddleware_WrongXAPIKey_Returns401(t *testing.T) {
	r := newAuthRouter("correct-api-key")

	w := authRequest(r, "wrong-key", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong X-API-Key, got %d", w.Code)
	}
}

func TestAuthMiddleware_WrongBearerToken_Returns401(t *testing.T) {
	r := newAuthRouter("correct-api-key")

	w := authRequest(r, "", "wrong-key")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong Bearer token, got %d", w.Code)
	}
}

func TestAuthMiddleware_HealthEndpointBypassesAuth(t *testing.T) {
	// /health is registered outside the auth group — no key needed.
	h := &Handlers{cfg: Config{Network: "mainnet", APIKey: "any-key"}}
	r := newTestRouter(h)

	req := httptest.NewRequest("GET", "/health", nil) // no API key
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /health without API key, got %d", w.Code)
	}
}

func TestAuthMiddleware_UnprotectedRoute_Returns200(t *testing.T) {
	r := newAuthRouter("correct-api-key")

	req := httptest.NewRequest("GET", "/open", nil) // no key, not protected
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for unprotected route, got %d", w.Code)
	}
}

func TestAuthMiddleware_BearerPrefixOnly_Returns401(t *testing.T) {
	r := newAuthRouter("correct-api-key")

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer ") // empty token after prefix
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for empty Bearer token, got %d", w.Code)
	}
}
