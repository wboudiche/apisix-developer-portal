-- Activity log for the application Overview feed. Append-only; one row per
-- lifecycle action (app created, subscribed, approved, rejected, unsubscribed).
-- product_id/plan_id are loose references (no FK) so an event survives the
-- product/plan being deleted; names are LEFT JOIN-resolved at read time and
-- fall back to empty when the referenced row is gone.
CREATE TABLE IF NOT EXISTS app_events (
  id             BIGSERIAL PRIMARY KEY,
  application_id BIGINT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
  kind           TEXT   NOT NULL,
  product_id     BIGINT,
  plan_id        BIGINT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Feed reads are always "latest N for this app", so index that access path.
CREATE INDEX IF NOT EXISTS app_events_app_idx ON app_events (application_id, created_at DESC);
