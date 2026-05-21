// Copyright (C) 2025 Dashi Project
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware validates the API key from X-API-Key header or Authorization: Bearer <key>.
// Phase 2: replace static key comparison with a Postgres lookup (customers table)
// and add per-key rate limiting via Redis.
func AuthMiddleware(apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")

		if key == "" {
			if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				key = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if key == "" {
			slog.Warn("auth rejected: no api key", "path", c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrAPIKeyRequired)
			return
		}

		if key != apiKey {
			slog.Warn("auth rejected: invalid api key", "path", c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrInvalidAPIKey)
			return
		}

		c.Next()
	}
}
