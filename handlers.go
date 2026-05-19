// Copyright (C) 2025 Dashi Project
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
)

// suiAddressRegex matches a valid Sui address: 0x followed by exactly 64 hex characters.
var suiAddressRegex = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)

// Handlers holds all dependencies shared across HTTP handlers.
type Handlers struct {
	db    *DB
	dashi *DashiClient
	sui   *SuiClient
	cfg   Config
}

// SponsorRequest is the payload for POST /v1/sponsor.
type SponsorRequest struct {
	TransactionKindBytes string `json:"transactionKindBytes" binding:"required"`
	Sender               string `json:"sender" binding:"required"`
}

// ExecuteRequest is the payload for POST /v1/execute.
type ExecuteRequest struct {
	SponsorshipID int64  `json:"sponsorshipId" binding:"required"`
	TxBytes       string `json:"txBytes"       binding:"required"`
	UserSig       string `json:"userSig"       binding:"required"`
}

// Health handles GET /health.
// No authentication required. Used by load balancers and monitoring.
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"network": h.cfg.Network,
		"version": version,
	})
}

// SponsorTransaction handles POST /v1/sponsor.
// Validates the request, reserves gas via sui-gas-pool, builds TransactionData,
// and returns it for the sender to sign. The signed transaction must then be
// submitted to POST /v1/execute.
func (h *Handlers) SponsorTransaction(c *gin.Context) {
	var req SponsorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: transactionKindBytes and sender are required"})
		return
	}

	if !suiAddressRegex.MatchString(req.Sender) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Sui address: must be 0x followed by 64 hex characters"})
		return
	}

	reservation, err := h.dashi.Reserve(c.Request.Context(), req.TransactionKindBytes, req.Sender)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "sponsorship failed: " + err.Error()})
		return
	}

	// Log to Postgres. If logging fails we do not abort — the reservation already
	// happened and the client must receive the response.
	if logErr := h.db.LogSponsorship(c.Request.Context(), &SponsorshipRecord{
		SponsorshipID: fmt.Sprintf("%d", reservation.ReservationID),
		Sender:        req.Sender,
		Status:        "reserved",
		NetworkFee:    3_000_000, // 0.003 SUI in MIST (1 SUI = 1_000_000_000 MIST)
		ServiceFee:    1_000_000, // 0.001 SUI in MIST
	}); logErr != nil {
		_ = c.Error(logErr)
	}

	c.JSON(http.StatusOK, gin.H{
		"sponsoredTransaction": reservation.TxBytes,
		"sponsorshipId":        reservation.ReservationID,
		"feeInfo": gin.H{
			"networkFee": "0.003 SUI",
			"serviceFee": "0.001 SUI",
			"totalFee":   "0.004 SUI",
		},
	})
}

// ExecuteSponsored handles POST /v1/execute.
// Validates the request, marks the sponsorship as submitted, then fires the
// gas-pool call in a background goroutine (Sui finalization can take minutes).
// Returns 202 immediately — poll GET /v1/execute/:id for the final digest.
func (h *Handlers) ExecuteSponsored(c *gin.Context) {
	var req ExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: sponsorshipId, txBytes and userSig are required"})
		return
	}

	idStr := fmt.Sprintf("%d", req.SponsorshipID)
	_ = h.db.UpdateSponsorshipStatus(c.Request.Context(), idStr, "submitted", "")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		digest, err := h.dashi.Execute(ctx, req.SponsorshipID, req.TxBytes, req.UserSig)
		if err != nil {
			_ = h.db.UpdateSponsorshipStatus(context.Background(), idStr, "failed", "")
			return
		}
		_ = h.db.UpdateSponsorshipStatus(context.Background(), idStr, "completed", digest)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"sponsorshipId": req.SponsorshipID,
		"status":        "submitted",
	})
}

// GetExecuteStatus handles GET /v1/execute/:id.
// Polls the DB for the current execution status of a sponsorship.
func (h *Handlers) GetExecuteStatus(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	rec, err := h.db.GetSponsorshipByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if rec == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sponsorship not found"})
		return
	}

	resp := gin.H{
		"sponsorshipId": id,
		"status":        rec.Status,
	}
	if rec.Digest != "" {
		resp["digest"] = rec.Digest
	}
	c.JSON(http.StatusOK, resp)
}

// GetSponsorStatus handles GET /v1/sponsor/:digest.
// Queries the Sui RPC for the current transaction status.
func (h *Handlers) GetSponsorStatus(c *gin.Context) {
	digest := c.Param("digest")
	if digest == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "digest is required"})
		return
	}

	status, err := h.sui.GetTransactionStatus(c.Request.Context(), digest)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query transaction status: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"digest": digest,
		"status": status,
	})
}

// GetBalance handles GET /v1/balance.
// Queries the Sui RPC for the sponsor wallet balance.
func (h *Handlers) GetBalance(c *gin.Context) {
	balance, err := h.sui.GetBalance(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to retrieve balance"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"balance":  balance,
		"currency": "SUI",
		"network":  h.cfg.Network,
	})
}
