package models

import (
	"time"
	"github.com/lib/pq"
)

// AppAPIAccess represents API access configuration for a specific app
type AppAPIAccess struct {
	ID                   uint           `json:"id"`
	AppName              string         `json:"app_name"`
	APIAccessEnabled     bool           `json:"api_access_enabled"`
	AllowedOperations    pq.StringArray `json:"allowed_operations"`
	RateLimitPerMinute   int            `json:"rate_limit_per_minute"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

// HasOperation checks if a specific operation is allowed for this app (deprecated - API tokens now have full access)
func (aaa *AppAPIAccess) HasOperation(operation string) bool {
	return aaa.APIAccessEnabled // If API access is enabled, all operations are allowed
}

// CanAccess checks if API access is enabled and operation is allowed
func (aaa *AppAPIAccess) CanAccess(operation string) bool {
	return aaa.APIAccessEnabled && aaa.HasOperation(operation)
}

// AppAPIAccessRequest represents the request payload for updating app API access
type AppAPIAccessRequest struct {
	APIAccessEnabled   *bool    `json:"api_access_enabled,omitempty"`
	AllowedOperations  []string `json:"allowed_operations,omitempty" validate:"omitempty,dive,oneof=read deploy restart env domains config logs *"`
	RateLimitPerMinute *int     `json:"rate_limit_per_minute,omitempty" validate:"omitempty,min=1,max=1000"`
}

// AppAPIAccessResponse represents the response for app API access settings
type AppAPIAccessResponse struct {
	AppName              string    `json:"app_name"`
	APIAccessEnabled     bool      `json:"api_access_enabled"`
	AllowedOperations    []string  `json:"allowed_operations"`
	RateLimitPerMinute   int       `json:"rate_limit_per_minute"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// Available operations constants
const (
	OperationRead    = "read"     // GET operations (logs, status, info)
	OperationDeploy  = "deploy"   // Deployment operations
	OperationRestart = "restart"  // Restart operations
	OperationEnv     = "env"      // Environment variable operations
	OperationDomains = "domains"  // Domain management
	OperationConfig  = "config"   // Configuration changes
	OperationLogs    = "logs"     // Log access
	OperationAll     = "*"        // All operations
)

// GetAllOperations returns all available operations (deprecated - now always full access)
func GetAllOperations() []string {
	return []string{OperationAll}
}

// GetDefaultOperations returns default allowed operations for new apps (deprecated - now always full access)
func GetDefaultOperations() []string {
	return []string{OperationAll}
}
