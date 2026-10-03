package model

import "time"

// User represents a registered system user.
type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// RegisterRequest contains payload for user signup.
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest contains payload for user signin.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse returns authentication status and session token.
type AuthResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token,omitempty"`
	User    *User  `json:"user,omitempty"`
	Error   string `json:"error,omitempty"`
}

// UserTracker maps a user to a tracked product with their requested interval.
type UserTracker struct {
	ID              int64     `json:"id"`
	UserID          int64     `json:"user_id"`
	ProductID       int64     `json:"product_id"`
	DesiredInterval string    `json:"desired_interval"`
	CreatedAt       time.Time `json:"created_at"`
}
