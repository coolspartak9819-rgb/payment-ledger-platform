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

type Provider interface {
	Authorize(context.Context, domain.Payment) (string, error)
	Capture(context.Context, domain.Payment, decimal.Decimal) error
	Refund(context.Context, domain.Payment, decimal.Decimal) error
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
	return s.enqueue(ctx, updated, "payment.authorized")
}

func (s *PaymentService) Capture(ctx context.Context, id string, amount decimal.Decimal) (domain.Payment, error) {
	p, err := s.Store.GetPayment(ctx, id)
	if err != nil {
		return p, err
	}
	if err = p.ValidateCapture(amount); err != nil {
		return p, err
	}
	if err = s.Provider.Capture(ctx, p, amount); err != nil {
		return p, err
	}
	updated, err := s.Store.ApplyAmount(ctx, id, domain.PaymentCaptured, amount)
	if err != nil {
		return p, err
	}
	entries := []domain.LedgerEntry{{ID: uuid.NewString(), PaymentID: id, AccountID: "merchant:pending", Currency: p.Currency, Debit: amount}, {ID: uuid.NewString(), PaymentID: id, AccountID: "merchant:available", Currency: p.Currency, Credit: amount}}
	if err = s.Store.AppendLedger(ctx, entries); err != nil {
		return p, err
	}
	return s.enqueue(ctx, updated, "payment.captured")
}

func (s *PaymentService) Refund(ctx context.Context, id string, amount decimal.Decimal) (domain.Payment, error) {
	p, err := s.Store.GetPayment(ctx, id)
	if err != nil {
		return p, err
	}
	if err = p.ValidateRefund(amount); err != nil {
		return p, err
	}
	if err = s.Provider.Refund(ctx, p, amount); err != nil {
		return p, err
	}
	updated, err := s.Store.ApplyAmount(ctx, id, domain.PaymentRefunded, amount)
	if err != nil {
		return p, err
	}
	entries := []domain.LedgerEntry{{ID: uuid.NewString(), PaymentID: id, AccountID: "merchant:available", Currency: p.Currency, Debit: amount}, {ID: uuid.NewString(), PaymentID: id, AccountID: "customer:refunded", Currency: p.Currency, Credit: amount}}
	if err = s.Store.AppendLedger(ctx, entries); err != nil {
		return p, err
	}
	return s.enqueue(ctx, updated, "payment.refunded")
}

func (s *PaymentService) enqueue(ctx context.Context, p domain.Payment, eventType string) (domain.Payment, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if err := s.Store.Enqueue(ctx, domain.OutboxEvent{ID: uuid.NewString(), AggregateID: p.ID, EventType: eventType, Payload: string(payload)}); err != nil {
		return p, err
	}
	return p, nil
}
