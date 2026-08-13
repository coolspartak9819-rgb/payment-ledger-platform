package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/service"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/shopspring/decimal"
)

type Server struct{ payments *service.PaymentService }

func New(payments *service.PaymentService) *Server { return &Server{payments: payments} }
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { write(w, 200, map[string]string{"status": "ready"}) })
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/plain")
		_, _ = w.Write([]byte("payment_platform_up 1\n"))
	})
	mux.HandleFunc("POST /v1/payments", s.create)
	mux.HandleFunc("GET /v1/payments/", s.get)
	mux.HandleFunc("POST /v1/payments/authorize", s.authorize)
	return mux
}
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MerchantID string `json:"merchant_id"`
		CustomerID string `json:"customer_id"`
		Currency   string `json:"currency"`
		Amount     string `json:"amount"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		write(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		write(w, 422, map[string]string{"error": "amount must be a decimal"})
		return
	}
	p, replay, err := s.payments.Create(r.Context(), domain.CreatePayment{MerchantID: req.MerchantID, CustomerID: req.CustomerID, Currency: req.Currency, Amount: amount}, r.Header.Get("x-idempotency-key"))
	if err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
	if replay {
		w.Header().Set("x-idempotent-replay", "true")
	}
	write(w, 201, p)
}
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/payments/")
	p, err := s.payments.Store.GetPayment(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		write(w, 404, map[string]string{"error": "payment not found"})
		return
	}
	if err != nil {
		write(w, 500, map[string]string{"error": "internal error"})
		return
	}
	write(w, 200, p)
}
func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	p, err := s.payments.Authorize(r.Context(), id)
	if err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
	write(w, 200, p)
}
func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
