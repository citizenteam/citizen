package services

import (
	"context"
	"fmt"
	"time"

	"backend/internal/database"
	"backend/internal/rbac/domain"
	rbacservice "backend/internal/rbac/service"
	"backend/pkg/logger"
)

var permLog = logger.Default().WithComponent("permission-service")

// PermissionService handles app-level permission checks
// This service delegates to the central RBAC service where possible
type PermissionService struct {
	rbac *rbacservice.Service
}

// NewPermissionService creates a new permission service
func NewPermissionService() *PermissionService {
	return &PermissionService{
		rbac: rbacservice.Default(),
	}
}

// CheckAppPermission checks if user has sufficient permission for app within organization
// This queries LOCAL database - no network calls
func (ps *PermissionService) CheckAppPermission(ctx context.Context, userID, organizationID, appID, requiredRole string) (bool, error) {
	role := domain.ParseRole(requiredRole)
	return ps.rbac.CheckAppPermission(ctx, userID, organizationID, appID, role)
}

// GetUserPermissions returns all app permissions for a user within organization
func (ps *PermissionService) GetUserPermissions(ctx context.Context, userID, organizationID string) ([]Permission, error) {
	rbacPerms, err := ps.rbac.GetUserPermissions(ctx, userID, organizationID)
	if err != nil {
		return nil, err
	}

	// Convert RBAC permissions to legacy format
	permissions := make([]Permission, 0, len(rbacPerms))
	for _, p := range rbacPerms {
		permissions = append(permissions, Permission{
			AppID: p.AppID,
			Role:  p.Role.String(),
		})
	}

	permLog.WithFields(map[string]interface{}{
		"user_id": userID,
		"org_id":  organizationID,
		"count":   len(permissions),
	}).Debug("Retrieved permissions")

	return permissions, nil
}

// GrantPermission grants or updates permission for user (via webhook)
func (ps *PermissionService) GrantPermission(ctx context.Context, userID, organizationID, appID, role, grantedBy string) error {
	rbacRole := domain.ParseRole(role)
	err := ps.rbac.GrantPermission(ctx, userID, organizationID, appID, rbacRole, grantedBy)
	if err != nil {
		return err
	}

	// Publish cache invalidation event
	ps.publishPermissionChange(ctx, userID, organizationID, appID)

	return nil
}

// RevokePermission revokes user's permission for app
func (ps *PermissionService) RevokePermission(ctx context.Context, userID, organizationID, appID string) error {
	err := ps.rbac.RevokePermission(ctx, userID, organizationID, appID)
	if err != nil {
		return err
	}

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
		permLog.WithField("error", err.Error()).Warn("Failed to publish permission change event")
	} else {
		permLog.WithField("channel", channel).Debug("Published permission change event")
	}
}

// Permission represents an app permission (legacy format)
type Permission struct {
	AppID     string    `json:"app_id"`
	Role      string    `json:"role"`
	GrantedAt time.Time `json:"granted_at,omitempty"`
}

// IsUserAssignedToInstance checks whether a CitizenAuth user has any permissions on this instance for an organization.
func (ps *PermissionService) IsUserAssignedToInstance(ctx context.Context, userID, organizationID string) (bool, error) {
	return ps.rbac.IsUserAssignedToInstance(ctx, userID, organizationID)
}

// InvalidateUserCache invalidates the permission cache for a user
func (ps *PermissionService) InvalidateUserCache(userID, organizationID string) {
	ps.rbac.InvalidateCache(userID, organizationID)
}
