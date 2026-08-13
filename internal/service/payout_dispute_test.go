package service_test

import (
	"context"
	"testing"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/service"
	"github.com/shopspring/decimal"
)

func capturedPayment(t *testing.T, amount string) (*service.PaymentService, domain.Payment) {
	t.Helper()
	payments := newService("1000")
	payment, _, err := payments.Create(context.Background(), input(amount), "lifecycle-"+amount)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = payments.Authorize(context.Background(), payment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = payments.Capture(context.Background(), payment.ID, decimal.RequireFromString(amount)); err != nil {
		t.Fatal(err)
	}
	return payments, payment
}

func TestFailedPayoutReturnsReservedBalance(t *testing.T) {
	payments, _ := capturedPayment(t, "80")
	payouts := service.PayoutService{Store: payments.Store}
	payout, err := payouts.Create(context.Background(), "merchant-1", "USD", decimal.NewFromInt(50))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = payouts.Complete(context.Background(), payout.ID, "provider-failed", false); err != nil {
		t.Fatal(err)
	}
	balance, err := payments.Store.AvailableBalance(context.Background(), "merchant-1", "USD")
	if err != nil || !balance.Equal(decimal.NewFromInt(80)) {
		t.Fatalf("balance=%s err=%v", balance, err)
	}
}

func TestPaidPayoutDoesNotReturnReservedBalance(t *testing.T) {
	payments, _ := capturedPayment(t, "80")
	payout, err := (service.PayoutService{Store: payments.Store}).Create(context.Background(), "merchant-1", "USD", decimal.NewFromInt(50))
	if err != nil {
		t.Fatal(err)
	}
	completed, err := (service.PayoutService{Store: payments.Store}).Complete(context.Background(), payout.ID, "provider-paid", true)
	if err != nil || completed.Status != domain.PayoutPaid {
		t.Fatalf("payout=%v err=%v", completed, err)
	}
	balance, _ := payments.Store.AvailableBalance(context.Background(), "merchant-1", "USD")
	if !balance.Equal(decimal.NewFromInt(30)) {
		t.Fatalf("balance=%s", balance)
	}
}

func TestDisputeFreezeAndResolution(t *testing.T) {
	payments, payment := capturedPayment(t, "100")
	disputes := service.DisputeService{Store: payments.Store}
	dispute, err := disputes.Open(context.Background(), payment.ID, "fraud", decimal.NewFromInt(40))
	if err != nil {
		t.Fatal(err)
	}
	balance, _ := payments.Store.AvailableBalance(context.Background(), "merchant-1", "USD")
	if !balance.Equal(decimal.NewFromInt(60)) {
		t.Fatalf("frozen balance=%s", balance)
	}
	won, err := disputes.Resolve(context.Background(), dispute.ID, true)
	if err != nil || won.Status != domain.DisputeWon {
		t.Fatalf("resolution=%v err=%v", won, err)
	}
	balance, _ = payments.Store.AvailableBalance(context.Background(), "merchant-1", "USD")
	if !balance.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("released balance=%s", balance)
	}
}
