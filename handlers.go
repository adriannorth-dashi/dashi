package main

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
)

// suiAddressRegex matches a valid Sui address: 0x followed by exactly 64 hex characters.
var suiAddressRegex = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)

// Handlers holds all dependencies shared across HTTP handlers.
type Handlers struct {
	db      *DB
	dashi *DashiClient
	sui     *SuiClient
	cfg     Config
}

// SponsorRequest is the payload for POST /v1/sponsor.
type SponsorRequest struct {
	TransactionKindBytes string `json:"transactionKindBytes" binding:"required"`
	Sender               string `json:"sender" binding:"required"`
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
// Validates the request, forwards to the gas backend, logs to Postgres, and returns
// the sponsored transaction bytes along with fee information.
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

	result, err := h.dashi.SponsorTransaction(c.Request.Context(), req.TransactionKindBytes, req.Sender)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "sponsorship failed: " + err.Error()})
		return
	}

	// Log to Postgres. If logging fails we do not abort — the sponsorship already
	// happened on-chain and the client must receive the response.
	if logErr := h.db.LogSponsorship(c.Request.Context(), &SponsorshipRecord{
		SponsorshipID: result.SponsorshipID,
		Sender:        req.Sender,
		Status:        "pending",
		NetworkFee:    3_000_000, // 0.003 SUI in MIST (1 SUI = 1_000_000_000 MIST)
		ServiceFee:    1_000_000, // 0.001 SUI in MIST
	}); logErr != nil {
		_ = c.Error(logErr)
	}

	c.JSON(http.StatusOK, gin.H{
		"sponsoredTransaction": result.SponsoredTransaction,
		"sponsorshipId":        result.SponsorshipID,
		"feeInfo": gin.H{
			"networkFee": "0.003 SUI",
			"serviceFee": "0.001 SUI",
			"totalFee":   "0.004 SUI",
		},
	})
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
// Phase 1: returns a placeholder value.
// Phase 2: will query sui-gas-pool for the live fund balance.
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
