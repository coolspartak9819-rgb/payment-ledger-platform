CREATE TYPE payout_status AS ENUM ('pending', 'paid', 'failed');
CREATE TABLE payouts (
  id uuid PRIMARY KEY,
  merchant_id text NOT NULL,
  currency char(3) NOT NULL,
  amount numeric(20, 6) NOT NULL CHECK (amount > 0),
  status payout_status NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE ledger_entries ALTER COLUMN payment_id DROP NOT NULL;
CREATE INDEX payouts_merchant_status_idx ON payouts (merchant_id, status);
