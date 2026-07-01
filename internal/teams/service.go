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
