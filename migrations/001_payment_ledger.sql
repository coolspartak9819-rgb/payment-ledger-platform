CREATE TYPE payment_status AS ENUM ('created', 'authorized', 'captured', 'refunded', 'failed');
CREATE TABLE payments (
  id uuid PRIMARY KEY,
  merchant_id text NOT NULL,
  customer_id text NOT NULL,
  currency char(3) NOT NULL,
  amount numeric(20, 6) NOT NULL CHECK (amount > 0),
  status payment_status NOT NULL,
  provider_reference text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE idempotency_keys (
  key text PRIMARY KEY,
  payment_id uuid NOT NULL REFERENCES payments(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE ledger_entries (
  id uuid PRIMARY KEY,
  payment_id uuid NOT NULL REFERENCES payments(id),
  account_id text NOT NULL,
  currency char(3) NOT NULL,
  debit numeric(20, 6) NOT NULL DEFAULT 0 CHECK (debit >= 0),
  credit numeric(20, 6) NOT NULL DEFAULT 0 CHECK (credit >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((debit = 0) <> (credit = 0))
);
CREATE TABLE outbox_events (
  id uuid PRIMARY KEY,
  aggregate_id uuid NOT NULL,
  event_type text NOT NULL,
  payload jsonb NOT NULL,
  claimed_at timestamptz,
  published_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX outbox_unpublished_idx ON outbox_events (created_at) WHERE published_at IS NULL;
