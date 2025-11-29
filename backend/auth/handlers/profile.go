package handlers

import (
	"backend/database/api"
	"backend/utils"

	"github.com/gofiber/fiber/v2"
)

// GetProfile endpoint
func GetProfile(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)

	user, err := api.Users.GetUserByID(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(utils.NewCitizenResponse(
			false,
			"User not found",
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Profil başarıyla getirildi",
		user,
	))
}
