package store

import (
	"context"
	"errors"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (s *PostgresStore) CreatePayment(ctx context.Context, payment domain.Payment, key string) (domain.Payment, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Payment{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var existing domain.Payment
	err = tx.QueryRow(ctx, `SELECT p.id,p.merchant_id,p.customer_id,p.currency,p.amount,p.status,p.provider_reference FROM idempotency_keys k JOIN payments p ON p.id=k.payment_id WHERE k.key=$1`, key).Scan(&existing.ID, &existing.MerchantID, &existing.CustomerID, &existing.Currency, &existing.Amount, &existing.Status, &existing.ProviderReference)
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO payments (id,merchant_id,customer_id,currency,amount,status) VALUES ($1,$2,$3,$4,$5,$6)`, payment.ID, payment.MerchantID, payment.CustomerID, payment.Currency, payment.Amount, payment.Status)
	if err != nil {
		return domain.Payment{}, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO idempotency_keys (key,payment_id) VALUES ($1,$2)`, key, payment.ID)
	if err != nil {
		return domain.Payment{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Payment{}, false, err
	}
	return payment, false, nil
}

func (s *PostgresStore) GetPayment(ctx context.Context, id string) (domain.Payment, error) {
	var p domain.Payment
	err := s.pool.QueryRow(ctx, `SELECT id,merchant_id,customer_id,currency,amount,status,COALESCE(provider_reference,'') FROM payments WHERE id=$1`, id).Scan(&p.ID, &p.MerchantID, &p.CustomerID, &p.Currency, &p.Amount, &p.Status, &p.ProviderReference)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, ErrNotFound
	}
	return p, err
}

func (s *PostgresStore) Transition(ctx context.Context, id string, to domain.PaymentStatus, reference string) (domain.Payment, error) {
	p, err := s.GetPayment(ctx, id)
	if err != nil {
		return p, err
	}
	if !p.CanTransition(to) {
		return domain.Payment{}, errors.New("invalid payment state transition")
	}
	return s.update(ctx, id, to, reference)
}
func (s *PostgresStore) update(ctx context.Context, id string, status domain.PaymentStatus, reference string) (domain.Payment, error) {
	var p domain.Payment
	err := s.pool.QueryRow(ctx, `UPDATE payments SET status=$2,provider_reference=NULLIF($3,''),updated_at=now() WHERE id=$1 RETURNING id,merchant_id,customer_id,currency,amount,status,COALESCE(provider_reference,'')`, id, status, reference).Scan(&p.ID, &p.MerchantID, &p.CustomerID, &p.Currency, &p.Amount, &p.Status, &p.ProviderReference)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, ErrNotFound
	}
	return p, err
}
func (s *PostgresStore) AppendLedger(ctx context.Context, entries []domain.LedgerEntry) error {
	if err := domain.ValidateBalanced(entries); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, entry := range entries {
		if _, err = tx.Exec(ctx, `INSERT INTO ledger_entries (id,payment_id,account_id,currency,debit,credit) VALUES ($1,$2,$3,$4,$5,$6)`, entry.ID, entry.PaymentID, entry.AccountID, entry.Currency, entry.Debit, entry.Credit); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *PostgresStore) Enqueue(ctx context.Context, event domain.OutboxEvent) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO outbox_events (id,aggregate_id,event_type,payload) VALUES ($1,$2,$3,$4::jsonb)`, event.ID, event.AggregateID, event.EventType, event.Payload)
	return err
}
func (s *PostgresStore) ClaimOutbox(ctx context.Context, n int) ([]domain.OutboxEvent, error) {
	rows, err := s.pool.Query(ctx, `WITH candidates AS (
  SELECT id FROM outbox_events
  WHERE published_at IS NULL AND (claimed_at IS NULL OR claimed_at < now() - interval '1 minute')
  ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $1
) UPDATE outbox_events e SET claimed_at=now() FROM candidates
WHERE e.id=candidates.id RETURNING e.id,e.aggregate_id,e.event_type,e.payload::text`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.OutboxEvent
	for rows.Next() {
		var e domain.OutboxEvent
		if err := rows.Scan(&e.ID, &e.AggregateID, &e.EventType, &e.Payload); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
func (s *PostgresStore) MarkPublished(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE outbox_events SET published_at=now() WHERE id=$1`, id)
	return err
}
func (s *PostgresStore) RegisterWebhook(ctx context.Context, id string) (bool, error) {
	command, err := s.pool.Exec(ctx, `INSERT INTO processed_webhooks (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, id)
	return command.RowsAffected() == 1, err
}

var _ Store = (*PostgresStore)(nil)
