// Copyright (C) 2025 Dashi Project
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
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
	start := time.Now()
	var req SponsorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrInvalidBody, err.Error())
		return
	}

	if !suiAddressRegex.MatchString(req.Sender) {
		respondError(c, http.StatusBadRequest, ErrInvalidAddress)
		return
	}

	reservation, err := h.dashi.Reserve(c.Request.Context(), req.TransactionKindBytes, req.Sender)
	if err != nil {
		respondError(c, http.StatusBadGateway, ErrGasPoolUnavailable, err.Error())
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
		slog.Error("failed to log sponsorship", "sponsorship_id", reservation.ReservationID, "err", logErr)
		_ = c.Error(logErr)
	}

	slog.Info("sponsorship reserved",
		"sponsorship_id", reservation.ReservationID,
		"sender", req.Sender,
		"duration_ms", time.Since(start).Milliseconds(),
	)

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
		respondError(c, http.StatusBadRequest, ErrExecuteInvalidBody, err.Error())
		return
	}

	idStr := fmt.Sprintf("%d", req.SponsorshipID)
	_ = h.db.UpdateSponsorshipStatus(c.Request.Context(), idStr, "submitted", "")

	slog.Info("execute submitted", "sponsorship_id", req.SponsorshipID)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		digest, err := h.dashi.Execute(ctx, req.SponsorshipID, req.TxBytes, req.UserSig)
		if err != nil {
			slog.Error("execute failed", "sponsorship_id", req.SponsorshipID, "err", err)
			_ = h.db.UpdateSponsorshipStatus(context.Background(), idStr, "failed", "")
			return
		}
		slog.Info("execute completed", "sponsorship_id", req.SponsorshipID, "digest", digest)
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
		respondError(c, http.StatusBadRequest, APIError{Error: "id is required", Hint: "Provide the sponsorship ID in the URL path"})
		return
	}

	rec, err := h.db.GetSponsorshipByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, ErrDatabase, err.Error())
		return
	}
	if rec == nil {
		respondError(c, http.StatusNotFound, ErrSponsorshipNotFound)
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
		respondError(c, http.StatusBadRequest, APIError{Error: "digest is required", Hint: "Provide the transaction digest in the URL path"})
		return
	}

	status, err := h.sui.GetTransactionStatus(c.Request.Context(), digest)
	if err != nil {
		respondError(c, http.StatusBadGateway, ErrSuiRPCUnavailable, err.Error())
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
		respondError(c, http.StatusServiceUnavailable, ErrBalanceUnavailable, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"balance":  balance,
		"currency": "SUI",
		"network":  h.cfg.Network,
	})
}
