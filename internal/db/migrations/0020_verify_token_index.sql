-- Verification lookups scan by token hash; partial index keeps it O(log n)
-- and tiny (only unverified accounts carry a token).
CREATE INDEX IF NOT EXISTS users_verify_token_hash_idx
  ON users (verify_token_hash) WHERE verify_token_hash IS NOT NULL;
