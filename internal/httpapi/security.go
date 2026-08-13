package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type contextKey string

const merchantContextKey contextKey = "merchant_id"

type MerchantAuthenticator struct{ keys map[string]string }

func NewMerchantAuthenticator(serialized string) MerchantAuthenticator {
	keys := map[string]string{}
	for _, item := range strings.Split(serialized, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), ":", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			keys[parts[0]] = parts[1]
		}
	}
	return MerchantAuthenticator{keys: keys}
}
func (a MerchantAuthenticator) Authenticate(key string) (string, bool) {
	if len(a.keys) == 0 {
		return "", true
	}
	for candidate, merchant := range a.keys {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(key)) == 1 {
			return merchant, true
		}
	}
	return "", false
}

type RateLimiter interface {
	Allow(context.Context, string) (bool, error)
}
type memoryRateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func NewMemoryRateLimiter(limit int, window time.Duration) RateLimiter {
	return &memoryRateLimiter{hits: map[string][]time.Time{}, limit: limit, window: window}
}
func (l *memoryRateLimiter) Allow(_ context.Context, key string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	recent := l.hits[key][:0]
	for _, hit := range l.hits[key] {
		if now.Sub(hit) < l.window {
			recent = append(recent, hit)
		}
	}
	if len(recent) >= l.limit {
		l.hits[key] = recent
		return false, nil
	}
	l.hits[key] = append(recent, now)
	return true, nil
}

type RedisRateLimiter struct {
	client *redis.Client
	limit  int
	window time.Duration
}

func NewRedisRateLimiter(url string, limit int, window time.Duration) (*RedisRateLimiter, error) {
	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &RedisRateLimiter{client: redis.NewClient(options), limit: limit, window: window}, nil
}
func (l *RedisRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	bucket := "payment-platform:rate:" + key
	count, err := l.client.Incr(ctx, bucket).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		if err = l.client.Expire(ctx, bucket, l.window).Err(); err != nil {
			return false, err
		}
	}
	return count <= int64(l.limit), nil
}
func (l *RedisRateLimiter) Close() error { return l.client.Close() }

func merchantFromContext(ctx context.Context) string {
	merchant, _ := ctx.Value(merchantContextKey).(string)
	return merchant
}
func requireMerchant(next http.Handler, authenticator MerchantAuthenticator, limiter RateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		merchant, ok := authenticator.Authenticate(r.Header.Get("x-api-key"))
		if !ok {
			write(w, http.StatusUnauthorized, map[string]string{"error": "valid x-api-key is required"})
			return
		}
		if merchant != "" {
			allowed, err := limiter.Allow(r.Context(), merchant)
			if err != nil {
				write(w, 503, map[string]string{"error": "rate limiter unavailable"})
				return
			}
			if !allowed {
				write(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), merchantContextKey, merchant))
		}
		next.ServeHTTP(w, r)
	})
}

var ErrMerchantMismatch = errors.New("merchant does not own payment")
