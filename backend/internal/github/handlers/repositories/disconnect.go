package repositories

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/pkg/logger"
	"backend/pkg/response"

	"github.com/gofiber/fiber/v2"
)

var logDisconnect = logger.Default().WithComponent("github-repos")

// DisconnectRepository disconnects a GitHub repository from Citizen app
func DisconnectRepository(c *fiber.Ctx) error {
	logDisconnect.Debug("DisconnectRepository called")

	appName := c.Params("app_name")
	if appName == "" {
		logDisconnect.Warn("App name is required")
		return response.BadRequest(c, "App name is required")
	}

	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		logDisconnect.Warn("User not authenticated")
		return response.Unauthorized(c, "User not authenticated")
	}

	logDisconnect.WithFields(map[string]interface{}{
		"app_name": appName,
		"user_id":  userID,
	}).Debug("Disconnecting repository")

	// Get repository connection from database to get webhook info
	repoConnection, err := api.GitHub.GetGitHubRepositoryConnection(c.Context(), userID.(int), appName)
	if err != nil {
		logDisconnect.WithField("error", err.Error()).Debug("Repository connection not found")
		return response.NotFound(c, "Repository connection not found")
	}

	webhookID := repoConnection.WebhookID
	fullName := repoConnection.FullName

	// Get access token using the service helper
	accessToken, tokenErr := githubservices.GetAccessToken()

	if tokenErr == nil && accessToken != "" && webhookID != nil {
		// Delete webhook if exists
		owner, repoName, parseErr := githubservices.ParseRepositoryFullName(fullName)
		if parseErr == nil {
			err = githubservices.DeleteWebhook(accessToken, owner, repoName, *webhookID)
			if err != nil {
				logDisconnect.WithField("error", err.Error()).Warn("Failed to delete webhook")
				// Continue with disconnection even if webhook deletion fails
			} else {
				logDisconnect.Info("Webhook deleted successfully")
			}
		}
	}

	// Soft delete repository connection from database
	err = api.GitHub.DisconnectGitHubRepository(c.Context(), userID.(int), appName)

	if err != nil {
		logDisconnect.WithField("error", err.Error()).Error("Failed to disconnect repository")
		return response.InternalServerError(c, "Failed to disconnect repository")
	}

	logDisconnect.WithField("app_name", appName).Info("Repository disconnected")

	return response.SuccessWithMessage(c, "Repository disconnected successfully", fiber.Map{
		"app_name": appName,
	})
}
