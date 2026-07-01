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
	t.Cleanup(func() {
		_, _ = repo.pool.Exec(ctx, `DELETE FROM applications WHERE team_id=$1`, team.ID)
		_, _ = repo.pool.Exec(ctx, `DELETE FROM teams WHERE id=$1`, team.ID)
	})
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
