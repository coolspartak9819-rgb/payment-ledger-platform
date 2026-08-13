package service

import (
	"context"
	"sort"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/shopspring/decimal"
)

type SettlementLine struct {
	ProviderReference string
	CapturedAmount    decimal.Decimal
}
type ReconciliationMismatch struct {
	ProviderReference, Reason string
	Expected, Reported        decimal.Decimal
}
type ReconciliationService struct{ Store store.Store }

// Reconcile detects provider settlement rows that do not agree with the durable payment record.
func (s ReconciliationService) Reconcile(ctx context.Context, lines []SettlementLine) ([]ReconciliationMismatch, error) {
	mismatches := make([]ReconciliationMismatch, 0)
	for _, line := range lines {
		payment, err := s.Store.GetPaymentByProviderReference(ctx, line.ProviderReference)
		if err == store.ErrNotFound {
			mismatches = append(mismatches, ReconciliationMismatch{ProviderReference: line.ProviderReference, Reason: "unknown_provider_payment", Reported: line.CapturedAmount})
			continue
		}
		if err != nil {
			return nil, err
		}
		if !payment.CapturedAmount.Equal(line.CapturedAmount) {
			mismatches = append(mismatches, ReconciliationMismatch{ProviderReference: line.ProviderReference, Reason: "captured_amount_mismatch", Expected: payment.CapturedAmount, Reported: line.CapturedAmount})
		}
	}
	sort.Slice(mismatches, func(i, j int) bool { return mismatches[i].ProviderReference < mismatches[j].ProviderReference })
	return mismatches, nil
}
