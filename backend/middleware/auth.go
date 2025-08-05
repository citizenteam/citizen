package middleware

import (
	"backend/database"
	"backend/handlers"
	"backend/models"
	"backend/utils"

	"github.com/gofiber/fiber/v2"
)

// Protected, SSO session ile yetkilendirme gerektirir
func Protected() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get SSO session
		ssoSessionID := c.Cookies("sso_session")
		
		// If SSO session is not found, return unauthorized
		if ssoSessionID == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
				false,
				"SSO session not found",
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