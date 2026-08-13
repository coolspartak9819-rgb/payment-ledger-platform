package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
)

type providerWebhook struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	PaymentID string `json:"payment_id"`
}

func verifySignature(secret string, body []byte, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	return hmac.Equal(mac.Sum(nil), expected)
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		write(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if !verifySignature(s.webhookSecret, body, r.Header.Get("x-provider-signature")) {
		write(w, http.StatusUnauthorized, map[string]string{"error": "invalid webhook signature"})
		return
	}
	var event providerWebhook
	if err := json.Unmarshal(body, &event); err != nil || event.ID == "" || event.PaymentID == "" {
		write(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook payload"})
		return
	}
	accepted, err := s.payments.Store.RegisterWebhook(r.Context(), event.ID)
	if err != nil {
		write(w, 500, map[string]string{"error": "webhook persistence failed"})
		return
	}
	if !accepted {
		write(w, 200, map[string]string{"status": "duplicate_ignored"})
		return
	}
	var payment domain.Payment
	switch event.Type {
	case "payment.captured":
		payment, err = s.payments.Capture(r.Context(), event.PaymentID)
	case "payment.refunded":
		payment, err = s.payments.Refund(r.Context(), event.PaymentID)
	default:
		write(w, http.StatusUnprocessableEntity, map[string]string{"error": "unsupported webhook type"})
		return
	}
	if err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
	write(w, 200, payment)
}
