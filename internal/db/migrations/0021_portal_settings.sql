-- Runtime-editable settings (spec 2026-07-13). One row per OVERRIDDEN key;
-- absence = env default; reset-to-env = DELETE. Secret values hold ciphertext
-- from the credential cipher, never plaintext.
CREATE TABLE IF NOT EXISTS portal_settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL
);

-- Audit trail: settings are portal-scoped, the app-scoped events table does
-- not fit; secrets are recorded as the literal string '(secret)'.
CREATE TABLE IF NOT EXISTS portal_settings_audit (
  id        BIGSERIAL PRIMARY KEY,
  key       TEXT NOT NULL,
  old_value TEXT,
  new_value TEXT,
  admin_id  BIGINT REFERENCES users(id) ON DELETE SET NULL,
  at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
