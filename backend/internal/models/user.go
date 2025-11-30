package models

import (
	"time"
)

// User represents the user model
type User struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"unique;not null"`
	Email     string    `json:"email" gorm:"unique;not null"`
	Password  string    `json:"-" gorm:"not null"` // Don't return password in JSON
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	
	// GitHub OAuth fields
	GitHubID          *int    `json:"github_id,omitempty" gorm:"unique"`
	GitHubUsername    *string `json:"github_username,omitempty"`
	GitHubAccessToken *string `json:"-" gorm:"column:github_access_token"` // Don't return token in JSON
	GitHubConnected   bool    `json:"github_connected" gorm:"default:false"`
}

// UserLogin is used for user authentication
type UserLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// UserRegister is used for user registration
type UserRegister struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
} 