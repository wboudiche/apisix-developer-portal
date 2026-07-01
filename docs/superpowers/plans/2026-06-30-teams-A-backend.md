# Teams / Organizations — Plan A (Backend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a **team** the unit of application ownership — every user has a personal team, apps belong to teams, and every "is this yours?" gate becomes team membership — with a one-time backfill of existing data.

**Architecture:** A new `internal/teams` package (Repo + Service + Handler) owns teams + memberships. A migration adds `teams`/`team_members`/`applications.team_id` and backfills a personal team per user. Registration creates a personal team in the same tx. Applications become team-scoped; the `owns` closure and try-it adapter switch to `teams.IsMemberOfApp`; approval emails go to the team's owners.

**Tech Stack:** Go 1.25, pgx/pgxpool, chi, stdlib. Postgres.

## Global Constraints

- Module `apisix-portal`. New package `internal/teams`. Touches `internal/db/migrations`, `internal/auth`, `internal/applications`, `internal/subscriptions` (view only), `internal/notify`, `internal/server`.
- **Roles:** `owner` | `member`. Team management (add/remove member, rename, delete) is **owner-only**. All app/subscription actions require **membership** (owner or member).
- **Personal team:** every user has exactly one `personal=true` team (a team of one, `owner`). Personal teams cannot be renamed, deleted, or have members added/removed.
- **Ownership predicate:** `teams.IsMemberOfApp(userID, appID)` replaces every `owner_id == userID` check. `applications.owner_id` is retained as "created_by" (do not drop the column).
- **Add-member:** by the email of an **already-registered** user; immediate (no accept). 404 unknown email, 409 already-member / personal team.
- Migrations run once each, in filename order, via `internal/db/migrate.go` (tracked in a migrations table). SQL files live in `internal/db/migrations/`.
- Tests: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/...`; `gofmt -w` every touched file; `go vet ./...`.

---

## Task 1: Migration `0013_teams.sql` + backfill

**Files:**
- Create: `internal/db/migrations/0013_teams.sql`
- Test: `internal/db/migrate_teams_test.go`

**Interfaces:**
- Produces: tables `teams(id,name,personal,created_at)`, `team_members(team_id,user_id,role,created_at)`, column `applications.team_id BIGINT NOT NULL REFERENCES teams(id)`; every pre-existing user has one `personal` team (as `owner`); every pre-existing app has `team_id` = its owner's personal team.

- [ ] **Step 1: Write the failing backfill test**

Create `internal/db/migrate_teams_test.go`:
```go
package db

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestTeamsBackfill(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Seed a user + app the OLD way (no team), then re-run the backfill logic is
	// not possible post-migration; instead assert the invariant holds for a
	// freshly-created user+app going through the app (covered in later tasks).
	// Here assert schema + that every existing user has exactly one personal team.
	suf := time.Now().Format("150405.000000000")
	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name,role) VALUES($1,'x','U','developer') RETURNING id`,
		"bf+"+suf+"@e.com").Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid) })
	// The backfill only covers users that existed at migration time; a user
	// inserted now has NO personal team yet (registration creates it — Task 3).
	// So assert the schema objects exist and are usable:
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM teams`).Scan(&n); err != nil {
		t.Fatalf("teams table: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM team_members`).Scan(&n); err != nil {
		t.Fatalf("team_members table: %v", err)
	}
	// applications.team_id exists and is NOT NULL
	var notnull bool
	if err := pool.QueryRow(ctx,
		`SELECT attnotnull FROM pg_attribute WHERE attrelid='applications'::regclass AND attname='team_id'`).
		Scan(&notnull); err != nil {
		t.Fatalf("team_id column: %v", err)
	}
	if !notnull {
		t.Error("applications.team_id should be NOT NULL")
	}
	// Every user that existed before this migration must have a personal team.
	// (Users created after migration get theirs at registration.) Assert no
	// pre-existing app is left without a team:
	var orphanApps int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM applications WHERE team_id IS NULL`).Scan(&orphanApps); err != nil {
		t.Fatalf("orphan apps: %v", err)
	}
	if orphanApps != 0 {
		t.Errorf("apps without team_id: %d", orphanApps)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/db/ -run TestTeamsBackfill -v`
Expected: FAIL — `teams` table / `team_id` column don't exist.

- [ ] **Step 3: Write the migration**

Create `internal/db/migrations/0013_teams.sql`:
```sql
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
```

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/db/ -run TestTeamsBackfill -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/0013_teams.sql internal/db/migrate_teams_test.go
git commit -m "feat(teams): migration + personal-team backfill"
```

---

## Task 2: `teams.Repo`

**Files:**
- Create: `internal/teams/team.go` (types), `internal/teams/repo.go`, `internal/teams/errors.go`
- Test: `internal/teams/repo_test.go`

**Interfaces:**
- Produces (types): `Team{ID int64; Name string; Personal bool; CreatedAt time.Time}`, `TeamSummary{ID int64; Name string; Personal bool; Role string; MemberCount int}`, `Member{UserID int64; Email, Name, Role string}`.
- Produces (errors): `ErrUserNotFound`, `ErrAlreadyMember`, `ErrPersonalTeam`, `ErrLastOwner`, `ErrTeamHasApps`, `ErrNotFound`.
- Produces (on `*Repo`, `func NewRepo(pool *pgxpool.Pool) *Repo`):
  - `Create(ctx, name string, ownerUserID int64) (Team, error)`
  - `ListForUser(ctx, userID int64) ([]TeamSummary, error)`
  - `Get(ctx, teamID int64) (Team, error)`
  - `Members(ctx, teamID int64) ([]Member, error)`
  - `Role(ctx, teamID, userID int64) (role string, isMember bool, err error)`
  - `PersonalTeamID(ctx, userID int64) (int64, error)`
  - `IsMemberOfApp(ctx, userID, appID int64) (bool, error)`
  - `AddMemberByEmail(ctx, teamID int64, email string) error`
  - `RemoveMember(ctx, teamID, userID int64) error`
  - `Rename(ctx, teamID int64, name string) error`
  - `Delete(ctx, teamID int64) error`
  - `OwnerEmailsForApp(ctx, appID int64) (emails []string, appName string, err error)`

- [ ] **Step 1: Write the failing test**

Create `internal/teams/repo_test.go`:
```go
package teams

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"
)

func testRepo(t *testing.T) (context.Context, *Repo, int64, int64) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	suf := time.Now().Format("150405.000000000")
	var u1, u2 int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name,role) VALUES($1,'x','Alice','developer') RETURNING id`, "a+"+suf+"@e.com").Scan(&u1); err != nil {
		t.Fatalf("seed u1: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name,role) VALUES($1,'x','Bob','developer') RETURNING id`, "b+"+suf+"@e.com").Scan(&u2); err != nil {
		t.Fatalf("seed u2: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1,$2)`, u1, u2) })
	return ctx, NewRepo(pool), u1, u2
}

func TestCreateAndListForUser(t *testing.T) {
	ctx, repo, u1, _ := testRepo(t)
	team, err := repo.Create(ctx, "Acme", u1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM teams WHERE id=$1`, team.ID) })
	teams, err := repo.ListForUser(ctx, u1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var found *TeamSummary
	for i := range teams {
		if teams[i].ID == team.ID {
			found = &teams[i]
		}
	}
	if found == nil || found.Role != "owner" || found.MemberCount != 1 {
		t.Fatalf("listForUser = %+v", teams)
	}
}

func TestAddRemoveMember(t *testing.T) {
	ctx, repo, u1, u2 := testRepo(t)
	team, _ := repo.Create(ctx, "Acme", u1)
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM teams WHERE id=$1`, team.ID) })
	bobEmail := mustEmail(t, ctx, repo, u2)
	if err := repo.AddMemberByEmail(ctx, team.ID, bobEmail); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := repo.AddMemberByEmail(ctx, team.ID, bobEmail); !errors.Is(err, ErrAlreadyMember) {
		t.Fatalf("dup add err = %v, want ErrAlreadyMember", err)
	}
	if err := repo.AddMemberByEmail(ctx, team.ID, "nobody@nowhere.test"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown add err = %v, want ErrUserNotFound", err)
	}
	role, ok, _ := repo.Role(ctx, team.ID, u2)
	if !ok || role != "member" {
		t.Fatalf("bob role = %q,%v", role, ok)
	}
	if err := repo.RemoveMember(ctx, team.ID, u2); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := repo.RemoveMember(ctx, team.ID, u1); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("remove last owner err = %v, want ErrLastOwner", err)
	}
}

func TestIsMemberOfAppAndOwnerEmails(t *testing.T) {
	ctx, repo, u1, u2 := testRepo(t)
	team, _ := repo.Create(ctx, "Acme", u1)
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM teams WHERE id=$1`, team.ID) })
	var appID int64
	if err := repo.pool.QueryRow(ctx, `INSERT INTO applications(owner_id,name,team_id) VALUES($1,'App',$2) RETURNING id`, u1, team.ID).Scan(&appID); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM applications WHERE id=$1`, appID) })
	if ok, _ := repo.IsMemberOfApp(ctx, u1, appID); !ok {
		t.Error("owner should be member of app")
	}
	if ok, _ := repo.IsMemberOfApp(ctx, u2, appID); ok {
		t.Error("non-member should not be member of app")
	}
	emails, name, err := repo.OwnerEmailsForApp(ctx, appID)
	if err != nil || name != "App" || len(emails) != 1 {
		t.Fatalf("ownerEmails = %v,%q,%v", emails, name, err)
	}
}

func TestDeleteRejectsTeamWithApps(t *testing.T) {
	ctx, repo, u1, _ := testRepo(t)
	team, _ := repo.Create(ctx, "Acme", u1)
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM applications WHERE team_id=$1`, team.ID); _, _ = repo.pool.Exec(ctx, `DELETE FROM teams WHERE id=$1`, team.ID) })
	var appID int64
	repo.pool.QueryRow(ctx, `INSERT INTO applications(owner_id,name,team_id) VALUES($1,'App',$2) RETURNING id`, u1, team.ID).Scan(&appID)
	if err := repo.Delete(ctx, team.ID); !errors.Is(err, ErrTeamHasApps) {
		t.Fatalf("delete with apps err = %v, want ErrTeamHasApps", err)
	}
}

func mustEmail(t *testing.T, ctx context.Context, repo *Repo, uid int64) string {
	t.Helper()
	var e string
	if err := repo.pool.QueryRow(ctx, `SELECT email FROM users WHERE id=$1`, uid).Scan(&e); err != nil {
		t.Fatalf("email: %v", err)
	}
	return e
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/teams/ -v`
Expected: FAIL — package/types/methods undefined.

- [ ] **Step 3: Implement the types + errors + repo**

Create `internal/teams/team.go`:
```go
package teams

import "time"

type Team struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Personal  bool      `json:"personal"`
	CreatedAt time.Time `json:"createdAt"`
}

type TeamSummary struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Personal    bool   `json:"personal"`
	Role        string `json:"role"`
	MemberCount int    `json:"memberCount"`
}

type Member struct {
	UserID int64  `json:"userId"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}
```

Create `internal/teams/errors.go`:
```go
package teams

import "errors"

var (
	ErrNotFound      = errors.New("team not found")
	ErrUserNotFound  = errors.New("user not found")
	ErrAlreadyMember = errors.New("already a member")
	ErrPersonalTeam  = errors.New("personal team cannot be modified")
	ErrLastOwner     = errors.New("cannot remove the last owner")
	ErrTeamHasApps   = errors.New("team still has applications")
)
```

Create `internal/teams/repo.go`:
```go
package teams

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Create makes a non-personal team with the given user as its sole owner.
func (r *Repo) Create(ctx context.Context, name string, ownerUserID int64) (Team, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Team{}, err
	}
	defer tx.Rollback(ctx)
	var t Team
	if err := tx.QueryRow(ctx,
		`INSERT INTO teams(name, personal) VALUES($1, false) RETURNING id, name, personal, created_at`, name).
		Scan(&t.ID, &t.Name, &t.Personal, &t.CreatedAt); err != nil {
		return Team{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO team_members(team_id, user_id, role) VALUES($1,$2,'owner')`, t.ID, ownerUserID); err != nil {
		return Team{}, err
	}
	return t, tx.Commit(ctx)
}

func (r *Repo) ListForUser(ctx context.Context, userID int64) ([]TeamSummary, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT t.id, t.name, t.personal, tm.role,
		        (SELECT count(*) FROM team_members m WHERE m.team_id = t.id) AS member_count
		 FROM teams t
		 JOIN team_members tm ON tm.team_id = t.id AND tm.user_id = $1
		 ORDER BY t.personal DESC, t.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeamSummary
	for rows.Next() {
		var s TeamSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Personal, &s.Role, &s.MemberCount); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repo) Get(ctx context.Context, teamID int64) (Team, error) {
	var t Team
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, personal, created_at FROM teams WHERE id=$1`, teamID).
		Scan(&t.ID, &t.Name, &t.Personal, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Team{}, ErrNotFound
	}
	return t, err
}

func (r *Repo) Members(ctx context.Context, teamID int64) ([]Member, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT u.id, u.email, u.name, tm.role
		 FROM team_members tm JOIN users u ON u.id = tm.user_id
		 WHERE tm.team_id = $1 ORDER BY tm.role, u.email`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repo) Role(ctx context.Context, teamID, userID int64) (string, bool, error) {
	var role string
	err := r.pool.QueryRow(ctx,
		`SELECT role FROM team_members WHERE team_id=$1 AND user_id=$2`, teamID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return role, true, nil
}

func (r *Repo) PersonalTeamID(ctx context.Context, userID int64) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`SELECT t.id FROM teams t JOIN team_members tm ON tm.team_id=t.id
		 WHERE tm.user_id=$1 AND t.personal ORDER BY t.id LIMIT 1`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

func (r *Repo) IsMemberOfApp(ctx context.Context, userID, appID int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM applications a
		   JOIN team_members tm ON tm.team_id = a.team_id
		   WHERE a.id = $1 AND tm.user_id = $2)`, appID, userID).Scan(&exists)
	return exists, err
}

func (r *Repo) AddMemberByEmail(ctx context.Context, teamID int64, email string) error {
	personal, err := r.isPersonal(ctx, teamID)
	if err != nil {
		return err
	}
	if personal {
		return ErrPersonalTeam
	}
	var uid int64
	err = r.pool.QueryRow(ctx, `SELECT id FROM users WHERE email=$1`, email).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO team_members(team_id, user_id, role) VALUES($1,$2,'member')`, teamID, uid)
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
		return ErrAlreadyMember
	}
	return err
}

func (r *Repo) RemoveMember(ctx context.Context, teamID, userID int64) error {
	personal, err := r.isPersonal(ctx, teamID)
	if err != nil {
		return err
	}
	if personal {
		return ErrPersonalTeam
	}
	// Refuse to remove the last owner.
	var role string
	err = r.pool.QueryRow(ctx, `SELECT role FROM team_members WHERE team_id=$1 AND user_id=$2`, teamID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already not a member
	}
	if err != nil {
		return err
	}
	if role == "owner" {
		var owners int
		if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM team_members WHERE team_id=$1 AND role='owner'`, teamID).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return ErrLastOwner
		}
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM team_members WHERE team_id=$1 AND user_id=$2`, teamID, userID)
	return err
}

func (r *Repo) Rename(ctx context.Context, teamID int64, name string) error {
	personal, err := r.isPersonal(ctx, teamID)
	if err != nil {
		return err
	}
	if personal {
		return ErrPersonalTeam
	}
	_, err = r.pool.Exec(ctx, `UPDATE teams SET name=$1 WHERE id=$2`, name, teamID)
	return err
}

func (r *Repo) Delete(ctx context.Context, teamID int64) error {
	personal, err := r.isPersonal(ctx, teamID)
	if err != nil {
		return err
	}
	if personal {
		return ErrPersonalTeam
	}
	var apps int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM applications WHERE team_id=$1`, teamID).Scan(&apps); err != nil {
		return err
	}
	if apps > 0 {
		return ErrTeamHasApps
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM teams WHERE id=$1`, teamID) // members cascade
	return err
}

func (r *Repo) OwnerEmailsForApp(ctx context.Context, appID int64) ([]string, string, error) {
	var appName string
	if err := r.pool.QueryRow(ctx, `SELECT name FROM applications WHERE id=$1`, appID).Scan(&appName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", nil
		}
		return nil, "", err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT u.email FROM applications a
		 JOIN team_members tm ON tm.team_id = a.team_id AND tm.role='owner'
		 JOIN users u ON u.id = tm.user_id
		 WHERE a.id = $1`, appID)
	if err != nil {
		return nil, appName, err
	}
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, appName, err
		}
		emails = append(emails, e)
	}
	return emails, appName, rows.Err()
}

func (r *Repo) isPersonal(ctx context.Context, teamID int64) (bool, error) {
	var personal bool
	err := r.pool.QueryRow(ctx, `SELECT personal FROM teams WHERE id=$1`, teamID).Scan(&personal)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return personal, err
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/teams/ && go vet ./internal/teams/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/teams/
git commit -m "feat(teams): repo — teams, membership, app-ownership predicate"
```

---

## Task 3: Personal team on registration

**Files:**
- Modify: `internal/auth/repo.go`
- Test: `internal/auth/repo_test.go` (create if absent)

**Interfaces:**
- Consumes: the `teams`/`team_members` tables (Task 1).
- Produces: `auth.Repo.Create` now also creates the user's personal team + owner membership in the same tx (signature unchanged).

- [ ] **Step 1: Write the failing test**

Create/append `internal/auth/repo_test.go`:
```go
package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"
)

func TestCreateAlsoCreatesPersonalTeam(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewRepo(pool)
	suf := time.Now().Format("150405.000000000")
	u, err := repo.Create(ctx, "reg+"+suf+"@e.com", "hash", "Reg User")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID) })
	var personalTeams int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM teams t JOIN team_members tm ON tm.team_id=t.id
		 WHERE tm.user_id=$1 AND tm.role='owner' AND t.personal`, u.ID).Scan(&personalTeams); err != nil {
		t.Fatalf("query: %v", err)
	}
	if personalTeams != 1 {
		t.Errorf("personal teams for new user = %d, want 1", personalTeams)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/auth/ -run TestCreateAlsoCreatesPersonalTeam -v`
Expected: FAIL — no personal team created (count 0).

- [ ] **Step 3: Rewrite `Create` to a transaction**

In `internal/auth/repo.go`, replace `Create` with (keep the `ErrEmailTaken` mapping):
```go
// Create inserts a developer user AND their personal team (a team of one) in a
// single transaction, returning the user.
func (r *Repo) Create(ctx context.Context, email, passwordHash, name string) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var u User
	err = tx.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, role)
		 VALUES ($1,$2,$3,'developer')
		 RETURNING id, email, name, role`,
		email, passwordHash, name,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	teamName := name
	if teamName == "" {
		teamName = email
	}
	var teamID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO teams(name, personal) VALUES($1, true) RETURNING id`, teamName).Scan(&teamID); err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO team_members(team_id, user_id, role) VALUES($1,$2,'owner')`, teamID, u.ID); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return u, nil
}
```
(`pgconn`/`errors` are already imported; if `pgconn` is not, add `"github.com/jackc/pgx/v5/pgconn"`.)

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/auth/ && go vet ./internal/auth/`
Expected: PASS (existing auth tests still pass).

- [ ] **Step 5: Commit**

```bash
git add internal/auth/repo.go internal/auth/repo_test.go
git commit -m "feat(teams): create personal team on registration"
```

---

## Task 4: `teams` Service + Handler + routes

**Files:**
- Create: `internal/teams/service.go`, `internal/teams/handler.go`
- Test: `internal/teams/handler_test.go`

**Interfaces:**
- Consumes: `Repo` (Task 2) + `auth.UserID(ctx)`.
- Produces: `func NewHandler(repo Store) *Handler` with `ServeHTTP`; a `Store` interface over the repo methods used. Routes (chi):
  - `GET /api/teams`, `POST /api/teams`, `GET /api/teams/{id}/members`,
    `POST /api/teams/{id}/members`, `DELETE /api/teams/{id}/members/{userId}`,
    `PATCH /api/teams/{id}`, `DELETE /api/teams/{id}`.

- [ ] **Step 1: Write the failing handler test**

Create `internal/teams/handler_test.go` (use a fake `Store` so it's DB-free; mirror `applications/handler_test.go` style with `auth.WithUser`/the context helper the codebase uses — check `internal/auth` for the context setter used in other handler tests):
```go
package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/auth"
)

type fakeStore struct {
	listed   []TeamSummary
	created  string
	addEmail string
	addErr   error
	roleFn   func(teamID, userID int64) (string, bool, error)
}

func (f *fakeStore) ListForUser(_ context.Context, _ int64) ([]TeamSummary, error) { return f.listed, nil }
func (f *fakeStore) Create(_ context.Context, name string, _ int64) (Team, error) {
	f.created = name
	return Team{ID: 1, Name: name}, nil
}
func (f *fakeStore) Members(_ context.Context, _ int64) ([]Member, error) { return nil, nil }
func (f *fakeStore) Role(_ context.Context, teamID, userID int64) (string, bool, error) {
	if f.roleFn != nil {
		return f.roleFn(teamID, userID)
	}
	return "owner", true, nil
}
func (f *fakeStore) AddMemberByEmail(_ context.Context, _ int64, email string) error {
	f.addEmail = email
	return f.addErr
}
func (f *fakeStore) RemoveMember(_ context.Context, _, _ int64) error { return nil }
func (f *fakeStore) Rename(_ context.Context, _ int64, _ string) error { return nil }
func (f *fakeStore) Delete(_ context.Context, _ int64) error           { return nil }

func withUser(r *http.Request, uid int64) *http.Request {
	return r.WithContext(auth.WithUser(r.Context(), uid, "u@e.com", "developer"))
}

func TestListTeams(t *testing.T) {
	f := &fakeStore{listed: []TeamSummary{{ID: 1, Name: "Personal", Personal: true, Role: "owner", MemberCount: 1}}}
	h := NewHandler(f)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, withUser(httptest.NewRequest("GET", "/api/teams", nil), 7))
	if rr.Code != 200 {
		t.Fatalf("code=%d", rr.Code)
	}
	var got []TeamSummary
	json.Unmarshal(rr.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Name != "Personal" {
		t.Fatalf("got %+v", got)
	}
}

func TestCreateTeam(t *testing.T) {
	f := &fakeStore{}
	h := NewHandler(f)
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"name":"Acme"}`)
	h.ServeHTTP(rr, withUser(httptest.NewRequest("POST", "/api/teams", body), 7))
	if rr.Code != 201 || f.created != "Acme" {
		t.Fatalf("code=%d created=%q", rr.Code, f.created)
	}
}

func TestAddMemberOwnerOnly(t *testing.T) {
	f := &fakeStore{roleFn: func(_, _ int64) (string, bool, error) { return "member", true, nil }}
	h := NewHandler(f)
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"email":"x@e.com"}`)
	h.ServeHTTP(rr, withUser(httptest.NewRequest("POST", "/api/teams/1/members", body), 7))
	if rr.Code != 403 {
		t.Fatalf("member adding member: code=%d, want 403", rr.Code)
	}
}

func TestAddMemberUnknownEmail404(t *testing.T) {
	f := &fakeStore{addErr: ErrUserNotFound}
	h := NewHandler(f)
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"email":"nobody@e.com"}`)
	h.ServeHTTP(rr, withUser(httptest.NewRequest("POST", "/api/teams/1/members", body), 7))
	if rr.Code != 404 {
		t.Fatalf("code=%d, want 404", rr.Code)
	}
}
```
**NOTE for the implementer:** verify the exact auth context setter — grep `internal/auth` for the helper used in other packages' handler tests (e.g. `auth.WithUser`); use whatever the codebase actually exports. If the setter differs, adjust `withUser` accordingly.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/teams/ -run 'TestListTeams|TestCreateTeam|TestAddMember' -v`
Expected: FAIL — `NewHandler`/`Store` undefined.

- [ ] **Step 3: Implement the Store interface, Service rules, and Handler**

Create `internal/teams/service.go`:
```go
package teams

import "context"

// Store is the subset of *Repo the handler needs (a fake satisfies it in tests).
type Store interface {
	ListForUser(ctx context.Context, userID int64) ([]TeamSummary, error)
	Create(ctx context.Context, name string, ownerUserID int64) (Team, error)
	Members(ctx context.Context, teamID int64) ([]Member, error)
	Role(ctx context.Context, teamID, userID int64) (string, bool, error)
	AddMemberByEmail(ctx context.Context, teamID int64, email string) error
	RemoveMember(ctx context.Context, teamID, userID int64) error
	Rename(ctx context.Context, teamID int64, name string) error
	Delete(ctx context.Context, teamID int64) error
}
```

Create `internal/teams/handler.go`:
```go
package teams

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

type Handler struct {
	store  Store
	router chi.Router
}

func NewHandler(store Store) *Handler {
	h := &Handler{store: store, router: chi.NewRouter()}
	h.router.Get("/api/teams", h.list)
	h.router.Post("/api/teams", h.create)
	h.router.Get("/api/teams/{id}/members", h.members)
	h.router.Post("/api/teams/{id}/members", h.addMember)
	h.router.Delete("/api/teams/{id}/members/{userId}", h.removeMember)
	h.router.Patch("/api/teams/{id}", h.rename)
	h.router.Delete("/api/teams/{id}", h.deleteTeam)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ts, err := h.store.ListForUser(r.Context(), auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not list teams")
		return
	}
	if ts == nil {
		ts = []TeamSummary{}
	}
	httpx.JSON(w, http.StatusOK, ts)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct{ Name string `json:"name"` }
	if err := httpx.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	t, err := h.store.Create(r.Context(), strings.TrimSpace(body.Name), auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not create team")
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

func (h *Handler) members(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	ms, err := h.store.Members(r.Context(), teamID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not list members")
		return
	}
	if ms == nil {
		ms = []Member{}
	}
	httpx.JSON(w, http.StatusOK, ms)
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.requireOwner(w, r)
	if !ok {
		return
	}
	var body struct{ Email string `json:"email"` }
	if err := httpx.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Email) == "" {
		httpx.Error(w, http.StatusBadRequest, "email is required")
		return
	}
	err := h.store.AddMemberByEmail(r.Context(), teamID, strings.TrimSpace(body.Email))
	switch {
	case errors.Is(err, ErrUserNotFound):
		httpx.Error(w, http.StatusNotFound, "no user with that email")
	case errors.Is(err, ErrAlreadyMember):
		httpx.Error(w, http.StatusConflict, "already a member")
	case errors.Is(err, ErrPersonalTeam):
		httpx.Error(w, http.StatusConflict, "cannot add members to a personal team")
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "could not add member")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.requireOwner(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad user id")
		return
	}
	err = h.store.RemoveMember(r.Context(), teamID, userID)
	switch {
	case errors.Is(err, ErrLastOwner):
		httpx.Error(w, http.StatusConflict, "cannot remove the last owner")
	case errors.Is(err, ErrPersonalTeam):
		httpx.Error(w, http.StatusConflict, "cannot modify a personal team")
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "could not remove member")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) rename(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.requireOwner(w, r)
	if !ok {
		return
	}
	var body struct{ Name string `json:"name"` }
	if err := httpx.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	err := h.store.Rename(r.Context(), teamID, strings.TrimSpace(body.Name))
	switch {
	case errors.Is(err, ErrPersonalTeam):
		httpx.Error(w, http.StatusConflict, "cannot rename a personal team")
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "could not rename team")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) deleteTeam(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.requireOwner(w, r)
	if !ok {
		return
	}
	err := h.store.Delete(r.Context(), teamID)
	switch {
	case errors.Is(err, ErrTeamHasApps):
		httpx.Error(w, http.StatusConflict, "team still has applications")
	case errors.Is(err, ErrPersonalTeam):
		httpx.Error(w, http.StatusConflict, "cannot delete a personal team")
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "could not delete team")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) teamID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil
}

func (h *Handler) requireMember(w http.ResponseWriter, r *http.Request) (int64, bool) {
	teamID, ok := h.teamID(r)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "bad team id")
		return 0, false
	}
	_, isMember, err := h.store.Role(r.Context(), teamID, auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not check membership")
		return 0, false
	}
	if !isMember {
		httpx.Error(w, http.StatusForbidden, "not a team member")
		return 0, false
	}
	return teamID, true
}

func (h *Handler) requireOwner(w http.ResponseWriter, r *http.Request) (int64, bool) {
	teamID, ok := h.teamID(r)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "bad team id")
		return 0, false
	}
	role, isMember, err := h.store.Role(r.Context(), teamID, auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not check membership")
		return 0, false
	}
	if !isMember || role != "owner" {
		httpx.Error(w, http.StatusForbidden, "owner only")
		return 0, false
	}
	return teamID, true
}
```
**NOTE for the implementer:** confirm the exact `httpx` helper names (`JSON`, `Error`, `DecodeJSON`) and the `auth.WithUser`/`auth.UserID` signatures by reading `internal/httpx` and `internal/auth` — use the real names (other handlers in this repo are the reference). Adjust if they differ.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/teams/ && go vet ./internal/teams/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/teams/service.go internal/teams/handler.go internal/teams/handler_test.go
git commit -m "feat(teams): service + handler + routes (owner-gated management)"
```

---

## Task 5: Applications become team-scoped

**Files:**
- Modify: `internal/applications/app.go`, `internal/applications/repo.go`, `internal/applications/handler.go`
- Test: `internal/applications/repo_test.go` (extend), `internal/applications/handler_test.go` (extend)

**Interfaces:**
- Consumes: `teams.Repo.PersonalTeamID`, `teams.Repo.Role` (membership).
- Produces:
  - `Application` gains `TeamID int64` (`json:"teamId"`) + `TeamName string` (`json:"teamName"`).
  - `Repo.Create(ctx, ownerID, teamID int64, name, description string) (Application, error)`.
  - `Repo.ListForUser(ctx, userID int64, p paging.Params) ([]Application, int, error)` — apps across the caller's teams.
  - `Repo.Get(ctx, id int64) (Application, error)` — by id, includes team id+name (no owner arg).
  - The handler's `Store` interface + create/list updated; `create` reads `teamId` (default personal), verifies membership.

- [ ] **Step 1: Write the failing tests**

Extend `internal/applications/repo_test.go` (DB-backed; seed a user via SQL, get their personal team, create an app under it):
```go
func TestCreateUnderTeamAndListForUser(t *testing.T) {
	ctx, repo, pool, uid, teamID := appTestSetup(t) // helper: seeds user + personal team, returns ids
	_ = pool
	app, err := repo.Create(ctx, uid, teamID, "TeamApp", "d")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if app.TeamID != teamID {
		t.Fatalf("app.TeamID=%d want %d", app.TeamID, teamID)
	}
	apps, total, err := repo.ListForUser(ctx, uid, paging.Params{})
	if err != nil || total < 1 {
		t.Fatalf("list: total=%d err=%v", total, err)
	}
	var found bool
	for _, a := range apps {
		if a.ID == app.ID {
			found = true
			if a.TeamName == "" {
				t.Error("TeamName not populated in list")
			}
		}
	}
	if !found {
		t.Error("created app not in ListForUser")
	}
	got, err := repo.Get(ctx, app.ID)
	if err != nil || got.TeamID != teamID {
		t.Fatalf("get: %+v err=%v", got, err)
	}
}
```
(Write the `appTestSetup` helper in the test file: connect+migrate, insert a user, read their personal team id via `SELECT t.id FROM teams t JOIN team_members tm ON tm.team_id=t.id WHERE tm.user_id=$1 AND t.personal` — but a raw-SQL user insert does NOT create a personal team, so insert the team+membership in the helper too, mirroring the teams repo test's seeding. Register the cleanup.)

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/applications/ -run TestCreateUnderTeam -v`
Expected: FAIL — `Create` arity / `ListForUser` / `TeamID` undefined.

- [ ] **Step 3: Update the type, repo, and handler**

In `internal/applications/app.go`, add to `Application`:
```go
	TeamID   int64  `json:"teamId"`
	TeamName string `json:"teamName"`
```

In `internal/applications/repo.go`, replace `Create`, `ListByOwner`, `Get`:
```go
func (r *Repo) Create(ctx context.Context, ownerID, teamID int64, name, description string) (Application, error) {
	var a Application
	err := r.pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id, team_id, name, description) VALUES($1,$2,$3,$4)
		 RETURNING id, owner_id, team_id, name, description, created_at`,
		ownerID, teamID, name, description,
	).Scan(&a.ID, &a.OwnerID, &a.TeamID, &a.Name, &a.Description, &a.CreatedAt)
	return a, err
}

// ListForUser returns apps across every team the user belongs to.
func (r *Repo) ListForUser(ctx context.Context, userID int64, p paging.Params) ([]Application, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM applications a
		 WHERE a.team_id IN (SELECT team_id FROM team_members WHERE user_id=$1)`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT a.id, a.owner_id, a.team_id, t.name, a.name, a.description, a.created_at,
		        (SELECT count(*) FROM subscriptions s WHERE s.application_id = a.id) AS sub_count
		 FROM applications a JOIN teams t ON t.id = a.team_id
		 WHERE a.team_id IN (SELECT team_id FROM team_members WHERE user_id=$1)
		 ORDER BY a.created_at DESC LIMIT $2 OFFSET $3`,
		userID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Application
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.ID, &a.OwnerID, &a.TeamID, &a.TeamName, &a.Name, &a.Description, &a.CreatedAt, &a.SubscriptionCount); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

// Get fetches an application by id (membership is enforced by the caller).
func (r *Repo) Get(ctx context.Context, id int64) (Application, error) {
	var a Application
	err := r.pool.QueryRow(ctx,
		`SELECT a.id, a.owner_id, a.team_id, t.name, a.name, a.description, a.created_at
		 FROM applications a JOIN teams t ON t.id = a.team_id WHERE a.id=$1`, id,
	).Scan(&a.ID, &a.OwnerID, &a.TeamID, &a.TeamName, &a.Name, &a.Description, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	return a, err
}
```

In `internal/applications/handler.go`: update the `Store` interface (`Create` gains `teamID`, `ListByOwner` → `ListForUser`), add a `teams` membership dependency, and rewrite `create` + `list`:
```go
// Store — update signatures:
//   Create(ctx, ownerID, teamID int64, name, description string) (Application, error)
//   ListForUser(ctx, userID int64, p paging.Params) ([]Application, int, error)

// Membership resolves the caller's default team + validates chosen teams.
type Membership interface {
	PersonalTeamID(ctx context.Context, userID int64) (int64, error)
	Role(ctx context.Context, teamID, userID int64) (string, bool, error)
}

// NewHandler gains a Membership arg:
func NewHandler(store Store, teams Membership, eventLog EventLogger) *Handler { /* store teams on h */ }

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		TeamID      int64  `json:"teamId"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	teamID := body.TeamID
	if teamID == 0 {
		var err error
		if teamID, err = h.teams.PersonalTeamID(r.Context(), uid); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "no personal team")
			return
		}
	} else {
		_, isMember, err := h.teams.Role(r.Context(), teamID, uid)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "membership check failed")
			return
		}
		if !isMember {
			httpx.Error(w, http.StatusForbidden, "not a member of that team")
			return
		}
	}
	a, err := h.store.Create(r.Context(), uid, teamID, strings.TrimSpace(body.Name), body.Description)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not create application")
		return
	}
	httpx.JSON(w, http.StatusCreated, a)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p := paging.FromQuery(r) // use the repo's existing paging helper
	apps, total, err := h.store.ListForUser(r.Context(), auth.UserID(r.Context()), p)
	// ... unchanged response shaping
}
```
**NOTE for the implementer:** keep the existing list response envelope (items/total/paging) exactly as it is today — only the store call changes from `ListByOwner` to `ListForUser`. Read the current `list` body and preserve its output shape. Match the real `paging` + `httpx` helper names.

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/applications/ && go vet ./internal/applications/`
Expected: PASS (update any existing app tests that referenced the old `Create`/`ListByOwner`/`Get` signatures).

- [ ] **Step 5: Commit**

```bash
git add internal/applications/
git commit -m "feat(teams): applications belong to teams (create-under-team, list across teams)"
```

---

## Task 6: Ownership swap — membership gates everywhere

**Files:**
- Modify: `internal/server/server.go` (the `owns` closure + wiring), `internal/server/tryit_adapters.go`
- Test: covered by the server building + the existing subscriptions/tryit tests still passing; add `internal/server` wiring is compile-checked.

**Interfaces:**
- Consumes: `teams.Repo.IsMemberOfApp`.
- Produces: the `owns` closure calls `teamsRepo.IsMemberOfApp(userID, appID)`; `tryitAccessAdapter.OwnsApp` calls the same. `applications.Repo.Get(ctx, id)` (single-arg) used where an app is fetched for display.

- [ ] **Step 1: Rewire the `owns` closure**

In `internal/server/server.go`, replace the `owns` closure body:
```go
	teamsRepo := teams.NewRepo(pool)
	owns := func(ctx context.Context, appID, userID int64) (bool, error) {
		return teamsRepo.IsMemberOfApp(ctx, userID, appID)
	}
```
(Add `"apisix-portal/internal/teams"` import. Place `teamsRepo` where `appsRepo` is created so it's in scope for both the apps handler (Task 5 wiring) and here.)

- [ ] **Step 2: Rewire the try-it adapter**

In `internal/server/tryit_adapters.go`, change `tryitAccessAdapter` to hold the teams repo and rewrite `OwnsApp`:
```go
type tryitAccessAdapter struct {
	teams *teams.Repo
	subs  *subscriptions.Repo
}

func (a tryitAccessAdapter) OwnsApp(ctx context.Context, appID, userID int64) (bool, error) {
	return a.teams.IsMemberOfApp(ctx, userID, appID)
}
```
Update its construction in `server.go` (`tryAccess := tryitAccessAdapter{teams: teamsRepo, subs: subRepo}`). Add the `teams` import; drop the now-unused `apps`/`applications.ErrNotFound` usage in that file if it becomes unused.

- [ ] **Step 3: Build + run the affected suites**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go build ./... && go test ./internal/server/ ./internal/subscriptions/ ./internal/tryit/ && go vet ./internal/server/`
Expected: PASS. (The subscriptions `detail` handler's ownership now flows through the membership `owns`; a teammate can view, a non-member gets 403 — unchanged handler logic, new predicate.)

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go internal/server/tryit_adapters.go
git commit -m "feat(teams): membership replaces owner check for apps + try-it"
```

---

## Task 7: Approval emails go to team owners

**Files:**
- Modify: `internal/notify/repo.go`, `internal/notify/notifier.go`
- Test: `internal/notify/repo_test.go` (extend), `internal/notify/notifier_test.go` (adjust fake)

**Interfaces:**
- Consumes: the team-owner query.
- Produces: `notify.Repo.OwnerEmailForApp` returns the **team owners'** emails. To keep the `Resolver`/deliver contract, change it to `OwnerEmailsForApp(ctx, appID) ([]string, string, error)` and have `deliver` send to that slice for *Approved*/*Rejected*.

- [ ] **Step 1: Write/adjust the failing test**

In `internal/notify/repo_test.go`, add a test seeding a user + personal team + app under it, asserting `OwnerEmailsForApp` returns that owner's email:
```go
func TestOwnerEmailsForAppReturnsTeamOwners(t *testing.T) {
	ctx, repo, appID, ownerEmail := seedTeamApp(t) // helper: user+personal team+app
	emails, name, err := repo.OwnerEmailsForApp(ctx, appID)
	if err != nil || name == "" {
		t.Fatalf("err=%v name=%q", err, name)
	}
	var found bool
	for _, e := range emails {
		if e == ownerEmail {
			found = true
		}
	}
	if !found {
		t.Fatalf("owner email %q not in %v", ownerEmail, emails)
	}
}
```
(Write `seedTeamApp`: insert user, personal team, owner membership, and an app with that team_id; return appID + the email. Mirror the teams repo test seeding.)

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/notify/ -run TestOwnerEmailsForApp -v`
Expected: FAIL — method undefined / old signature.

- [ ] **Step 3: Update the notify repo + deliver + Resolver**

In `internal/notify/repo.go`, replace `OwnerEmailForApp` with:
```go
// OwnerEmailsForApp returns the emails of the owners of the app's team + the app name.
func (r *Repo) OwnerEmailsForApp(ctx context.Context, appID int64) ([]string, string, error) {
	var name string
	if err := r.pool.QueryRow(ctx, `SELECT name FROM applications WHERE id=$1`, appID).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", nil
		}
		return nil, "", err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT u.email FROM applications a
		 JOIN team_members tm ON tm.team_id = a.team_id AND tm.role='owner'
		 JOIN users u ON u.id = tm.user_id
		 WHERE a.id=$1`, appID)
	if err != nil {
		return nil, name, err
	}
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, name, err
		}
		emails = append(emails, e)
	}
	return emails, name, rows.Err()
}
```
In `internal/notify/notifier.go`, update the `Resolver` interface method (`OwnerEmailForApp` → `OwnerEmailsForApp(...) ([]string, string, error)`) and in `deliver`, for `kindApproved`/`kindRejected`, set `to = ownerEmails` (the slice) instead of `[]string{owner}` and use the returned name for `appName`. Update the fake `Resolver` in `notifier_test.go` accordingly (return `[]string{"dev@example.com"}`).

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/notify/ && go vet ./internal/notify/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notify/
git commit -m "feat(teams): approval emails resolve the app's team owners"
```

---

## Task 8: Wire the teams handler + full suite + live

**Files:**
- Modify: `internal/server/server.go`
- Test: full backend suite + live

**Interfaces:**
- Consumes: `teams.NewHandler`, `teams.NewRepo`, and the Task-5 `applications.NewHandler(store, teamsRepo, eventRepo)` signature.

- [ ] **Step 1: Wire the routes**

In `internal/server/server.go`:
- Build `teamsRepo := teams.NewRepo(pool)` once (from Task 6) and reuse it.
- Pass it to the apps handler: `appsH := applications.NewHandler(appsRepo, teamsRepo, eventRepo)`.
- Build the teams handler + mount it:
```go
	teamsH := teams.NewHandler(teamsRepo)
	mux.Handle("/api/teams", requireAuth(teamsH))
	mux.Handle("/api/teams/", requireAuth(teamsH))
```
- Wire the notify Notifier to the teams-aware repo if needed (the notify repo already queries by team after Task 7 — no wiring change beyond it compiling).

- [ ] **Step 2: Build + full backend suite**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go build ./... && go test ./internal/... ./cmd/... && go vet ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(teams): wire teams handler + team-aware apps handler"
```

- [ ] **Step 4: Live verification**

Bring the stack up (`docker compose up -d postgres apisix`), run the portal, then with `curl` (register returns a token):
1. Register user A and user B (two emails).
2. As A: `GET /api/teams` → shows A's personal team (`personal:true, role:owner`).
3. As A: `POST /api/teams {"name":"Acme"}` → 201; `POST /api/teams/{id}/members {"email":"<B>"}` → 204; adding a bogus email → 404; adding B again → 409.
4. As A: `POST /api/applications {"name":"Shared","teamId":<Acme>}` → 201 with `teamId=<Acme>`.
5. As B: `GET /api/applications` includes "Shared" (with `teamName:"Acme"`); `GET /api/applications/{sharedId}` → 200 (member access); B can `POST …/rotate-key`.
6. As A: `DELETE /api/teams/{Acme}/members/{B}` → 204; now B `GET /api/applications/{sharedId}` → 403.
7. As A: `DELETE /api/teams/{Acme}` while it still has "Shared" → 409 (`ErrTeamHasApps`).
8. Confirm a pre-existing app still resolves (migration backfilled its personal team): `GET /api/applications` for an old user shows its apps with a personal `teamName`.
**Look at the output.**

---

## Self-Review notes

- **Spec coverage:** migration + backfill (T1) ✅; teams repo incl. `IsMemberOfApp`/`OwnerEmailsForApp`/`PersonalTeamID` (T2) ✅; personal-team-on-register (T3) ✅; teams service/handler/routes with owner-gated management + add-by-email 404/409 (T4) ✅; apps team-scoped create/list/get + `teamId`/`teamName` (T5) ✅; ownership swap for apps + try-it (T6) ✅; approval emails → team owners (T7) ✅; wiring + live (T8) ✅. Deferred items (owner transfer, email-invite, per-app sharing, billing) intentionally absent.
- **Type consistency:** `IsMemberOfApp(ctx, userID, appID)` argument order is identical in the repo (T2), the `owns` closure, and the try-it adapter (T6). `applications.Repo.Create(ctx, ownerID, teamID, name, description)` matches its handler call (T5). `applications.Repo.Get(ctx, id)` single-arg is used by T6 wiring. `teams.Store` (T4) is a subset of `*Repo` (T2) — every method signature matches. `notify.Repo.OwnerEmailsForApp(ctx, appID) ([]string, string, error)` matches the `Resolver` + `deliver` update (T7).
- **Implementer notes:** verify real helper names before coding — `httpx.JSON/Error/DecodeJSON`, `paging.FromQuery`/`paging.Params`, `auth.UserID`/`auth.WithUser`, and the chi import path — by reading a sibling handler (`internal/applications/handler.go`) and `internal/httpx`; the code blocks use the conventional names but the repo is the source of truth. Existing tests that referenced `applications.Create`/`ListByOwner`/`Get` old signatures (T5) and any `notify` `OwnerEmailForApp` callers (T7) must be updated in the same task. `internal/db/migrate.go` tracks applied migrations, so `0013` runs once; the backfill only covers users present at migration time (new users get their personal team at registration — T3).
```
