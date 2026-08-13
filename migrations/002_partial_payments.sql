ALTER TABLE payments ADD COLUMN captured_amount numeric(20, 6) NOT NULL DEFAULT 0 CHECK (captured_amount >= 0);
ALTER TABLE payments ADD COLUMN refunded_amount numeric(20, 6) NOT NULL DEFAULT 0 CHECK (refunded_amount >= 0);
ALTER TABLE payments ADD CONSTRAINT payment_amount_invariants CHECK (refunded_amount <= captured_amount AND captured_amount <= amount);
