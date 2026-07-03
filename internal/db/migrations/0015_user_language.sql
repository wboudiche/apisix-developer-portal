ALTER TABLE users
  ADD COLUMN language TEXT NOT NULL DEFAULT 'fr'
  CHECK (language IN ('fr','en'));
