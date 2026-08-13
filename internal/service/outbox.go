package service

import (
	"context"
	"log"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/nats-io/nats.go"
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

type NATSPublisher struct{ js nats.JetStreamContext }

func NewNATSPublisher(url string) (*NATSPublisher, func() error, error) {
	connection, err := nats.Connect(url)
	if err != nil {
		return nil, nil, err
	}
	js, err := connection.JetStream()
	if err != nil {
		connection.Close()
		return nil, nil, err
	}
	_, err = js.StreamInfo("PAYMENT_EVENTS")
	if err == nats.ErrStreamNotFound {
		_, err = js.AddStream(&nats.StreamConfig{Name: "PAYMENT_EVENTS", Subjects: []string{"payments.events"}})
	}
	if err != nil {
		connection.Close()
		return nil, nil, err
	}
	return &NATSPublisher{js: js}, connection.Drain, nil
}
func (p *NATSPublisher) Publish(ctx context.Context, event domain.OutboxEvent) error {
	_, err := p.js.Publish("payments.events", []byte(event.Payload), nats.Context(ctx), nats.MsgId(event.ID))
	return err
}
