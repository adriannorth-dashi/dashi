// Copyright (C) 2025 Dashi Project
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // embed tz database so the binary works without system tzdata

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

const version = "1.0.0"

// initLogger configures the global slog logger.
// LOG_LEVEL env var accepts: debug, info, warn, error (case-insensitive). Default: info.
// Output is always JSON so log aggregators (Loki, CloudWatch, etc.) can parse it.
func initLogger() {
	levelStr := getEnv("LOG_LEVEL", "info")
	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port             string
	Network          string
	RPCURL           string
	GasPoolURL       string // Phase 2: sui-gas-pool base URL
	GasPoolAuthToken string // Phase 2: GAS_STATION_AUTH bearer token
	SponsorAddress   string // sponsor wallet address for balance queries
	APIKey           string
	DatabaseURL      string
	RedisURL         string
}

// newRouter constructs the Gin router with all routes and middleware wired up.
// Extracted from main() so tests can call it directly and cover the routing logic.
func newRouter(h *Handlers) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.GET("/health", h.Health)
	v1 := r.Group("/v1")
	v1.Use(AuthMiddleware(h.cfg.APIKey))
	v1.POST("/sponsor", h.SponsorTransaction)
	v1.POST("/execute", h.ExecuteSponsored)
	v1.GET("/execute/:id", h.GetExecuteStatus)
	v1.GET("/sponsor/:digest", h.GetSponsorStatus)
	v1.GET("/balance", h.GetBalance)
	return r
}

func loadConfig() Config {
	_ = godotenv.Load()
	return Config{
		Port:             getEnv("PORT", "8080"),
		Network:          getEnv("SUI_NETWORK", "mainnet"),
		RPCURL:           getEnv("SUI_RPC_URL", "https://fullnode.mainnet.sui.io:443"),
		GasPoolURL:       getEnv("GASPOOL_URL", "http://127.0.0.1:9527"),
		GasPoolAuthToken: getEnv("GASPOOL_AUTH_TOKEN", ""),
		SponsorAddress:   getEnv("SPONSOR_ADDRESS", ""),
		APIKey:           getEnv("API_KEY", ""),
		DatabaseURL:      getEnv("DATABASE_URL", ""),
		RedisURL:         getEnv("REDIS_URL", "redis://redis:6379"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	cfg := loadConfig()
	initLogger()

	if cfg.APIKey == "" {
		slog.Error("API_KEY must be set")
		os.Exit(1)
	}

	db, err := NewDB(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		slog.Error("database migration failed", "err", err)
		os.Exit(1)
	}

	h := &Handlers{
		db:    db,
		dashi: NewDashiClient(cfg.GasPoolURL, cfg.GasPoolAuthToken, cfg.RPCURL),
		sui:   NewSuiClient(cfg.RPCURL, cfg.SponsorAddress),
		cfg:   cfg,
	}

	router := newRouter(h)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("dashi starting", "version", version, "port", cfg.Port, "network", cfg.Network)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down gracefully")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("forced shutdown", "err", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
