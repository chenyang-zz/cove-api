package response

import (
	"time"

	"github.com/google/uuid"
)

type AuthResponse struct {
	UserID       uuid.UUID `json:"user_id"`
	Username     string    `json:"username"`
	Email        *string   `json:"email,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
}

type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Nickname  *string   `json:"nickname"`
	Email     *string   `json:"email"`
	Avatar    *string   `json:"avatar"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}
