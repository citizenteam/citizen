package middleware

import (
	"backend/database"
	"backend/handlers"
	"backend/models"
	"backend/services"
	"backend/utils"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

var permissionSvc = services.NewPermissionService()

// Protected, SSO session veya JWT ile yetkilendirme gerektirir
func Protected() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// First check if JWT auth already succeeded
		if c.Locals("auth_type") == "jwt" {
			citizenAuthUserID, _ := c.Locals("citizenauth_user_id").(string)
			if citizenAuthUserID == "" {
				return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
					false,
					"CitizenAuth user context missing",
					nil,
				))
			}

			organizationID, _ := c.Locals("organization_id").(string)
			if organizationID == "" {
				return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
					false,
					"Organization context missing",
					nil,
				))
			}

			if err := enforceAccessControls(c, citizenAuthUserID, organizationID); err != nil {
				return err
			}

			localUserID, user, err := ensureLocalUserFromJWT(c, organizationID)
			if err != nil {
				return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
					false,
					"Failed to map CitizenAuth user",
					nil,
				))
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
			return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
				false,
				"Authentication required (SSO session or JWT)",
				nil,
			))
		}

		// Validate SSO session
		session, err := handlers.GetSSOSession(ssoSessionID)
		if err != nil || session == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
				false,
				"Invalid or expired SSO session",
				nil,
			))
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
			return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
				false,
				"User not found",
				nil,
			))
		}

		// Save user ID to locals
		c.Locals("user_id", session.UserID)
		c.Locals("user", user)

		citizenAuthUserID, organizationID, err := getCitizenauthMappingForLocalUser(c.Context(), session.UserID)
		if err != nil {
			return c.Status(fiber.StatusForbidden).JSON(utils.NewCitizenResponse(
				false,
				"CitizenAuth user mapping not found for local account",
				nil,
			))
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
			return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
				false,
				"Organization context missing",
				nil,
			))
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
func enforceAccessControls(c *fiber.Ctx, citizenAuthUserID, organizationID string) error {
	assigned, err := permissionSvc.IsUserAssignedToInstance(c.Context(), citizenAuthUserID, organizationID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to verify instance assignment",
			nil,
		))
	}
	if !assigned {
		return c.Status(fiber.StatusForbidden).JSON(utils.NewCitizenResponse(
			false,
			"Access denied: user is not assigned to this Citizen instance",
			nil,
		))
	}

	permissions, err := permissionSvc.GetUserPermissions(c.Context(), citizenAuthUserID, organizationID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to load user permissions",
			nil,
		))
	}
	c.Locals("app_permissions", permissions)

	appID := c.Params("app_name")
	if appID != "" {
		requiredRole := determineRequiredRole(c.Method())
		hasPermission, err := permissionSvc.CheckAppPermission(c.Context(), citizenAuthUserID, organizationID, appID, requiredRole)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
				false,
				"Failed to verify application permissions",
				nil,
			))
		}
		if !hasPermission {
			return c.Status(fiber.StatusForbidden).JSON(utils.NewCitizenResponse(
				false,
				fmt.Sprintf("Insufficient permissions for app '%s' (requires %s access)", appID, requiredRole),
				nil,
			))
		}
	}

	return nil
}

func determineRequiredRole(method string) string {
	switch method {
	case fiber.MethodGet, fiber.MethodHead:
		return "viewer"
	case fiber.MethodDelete:
		return "admin"
	default:
		return "member"
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
