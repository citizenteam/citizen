package models

import (
	"time"
)

// AppCustomDomain represents custom domain information for an app
type AppCustomDomain struct {
	ID        int       `json:"id"`
	AppName   string    `json:"app_name"`
	Domain    string    `json:"domain"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AppPublicSetting represents public app setting
type AppPublicSetting struct {
	ID        int       `json:"id"`
	AppName   string    `json:"app_name"`
	IsPublic  bool      `json:"is_public"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SetCustomDomainRequest represents request for setting custom domain
type SetCustomDomainRequest struct {
	AppName string `json:"app_name"`
	Domain  string `json:"domain"`
}

// SetPublicAppRequest represents request for setting public app
type SetPublicAppRequest struct {
	AppName  string `json:"app_name"`
	IsPublic bool   `json:"is_public"`
} 