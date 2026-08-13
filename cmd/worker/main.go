package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/service"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	publisher := service.EventPublisher(service.LogPublisher{})
	if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
		natsPublisher, closeNATS, err := service.NewNATSPublisher(natsURL)
		if err != nil {
			log.Fatalf("NATS initialization failed: %v", err)
		}
		defer func() { _ = closeNATS() }()
		publisher = natsPublisher
	}
	worker := service.OutboxWorker{Store: store.NewPostgres(pool), Publisher: publisher}
	for {
		if err := worker.Deliver(context.Background(), 100); err != nil {
			log.Printf("outbox delivery failed: %v", err)
		}
		time.Sleep(time.Second)
	}
}
