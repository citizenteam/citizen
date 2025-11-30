package models

import "time"

// SSOSession structure
type SSOSession struct {
	SessionID      string    `json:"session_id"`
	UserID         int       `json:"user_id"`
	MainDomain     string    `json:"main_domain"`
	DeviceID       string    `json:"device_id"`
	OrganizationID *string   `json:"organization_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivity   time.Time `json:"last_activity"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// Domain types
type DomainType int

const (
	DomainTypeLogin DomainType = iota
	DomainTypeSubdomain
	DomainTypeCustom
)

// Cookie configuration
type CookieConfig struct {
	Domain   string
	SameSite string
	Secure   bool
}
