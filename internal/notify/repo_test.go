package notify

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"
)

func testRepo(t *testing.T) (context.Context, *Repo, int64) {
	t.Helper()
	ctx, repo, appID, _ := seedTeamApp(t)
	return ctx, repo, appID
}

// seedTeamApp seeds a user + personal team (with that user as owner) + an app
// under that team. It returns the appID and the owner's email.
func seedTeamApp(t *testing.T) (context.Context, *Repo, int64, string) {
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
	ownerEmail := "owner+" + suf + "@e.com"
	var uid, teamID, appID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name,role) VALUES($1,'x','Dev','developer') RETURNING id`,
		ownerEmail).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams(name,personal) VALUES($1,true) RETURNING id`,
		"Dev "+suf).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner')`,
		teamID, uid); err != nil {
		t.Fatalf("seed team member: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO applications(owner_id,name,team_id) VALUES($1,$2,$3) RETURNING id`,
		uid, "App "+suf, teamID).Scan(&appID); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM applications WHERE id=$1`, appID)
		_, _ = pool.Exec(ctx, `DELETE FROM teams WHERE id=$1`, teamID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})
	return ctx, NewRepo(pool), appID, ownerEmail
}

func TestOwnerEmailAndAdmins(t *testing.T) {
	ctx, repo, appID := testRepo(t)
	recipients, name, err := repo.OwnerEmailsForApp(ctx, appID)
	if err != nil || len(recipients) == 0 || name == "" {
		t.Fatalf("OwnerEmailsForApp = %v,%q,%v", recipients, name, err)
	}
	admins, err := repo.AdminEmails(ctx)
	if err != nil {
		t.Fatalf("AdminEmails: %v", err)
	}
	for _, a := range admins {
		if a.Email == "" {
			t.Fatal("empty admin email")
		}
	}
}

func TestOwnerEmailsForAppReturnsTeamOwners(t *testing.T) {
	ctx, repo, appID, ownerEmail := seedTeamApp(t)
	recipients, name, err := repo.OwnerEmailsForApp(ctx, appID)
	if err != nil || name == "" {
		t.Fatalf("err=%v name=%q", err, name)
	}
	var found bool
	for _, rc := range recipients {
		if rc.Email == ownerEmail {
			found = true
		}
	}
	if !found {
		t.Fatalf("owner email %q not in %v", ownerEmail, recipients)
	}
}
