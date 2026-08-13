package service

import (
	"context"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/shopspring/decimal"
)

type DeterministicProvider struct{}

func (DeterministicProvider) Authorize(_ context.Context, p domain.Payment) (string, error) {
	return "demo_" + p.ID[:8], nil
}
func (DeterministicProvider) Capture(_ context.Context, _ domain.Payment, _ decimal.Decimal) error {
	return nil
}
func (DeterministicProvider) Refund(_ context.Context, _ domain.Payment, _ decimal.Decimal) error {
	return nil
}

type ThresholdRisk struct{ MaxAmount string }

func (r ThresholdRisk) Check(_ context.Context, p domain.Payment) (bool, error) {
	max, err := decimal.NewFromString(r.MaxAmount)
	if err != nil {
		return false, err
	}
	return p.Amount.LessThanOrEqual(max), nil
}
