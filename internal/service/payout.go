package service

import (
	"context"
	"encoding/json"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PayoutService struct{ Store store.Store }

func (s PayoutService) Create(ctx context.Context, merchant, currency string, amount decimal.Decimal) (domain.Payout, error) {
	p, err := domain.NewPayout(merchant, currency, amount)
	if err != nil {
		return p, err
	}
	payload, _ := json.Marshal(p)
	entries := []domain.LedgerEntry{{ID: uuid.NewString(), AccountID: "merchant:" + merchant + ":available", Currency: p.Currency, Debit: amount}, {ID: uuid.NewString(), AccountID: "merchant:" + merchant + ":payout_pending", Currency: p.Currency, Credit: amount}}
	return s.Store.CreatePayout(ctx, p, entries, domain.OutboxEvent{ID: uuid.NewString(), AggregateID: p.ID, EventType: "payout.created", Payload: string(payload)})
}
func (s PayoutService) Complete(ctx context.Context, payoutID, reference string, success bool) (domain.Payout, error) {
	// The store owns the read-modify-write transaction; provider callbacks only choose the terminal branch.
	status := domain.PayoutFailed
	eventType := "payout.failed"
	entries := []domain.LedgerEntry{}
	// Read payout details through the terminal command is deliberately avoided in this small port; lookup is provider-owned in production.
	payout, err := s.Store.GetPayout(ctx, payoutID)
	if err != nil {
		return domain.Payout{}, err
	}
	if success {
		status = domain.PayoutPaid
		eventType = "payout.paid"
		entries = []domain.LedgerEntry{{ID: uuid.NewString(), AccountID: "merchant:" + payout.MerchantID + ":payout_pending", Currency: payout.Currency, Debit: payout.Amount}, {ID: uuid.NewString(), AccountID: "provider:payout_settled", Currency: payout.Currency, Credit: payout.Amount}}
	} else {
		entries = []domain.LedgerEntry{{ID: uuid.NewString(), AccountID: "merchant:" + payout.MerchantID + ":payout_pending", Currency: payout.Currency, Debit: payout.Amount}, {ID: uuid.NewString(), AccountID: "merchant:" + payout.MerchantID + ":available", Currency: payout.Currency, Credit: payout.Amount}}
	}
	payout.Status = status
	payout.ProviderReference = reference
	payload, _ := json.Marshal(payout)
	return s.Store.TransitionPayout(ctx, payoutID, status, reference, entries, domain.OutboxEvent{ID: uuid.NewString(), AggregateID: payoutID, EventType: eventType, Payload: string(payload)})
}
