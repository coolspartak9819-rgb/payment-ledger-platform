package store

import (
	"context"
	"errors"
	"sync"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/shopspring/decimal"
)

var ErrNotFound = errors.New("payment not found")

type Store interface {
	CreatePayment(context.Context, domain.Payment, string) (domain.Payment, bool, error)
	GetPayment(context.Context, string) (domain.Payment, error)
	GetPaymentByProviderReference(context.Context, string) (domain.Payment, error)
	Transition(context.Context, string, domain.PaymentStatus, string) (domain.Payment, error)
	ApplyAmount(context.Context, string, domain.PaymentStatus, decimal.Decimal) (domain.Payment, error)
	AppendLedger(context.Context, []domain.LedgerEntry) error
	Enqueue(context.Context, domain.OutboxEvent) error
	ClaimOutbox(context.Context, int) ([]domain.OutboxEvent, error)
	MarkPublished(context.Context, string) error
	RegisterWebhook(context.Context, string) (bool, error)
	CommitPaymentCommand(context.Context, domain.PaymentCommand) (domain.Payment, error)
	CreatePayout(context.Context, domain.Payout, []domain.LedgerEntry, domain.OutboxEvent) (domain.Payout, error)
	GetPayout(context.Context, string) (domain.Payout, error)
	AvailableBalance(context.Context, string, string) (decimal.Decimal, error)
	TransitionPayout(context.Context, string, domain.PayoutStatus, string, []domain.LedgerEntry, domain.OutboxEvent) (domain.Payout, error)
	CreateDispute(context.Context, domain.Dispute, []domain.LedgerEntry, domain.OutboxEvent) (domain.Dispute, error)
	GetDispute(context.Context, string) (domain.Dispute, error)
	ResolveDispute(context.Context, string, domain.DisputeStatus, []domain.LedgerEntry, domain.OutboxEvent) (domain.Dispute, error)
}

type MemoryStore struct {
	mu       sync.Mutex
	payments map[string]domain.Payment
	keys     map[string]string
	outbox   map[string]domain.OutboxEvent
	ledger   []domain.LedgerEntry
	webhooks map[string]struct{}
	payouts  map[string]domain.Payout
	disputes map[string]domain.Dispute
}

func NewMemory() *MemoryStore {
	return &MemoryStore{payments: map[string]domain.Payment{}, keys: map[string]string{}, outbox: map[string]domain.OutboxEvent{}, webhooks: map[string]struct{}{}, payouts: map[string]domain.Payout{}, disputes: map[string]domain.Dispute{}}
}
func (s *MemoryStore) CreatePayment(_ context.Context, p domain.Payment, key string) (domain.Payment, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.keys[key]; ok {
		return s.payments[existing], true, nil
	}
	s.payments[p.ID] = p
	s.keys[key] = p.ID
	return p, false, nil
}
func (s *MemoryStore) GetPayment(_ context.Context, id string) (domain.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payments[id]
	if !ok {
		return domain.Payment{}, ErrNotFound
	}
	return p, nil
}
func (s *MemoryStore) GetPaymentByProviderReference(_ context.Context, reference string) (domain.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.payments {
		if p.ProviderReference == reference {
			return p, nil
		}
	}
	return domain.Payment{}, ErrNotFound
}
func (s *MemoryStore) Transition(_ context.Context, id string, to domain.PaymentStatus, reference string) (domain.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payments[id]
	if !ok {
		return domain.Payment{}, ErrNotFound
	}
	if !p.CanTransition(to) {
		return domain.Payment{}, errors.New("invalid payment state transition")
	}
	p.Status, p.ProviderReference = to, reference
	s.payments[id] = p
	return p, nil
}
func (s *MemoryStore) ApplyAmount(_ context.Context, id string, action domain.PaymentStatus, amount decimal.Decimal) (domain.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payments[id]
	if !ok {
		return domain.Payment{}, ErrNotFound
	}
	if action == domain.PaymentCaptured {
		if err := p.ValidateCapture(amount); err != nil {
			return domain.Payment{}, err
		}
		if !p.CanTransition(domain.PaymentCaptured) {
			return domain.Payment{}, errors.New("invalid payment state transition")
		}
		p.CapturedAmount = p.CapturedAmount.Add(amount)
		p.Status = domain.PaymentCaptured
	} else if action == domain.PaymentRefunded {
		if err := p.ValidateRefund(amount); err != nil {
			return domain.Payment{}, err
		}
		if !p.CanTransition(domain.PaymentRefunded) {
			return domain.Payment{}, errors.New("invalid payment state transition")
		}
		p.RefundedAmount = p.RefundedAmount.Add(amount)
		p.Status = domain.PaymentRefunded
	} else {
		return domain.Payment{}, errors.New("unsupported amount action")
	}
	s.payments[id] = p
	return p, nil
}
func (s *MemoryStore) AppendLedger(_ context.Context, entries []domain.LedgerEntry) error {
	if err := domain.ValidateBalanced(entries); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ledger = append(s.ledger, entries...)
	return nil
}
func (s *MemoryStore) Enqueue(_ context.Context, event domain.OutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outbox[event.ID] = event
	return nil
}
func (s *MemoryStore) ClaimOutbox(_ context.Context, n int) ([]domain.OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]domain.OutboxEvent, 0, n)
	for _, e := range s.outbox {
		if !e.Published {
			result = append(result, e)
			if len(result) == n {
				break
			}
		}
	}
	return result, nil
}
func (s *MemoryStore) MarkPublished(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.outbox[id]
	if !ok {
		return ErrNotFound
	}
	e.Published = true
	s.outbox[id] = e
	return nil
}
func (s *MemoryStore) RegisterWebhook(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.webhooks[id]; exists {
		return false, nil
	}
	s.webhooks[id] = struct{}{}
	return true, nil
}
func (s *MemoryStore) CommitPaymentCommand(_ context.Context, command domain.PaymentCommand) (domain.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payments[command.PaymentID]
	if !ok {
		return domain.Payment{}, ErrNotFound
	}
	if !p.CanTransition(command.Status) {
		return domain.Payment{}, errors.New("invalid payment state transition")
	}
	if command.Status == domain.PaymentCaptured {
		if err := p.ValidateCapture(command.Amount); err != nil {
			return domain.Payment{}, err
		}
		p.CapturedAmount = p.CapturedAmount.Add(command.Amount)
	}
	if command.Status == domain.PaymentRefunded {
		if err := p.ValidateRefund(command.Amount); err != nil {
			return domain.Payment{}, err
		}
		p.RefundedAmount = p.RefundedAmount.Add(command.Amount)
	}
	if err := domain.ValidateBalanced(command.Ledger); err != nil {
		return domain.Payment{}, err
	}
	p.Status = command.Status
	if command.ProviderReference != "" {
		p.ProviderReference = command.ProviderReference
	}
	s.payments[p.ID] = p
	s.ledger = append(s.ledger, command.Ledger...)
	payload := command.Event
	payload.AggregateID = p.ID
	s.outbox[payload.ID] = payload
	return p, nil
}
func (s *MemoryStore) AvailableBalance(_ context.Context, merchant, currency string) (decimal.Decimal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	balance := decimal.Zero
	for _, entry := range s.ledger {
		if entry.AccountID == "merchant:"+merchant+":available" && entry.Currency == currency {
			balance = balance.Add(entry.Credit).Sub(entry.Debit)
		}
	}
	return balance, nil
}
func (s *MemoryStore) CreatePayout(_ context.Context, p domain.Payout, entries []domain.LedgerEntry, event domain.OutboxEvent) (domain.Payout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	balance := decimal.Zero
	for _, e := range s.ledger {
		if e.AccountID == "merchant:"+p.MerchantID+":available" && e.Currency == p.Currency {
			balance = balance.Add(e.Credit).Sub(e.Debit)
		}
	}
	if balance.LessThan(p.Amount) {
		return domain.Payout{}, errors.New("insufficient available balance")
	}
	if err := domain.ValidateBalanced(entries); err != nil {
		return domain.Payout{}, err
	}
	s.payouts[p.ID] = p
	s.ledger = append(s.ledger, entries...)
	s.outbox[event.ID] = event
	return p, nil
}
func (s *MemoryStore) GetPayout(_ context.Context, id string) (domain.Payout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payouts[id]
	if !ok {
		return domain.Payout{}, ErrNotFound
	}
	return p, nil
}
func (s *MemoryStore) TransitionPayout(_ context.Context, id string, status domain.PayoutStatus, reference string, entries []domain.LedgerEntry, event domain.OutboxEvent) (domain.Payout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payouts[id]
	if !ok {
		return domain.Payout{}, ErrNotFound
	}
	if p.Status != domain.PayoutPending {
		return domain.Payout{}, errors.New("payout is already resolved")
	}
	if status != domain.PayoutPaid && status != domain.PayoutFailed {
		return domain.Payout{}, errors.New("invalid payout status")
	}
	if err := domain.ValidateBalanced(entries); err != nil {
		return domain.Payout{}, err
	}
	p.Status = status
	p.ProviderReference = reference
	s.payouts[id] = p
	s.ledger = append(s.ledger, entries...)
	s.outbox[event.ID] = event
	return p, nil
}
func (s *MemoryStore) CreateDispute(_ context.Context, d domain.Dispute, entries []domain.LedgerEntry, event domain.OutboxEvent) (domain.Dispute, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.disputes[d.ID]; ok {
		return domain.Dispute{}, errors.New("dispute already exists")
	}
	if err := domain.ValidateBalanced(entries); err != nil {
		return domain.Dispute{}, err
	}
	s.disputes[d.ID] = d
	s.ledger = append(s.ledger, entries...)
	s.outbox[event.ID] = event
	return d, nil
}
func (s *MemoryStore) GetDispute(_ context.Context, id string) (domain.Dispute, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.disputes[id]
	if !ok {
		return domain.Dispute{}, ErrNotFound
	}
	return d, nil
}
func (s *MemoryStore) ResolveDispute(_ context.Context, id string, status domain.DisputeStatus, entries []domain.LedgerEntry, event domain.OutboxEvent) (domain.Dispute, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.disputes[id]
	if !ok {
		return domain.Dispute{}, ErrNotFound
	}
	if d.Status != domain.DisputeOpen {
		return domain.Dispute{}, errors.New("dispute is already resolved")
	}
	if status != domain.DisputeWon && status != domain.DisputeLost {
		return domain.Dispute{}, errors.New("invalid dispute status")
	}
	if err := domain.ValidateBalanced(entries); err != nil {
		return domain.Dispute{}, err
	}
	d.Status = status
	s.disputes[id] = d
	s.ledger = append(s.ledger, entries...)
	s.outbox[event.ID] = event
	return d, nil
}
