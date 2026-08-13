package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/google/uuid"
)

type Provider interface {
	Authorize(context.Context, domain.Payment) (string, error)
}
type RiskEngine interface {
	Check(context.Context, domain.Payment) (bool, error)
}
type Metrics interface{ Inc(string) }

type PaymentService struct {
	Store    store.Store
	Provider Provider
	Risk     RiskEngine
	Metrics  Metrics
}

func (s *PaymentService) Create(ctx context.Context, in domain.CreatePayment, key string) (domain.Payment, bool, error) {
	if key == "" {
		return domain.Payment{}, false, fmt.Errorf("x-idempotency-key is required")
	}
	p, err := domain.NewPayment(in)
	if err != nil {
		return domain.Payment{}, false, err
	}
	p, replay, err := s.Store.CreatePayment(ctx, p, key)
	if replay || err != nil {
		return p, replay, err
	}
	if s.Metrics != nil {
		s.Metrics.Inc("payments_created")
	}
	return p, false, nil
}
func (s *PaymentService) Authorize(ctx context.Context, id string) (domain.Payment, error) {
	p, err := s.Store.GetPayment(ctx, id)
	if err != nil {
		return p, err
	}
	if s.Risk != nil {
		allowed, riskErr := s.Risk.Check(ctx, p)
		if riskErr != nil {
			return p, riskErr
		}
		if !allowed {
			_, _ = s.Store.Transition(ctx, id, domain.PaymentFailed, "risk_blocked")
			return domain.Payment{}, fmt.Errorf("payment blocked by risk policy")
		}
	}
	reference, err := s.Provider.Authorize(ctx, p)
	if err != nil {
		_, _ = s.Store.Transition(ctx, id, domain.PaymentFailed, "provider_error")
		return p, err
	}
	updated, err := s.Store.Transition(ctx, id, domain.PaymentAuthorized, reference)
	if err != nil {
		return p, err
	}
	amount := p.Amount
	entries := []domain.LedgerEntry{{ID: uuid.NewString(), PaymentID: id, AccountID: "customer:pending", Currency: p.Currency, Debit: amount}, {ID: uuid.NewString(), PaymentID: id, AccountID: "merchant:pending", Currency: p.Currency, Credit: amount}}
	if err := s.Store.AppendLedger(ctx, entries); err != nil {
		return p, err
	}
	payload, _ := json.Marshal(updated)
	_ = s.Store.Enqueue(ctx, domain.OutboxEvent{ID: uuid.NewString(), AggregateID: id, EventType: "payment.authorized", Payload: string(payload)})
	return updated, nil
}
