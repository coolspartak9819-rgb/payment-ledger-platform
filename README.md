# Payment Ledger Platform

Production-oriented payment orchestration and double-entry ledger service.

This project models the core of a real payment platform: every money movement
is idempotent, auditable and represented by balanced ledger entries. Payment
state transitions are guarded, retries are safe, and asynchronous side effects
are delivered through an outbox worker.

## Capabilities

- payment intents with idempotency keys;
- explicit state machine: `created`, `authorized`, `captured`, `refunded`, `failed`;
- double-entry ledger with debit/credit balance validation;
- transactional outbox for provider and webhook events;
- NATS JetStream publisher with broker-level event de-duplication;
- settlement reconciliation service for provider-report mismatches;
- merchant webhooks with HMAC signatures, retry/backoff and JetStream DLQ;
- merchant API keys, Redis-backed rate limiting and request trace IDs;
- atomic payment state/ledger/outbox commands and merchant payouts;
- payout terminal webhooks and dispute/chargeback ledger lifecycle;
- fraud-risk decision boundary and provider adapter interface;
- PostgreSQL persistence with an in-memory test store;
- reconciliation-ready settlement model;
- Prometheus metrics, health/readiness endpoints and structured errors;
- Docker Compose and Kubernetes manifests.

The API uses an in-memory store when `DATABASE_URL` is absent and switches to
PostgreSQL when it is configured. The separate worker atomically leases outbox
rows with `FOR UPDATE SKIP LOCKED`, publishes them, and marks them only after
delivery. An unacknowledged lease expires and can be retried after one minute.

## Local run

```bash
go test ./...
go vet ./...
docker compose up --build
go run ./scripts/load.go
sh scripts/e2e.sh
```

The load scenario defaults to 500 concurrent-safe create requests. Adjust it
with `TOTAL=5000 CONCURRENCY=100 go run ./scripts/load.go`. PostgreSQL
integration coverage is opt-in to keep normal unit tests deterministic:

```bash
TEST_DATABASE_URL=postgresql://payments:payments@localhost:5432/payments go test ./internal/store -run Postgres
```

Create an idempotent payment:

```bash
curl -X POST http://localhost:8080/v1/payments \
  -H 'content-type: application/json' \
  -H 'x-idempotency-key: order-123-payment' \
  -d '{"merchant_id":"merchant-demo","customer_id":"customer-1","currency":"USD","amount":"49.90"}'
```

The production design keeps provider credentials outside the repository and
expects a secret manager or sealed secrets in Kubernetes. The demo runtime
uses a deterministic provider adapter so the complete flow is reproducible.

## Architecture

`HTTP API -> payment service -> PostgreSQL transaction boundary -> ledger + outbox`

Authorization is intentionally synchronous because a caller needs a final
decision. Webhooks, notifications and downstream settlement integrations are
asynchronous outbox consumers. This avoids the common failure mode where a
payment is committed but the event is lost between database commit and broker
publication.

The database constraint on `idempotency_keys.key` turns network retries into a
replay of the original payment. Each authorization adds exactly two balanced
ledger entries: a debit from the customer pending account and a credit to the
merchant pending account. A production provider adapter would use its own
idempotency key and signed webhooks before moving funds to `captured`.

When `NATS_URL` is configured, the worker publishes outbox events to the
`payments.events` JetStream subject with the outbox event ID as `Nats-Msg-Id`.
JetStream then suppresses duplicate broker publishes during retry windows.

`ReconciliationService` compares provider settlement lines with captured ledger
amounts and returns explicit mismatches such as unknown provider references or
amount differences. In a production deployment these mismatches feed a review
queue rather than being silently corrected.

Set `MERCHANT_WEBHOOK_URL` and `MERCHANT_WEBHOOK_SECRET` on the worker to
deliver payment events to a merchant endpoint. Delivery retries three times
with bounded backoff. Failures are retained in `payments.webhooks.dlq` for
review, while normal payment events remain in `payments.events`.

## Operations and access

Set `MERCHANT_API_KEYS=key:merchant-id,key:merchant-id` to require a merchant
API key on payment endpoints. The authenticated key must match the request
`merchant_id`, and it cannot read or mutate another merchant's payment.
`REDIS_URL` enables a distributed fixed-window rate limiter; the API falls
back to in-memory limiting only for local development when Redis is absent.

Use `ADMIN_API_KEY` on `POST /v1/admin/reconciliation` to submit provider
settlement rows. The endpoint returns mismatches for a review workflow. Every
HTTP response carries `x-request-id`; an inbound W3C `traceparent` is echoed
to allow trace correlation across service boundaries.

## Atomic money movement and payouts

Payment authorization, capture and refund now commit the payment state, the
balanced ledger entries and the corresponding outbox event in one PostgreSQL
transaction. A failure rolls back all three records together.

`POST /v1/payouts` moves funds from `merchant:<id>:available` to
`merchant:<id>:payout_pending`. It is merchant-key protected and uses a
PostgreSQL advisory transaction lock keyed by merchant and currency, preventing
concurrent payout requests from overspending the available ledger balance.

Provider callbacks reach `POST /v1/provider/payout-webhooks` and use the same
HMAC and event de-duplication controls as payment webhooks. A failed payout
atomically reverses its reservation back to merchant available balance.

Admins can open and resolve disputes using the admin API. Opening a dispute
moves the disputed amount to `dispute_frozen`; a merchant win releases it back
to available balance, while a loss moves it to the chargeback account. Every
transition writes its balanced entries and outbox event in the same transaction.

## Kubernetes

Apply the files under `deploy/k8s` after supplying a real secret:

```bash
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/secret.example.yaml
kubectl apply -f deploy/k8s/api.yaml -f deploy/k8s/worker.yaml
```
