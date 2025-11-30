package status

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/internal/utils"
	"log"

	"github.com/gofiber/fiber/v2"
)

// GetGitHubStatus returns GitHub connection status for current user
func GetGitHubStatus(c *fiber.Ctx) error {
	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	log.Printf("[GITHUB] GetGitHubStatus called for user: %v", userID)

	// Check if GitHub is configured at instance level
	isConfigured := githubservices.IsGitHubConfigured()

	// Get user's GitHub connection info
	user, err := api.Users.GetUserByID(c.Context(), userID.(int))
	if err != nil {
		log.Printf("[GITHUB] Failed to get user info: %v", err)
		// User might not have connected GitHub yet, that's okay
		return c.JSON(utils.NewCitizenResponse(
			true,
			"GitHub status retrieved",
			fiber.Map{
				"configured":       isConfigured,
				"github_connected": false,
				"github_user":      nil,
			},
		))
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

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub status retrieved",
		responseData,
	))
}

// DisconnectGitHubAccount disconnects user's GitHub account
func DisconnectGitHubAccount(c *fiber.Ctx) error {
	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	log.Printf("[GITHUB] DisconnectGitHubAccount called for user: %v", userID)

	// Check for optional query param to also disconnect all repository connections
	disconnectRepos := c.QueryBool("disconnect_repos", false)

	if disconnectRepos {
		// Get all repository connections for this user
		connections, err := api.GitHub.GetGitHubRepositoryConnections(c.Context(), userID.(int))
		if err != nil {
			log.Printf("[GITHUB] Failed to get repository connections: %v", err)
		} else {
			// Get access token for webhook cleanup
			accessToken, _ := githubservices.GetAccessToken(c.Context(), userID.(int))

			// Disconnect each repository and clean up webhooks
			for _, conn := range connections {
				appName := conn["app_name"].(string)
				fullName := conn["full_name"].(string)

				// Delete webhook if exists
				if webhookID, ok := conn["webhook_id"].(*int64); ok && webhookID != nil && accessToken != "" {
					owner, repoName, parseErr := githubservices.ParseRepositoryFullName(fullName)
					if parseErr == nil {
						if err := githubservices.DeleteWebhook(accessToken, owner, repoName, *webhookID); err != nil {
							log.Printf("[GITHUB] Failed to delete webhook for %s: %v", appName, err)
						} else {
							log.Printf("[GITHUB] Webhook deleted for %s", appName)
						}
					}
				}

				// Soft delete repository connection
				if err := api.GitHub.DisconnectGitHubRepository(c.Context(), userID.(int), appName); err != nil {
					log.Printf("[GITHUB] Failed to disconnect repository %s: %v", appName, err)
				} else {
					log.Printf("[GITHUB] Repository disconnected: %s", appName)
				}
			}
		}
	}

	// Disconnect GitHub account from user
	err := api.Users.DisconnectGitHub(c.Context(), userID.(int))
	if err != nil {
		log.Printf("[GITHUB] Failed to disconnect GitHub account: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to disconnect GitHub account",
			nil,
		))
	}

	log.Printf("[GITHUB] ✅ GitHub account disconnected for user: %v", userID)

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub account disconnected successfully",
		fiber.Map{
			"disconnected_repos": disconnectRepos,
		},
	))
}
