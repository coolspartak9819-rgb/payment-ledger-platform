ALTER TABLE payouts ADD COLUMN provider_reference text;
CREATE TYPE dispute_status AS ENUM ('open', 'won', 'lost');
CREATE TABLE disputes (
  id uuid PRIMARY KEY,
  payment_id uuid NOT NULL REFERENCES payments(id),
  merchant_id text NOT NULL,
  currency char(3) NOT NULL,
  amount numeric(20, 6) NOT NULL CHECK (amount > 0),
  reason text NOT NULL,
  status dispute_status NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX disputes_merchant_status_idx ON disputes (merchant_id, status);
