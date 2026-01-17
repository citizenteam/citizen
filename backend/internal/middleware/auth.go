package middleware

import (
	authhandlers "backend/internal/auth/handlers"
	authservices "backend/internal/auth/services"
	"backend/internal/database"
	"backend/internal/models"
	"backend/internal/rbac/domain"
	rbacservice "backend/internal/rbac/service"
	"backend/pkg/response"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

var rbacSvc = rbacservice.Default()

// Protected, SSO session veya JWT ile yetkilendirme gerektirir
func Protected() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Check if path is public - skip authentication for public paths
		if authhandlers.IsPublicPath(c.Path()) {
			return c.Next()
		}

		// Check if JWT or device token auth already succeeded
		authType := c.Locals("auth_type")
		if authType == "jwt" || authType == "device_token" {
			citizenAuthUserID, _ := c.Locals("citizenauth_user_id").(string)
			if citizenAuthUserID == "" {
				return response.Unauthorized(c, "CitizenAuth user context missing")
			}

			organizationID, _ := c.Locals("organization_id").(string)
			if organizationID == "" {
				return response.Unauthorized(c, "Organization context missing")
			}

			if err := enforceAccessControls(c, citizenAuthUserID, organizationID); err != nil {
				return err
			}

			localUserID, user, err := ensureLocalUserFromJWT(c, organizationID)
			if err != nil {
				return response.Unauthorized(c, "Failed to map CitizenAuth user")
			}

			// Store mapped user details for downstream handlers
			c.Locals("user_id", localUserID)
			c.Locals("user", user)
			return c.Next()
		}

		// Fallback to SSO session
		ssoSessionID := c.Cookies("sso_session")

		// If SSO session is not found, return unauthorized
		if ssoSessionID == "" {
			return response.Unauthorized(c, "Authentication required (SSO session or JWT)")
		}

		// Validate SSO session
		session, err := authservices.GetSSOSession(ssoSessionID)
		if err != nil || session == nil {
			return response.Unauthorized(c, "Invalid or expired SSO session")
		}

		if session.OrganizationID != nil && *session.OrganizationID != "" {
			c.Locals("organization_id", *session.OrganizationID)
		}

		// Check user
		var user models.User
		err = database.DB.QueryRow(c.Context(),
			"SELECT id, username, email, created_at, updated_at FROM users WHERE id = $1",
			session.UserID).Scan(&user.ID, &user.Username, &user.Email, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return response.Unauthorized(c, "User not found")
		}

		// Save user ID to locals
		c.Locals("user_id", session.UserID)
		c.Locals("user", user)

		citizenAuthUserID, organizationID, err := getCitizenauthMappingForLocalUser(c.Context(), session.UserID)
		if err != nil {
			return response.Forbidden(c, "CitizenAuth user mapping not found for local account")
		}
		c.Locals("citizenauth_user_id", citizenAuthUserID)
		if organizationID != "" {
			c.Locals("organization_id", organizationID)
		}
		if organizationID == "" {
			if orgCtx, ok := c.Locals("organization_id").(string); ok {
				organizationID = orgCtx
			}
		}
		if organizationID == "" {
			return response.Unauthorized(c, "Organization context missing")
		}

		if err := enforceAccessControls(c, citizenAuthUserID, organizationID); err != nil {
			return err
		}

		return c.Next()
	}
}

// ensureLocalUserFromJWT maps a CitizenAuth user UUID from JWT claims to a local Citizen user record.
// It creates the local user when needed and returns both the local user ID and hydrated user model.
func ensureLocalUserFromJWT(c *fiber.Ctx, organizationID string) (int, models.User, error) {
	citizenAuthUserID, _ := c.Locals("citizenauth_user_id").(string)
	if citizenAuthUserID == "" {
		if fromLegacy, ok := c.Locals("user_id").(string); ok {
			citizenAuthUserID = fromLegacy
		}
	}

	if citizenAuthUserID == "" {
		return 0, models.User{}, fmt.Errorf("citizenauth user id missing in context")
	}

	email, _ := c.Locals("email").(string)
	name, _ := c.Locals("name").(string)

	if organizationID == "" {
		return 0, models.User{}, fmt.Errorf("organization id missing in context")
	}

	var localUserID int
	mapQuery := `SELECT get_or_create_local_user($1, $2, $3, $4)`
	if err := database.DB.QueryRow(c.Context(), mapQuery, citizenAuthUserID, email, name, organizationID).Scan(&localUserID); err != nil {
		return 0, models.User{}, fmt.Errorf("map citizenauth user: %w", err)
	}

	var user models.User
	err := database.DB.QueryRow(c.Context(),
		"SELECT id, username, email, created_at, updated_at FROM users WHERE id = $1",
		localUserID).Scan(&user.ID, &user.Username, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return 0, models.User{}, fmt.Errorf("load local user: %w", err)
	}

	return localUserID, user, nil
}

// enforceAccessControls ensures the CitizenAuth user is assigned to this instance and has required app permissions.
// Uses centralized RBAC service for permission checks.
func enforceAccessControls(c *fiber.Ctx, citizenAuthUserID, organizationID string) error {
	// Check if user is assigned to this instance
	assigned, err := rbacSvc.IsUserAssignedToInstance(c.Context(), citizenAuthUserID, organizationID)
	if err != nil {
		return response.InternalServerError(c, "Failed to verify instance assignment")
	}
	if !assigned {
		return response.Forbidden(c, "Access denied: user is not assigned to this Citizen instance")
	}

	// Load user permissions using RBAC service (with caching)
	permissions, err := rbacSvc.GetUserPermissions(c.Context(), citizenAuthUserID, organizationID)
	if err != nil {
		return response.InternalServerError(c, "Failed to load user permissions")
	}

	// Store RBAC permissions in context for downstream use
	c.Locals("rbac_permissions", permissions)

	// App-specific permission check (if app_name is in route params)
	appID := c.Params("app_name")
	if appID != "" {
		requiredRole := determineRequiredRole(c.Method())
		if !permissions.HasAppPermission(appID, requiredRole) {
			return response.Forbidden(c, "Insufficient permissions for this application")
		}
	}

	return nil
}

// determineRequiredRole returns the minimum required RBAC role based on HTTP method
func determineRequiredRole(method string) domain.Role {
	switch method {
	case fiber.MethodGet, fiber.MethodHead:
		return domain.RoleViewer
	case fiber.MethodDelete:
		return domain.RoleAdmin
	default:
		return domain.RoleMember
	}
}

func getCitizenauthMappingForLocalUser(ctx context.Context, localUserID int) (string, string, error) {
	var citizenAuthUserID sql.NullString
	var organizationID sql.NullString
	err := database.DB.QueryRow(ctx,
		"SELECT citizenauth_user_id, organization_id FROM citizenauth_user_mapping WHERE local_user_id = $1",
		localUserID,
	).Scan(&citizenAuthUserID, &organizationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", sql.ErrNoRows
		}
		return "", "", err
	}

	return citizenAuthUserID.String, organizationID.String, nil
}
