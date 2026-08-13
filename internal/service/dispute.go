package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type DisputeService struct{ Store store.Store }

func (s DisputeService) Open(ctx context.Context, paymentID, reason string, amount decimal.Decimal) (domain.Dispute, error) {
	p, err := s.Store.GetPayment(ctx, paymentID)
	if err != nil {
		return domain.Dispute{}, err
	}
	if !amount.GreaterThan(decimal.Zero) || amount.GreaterThan(p.CapturedAmount.Sub(p.RefundedAmount)) {
		return domain.Dispute{}, fmt.Errorf("dispute amount exceeds settled payment amount")
	}
	d, err := domain.NewDispute(paymentID, p.MerchantID, p.Currency, reason, amount)
	if err != nil {
		return d, err
	}
	payload, _ := json.Marshal(d)
	entries := []domain.LedgerEntry{{ID: uuid.NewString(), PaymentID: paymentID, AccountID: "merchant:" + p.MerchantID + ":available", Currency: p.Currency, Debit: amount}, {ID: uuid.NewString(), PaymentID: paymentID, AccountID: "merchant:" + p.MerchantID + ":dispute_frozen", Currency: p.Currency, Credit: amount}}
	return s.Store.CreateDispute(ctx, d, entries, domain.OutboxEvent{ID: uuid.NewString(), AggregateID: d.ID, EventType: "dispute.opened", Payload: string(payload)})
}
func (s DisputeService) Resolve(ctx context.Context, disputeID string, merchantWins bool) (domain.Dispute, error) {
	d, err := s.Store.GetDispute(ctx, disputeID)
	if err != nil {
		return domain.Dispute{}, err
	}
	status := domain.DisputeLost
	eventType := "dispute.lost"
	entries := []domain.LedgerEntry{{ID: uuid.NewString(), PaymentID: d.PaymentID, AccountID: "merchant:" + d.MerchantID + ":dispute_frozen", Currency: d.Currency, Debit: d.Amount}, {ID: uuid.NewString(), PaymentID: d.PaymentID, AccountID: "customer:chargeback", Currency: d.Currency, Credit: d.Amount}}
	if merchantWins {
		status = domain.DisputeWon
		eventType = "dispute.won"
		entries = []domain.LedgerEntry{{ID: uuid.NewString(), PaymentID: d.PaymentID, AccountID: "merchant:" + d.MerchantID + ":dispute_frozen", Currency: d.Currency, Debit: d.Amount}, {ID: uuid.NewString(), PaymentID: d.PaymentID, AccountID: "merchant:" + d.MerchantID + ":available", Currency: d.Currency, Credit: d.Amount}}
	}
	d.Status = status
	payload, _ := json.Marshal(d)
	return s.Store.ResolveDispute(ctx, disputeID, status, entries, domain.OutboxEvent{ID: uuid.NewString(), AggregateID: disputeID, EventType: eventType, Payload: string(payload)})
}
