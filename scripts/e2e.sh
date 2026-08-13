#!/usr/bin/env sh
set -eu

base_url="${BASE_URL:-http://localhost:8080}"
key="e2e-payment-$(date +%s)"
payload='{"merchant_id":"e2e-merchant","customer_id":"e2e-customer","currency":"USD","amount":"49.90"}'

created="$(curl -fsS -X POST "$base_url/v1/payments" -H 'content-type: application/json' -H "x-idempotency-key: $key" -d "$payload")"
payment_id="$(printf '%s' "$created" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
[ -n "$payment_id" ]
curl -fsS -X POST "$base_url/v1/payments/$payment_id/authorize" >/dev/null
curl -fsS -X POST "$base_url/v1/payments/$payment_id/capture" -H 'content-type: application/json' -d '{"amount":"30.00"}' >/dev/null
curl -fsS -X POST "$base_url/v1/payments/$payment_id/refund" -H 'content-type: application/json' -d '{"amount":"10.00"}' >/dev/null
printf 'e2e payment lifecycle passed: %s\n' "$payment_id"
