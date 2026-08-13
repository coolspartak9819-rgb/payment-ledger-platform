package service_test

import (
	"context"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/service"
	"github.com/shopspring/decimal"
	"testing"
)

func TestReconciliationFindsUnknownAndMismatchedSettlements(t *testing.T) {
	payments := newService("1000")
	p, _, err := payments.Create(context.Background(), input("100"), "settlement-order")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = payments.Authorize(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = payments.Capture(context.Background(), p.ID, decimal.NewFromInt(80)); err != nil {
		t.Fatal(err)
	}
	mismatches, err := service.ReconciliationService{Store: payments.Store}.Reconcile(context.Background(), []service.SettlementLine{{ProviderReference: "demo_" + p.ID[:8], CapturedAmount: decimal.NewFromInt(70)}, {ProviderReference: "provider-unknown", CapturedAmount: decimal.NewFromInt(5)}})
	if err != nil || len(mismatches) != 2 {
		t.Fatalf("mismatches=%v err=%v", mismatches, err)
	}
}
