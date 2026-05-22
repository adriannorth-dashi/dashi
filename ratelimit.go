// Copyright (C) 2025 Dashi Project
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimiter holds a Redis client and applies fixed-window rate limits.
// All counters are stored in Redis, so limits hold across multiple API replicas.
type RateLimiter struct {
	rdb *redis.Client
}

// NewRateLimiter connects to Redis and returns a ready RateLimiter.
// redisURL must be in the form "redis://host:port" or "redis://:password@host:port/db".
func NewRateLimiter(redisURL string) (*RateLimiter, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RateLimiter{rdb: rdb}, nil
}

// Close releases the underlying Redis connection.
func (rl *RateLimiter) Close() error {
	return rl.rdb.Close()
}

// allow checks a fixed-window counter for key and increments it atomically.
// The window is aligned to the current UTC minute.
// Returns (allowed, currentCount, error).
// On Redis errors the limiter fails open (request is allowed).
func (rl *RateLimiter) allow(ctx context.Context, key string, limit int) (bool, int64, error) {
	// Window key changes every minute — e.g. "rl:global:28562130"
	windowKey := fmt.Sprintf("rl:%s:%d", key, time.Now().UTC().Unix()/60)

	pipe := rl.rdb.Pipeline()
	incr := pipe.Incr(ctx, windowKey)
	pipe.Expire(ctx, windowKey, 2*time.Minute) // keep for two windows so the previous window is visible

	if _, err := pipe.Exec(ctx); err != nil {
		return true, 0, fmt.Errorf("redis pipeline: %w", err)
	}

	count := incr.Val()
	return count <= int64(limit), count, nil
}

// apiKeyHash returns a 16-char hex prefix of SHA-256(key).
// Stored in Redis as the per-key identifier — never the raw key.
func apiKeyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", sum[:8]) // 8 bytes → 16 hex chars
}

// extractAPIKey reads the API key from either X-API-Key header or Authorization: Bearer.
func extractAPIKey(c *gin.Context) string {
	if key := c.GetHeader("X-API-Key"); key != "" {
		return key
	}
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// setRateLimitHeaders writes X-RateLimit-* response headers.
func setRateLimitHeaders(c *gin.Context, limit int, count int64) {
	remaining := int64(limit) - count
	if remaining < 0 {
		remaining = 0
	}
	c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
	c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(nextMinute(), 10))
}

// nextMinute returns the Unix timestamp of the start of the next UTC minute.
func nextMinute() int64 {
	now := time.Now().UTC()
	return now.Truncate(time.Minute).Add(time.Minute).Unix()
}

// Global returns a middleware that enforces a shared request cap across all callers.
// Applied before authentication so it protects the server from unauthenticated floods.
func (rl *RateLimiter) Global(limitPerMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowed, _, err := rl.allow(c.Request.Context(), "global", limitPerMinute)
		if err != nil {
			slog.Warn("rate limiter: redis error (global)", "err", err)
			c.Next() // fail open on Redis errors
			return
		}
		if !allowed {
			slog.Warn("rate limit exceeded: global",
				"path", c.Request.URL.Path,
				"limit", limitPerMinute,
			)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, APIError{
				Error: "Rate limit exceeded",
				Hint:  "Service is temporarily overloaded, please retry in a moment",
			})
			return
		}
		c.Next()
	}
}

// PerKey returns a middleware that enforces a per-API-key request cap.
// The key is hashed with SHA-256 before being stored in Redis.
// Requests without an API key bypass this check (AuthMiddleware handles rejection).
func (rl *RateLimiter) PerKey(limitPerMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := extractAPIKey(c)
		if key == "" {
			c.Next()
			return
		}

		hash := apiKeyHash(key)
		allowed, count, err := rl.allow(c.Request.Context(), "key:"+hash, limitPerMinute)
		if err != nil {
			slog.Warn("rate limiter: redis error (per-key)", "err", err)
			c.Next() // fail open on Redis errors
			return
		}

		setRateLimitHeaders(c, limitPerMinute, count)

		if !allowed {
			slog.Warn("rate limit exceeded: per-key",
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
				"limit", limitPerMinute,
			)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, APIError{
				Error: "Rate limit exceeded",
				Hint:  fmt.Sprintf("Maximum %d requests per minute per API key", limitPerMinute),
			})
			return
		}
		c.Next()
	}
}

// perKeyOrNoop returns rl.PerKey(limit) when rl is non-nil, or a no-op middleware otherwise.
// This lets newRouter always inline per-route middleware regardless of whether
// rate limiting is enabled — keeping the routing code free of nil checks.
func perKeyOrNoop(rl *RateLimiter, limitPerMinute int) gin.HandlerFunc {
	if rl == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return rl.PerKey(limitPerMinute)
}
