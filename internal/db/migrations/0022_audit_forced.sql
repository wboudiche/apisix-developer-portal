-- Follow-up (post-review): a settings change persisted despite a failing
-- probe (force=true) was indistinguishable from a clean save in the audit
-- trail. Record it explicitly.
ALTER TABLE portal_settings_audit ADD COLUMN IF NOT EXISTS forced BOOLEAN NOT NULL DEFAULT FALSE;
