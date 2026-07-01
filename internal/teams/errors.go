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
