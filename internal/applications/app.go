package applications

import "time"

type Application struct {
	ID          int64     `json:"id"`
	OwnerID     int64     `json:"ownerId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}
