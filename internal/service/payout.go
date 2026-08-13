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
