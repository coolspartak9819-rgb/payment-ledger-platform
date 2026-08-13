package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

func envInt(name string, fallback int) int {
	if value, err := strconv.Atoi(os.Getenv(name)); err == nil && value > 0 {
		return value
	}
	return fallback
}
func main() {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	total, concurrency := envInt("TOTAL", 500), envInt("CONCURRENCY", 25)
	var next, success, failures int64
	client := &http.Client{Timeout: 5 * time.Second}
	started := time.Now()
	var group sync.WaitGroup
	for range concurrency {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				i := int(atomic.AddInt64(&next, 1)) - 1
				if i >= total {
					return
				}
				payload := []byte(fmt.Sprintf(`{"merchant_id":"load-merchant","customer_id":"customer-%d","currency":"USD","amount":"10.00"}`, i))
				request, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/payments", bytes.NewReader(payload))
				request.Header.Set("content-type", "application/json")
				request.Header.Set("x-idempotency-key", fmt.Sprintf("load-%d", i))
				response, err := client.Do(request)
				if err == nil && response.StatusCode == http.StatusCreated {
					atomic.AddInt64(&success, 1)
				} else {
					atomic.AddInt64(&failures, 1)
				}
				if response != nil {
					_ = response.Body.Close()
				}
			}
		}()
	}
	group.Wait()
	elapsed := time.Since(started).Seconds()
	fmt.Printf("payments=%d success=%d failed=%d elapsed=%.2fs rps=%.2f\n", total, success, failures, elapsed, float64(success)/elapsed)
}
