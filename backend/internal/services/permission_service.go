package services

import (
	"backend/internal/database"
	"backend/internal/utils"
	"context"
	"fmt"
	"log"
	"time"
)

// PermissionService handles app-level permission checks
type PermissionService struct {
}

// NewPermissionService creates a new permission service
func NewPermissionService() *PermissionService {
	return &PermissionService{}
}

// CheckAppPermission checks if user has sufficient permission for app within organization
// This queries LOCAL database - no network calls
func (ps *PermissionService) CheckAppPermission(ctx context.Context, userID, organizationID, appID, requiredRole string) (bool, error) {
    query := `SELECT check_app_permission($1, $2, $3, $4)`

    var hasPermission bool
    err := database.DB.QueryRow(ctx, query, userID, organizationID, appID, requiredRole).Scan(&hasPermission)
    if err != nil {
        return false, fmt.Errorf("permission check failed: %w", err)
    }

    utils.AuthDebugLog("Permission check: user=%s, org=%s, app=%s, required=%s, result=%v",
        userID, organizationID, appID, requiredRole, hasPermission)

    return hasPermission, nil
}

// GetUserPermissions returns all app permissions for a user within organization
func (ps *PermissionService) GetUserPermissions(ctx context.Context, userID, organizationID string) ([]Permission, error) {
    query := `
        SELECT app_id, role, granted_at
        FROM app_permissions
        WHERE user_id = $1 AND organization_id = $2
        ORDER BY granted_at DESC
    `

    rows, err := database.DB.Query(ctx, query, userID, organizationID)
    if err != nil {
        return nil, fmt.Errorf("failed to get permissions: %w", err)
    }
	defer rows.Close()
	
	var permissions []Permission
	for rows.Next() {
		var perm Permission
		if err := rows.Scan(&perm.AppID, &perm.Role, &perm.GrantedAt); err != nil {
			log.Printf("⚠️  [PERMISSION] Scan error: %v", err)
			continue
		}
		permissions = append(permissions, perm)
	}
	
    log.Printf("✅ [PERMISSION] Retrieved %d permissions for user %s (org %s)", len(permissions), userID, organizationID)

    return permissions, nil
}

// GrantPermission grants or updates permission for user (via webhook)
func (ps *PermissionService) GrantPermission(ctx context.Context, userID, organizationID, appID, role, grantedBy string) error {
    query := `SELECT grant_app_permission($1, $2, $3, $4, $5)`

    _, err := database.DB.Exec(ctx, query, userID, organizationID, appID, role, grantedBy)
    if err != nil {
        return fmt.Errorf("failed to grant permission: %w", err)
    }

    log.Printf("✅ [PERMISSION] Granted %s access to user %s for app %s (org %s by %s)",
        role, userID, appID, organizationID, grantedBy)

    // Publish cache invalidation event
    ps.publishPermissionChange(ctx, userID, organizationID, appID)

    return nil
}

// RevokePermission revokes user's permission for app
func (ps *PermissionService) RevokePermission(ctx context.Context, userID, organizationID, appID string) error {
    query := `SELECT revoke_app_permission($1, $2, $3)`

    var revoked bool
    err := database.DB.QueryRow(ctx, query, userID, organizationID, appID).Scan(&revoked)
    if err != nil {
        return fmt.Errorf("failed to revoke permission: %w", err)
    }

    if !revoked {
        return fmt.Errorf("permission not found")
    }

    log.Printf("🗑️  [PERMISSION] Revoked user %s access to app %s (org %s)", userID, appID, organizationID)

    // Publish cache invalidation event
    ps.publishPermissionChange(ctx, userID, organizationID, appID)

    return nil
}

// publishPermissionChange publishes permission change event to Redis
func (ps *PermissionService) publishPermissionChange(ctx context.Context, userID, organizationID, appID string) {
    // Publish to Redis for cache invalidation
    if database.RedisClient == nil {
        return // Redis not available
    }

    channel := "citizen:permission_change"
    message := fmt.Sprintf(`{"user_id":"%s","organization_id":"%s","app_id":"%s","timestamp":%d}`, 
        userID, organizationID, appID, time.Now().Unix())
	
	if err := database.RedisClient.Publish(ctx, channel, message).Err(); err != nil {
		log.Printf("⚠️  [PERMISSION] Failed to publish change event: %v", err)
	} else {
		log.Printf("📡 [PERMISSION] Published change event to %s", channel)
	}
}

// Permission represents an app permission
type Permission struct {
	AppID     string    `json:"app_id"`
	Role      string    `json:"role"`
	GrantedAt time.Time `json:"granted_at"`
}

// IsUserAssignedToInstance checks whether a CitizenAuth user has any permissions on this instance for an organization.
func (ps *PermissionService) IsUserAssignedToInstance(ctx context.Context, userID, organizationID string) (bool, error) {
    query := `SELECT EXISTS (SELECT 1 FROM app_permissions WHERE user_id = $1 AND organization_id = $2)`

    var assigned bool
    if err := database.DB.QueryRow(ctx, query, userID, organizationID).Scan(&assigned); err != nil {
        return false, fmt.Errorf("failed to check instance assignment: %w", err)
    }

    return assigned, nil
}
