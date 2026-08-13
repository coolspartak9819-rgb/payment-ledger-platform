package store

import (
	"context"
	"errors"
	"sync"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
)

var ErrNotFound = errors.New("payment not found")

type Store interface {
	CreatePayment(context.Context, domain.Payment, string) (domain.Payment, bool, error)
	GetPayment(context.Context, string) (domain.Payment, error)
	Transition(context.Context, string, domain.PaymentStatus, string) (domain.Payment, error)
	AppendLedger(context.Context, []domain.LedgerEntry) error
	Enqueue(context.Context, domain.OutboxEvent) error
	ClaimOutbox(context.Context, int) ([]domain.OutboxEvent, error)
	MarkPublished(context.Context, string) error
}

type MemoryStore struct {
	mu       sync.Mutex
	payments map[string]domain.Payment
	keys     map[string]string
	outbox   map[string]domain.OutboxEvent
	ledger   []domain.LedgerEntry
}

func NewMemory() *MemoryStore {
	return &MemoryStore{payments: map[string]domain.Payment{}, keys: map[string]string{}, outbox: map[string]domain.OutboxEvent{}}
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
