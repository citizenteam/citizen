// Package domain contains RBAC domain types and constants
package domain

import "time"

// RBACContext holds all authorization context for a request
type RBACContext struct {
	// User identifiers
	LocalUserID       int    // Local database user ID (integer)
	CitizenAuthUserID string // CitizenAuth UUID (string)

	// Organization context
	OrganizationID string // Current organization UUID

	// Role and permissions
	IsSuperAdmin bool        // Platform-wide super admin (from JWT)
	Permissions  Permissions // User's permissions from app_permissions table

	// Cached computed values
	highestRole *Role
}

// NewRBACContext creates a new RBAC context
func NewRBACContext(localUserID int, citizenAuthUserID, orgID string, isSuperAdmin bool) *RBACContext {
	return &RBACContext{
		LocalUserID:       localUserID,
		CitizenAuthUserID: citizenAuthUserID,
		OrganizationID:    orgID,
		IsSuperAdmin:      isSuperAdmin,
		Permissions:       make(Permissions, 0),
	}
}

// SetPermissions sets the user's permissions
func (ctx *RBACContext) SetPermissions(perms Permissions) {
	ctx.Permissions = perms
	ctx.highestRole = nil // Reset cache
}

// GetHighestRole returns the user's highest role (cached)
func (ctx *RBACContext) GetHighestRole() Role {
	if ctx.IsSuperAdmin {
		return RoleAdmin
	}
	if ctx.highestRole == nil {
		role := ctx.Permissions.GetHighestRole()
		ctx.highestRole = &role
	}
	return *ctx.highestRole
}

// AuditInfo returns audit-safe information about the context
type AuditInfo struct {
	LocalUserID       int       `json:"local_user_id"`
	CitizenAuthUserID string    `json:"citizenauth_user_id"`
	OrganizationID    string    `json:"organization_id"`
	Role              string    `json:"role"`
	IsSuperAdmin      bool      `json:"is_super_admin"`
	Timestamp         time.Time `json:"timestamp"`
}

// GetAuditInfo returns audit information for logging
func (ctx *RBACContext) GetAuditInfo() AuditInfo {
	return AuditInfo{
		LocalUserID:       ctx.LocalUserID,
		CitizenAuthUserID: ctx.CitizenAuthUserID,
		OrganizationID:    ctx.OrganizationID,
		Role:              ctx.GetHighestRole().String(),
		IsSuperAdmin:      ctx.IsSuperAdmin,
		Timestamp:         time.Now(),
	}
}
