package handlers

import (
	"backend/database/api"
	"backend/models"
	"backend/utils"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// GetGitHubStatus returns GitHub connection status for user
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

	// Check if GitHub OAuth is configured
	isConfigured := utils.IsGitHubConfigured()

	// Get user's GitHub connection status from database
	user, err := api.Users.GetUserByID(c.Context(), userID.(int))
	if err != nil {
		log.Printf("[GITHUB] Failed to get user GitHub status: %v", err)
		// Return default values if query fails
		user = &models.User{
			GitHubConnected: false,
		}
	}

	githubConnected := user.GitHubConnected
	githubUsername := user.GitHubUsername
	githubID := user.GitHubID
	appID, appSlug, appName, _, installationID := utils.GetGitHubAppConfig()

	if !githubConnected && installationID != nil {
		githubConnected = true
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub status fetched successfully",
		fiber.Map{
			"github_configured":      isConfigured,
			"github_connected":       githubConnected,
			"github_username":        githubUsername,
			"github_id":              githubID,
			"github_app_id":          appID,
			"github_app_slug":        appSlug,
			"github_app_name":        appName,
			"github_installation_id": installationID,
		},
	))
}

// DisconnectGitHubAccount disconnects user's GitHub account from Citizen
// This removes the GitHub OAuth connection but keeps repository connections intact
func DisconnectGitHubAccount(c *fiber.Ctx) error {
	log.Printf("[GITHUB] DisconnectGitHubAccount called")

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

	// Get user info before disconnecting (for logging)
	user, err := api.Users.GetUserByID(c.Context(), userID.(int))
	if err != nil {
		log.Printf("[GITHUB] Failed to get user info: %v", err)
	}

	githubUsername := ""
	if user != nil && user.GitHubUsername != nil {
		githubUsername = *user.GitHubUsername
	}

	// Check for optional query param to also disconnect all repository connections
	disconnectRepos := c.QueryBool("disconnect_repos", false)

	if disconnectRepos {
		// Get all repository connections for this user
		connections, err := api.GitHub.GetGitHubRepositoryConnections(c.Context(), userID.(int))
		if err != nil {
			log.Printf("[GITHUB] Failed to get repository connections: %v", err)
		} else {
			// Get access token for webhook cleanup
			accessToken, _ := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))
			if accessToken == "" {
				// Try installation token
				if tokenResp, instErr := utils.GetGitHubInstallationToken(); instErr == nil {
					accessToken = tokenResp.Token
				}
			}

			// Disconnect each repository and clean up webhooks
			for _, conn := range connections {
				appName := conn["app_name"].(string)
				fullName := conn["full_name"].(string)

				// Delete webhook if exists
				if webhookID, ok := conn["webhook_id"].(*int64); ok && webhookID != nil && accessToken != "" {
					repoParts := strings.Split(fullName, "/")
					if len(repoParts) == 2 {
						owner, repoName := repoParts[0], repoParts[1]
						if err := utils.DeleteWebhook(accessToken, owner, repoName, *webhookID); err != nil {
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
	err = api.Users.DisconnectGitHub(c.Context(), userID.(int))
	if err != nil {
		log.Printf("[GITHUB] Failed to disconnect GitHub account: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to disconnect GitHub account",
			nil,
		))
	}

	log.Printf("[GITHUB] ✅ GitHub account disconnected for user %v (was: %s)", userID, githubUsername)

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub account disconnected successfully",
		fiber.Map{
			"disconnected_repos": disconnectRepos,
		},
	))
}
