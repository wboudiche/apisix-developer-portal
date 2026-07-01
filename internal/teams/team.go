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
