package models

import (
	"time"
)

// APIToken represents an API token for external access
type APIToken struct {
	ID           uint      `json:"id"`
	UserID       uint      `json:"user_id"`
	TokenHash    string    `json:"-"`                        // SHA-256 hash, never returned in JSON
	TokenPrefix  string    `json:"token_prefix"`             // First chars for identification
	Name         string    `json:"name"`                     // User-defined name
	Description  *string   `json:"description,omitempty"`   // Optional description
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`   // Optional expiration
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"` // Last usage timestamp
	LastUsedIP   *string   `json:"last_used_ip,omitempty"`  // Last used IP
	UsageCount   int       `json:"usage_count"`             // Total usage count
	IsActive     bool      `json:"is_active"`               // Token status
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// IsExpired checks if the token is expired
func (at *APIToken) IsExpired() bool {
	if at.ExpiresAt == nil {
		return false // No expiration set
	}
	return time.Now().After(*at.ExpiresAt)
}

// CanUse checks if token can be used (active and not expired)
func (at *APIToken) CanUse() bool {
	return at.IsActive && !at.IsExpired()
}

// APITokenRequest represents the request payload for creating an API token
type APITokenRequest struct {
	Name        string     `json:"name" validate:"required,min=3,max=100"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=500"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// APITokenResponse represents the response when creating a token (includes the raw token)
type APITokenResponse struct {
	ID          uint       `json:"id"`
	TokenPrefix string     `json:"token_prefix"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	Token       string     `json:"token"` // Raw token - only returned once during creation
}

// APITokenListResponse represents a token in list responses (no raw token)
type APITokenListResponse struct {
	ID          uint       `json:"id"`
	TokenPrefix string     `json:"token_prefix"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	UsageCount  int        `json:"usage_count"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
