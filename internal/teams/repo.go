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
			return nil, "", ErrNotFound
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
