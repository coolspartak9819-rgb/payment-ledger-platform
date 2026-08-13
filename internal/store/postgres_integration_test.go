package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

func TestPostgresStoreIdempotencyAndOutbox(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx := context.Background()
	_, err = pool.Exec(ctx, `TRUNCATE outbox_events, ledger_entries, idempotency_keys, payments CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
	s := store.NewPostgres(pool)
	p := domain.Payment{ID: uuid.NewString(), MerchantID: "integration", CustomerID: "customer", Currency: "USD", Amount: decimal.NewFromInt(42), Status: domain.PaymentCreated}
	created, replay, err := s.CreatePayment(ctx, p, "integration-key")
	if err != nil || replay {
		t.Fatalf("create replay=%v err=%v", replay, err)
	}
	again, replay, err := s.CreatePayment(ctx, domain.Payment{ID: uuid.NewString(), MerchantID: "integration", CustomerID: "customer", Currency: "USD", Amount: decimal.NewFromInt(42), Status: domain.PaymentCreated}, "integration-key")
	if err != nil || !replay || created.ID != again.ID {
		t.Fatalf("idempotency failed: %s %s replay=%v err=%v", created.ID, again.ID, replay, err)
	}
	entries := []domain.LedgerEntry{{ID: uuid.NewString(), PaymentID: created.ID, AccountID: "customer", Currency: "USD", Debit: decimal.NewFromInt(42)}, {ID: uuid.NewString(), PaymentID: created.ID, AccountID: "merchant", Currency: "USD", Credit: decimal.NewFromInt(42)}}
	if err = s.AppendLedger(ctx, entries); err != nil {
		t.Fatal(err)
	}
	event := domain.OutboxEvent{ID: uuid.NewString(), AggregateID: created.ID, EventType: "payment.authorized", Payload: `{"id":"` + created.ID + `"}`}
	if err = s.Enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimOutbox(ctx, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	if err = s.MarkPublished(ctx, event.ID); err != nil {
		t.Fatal(err)
	}
}
