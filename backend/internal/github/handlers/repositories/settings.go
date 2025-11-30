package repositories

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/pkg/logger"
	"backend/pkg/response"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

var logSettings = logger.Default().WithComponent("github-repos")

// ToggleAutoDeploy toggles auto deploy for a repository
func ToggleAutoDeploy(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return response.BadRequest(c, "App name is required")
	}

	var toggleData struct {
		AutoDeploy bool `json:"auto_deploy"`
	}

	if err := c.BodyParser(&toggleData); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}

	logSettings.WithFields(map[string]interface{}{
		"app_name":    appName,
		"auto_deploy": toggleData.AutoDeploy,
	}).Debug("ToggleAutoDeploy called")

	// Get repository connection
	repoConnection, err := api.GitHub.GetGitHubRepositoryConnectionByAppName(c.Context(), appName)
	if err != nil {
		logSettings.WithField("error", err.Error()).Warn("Failed to get repository connection")
		return response.NotFound(c, "Repository not connected")
	}

	var webhookID *int64 = repoConnection.WebhookID

	// Get access token for webhook management
	accessToken := githubservices.GetAccessTokenOptional()
	if accessToken == "" {
		logSettings.Debug("Failed to get installation token")
		// Continue without webhook management, just update database
	} else if repoConnection.FullName != "" {
		owner, repoName, parseErr := githubservices.ParseRepositoryFullName(repoConnection.FullName)
		if parseErr == nil {
			webhookURL := githubservices.GetWebhookURL(c.BaseURL())

			if toggleData.AutoDeploy {
				// Create webhook if not exists
				if webhookID == nil {
					webhookID, err = githubservices.CreateOrFindWebhook(accessToken, owner, repoName, webhookURL)
					if err != nil {
						logSettings.WithField("error", err.Error()).Warn("Failed to create webhook")
					} else if webhookID != nil {
						logSettings.WithField("webhook_id", *webhookID).Info("Created/found webhook")
					}
				}
			} else {
				// Delete webhook if exists
				if webhookID != nil {
					err := githubservices.DeleteWebhook(accessToken, owner, repoName, *webhookID)
					if err != nil {
						logSettings.WithField("error", err.Error()).Warn("Failed to delete webhook")
					} else {
						logSettings.WithField("webhook_id", *webhookID).Info("Deleted webhook")
						webhookID = nil
					}
				}
			}
		}
	}

	// Update database
	err = api.GitHub.UpdateAutoDeploy(c.Context(), appName, toggleData.AutoDeploy, webhookID)
	if err != nil {
		logSettings.WithField("error", err.Error()).Error("Failed to update auto deploy in database")
		return response.InternalServerError(c, "Failed to update auto deploy setting")
	}

	statusText := map[bool]string{true: "enabled", false: "disabled"}[toggleData.AutoDeploy]
	logSettings.WithFields(map[string]interface{}{
		"app_name": appName,
		"status":   statusText,
	}).Info("Auto deploy updated")

	return response.SuccessWithMessage(c, fmt.Sprintf("Auto deploy %s successfully", statusText), fiber.Map{
		"app_name":    appName,
		"auto_deploy": toggleData.AutoDeploy,
		"webhook_id":  webhookID,
	})
}

// GetRepositoryConnections lists connected repositories for user
func GetRepositoryConnections(c *fiber.Ctx) error {
	logSettings.Debug("GetRepositoryConnections called")

	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		logSettings.Warn("User not authenticated")
		return response.Unauthorized(c, "User not authenticated")
	}

	logSettings.WithField("user_id", userID).Debug("Getting repository connections")

	// Get repository connections from database
	connections, err := api.GitHub.GetGitHubRepositoryConnections(c.Context(), userID.(int))
	if err != nil {
		logSettings.WithField("error", err.Error()).Error("Failed to fetch repository connections")
		return response.InternalServerError(c, "Failed to fetch repository connections")
	}

	logSettings.WithField("count", len(connections)).Debug("Found repository connections")

	return response.SuccessWithMessage(c, "Repository connections fetched successfully", fiber.Map{
		"connections": connections,
		"total":       len(connections),
	})
}
