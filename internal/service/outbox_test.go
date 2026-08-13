package service_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/service"
)

type recordingPublisher struct{ calls int32 }

func (p *recordingPublisher) Publish(context.Context, domain.OutboxEvent) error {
	atomic.AddInt32(&p.calls, 1)
	return nil
}

func TestMerchantWebhookSignsPayloadAndRetries(t *testing.T) {
	var calls int32
	secret := "merchant-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&calls, 1)
		if r.Header.Get("x-payment-event-id") != "event-1" {
			t.Error("missing event id")
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(`{"payment":"p1"}`))
		if r.Header.Get("x-payment-signature") != hex.EncodeToString(mac.Sum(nil)) {
			t.Error("bad signature")
		}
		if attempt < 2 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	publisher := service.MerchantWebhookPublisher{URL: server.URL, Secret: secret, Attempts: 3}
	if err := publisher.Publish(context.Background(), domain.OutboxEvent{ID: "event-1", Payload: `{"payment":"p1"}`}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestMerchantWebhookSendsFailureToDLQ(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	dlq := &recordingPublisher{}
	publisher := service.MerchantWebhookPublisher{URL: server.URL, Attempts: 1, DeadLetter: dlq}
	if err := publisher.Publish(context.Background(), domain.OutboxEvent{ID: "event-2", Payload: `{}`}); err == nil {
		t.Fatal("expected delivery error")
	}
	if dlq.calls != 1 {
		t.Fatalf("dlq calls=%d", dlq.calls)
	}
}
