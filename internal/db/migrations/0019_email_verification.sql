-- Email verification (opt-in via REQUIRE_EMAIL_VERIFICATION). DEFAULT TRUE
-- grandfathers every existing account; the register path explicitly inserts
-- FALSE when the feature is enabled.
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT TRUE,
  ADD COLUMN IF NOT EXISTS verify_token_hash TEXT,
  ADD COLUMN IF NOT EXISTS verify_token_expires_at TIMESTAMPTZ;
