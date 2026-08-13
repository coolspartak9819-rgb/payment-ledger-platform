package domain

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PaymentStatus string

const (
	PaymentCreated    PaymentStatus = "created"
	PaymentAuthorized PaymentStatus = "authorized"
	PaymentCaptured   PaymentStatus = "captured"
	PaymentRefunded   PaymentStatus = "refunded"
	PaymentFailed     PaymentStatus = "failed"
)

type Payment struct {
	ID                string          `json:"id"`
	MerchantID        string          `json:"merchant_id"`
	CustomerID        string          `json:"customer_id"`
	Currency          string          `json:"currency"`
	Amount            decimal.Decimal `json:"amount"`
	CapturedAmount    decimal.Decimal `json:"captured_amount"`
	RefundedAmount    decimal.Decimal `json:"refunded_amount"`
	Status            PaymentStatus   `json:"status"`
	ProviderReference string          `json:"provider_reference,omitempty"`
}

type CreatePayment struct {
	MerchantID, CustomerID, Currency string
	Amount                           decimal.Decimal
}

func NewPayment(in CreatePayment) (Payment, error) {
	if strings.TrimSpace(in.MerchantID) == "" || strings.TrimSpace(in.CustomerID) == "" {
		return Payment{}, errors.New("merchant_id and customer_id are required")
	}
	if len(in.Currency) != 3 {
		return Payment{}, errors.New("currency must be a 3-letter ISO code")
	}
	if !in.Amount.GreaterThan(decimal.Zero) {
		return Payment{}, errors.New("amount must be greater than zero")
	}
	return Payment{ID: uuid.NewString(), MerchantID: in.MerchantID, CustomerID: in.CustomerID, Currency: strings.ToUpper(in.Currency), Amount: in.Amount, Status: PaymentCreated}, nil
}

func (p Payment) CanTransition(to PaymentStatus) bool {
	return map[PaymentStatus]map[PaymentStatus]bool{PaymentCreated: {PaymentAuthorized: true, PaymentFailed: true}, PaymentAuthorized: {PaymentCaptured: true, PaymentFailed: true}, PaymentCaptured: {PaymentCaptured: true, PaymentRefunded: true}, PaymentRefunded: {PaymentRefunded: true}}[p.Status][to]
}

func (p Payment) ValidateCapture(amount decimal.Decimal) error {
	if !amount.GreaterThan(decimal.Zero) || p.CapturedAmount.Add(amount).GreaterThan(p.Amount) {
		return errors.New("capture amount exceeds remaining authorized amount")
	}
	return nil
}
func (p Payment) ValidateRefund(amount decimal.Decimal) error {
	if !amount.GreaterThan(decimal.Zero) || p.RefundedAmount.Add(amount).GreaterThan(p.CapturedAmount) {
		return errors.New("refund amount exceeds captured amount")
	}
	return nil
}

type LedgerEntry struct {
	ID, PaymentID, AccountID, Currency string
	Debit, Credit                      decimal.Decimal
}

func ValidateBalanced(entries []LedgerEntry) error {
	if len(entries) < 2 {
		return errors.New("a ledger transaction requires at least two entries")
	}
	debit, credit := decimal.Zero, decimal.Zero
	for _, entry := range entries {
		if entry.Debit.IsNegative() || entry.Credit.IsNegative() || (entry.Debit.IsZero() == entry.Credit.IsZero()) {
			return errors.New("each entry must contain either debit or credit")
		}
		debit = debit.Add(entry.Debit)
		credit = credit.Add(entry.Credit)
	}
	if !debit.Equal(credit) {
		return errors.New("ledger transaction is not balanced")
	}
	return nil
}

type OutboxEvent struct {
	ID, AggregateID, EventType, Payload string
	Published                           bool
}
