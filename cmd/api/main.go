package main

import (
	"context"
	"log"
	"net/http"
	"os"

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
	if err := http.ListenAndServe(":"+port, httpapi.New(payments, os.Getenv("PROVIDER_WEBHOOK_SECRET")).Routes()); err != nil {
		log.Printf("API stopped: %v", err)
		os.Exit(1)
	}
}
