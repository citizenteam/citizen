// Package repository provides database access for RBAC
package repository

import (
	"context"
	"fmt"

	"backend/internal/database"
	"backend/internal/database/api"
	"backend/internal/rbac/domain"
	"backend/pkg/logger"
)

var log = logger.Default().WithComponent("rbac-repo")

// PermissionRepository handles permission database operations
type PermissionRepository struct{}

// NewPermissionRepository creates a new permission repository
func NewPermissionRepository() *PermissionRepository {
	return &PermissionRepository{}
}

// GetUserPermissions retrieves all permissions for a user in an organization
func (r *PermissionRepository) GetUserPermissions(ctx context.Context, citizenAuthUserID, organizationID string) (domain.Permissions, error) {
	if citizenAuthUserID == "" || organizationID == "" {
		return nil, nil
	}

	// Validate UUID formats
	if !isValidUUID(citizenAuthUserID) {
		return nil, fmt.Errorf("invalid user ID format")
	}
	if !isValidUUID(organizationID) {
		return nil, fmt.Errorf("invalid organization ID format")
	}

	query := `
		SELECT app_id, role
		FROM app_permissions
		WHERE user_id = $1 AND organization_id = $2::uuid
		ORDER BY granted_at DESC
	`

	rows, err := database.DB.Query(ctx, query, citizenAuthUserID, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions: %w", err)
	}
	defer rows.Close()

	var permissions domain.Permissions
	for rows.Next() {
		var appID, roleStr string
		if err := rows.Scan(&appID, &roleStr); err != nil {
			log.WithField("error", err.Error()).Warn("Failed to scan permission row")
			continue
		}
		permissions = append(permissions, domain.Permission{
			AppID: appID,
			Role:  domain.ParseRole(roleStr),
		})
	}

	return permissions, nil
}

// isValidUUID validates UUID format (8-4-4-4-12 hex with dashes)
func isValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// CheckAppPermission checks if user has required role for an app
func (r *PermissionRepository) CheckAppPermission(ctx context.Context, citizenAuthUserID, organizationID, appID string, requiredRole domain.Role) (bool, error) {
	if citizenAuthUserID == "" || organizationID == "" {
		return false, nil
	}

	// Validate UUID formats
	if !isValidUUID(citizenAuthUserID) || !isValidUUID(organizationID) {
		return false, nil // Invalid format = no permission
	}

	// Use database function for role hierarchy check
	query := `SELECT check_app_permission($1, $2::uuid, $3, $4)`

	var hasPermission bool
	err := database.DB.QueryRow(ctx, query, citizenAuthUserID, organizationID, appID, requiredRole.String()).Scan(&hasPermission)
	if err != nil {
		return false, fmt.Errorf("permission check failed: %w", err)
	}

	return hasPermission, nil
}

// IsUserAssignedToInstance checks if user has any permissions in the organization
func (r *PermissionRepository) IsUserAssignedToInstance(ctx context.Context, citizenAuthUserID, organizationID string) (bool, error) {
	if citizenAuthUserID == "" || organizationID == "" {
		return false, nil
	}

	// Validate UUID formats
	if !isValidUUID(citizenAuthUserID) || !isValidUUID(organizationID) {
		return false, nil // Invalid format = not assigned
	}

	query := `SELECT EXISTS (SELECT 1 FROM app_permissions WHERE user_id = $1 AND organization_id = $2::uuid)`

	var assigned bool
	err := database.DB.QueryRow(ctx, query, citizenAuthUserID, organizationID).Scan(&assigned)
	if err != nil {
		return false, fmt.Errorf("failed to check instance assignment: %w", err)
	}

	return assigned, nil
}

// GrantPermission grants or updates permission for a user
func (r *PermissionRepository) GrantPermission(ctx context.Context, citizenAuthUserID, organizationID, appID string, role domain.Role, grantedBy string) error {
	// Validate UUID formats
	if !isValidUUID(citizenAuthUserID) {
		return fmt.Errorf("invalid user ID format")
	}
	if !isValidUUID(organizationID) {
		return fmt.Errorf("invalid organization ID format")
	}

	query := `SELECT grant_app_permission($1, $2::uuid, $3, $4, $5)`

	_, err := database.DB.Exec(ctx, query, citizenAuthUserID, organizationID, appID, role.String(), grantedBy)
	if err != nil {
		return fmt.Errorf("failed to grant permission: %w", err)
	}

	// Audit trail - log to database asynchronously
	go r.logPermissionAudit(ctx, "permission.granted", organizationID, appID, role.String(), grantedBy)

	log.WithFields(map[string]interface{}{
		"app_id": appID,
		"role":   role.String(),
	}).Info("Permission granted")

	return nil
}

// RevokePermission revokes a user's permission for an app
func (r *PermissionRepository) RevokePermission(ctx context.Context, citizenAuthUserID, organizationID, appID string) error {
	// Validate UUID formats
	if !isValidUUID(citizenAuthUserID) {
		return fmt.Errorf("invalid user ID format")
	}
	if !isValidUUID(organizationID) {
		return fmt.Errorf("invalid organization ID format")
	}

	query := `SELECT revoke_app_permission($1, $2::uuid, $3)`

	var revoked bool
	err := database.DB.QueryRow(ctx, query, citizenAuthUserID, organizationID, appID).Scan(&revoked)
	if err != nil {
		return fmt.Errorf("failed to revoke permission: %w", err)
	}

	if !revoked {
		return fmt.Errorf("permission not found")
	}

	// Audit trail - log to database asynchronously
	go r.logPermissionAudit(ctx, "permission.revoked", organizationID, appID, "", "")

	log.WithFields(map[string]interface{}{
		"app_id": appID,
	}).Info("Permission revoked")

	return nil
}

// logPermissionAudit logs permission changes to the audit log table
func (r *PermissionRepository) logPermissionAudit(ctx context.Context, eventType, orgID, appID, role, performedBy string) {
	// Use background context since original context may be cancelled
	bgCtx := context.Background()

	query := `
		INSERT INTO permission_audit_logs (event_type, organization_id, app_id, role, performed_by, created_at)
		VALUES ($1, $2::uuid, $3, $4, $5, NOW())
	`

	_, err := database.DB.Exec(bgCtx, query, eventType, orgID, appID, role, performedBy)
	if err != nil {
		// Log error but don't fail the operation
		log.WithField("error", err.Error()).Warn("Failed to write permission audit log")
	}
}

// SetRLSContext sets the session context for Row Level Security
// Delegates to api.SetRLSContext for centralized, secure implementation
func (r *PermissionRepository) SetRLSContext(ctx context.Context, localUserID int, citizenAuthUserID, organizationID string) error {
	return api.SetRLSContext(ctx, localUserID, citizenAuthUserID, organizationID)
}
