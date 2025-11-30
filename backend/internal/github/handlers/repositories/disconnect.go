package repositories

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/internal/utils"
	"log"

	"github.com/gofiber/fiber/v2"
)

// DisconnectRepository disconnects a GitHub repository from Citizen app
func DisconnectRepository(c *fiber.Ctx) error {
	log.Printf("[GITHUB] DisconnectRepository called")

	appName := c.Params("app_name")
	if appName == "" {
		log.Printf("[GITHUB] App name is required")
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		log.Printf("[GITHUB] User not authenticated")
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	log.Printf("[GITHUB] Disconnecting repository for app: %s, user: %v", appName, userID)

	// Get repository connection from database to get webhook info
	repoConnection, err := api.GitHub.GetGitHubRepositoryConnection(c.Context(), userID.(int), appName)
	if err != nil {
		log.Printf("[GITHUB] Repository connection not found: %v", err)
		return c.Status(fiber.StatusNotFound).JSON(utils.NewCitizenResponse(
			false,
			"Repository connection not found",
			nil,
		))
	}

	webhookID := repoConnection.WebhookID
	fullName := repoConnection.FullName

	// Get access token using the service helper
	accessToken, tokenErr := githubservices.GetAccessToken(c.Context(), userID.(int))

	if tokenErr == nil && accessToken != "" && webhookID != nil {
		// Delete webhook if exists
		owner, repoName, parseErr := githubservices.ParseRepositoryFullName(fullName)
		if parseErr == nil {
			err = githubservices.DeleteWebhook(accessToken, owner, repoName, *webhookID)
			if err != nil {
				log.Printf("[GITHUB] Failed to delete webhook: %v", err)
				// Continue with disconnection even if webhook deletion fails
			} else {
				log.Printf("[GITHUB] Webhook deleted successfully")
			}
		}
	}

	// Soft delete repository connection from database
	err = api.GitHub.DisconnectGitHubRepository(c.Context(), userID.(int), appName)

	if err != nil {
		log.Printf("[GITHUB] Failed to disconnect repository: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to disconnect repository",
			nil,
		))
	}

	log.Printf("[GITHUB] ✅ Repository disconnected from app: %s", appName)

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Repository disconnected successfully",
		fiber.Map{
			"app_name": appName,
		},
	))
}
