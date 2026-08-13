package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/service"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
)

func testServer() *Server {
	return New(&service.PaymentService{Store: store.NewMemory(), Provider: service.DeterministicProvider{}, Risk: service.ThresholdRisk{MaxAmount: "1000"}}, Config{MerchantAPIKeys: "key-a:merchant-a, key-b:merchant-b", AdminAPIKey: "admin"})
}
func TestMerchantKeyCannotCreatePaymentForAnotherMerchant(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/payments", bytes.NewBufferString(`{"merchant_id":"merchant-b","customer_id":"c","currency":"USD","amount":"10"}`))
	request.Header.Set("x-api-key", "key-a")
	request.Header.Set("x-idempotency-key", "key")
	response := httptest.NewRecorder()
	testServer().Routes().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d", response.Code)
	}
}
func TestMerchantRoutesRequireKeyWhenKeysConfigured(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/payments", bytes.NewBufferString(`{}`))
	response := httptest.NewRecorder()
	testServer().Routes().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
}
func TestAdminReconciliationRequiresAdminKey(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/reconciliation", bytes.NewBufferString(`{"lines":[]}`))
	response := httptest.NewRecorder()
	testServer().Routes().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
	request.Header.Set("x-admin-api-key", "admin")
	response = httptest.NewRecorder()
	testServer().Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("admin status=%d body=%s", response.Code, response.Body.String())
	}
}
func TestResponseHasRequestTraceID(t *testing.T) {
	response := httptest.NewRecorder()
	testServer().Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Header().Get("x-request-id") == "" {
		t.Fatal("missing x-request-id")
	}
}
