// Copyright (C) 2025 Dashi Project
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIError is the standard error envelope returned by every non-2xx response.
type APIError struct {
	Error string `json:"error"`
	Hint  string `json:"hint"`
}

// Sentinel errors — one definition, used consistently across all handlers.
var (
	ErrInvalidBody = APIError{
		Error: "Invalid request body",
		Hint:  "transactionKindBytes and sender are required",
	}
	ErrInvalidAddress = APIError{
		Error: "Invalid Sui address format",
		Hint:  "Address must start with 0x followed by 64 hex characters",
	}
	ErrGasPoolUnavailable = APIError{
		Error: "Gas pool is unavailable",
		Hint:  "Check if sui-gas-pool container is running: docker compose ps",
	}
	ErrInsufficientBalance = APIError{
		Error: "Insufficient gas fund balance",
		Hint:  "Deposit more SUI to your sponsor wallet and reload the gas pool",
	}
	ErrExecuteInvalidBody = APIError{
		Error: "Invalid request body",
		Hint:  "sponsorshipId, txBytes and userSig are required",
	}
	ErrSponsorshipNotFound = APIError{
		Error: "Sponsorship not found",
		Hint:  "Check the sponsorship ID returned from POST /v1/sponsor",
	}
	ErrDatabase = APIError{
		Error: "Database error",
		Hint:  "Check docker compose logs api for details",
	}
	ErrSuiRPCUnavailable = APIError{
		Error: "Sui RPC is unavailable",
		Hint:  "Check SUI_RPC_URL in your .env file or try a different RPC provider",
	}
	ErrBalanceUnavailable = APIError{
		Error: "Failed to retrieve balance",
		Hint:  "Check SUI_RPC_URL in your .env file or try a different RPC provider",
	}
	ErrAPIKeyRequired = APIError{
		Error: "API key required",
		Hint:  "Add header: X-API-Key: your-api-key",
	}
	ErrInvalidAPIKey = APIError{
		Error: "Invalid API key",
		Hint:  "Check API_KEY in your .env file",
	}
	ErrUnexpected = APIError{
		Error: "An unexpected error occurred",
		Hint:  "Check docker compose logs api for details",
	}
)

// respondError writes a structured JSON error response and logs it.
// detail is optional — when non-empty it is attached to the log entry only (not sent to the client).
func respondError(c *gin.Context, status int, apiErr APIError, detail ...string) {
	args := []any{
		"status", status,
		"error", apiErr.Error,
		"path", c.Request.URL.Path,
		"method", c.Request.Method,
	}
	if len(detail) > 0 && detail[0] != "" {
		args = append(args, "detail", detail[0])
	}

	if status >= http.StatusInternalServerError {
		slog.Error("api error", args...)
	} else {
		slog.Warn("api error", args...)
	}

	c.JSON(status, apiErr)
}
