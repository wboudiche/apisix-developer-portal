CREATE TABLE teams (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    personal   BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE team_members (
    team_id    BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('owner','member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);

ALTER TABLE applications ADD COLUMN team_id BIGINT REFERENCES teams(id);

-- Backfill: one personal team per existing user; point their apps at it.
DO $$
DECLARE
    u   RECORD;
    tid BIGINT;
BEGIN
    FOR u IN SELECT id, COALESCE(NULLIF(name, ''), email) AS nm FROM users LOOP
        INSERT INTO teams(name, personal) VALUES (u.nm, true) RETURNING id INTO tid;
        INSERT INTO team_members(team_id, user_id, role) VALUES (tid, u.id, 'owner');
        UPDATE applications SET team_id = tid WHERE owner_id = u.id;
    END LOOP;
END $$;

ALTER TABLE applications ALTER COLUMN team_id SET NOT NULL;

CREATE INDEX idx_team_members_user ON team_members(user_id);
CREATE INDEX idx_applications_team ON applications(team_id);
