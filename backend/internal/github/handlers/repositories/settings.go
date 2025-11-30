package repositories

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/internal/utils"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
)

// ToggleAutoDeploy toggles auto deploy for a repository
func ToggleAutoDeploy(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var toggleData struct {
		AutoDeploy bool `json:"auto_deploy"`
	}

	if err := c.BodyParser(&toggleData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	log.Printf("[GITHUB] ToggleAutoDeploy called for app %s, auto_deploy=%v", appName, toggleData.AutoDeploy)

	// Get repository connection
	repoConnection, err := api.GitHub.GetGitHubRepositoryConnectionByAppName(c.Context(), appName)
	if err != nil {
		log.Printf("[GITHUB] Failed to get repository connection: %v", err)
		return c.Status(fiber.StatusNotFound).JSON(utils.NewCitizenResponse(
			false,
			"Repository not connected",
			nil,
		))
	}

	var webhookID *int64 = repoConnection.WebhookID

	// Get access token for webhook management
	accessToken, tokenErr := githubservices.GetAccessTokenOptional(c.Context(), nil)
	if tokenErr != nil || accessToken == "" {
		log.Printf("[GITHUB] Failed to get installation token")
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
						log.Printf("[GITHUB] Failed to create webhook: %v", err)
					} else if webhookID != nil {
						log.Printf("[GITHUB] Created/found webhook with ID: %d", *webhookID)
					}
				}
			} else {
				// Delete webhook if exists
				if webhookID != nil {
					err := githubservices.DeleteWebhook(accessToken, owner, repoName, *webhookID)
					if err != nil {
						log.Printf("[GITHUB] Failed to delete webhook: %v", err)
					} else {
						log.Printf("[GITHUB] Deleted webhook with ID: %d", *webhookID)
						webhookID = nil
					}
				}
			}
		}
	}

	// Update database
	err = api.GitHub.UpdateAutoDeploy(c.Context(), appName, toggleData.AutoDeploy, webhookID)
	if err != nil {
		log.Printf("[GITHUB] Failed to update auto deploy in database: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to update auto deploy setting",
			nil,
		))
	}

	statusText := map[bool]string{true: "enabled", false: "disabled"}[toggleData.AutoDeploy]
	log.Printf("[GITHUB] ✅ Auto deploy %s for app: %s", statusText, appName)

	return c.JSON(utils.NewCitizenResponse(
		true,
		fmt.Sprintf("Auto deploy %s successfully", statusText),
		fiber.Map{
			"app_name":    appName,
			"auto_deploy": toggleData.AutoDeploy,
			"webhook_id":  webhookID,
		},
	))
}

// GetRepositoryConnections lists connected repositories for user
func GetRepositoryConnections(c *fiber.Ctx) error {
	log.Printf("[GITHUB] GetRepositoryConnections called")

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

	log.Printf("[GITHUB] Getting repository connections for user: %v", userID)

	// Get repository connections from database
	connections, err := api.GitHub.GetGitHubRepositoryConnections(c.Context(), userID.(int))
	if err != nil {
		log.Printf("[GITHUB] Failed to fetch repository connections: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to fetch repository connections",
			nil,
		))
	}

	log.Printf("[GITHUB] Found %d repository connections", len(connections))

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Repository connections fetched successfully",
		fiber.Map{
			"connections": connections,
			"total":       len(connections),
		},
	))
}
