package middleware

import (
	"backend/database"
	"backend/handlers"
	"backend/models"
	"backend/utils"

	"github.com/gofiber/fiber/v2"
)

// Protected, SSO session veya JWT ile yetkilendirme gerektirir
func Protected() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// First check if JWT auth already succeeded
		if c.Locals("auth_type") == "jwt" {
			// JWT auth already validated by JWTAuth middleware
			// Convert string user_id to int for backward compatibility
			// For now, use a default user_id or skip user lookup
			c.Locals("user_id", 1) // TODO: Use proper user mapping
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
		
		return c.Next()
	}
}

