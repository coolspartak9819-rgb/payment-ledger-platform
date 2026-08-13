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

## Kubernetes

Apply the files under `deploy/k8s` after supplying a real secret:

```bash
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/secret.example.yaml
kubectl apply -f deploy/k8s/api.yaml -f deploy/k8s/worker.yaml
```
