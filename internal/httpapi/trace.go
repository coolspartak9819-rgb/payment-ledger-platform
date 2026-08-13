package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type traceKey string

const requestIDKey traceKey = "request_id"

func requestTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("x-request-id")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		w.Header().Set("x-request-id", requestID)
		if parent := r.Header.Get("traceparent"); parent != "" {
			w.Header().Set("traceparent", parent)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, requestID)))
	})
}
