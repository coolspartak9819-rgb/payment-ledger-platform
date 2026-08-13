package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/httpapi"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/service"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	var persistence store.Store = store.NewMemory()
	var pool *pgxpool.Pool
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		var err error
		pool, err = pgxpool.New(context.Background(), databaseURL)
		if err != nil {
			log.Printf("database initialization failed: %v", err)
			os.Exit(1)
		}
		defer pool.Close()
		persistence = store.NewPostgres(pool)
	}
	payments := &service.PaymentService{Store: persistence, Provider: service.DeterministicProvider{}, Risk: service.ThresholdRisk{MaxAmount: "10000"}}
	log.Printf("payment ledger API started on port %s", port)
	metrics := service.NewCounterMetrics()
	limiter := httpapi.NewMemoryRateLimiter(120, time.Minute)
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		redisLimiter, err := httpapi.NewRedisRateLimiter(redisURL, 120, time.Minute)
		if err != nil {
			log.Printf("Redis limiter unavailable, using memory: %v", err)
		} else {
			limiter = redisLimiter
			defer func() { _ = redisLimiter.Close() }()
		}
	}
	config := httpapi.Config{WebhookSecret: os.Getenv("PROVIDER_WEBHOOK_SECRET"), MerchantAPIKeys: os.Getenv("MERCHANT_API_KEYS"), AdminAPIKey: os.Getenv("ADMIN_API_KEY"), Limiter: limiter, Metrics: metrics}
	if err := http.ListenAndServe(":"+port, httpapi.New(payments, config).Routes()); err != nil {
		log.Printf("API stopped: %v", err)
		os.Exit(1)
	}
}
