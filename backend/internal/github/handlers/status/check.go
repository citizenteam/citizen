package status

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/pkg/logger"
	"backend/pkg/response"

	"github.com/gofiber/fiber/v2"
)

var log = logger.Default().WithComponent("github-status")

// GetGitHubStatus returns GitHub connection status for current user
func GetGitHubStatus(c *fiber.Ctx) error {
	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return response.Unauthorized(c, "User not authenticated")
	}

	log.WithField("user_id", userID).Debug("GetGitHubStatus called")

	// Check if GitHub is configured at instance level
	isConfigured := githubservices.IsGitHubConfigured()

	// Get user's GitHub connection info
	user, err := api.Users.GetUserByID(c.Context(), userID.(int))
	if err != nil {
		log.WithField("error", err.Error()).Debug("Failed to get user info")
		// User might not have connected GitHub yet, that's okay
		return response.SuccessWithMessage(c, "GitHub status retrieved", fiber.Map{
			"configured":       isConfigured,
			"github_connected": false,
			"github_user":      nil,
		})
	}

	githubUserID := user.GitHubID
	githubLogin := user.GitHubUsername
	githubConnected := user.GitHubConnected

	// Check if GitHub App (manifest) is configured
	appID, appSlug, appName, _, installationID := githubservices.GetGitHubAppConfig()

	if !githubConnected && installationID != nil {
		githubConnected = true
	}

	responseData := fiber.Map{
		"github_configured": isConfigured,
		"github_connected":  githubConnected,
		"github_username":   githubLogin,
		"github_id":         githubUserID,
	}

	// Add GitHub App info if available
	if appID != nil {
		responseData["github_app_id"] = *appID
		responseData["github_app_slug"] = appSlug
		responseData["github_app_name"] = appName
		responseData["github_installation_id"] = installationID
	}

	return response.SuccessWithMessage(c, "GitHub status retrieved", responseData)
}

// DisconnectGitHubAccount disconnects user's GitHub account
func DisconnectGitHubAccount(c *fiber.Ctx) error {
	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return response.Unauthorized(c, "User not authenticated")
	}

	log.WithField("user_id", userID).Debug("DisconnectGitHubAccount called")

	// Check for optional query param to also disconnect all repository connections
	disconnectRepos := c.QueryBool("disconnect_repos", false)

	if disconnectRepos {
		// Get all repository connections for this user
		connections, err := api.GitHub.GetGitHubRepositoryConnections(c.Context(), userID.(int))
		if err != nil {
			log.WithField("error", err.Error()).Warn("Failed to get repository connections")
		} else {
			// Get access token for webhook cleanup
			accessToken, _ := githubservices.GetAccessToken()

			// Disconnect each repository and clean up webhooks
			for _, conn := range connections {
				appName := conn["app_name"].(string)
				fullName := conn["full_name"].(string)

				// Delete webhook if exists
				if webhookID, ok := conn["webhook_id"].(*int64); ok && webhookID != nil && accessToken != "" {
					owner, repoName, parseErr := githubservices.ParseRepositoryFullName(fullName)
					if parseErr == nil {
						if err := githubservices.DeleteWebhook(accessToken, owner, repoName, *webhookID); err != nil {
							log.WithFields(map[string]interface{}{
								"app_name": appName,
								"error":    err.Error(),
							}).Warn("Failed to delete webhook")
						} else {
							log.WithField("app_name", appName).Info("Webhook deleted")
						}
					}
				}

				// Soft delete repository connection
				if err := api.GitHub.DisconnectGitHubRepository(c.Context(), userID.(int), appName); err != nil {
					log.WithFields(map[string]interface{}{
						"app_name": appName,
						"error":    err.Error(),
					}).Warn("Failed to disconnect repository")
				} else {
					log.WithField("app_name", appName).Info("Repository disconnected")
				}
			}
		}
	}

	// Disconnect GitHub account from user
	err := api.Users.DisconnectGitHub(c.Context(), userID.(int))
	if err != nil {
		log.WithField("error", err.Error()).Error("Failed to disconnect GitHub account")
		return response.InternalServerError(c, "Failed to disconnect GitHub account")
	}

	log.WithField("user_id", userID).Info("GitHub account disconnected")

	return response.SuccessWithMessage(c, "GitHub account disconnected successfully", fiber.Map{
		"disconnected_repos": disconnectRepos,
	})
}
