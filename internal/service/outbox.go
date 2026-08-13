package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/nats-io/nats.go"
)

type EventPublisher interface {
	Publish(context.Context, domain.OutboxEvent) error
}

type CompositePublisher []EventPublisher

func (publishers CompositePublisher) Publish(ctx context.Context, event domain.OutboxEvent) error {
	for _, publisher := range publishers {
		if err := publisher.Publish(ctx, event); err != nil {
			return err
		}
	}
	return nil
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
		_, err = js.AddStream(&nats.StreamConfig{Name: "PAYMENT_EVENTS", Subjects: []string{"payments.events", "payments.webhooks.dlq"}})
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

type NATSDeadLetterPublisher struct{ js nats.JetStreamContext }

func (p NATSDeadLetterPublisher) Publish(ctx context.Context, event domain.OutboxEvent) error {
	_, err := p.js.Publish("payments.webhooks.dlq", []byte(event.Payload), nats.Context(ctx), nats.MsgId(event.ID))
	return err
}
func (p *NATSPublisher) DeadLetter() EventPublisher { return NATSDeadLetterPublisher{js: p.js} }

type MerchantWebhookPublisher struct {
	URL, Secret string
	Client      *http.Client
	Attempts    int
	DeadLetter  EventPublisher
}

func (p MerchantWebhookPublisher) Publish(ctx context.Context, event domain.OutboxEvent) error {
	if p.URL == "" {
		return nil
	}
	attempts := p.Attempts
	if attempts < 1 {
		attempts = 3
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, strings.NewReader(event.Payload))
		if err != nil {
			return err
		}
		request.Header.Set("content-type", "application/json")
		request.Header.Set("x-payment-event-id", event.ID)
		request.Header.Set("x-payment-event-type", event.EventType)
		mac := hmac.New(sha256.New, []byte(p.Secret))
		_, _ = mac.Write([]byte(event.Payload))
		request.Header.Set("x-payment-signature", hex.EncodeToString(mac.Sum(nil)))
		response, err := client.Do(request)
		if err == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			_ = response.Body.Close()
			return nil
		}
		if response != nil {
			_ = response.Body.Close()
		}
		last = err
		if last == nil {
			last = fmt.Errorf("merchant webhook status %d", response.StatusCode)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
		}
	}
	if p.DeadLetter != nil {
		if err := p.DeadLetter.Publish(ctx, domain.OutboxEvent{ID: event.ID, AggregateID: event.AggregateID, EventType: "merchant_webhook.failed", Payload: event.Payload}); err != nil {
			return err
		}
	}
	return last
}
