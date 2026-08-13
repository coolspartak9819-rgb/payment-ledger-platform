package service

import (
	"context"
	"log"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
)

type EventPublisher interface {
	Publish(context.Context, domain.OutboxEvent) error
}

type OutboxWorker struct {
	Store     store.Store
	Publisher EventPublisher
}

func (w OutboxWorker) Deliver(ctx context.Context, batchSize int) error {
	events, err := w.Store.ClaimOutbox(ctx, batchSize)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := w.Publisher.Publish(ctx, event); err != nil {
			return err
		}
		if err := w.Store.MarkPublished(ctx, event.ID); err != nil {
			return err
		}
	}
	return nil
}

type LogPublisher struct{}

func (LogPublisher) Publish(_ context.Context, event domain.OutboxEvent) error {
	log.Printf("published event id=%s type=%s aggregate=%s", event.ID, event.EventType, event.AggregateID)
	return nil
}
