package main

import (
	"context"
	"log"
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

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port             string
	Network          string
	RPCURL           string
	GasPoolURL       string // Phase 2: sui-gas-pool base URL
	GasPoolAuthToken string // Phase 2: GAS_STATION_AUTH bearer token
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
	v1.GET("/sponsor/:digest", h.GetSponsorStatus)
	v1.GET("/balance", h.GetBalance)
	return r
}

func loadConfig() Config {
	_ = godotenv.Load()
	return Config{
		Port:             getEnv("PORT", "8080"),
		Network:          getEnv("SUI_NETWORK", "testnet"),
		RPCURL:           getEnv("SUI_RPC_URL", "https://fullnode.testnet.sui.io:443"),
		GasPoolURL:       getEnv("GASPOOL_URL", "http://127.0.0.1:9527"),
		GasPoolAuthToken: getEnv("GASPOOL_AUTH_TOKEN", ""),
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

	if cfg.APIKey == "" {
		log.Fatal("API_KEY must be set")
	}

	db, err := NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		log.Fatalf("database migration failed: %v", err)
	}

	h := &Handlers{
		db:      db,
		shinami: NewShinamiClient(cfg.GasPoolURL, cfg.GasPoolAuthToken),
		sui:     NewSuiClient(cfg.RPCURL),
		cfg:     cfg,
	}

	router := newRouter(h)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Dashi %s starting on :%s (network: %s)", version, cfg.Port, cfg.Network)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("server stopped")
}
