package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/service"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/shopspring/decimal"
)

func signature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
func TestSignedWebhookCapturesOnce(t *testing.T) {
	payments := &service.PaymentService{Store: store.NewMemory(), Provider: service.DeterministicProvider{}, Risk: service.ThresholdRisk{MaxAmount: "1000"}}
	p, _, err := payments.Create(context.Background(), domain.CreatePayment{MerchantID: "m", CustomerID: "c", Currency: "USD", Amount: decimal.NewFromInt(10)}, "key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = payments.Authorize(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	secret := "webhook-secret"
	payload := []byte(`{"id":"event-1","type":"payment.captured","payment_id":"` + p.ID + `","amount":"10.00"}`)
	handler := New(payments, Config{WebhookSecret: secret}).Routes()
	request := httptest.NewRequest(http.MethodPost, "/v1/provider/webhooks", bytes.NewReader(payload))
	request.Header.Set("x-provider-signature", signature(secret, payload))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	duplicate := httptest.NewRecorder()
	request2 := httptest.NewRequest(http.MethodPost, "/v1/provider/webhooks", bytes.NewReader(payload))
	request2.Header.Set("x-provider-signature", signature(secret, payload))
	handler.ServeHTTP(duplicate, request2)
	if duplicate.Code != http.StatusOK || !bytes.Contains(duplicate.Body.Bytes(), []byte("duplicate_ignored")) {
		t.Fatalf("duplicate response=%d %s", duplicate.Code, duplicate.Body.String())
	}
}
