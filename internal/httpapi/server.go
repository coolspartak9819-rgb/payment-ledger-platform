package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/domain"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/service"
	"github.com/coolspartak9819-rgb/payment-ledger-platform/internal/store"
	"github.com/shopspring/decimal"
)

type Server struct {
	payments      *service.PaymentService
	webhookSecret string
	authenticator MerchantAuthenticator
	limiter       RateLimiter
	adminKey      string
	metrics       *service.CounterMetrics
}

type Config struct {
	WebhookSecret, MerchantAPIKeys, AdminAPIKey string
	Limiter                                     RateLimiter
	Metrics                                     *service.CounterMetrics
}

func New(payments *service.PaymentService, config Config) *Server {
	if config.Limiter == nil {
		config.Limiter = NewMemoryRateLimiter(120, time.Minute)
	}
	if config.Metrics == nil {
		config.Metrics = service.NewCounterMetrics()
	}
	return &Server{payments: payments, webhookSecret: config.WebhookSecret, authenticator: NewMerchantAuthenticator(config.MerchantAPIKeys), limiter: config.Limiter, adminKey: config.AdminAPIKey, metrics: config.Metrics}
}
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { write(w, 200, map[string]string{"status": "ready"}) })
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/plain")
		_, _ = w.Write([]byte("payment_platform_up 1\n"))
		for _, name := range s.metrics.Names() {
			_, _ = fmt.Fprintf(w, "payment_platform_%s_total %d\n", name, s.metrics.Snapshot()[name])
		}
	})
	protect := func(handler http.HandlerFunc) http.Handler {
		return requireMerchant(handler, s.authenticator, s.limiter)
	}
	mux.Handle("POST /v1/payments", protect(s.create))
	mux.Handle("GET /v1/payments/{id}", protect(s.get))
	mux.Handle("POST /v1/payments/{id}/authorize", protect(s.authorize))
	mux.Handle("POST /v1/payments/{id}/capture", protect(s.capture))
	mux.Handle("POST /v1/payments/{id}/refund", protect(s.refund))
	mux.Handle("POST /v1/payouts", protect(s.createPayout))
	mux.HandleFunc("POST /v1/provider/webhooks", s.webhook)
	mux.HandleFunc("POST /v1/provider/payout-webhooks", s.payoutWebhook)
	mux.HandleFunc("POST /v1/admin/reconciliation", s.reconcile)
	mux.HandleFunc("POST /v1/admin/disputes", s.openDispute)
	mux.HandleFunc("POST /v1/admin/disputes/{id}/resolve", s.resolveDispute)
	return requestTracing(mux)
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
	if merchant := merchantFromContext(r.Context()); merchant != "" && merchant != req.MerchantID {
		write(w, 403, map[string]string{"error": "API key does not match merchant_id"})
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
	s.metrics.Inc("payments_created")
	write(w, 201, p)
}
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.payments.Store.GetPayment(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		write(w, 404, map[string]string{"error": "payment not found"})
		return
	}
	if err != nil {
		write(w, 500, map[string]string{"error": "internal error"})
		return
	}
	if merchant := merchantFromContext(r.Context()); merchant != "" && p.MerchantID != merchant {
		write(w, 404, map[string]string{"error": "payment not found"})
		return
	}
	write(w, 200, p)
}
func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.ownsPayment(w, r, id) {
		return
	}
	p, err := s.payments.Authorize(r.Context(), id)
	if err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
	s.metrics.Inc("payments_authorized")
	write(w, 200, p)
}
func (s *Server) capture(w http.ResponseWriter, r *http.Request) {
	if !s.ownsPayment(w, r, r.PathValue("id")) {
		return
	}
	amount, err := requestAmount(r)
	if err == nil {
		var p domain.Payment
		p, err = s.payments.Capture(r.Context(), r.PathValue("id"), amount)
		if err == nil {
			s.metrics.Inc("payments_captured")
			write(w, 200, p)
			return
		}
	}
	if err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
}
func (s *Server) refund(w http.ResponseWriter, r *http.Request) {
	if !s.ownsPayment(w, r, r.PathValue("id")) {
		return
	}
	amount, err := requestAmount(r)
	if err == nil {
		var p domain.Payment
		p, err = s.payments.Refund(r.Context(), r.PathValue("id"), amount)
		if err == nil {
			s.metrics.Inc("payments_refunded")
			write(w, 200, p)
			return
		}
	}
	if err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
}
func (s *Server) createPayout(w http.ResponseWriter, r *http.Request) {
	merchant := merchantFromContext(r.Context())
	if merchant == "" {
		write(w, 403, map[string]string{"error": "merchant API keys are required for payouts"})
		return
	}
	var request struct {
		Currency string `json:"currency"`
		Amount   string `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	amount, err := decimal.NewFromString(request.Amount)
	if err != nil {
		write(w, 422, map[string]string{"error": "amount must be decimal"})
		return
	}
	payout, err := service.PayoutService{Store: s.payments.Store}.Create(r.Context(), merchant, request.Currency, amount)
	if err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
	s.metrics.Inc("payouts_created")
	write(w, 201, payout)
}
func (s *Server) ownsPayment(w http.ResponseWriter, r *http.Request, id string) bool {
	merchant := merchantFromContext(r.Context())
	if merchant == "" {
		return true
	}
	p, err := s.payments.Store.GetPayment(r.Context(), id)
	if err != nil || p.MerchantID != merchant {
		write(w, 404, map[string]string{"error": "payment not found"})
		return false
	}
	return true
}
func (s *Server) reconcile(w http.ResponseWriter, r *http.Request) {
	if s.adminKey == "" || r.Header.Get("x-admin-api-key") != s.adminKey {
		write(w, 401, map[string]string{"error": "valid x-admin-api-key is required"})
		return
	}
	var request struct {
		Lines []struct {
			ProviderReference string `json:"provider_reference"`
			CapturedAmount    string `json:"captured_amount"`
		} `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	lines := make([]service.SettlementLine, 0, len(request.Lines))
	for _, line := range request.Lines {
		amount, err := decimal.NewFromString(line.CapturedAmount)
		if err != nil {
			write(w, 422, map[string]string{"error": "captured_amount must be decimal"})
			return
		}
		lines = append(lines, service.SettlementLine{ProviderReference: line.ProviderReference, CapturedAmount: amount})
	}
	mismatches, err := service.ReconciliationService{Store: s.payments.Store}.Reconcile(r.Context(), lines)
	if err != nil {
		write(w, 500, map[string]string{"error": "reconciliation failed"})
		return
	}
	s.metrics.Inc("reconciliation_runs")
	for range mismatches {
		s.metrics.Inc("reconciliation_mismatches")
	}
	write(w, 200, map[string]any{"mismatches": mismatches})
}
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.adminKey == "" || r.Header.Get("x-admin-api-key") != s.adminKey {
		write(w, 401, map[string]string{"error": "valid x-admin-api-key is required"})
		return false
	}
	return true
}
func (s *Server) openDispute(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var request struct {
		PaymentID string `json:"payment_id"`
		Reason    string `json:"reason"`
		Amount    string `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	amount, err := decimal.NewFromString(request.Amount)
	if err != nil {
		write(w, 422, map[string]string{"error": "amount must be decimal"})
		return
	}
	d, err := service.DisputeService{Store: s.payments.Store}.Open(r.Context(), request.PaymentID, request.Reason, amount)
	if err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
	s.metrics.Inc("disputes_opened")
	write(w, 201, d)
}
func (s *Server) resolveDispute(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var request struct {
		MerchantWins bool `json:"merchant_wins"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	d, err := service.DisputeService{Store: s.payments.Store}.Resolve(r.Context(), r.PathValue("id"), request.MerchantWins)
	if err != nil {
		write(w, 422, map[string]string{"error": err.Error()})
		return
	}
	s.metrics.Inc("disputes_" + string(d.Status))
	write(w, 200, d)
}
func requestAmount(r *http.Request) (decimal.Decimal, error) {
	var request struct {
		Amount string `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromString(request.Amount)
}
func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
