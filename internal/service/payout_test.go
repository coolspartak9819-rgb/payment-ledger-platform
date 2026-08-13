package service_test

import (
	"context"
	"testing"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/service"
	"github.com/shopspring/decimal"
)

func TestPayoutUsesAvailableBalance(t *testing.T) {
	payments := newService("1000")
	payment, _, err := payments.Create(context.Background(), input("100"), "payout")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = payments.Authorize(context.Background(), payment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = payments.Capture(context.Background(), payment.ID, decimal.NewFromInt(80)); err != nil {
		t.Fatal(err)
	}
	payouts := service.PayoutService{Store: payments.Store}
	payout, err := payouts.Create(context.Background(), "merchant-1", "USD", decimal.NewFromInt(50))
	if err != nil || !payout.Amount.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("payout=%v err=%v", payout, err)
	}
	if _, err = payouts.Create(context.Background(), "merchant-1", "USD", decimal.NewFromInt(40)); err == nil {
		t.Fatal("expected insufficient balance")
	}
}
