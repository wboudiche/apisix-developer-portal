ALTER TABLE plans
  ADD COLUMN price_cents INTEGER NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
  ADD COLUMN currency TEXT NOT NULL DEFAULT 'EUR';

CREATE TABLE billing_accounts (
  id         BIGSERIAL PRIMARY KEY,
  team_id    BIGINT NOT NULL UNIQUE REFERENCES teams(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE invoices (
  id                 BIGSERIAL PRIMARY KEY,
  billing_account_id BIGINT NOT NULL REFERENCES billing_accounts(id) ON DELETE CASCADE,
  team_id            BIGINT NOT NULL,
  subscription_id    BIGINT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
  plan_name          TEXT NOT NULL,
  price_cents        INTEGER NOT NULL,
  currency           TEXT NOT NULL,
  status             TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','paid','void')),
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  paid_at            TIMESTAMPTZ NULL
);
CREATE INDEX idx_invoices_team_created ON invoices(team_id, created_at DESC);
