package service_test

import (
	"context"
	"testing"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/service"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/shopspring/decimal"
)

func newService(max string) *service.PaymentService {
	return &service.PaymentService{Store: store.NewMemory(), Provider: service.DeterministicProvider{}, Risk: service.ThresholdRisk{MaxAmount: max}}
}
func input(amount string) domain.CreatePayment {
	return domain.CreatePayment{MerchantID: "merchant-1", CustomerID: "customer-1", Currency: "usd", Amount: decimal.RequireFromString(amount)}
}

func TestCreateIsIdempotent(t *testing.T) {
	s := newService("1000")
	first, replay, err := s.Create(context.Background(), input("24.50"), "order-1")
	if err != nil || replay {
		t.Fatalf("first create: replay=%v err=%v", replay, err)
	}
	second, replay, err := s.Create(context.Background(), input("24.50"), "order-1")
	if err != nil || !replay || first.ID != second.ID {
		t.Fatalf("idempotency failed: replay=%v first=%s second=%s err=%v", replay, first.ID, second.ID, err)
	}
}
func TestAuthorizeWritesBalancedLedgerAndOutbox(t *testing.T) {
	s := newService("1000")
	p, _, err := s.Create(context.Background(), input("24.50"), "order-2")
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Authorize(context.Background(), p.ID)
	if err != nil || result.Status != domain.PaymentAuthorized {
		t.Fatalf("authorize: status=%s err=%v", result.Status, err)
	}
	events, err := s.Store.ClaimOutbox(context.Background(), 10)
	if err != nil || len(events) != 1 || events[0].EventType != "payment.authorized" {
		t.Fatalf("outbox: events=%v err=%v", events, err)
	}
}
func TestRiskBlockMarksPaymentAsFailed(t *testing.T) {
	s := newService("10")
	p, _, err := s.Create(context.Background(), input("24.50"), "order-3")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Authorize(context.Background(), p.ID)
	if err == nil {
		t.Fatal("expected risk rejection")
	}
	saved, err := s.Store.GetPayment(context.Background(), p.ID)
	if err != nil || saved.Status != domain.PaymentFailed {
		t.Fatalf("payment should fail: status=%s err=%v", saved.Status, err)
	}
}
func TestLedgerRejectsUnbalancedTransaction(t *testing.T) {
	err := domain.ValidateBalanced([]domain.LedgerEntry{{Debit: decimal.NewFromInt(10)}, {Credit: decimal.NewFromInt(9)}})
	if err == nil {
		t.Fatal("expected unbalanced ledger error")
	}
}
func TestCaptureAndRefundCompletePaymentLifecycle(t *testing.T) {
	s := newService("1000")
	p, _, err := s.Create(context.Background(), input("24.50"), "order-4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	captured, err := s.Capture(context.Background(), p.ID, decimal.RequireFromString("24.50"))
	if err != nil || captured.Status != domain.PaymentCaptured {
		t.Fatalf("capture status=%s err=%v", captured.Status, err)
	}
	refunded, err := s.Refund(context.Background(), p.ID, decimal.RequireFromString("24.50"))
	if err != nil || refunded.Status != domain.PaymentRefunded {
		t.Fatalf("refund status=%s err=%v", refunded.Status, err)
	}
	events, err := s.Store.ClaimOutbox(context.Background(), 10)
	if err != nil || len(events) != 3 {
		t.Fatalf("expected 3 lifecycle events, got %d, error=%v", len(events), err)
	}
}
func TestPartialCaptureAndRefundRejectOverpayment(t *testing.T) {
	s := newService("1000")
	p, _, err := s.Create(context.Background(), input("100"), "order-5")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	captured, err := s.Capture(context.Background(), p.ID, decimal.NewFromInt(60))
	if err != nil || !captured.CapturedAmount.Equal(decimal.NewFromInt(60)) {
		t.Fatalf("capture=%s err=%v", captured.CapturedAmount, err)
	}
	refunded, err := s.Refund(context.Background(), p.ID, decimal.NewFromInt(20))
	if err != nil || !refunded.RefundedAmount.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("refund=%s err=%v", refunded.RefundedAmount, err)
	}
	if _, err = s.Refund(context.Background(), p.ID, decimal.NewFromInt(50)); err == nil {
		t.Fatal("expected refund limit rejection")
	}
	if _, err = s.Capture(context.Background(), p.ID, decimal.NewFromInt(50)); err == nil {
		t.Fatal("expected capture limit rejection")
	}
}
func TestMultiplePartialRefundsAreAllowedWithinCapturedLimit(t *testing.T) {
	s := newService("1000")
	p, _, err := s.Create(context.Background(), input("100"), "order-6")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Capture(context.Background(), p.ID, decimal.NewFromInt(100)); err != nil {
		t.Fatal(err)
	}
	refunded, err := s.Refund(context.Background(), p.ID, decimal.NewFromInt(30))
	if err != nil {
		t.Fatal(err)
	}
	refunded, err = s.Refund(context.Background(), p.ID, decimal.NewFromInt(40))
	if err != nil || !refunded.RefundedAmount.Equal(decimal.NewFromInt(70)) {
		t.Fatalf("refunded=%s err=%v", refunded.RefundedAmount, err)
	}
}
