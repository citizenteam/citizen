package repositories

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/internal/utils"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
)

// ConnectRepository connects a GitHub repository to Citizen app
func ConnectRepository(c *fiber.Ctx) error {
	log.Printf("[GITHUB] ConnectRepository called")

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

	log.Printf("[GITHUB] User ID: %v", userID)

	var connectData struct {
		AppName      string `json:"app_name"`
		RepositoryID int64  `json:"repository_id"`
		FullName     string `json:"full_name"`
		AutoDeploy   bool   `json:"auto_deploy"`
		DeployBranch string `json:"deploy_branch"`
	}

	if err := c.BodyParser(&connectData); err != nil {
		log.Printf("[GITHUB] Failed to parse request body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	log.Printf("[GITHUB] Connect data: %+v", connectData)

	if connectData.AppName == "" || connectData.RepositoryID == 0 || connectData.FullName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name, repository ID, and full name are required",
			nil,
		))
	}

	// Set default branch if not provided
	if connectData.DeployBranch == "" {
		connectData.DeployBranch = "main"
	}

	// Get access token using the service helper
	accessToken, err := githubservices.GetAccessToken(c.Context(), userID.(int))
	if err != nil {
		log.Printf("[GITHUB] Failed to get GitHub access token: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"GitHub not connected or access token not found",
			nil,
		))
	}

	// Parse repository full name
	owner, repoName, err := githubservices.ParseRepositoryFullName(connectData.FullName)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			err.Error(),
			nil,
		))
	}

	githubRepo, err := githubservices.GetRepositoryInfo(accessToken, owner, repoName)
	if err != nil {
		log.Printf("[GITHUB] Failed to get repository info: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to get repository information",
			nil,
		))
	}

	// Create webhook if auto deploy is enabled
	var webhookID *int64
	webhookURL := githubservices.GetWebhookURL(c.BaseURL())

	if connectData.AutoDeploy {
		webhookID, err = githubservices.CreateOrFindWebhook(accessToken, owner, repoName, webhookURL)
		if err != nil {
			log.Printf("[GITHUB] Failed to create webhook: %v", err)
			// Disable auto deploy on error
			connectData.AutoDeploy = false
		} else if webhookID != nil {
			log.Printf("[GITHUB] Webhook created/found with ID: %d", *webhookID)
		}
	}

	// Save repository connection to database
	log.Printf("[GITHUB] Saving repository connection to database...")
	log.Printf("[GITHUB] Parameters: userID=%v, appName=%s, repoID=%d, fullName=%s, autoDeploy=%t, deployBranch=%s, webhookID=%v",
		userID, connectData.AppName, connectData.RepositoryID, connectData.FullName, connectData.AutoDeploy, connectData.DeployBranch, webhookID)

	err = api.GitHub.ConnectGitHubRepository(c.Context(), userID.(int), connectData.AppName, connectData.RepositoryID, connectData.FullName, githubRepo.Name, githubRepo.Owner.Login, githubRepo.CloneURL, githubRepo.HTMLURL, githubRepo.Private, githubRepo.DefaultBranch, connectData.AutoDeploy, connectData.DeployBranch, webhookID)

	if err != nil {
		log.Printf("[GITHUB] ❌ Failed to save repository connection: %v", err)
		// Don't fail the entire connection, just log the error
	} else {
		log.Printf("[GITHUB] ✅ Repository connection saved successfully")
	}

	log.Printf("[GITHUB] ✅ Repository connected: %s to app %s", connectData.FullName, connectData.AppName)

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Repository connected successfully",
		fiber.Map{
			"app_name":       connectData.AppName,
			"repository":     githubRepo,
			"auto_deploy":    connectData.AutoDeploy,
			"deploy_branch":  connectData.DeployBranch,
			"webhook_id":     webhookID,
			"webhook_active": webhookID != nil,
		},
	))
}

// ConnectExistingAppToRepository connects an existing Citizen app to a GitHub repository
// This is for apps that were created without a repository and need to be linked later
func ConnectExistingAppToRepository(c *fiber.Ctx) error {
	log.Printf("[GITHUB] ConnectExistingAppToRepository called")

	appName := c.Params("app_name")
	if appName == "" {
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

	var connectData struct {
		RepositoryID int64  `json:"repository_id"`
		FullName     string `json:"full_name"`
		AutoDeploy   bool   `json:"auto_deploy"`
		DeployBranch string `json:"deploy_branch"`
	}

	if err := c.BodyParser(&connectData); err != nil {
		log.Printf("[GITHUB] Failed to parse request body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	if connectData.RepositoryID == 0 || connectData.FullName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Repository ID and full name are required",
			nil,
		))
	}

	// Check if app already has a repository connection
	existingConn, err := api.GitHub.GetGitHubRepositoryConnection(c.Context(), userID.(int), appName)
	if err == nil && existingConn != nil {
		return c.Status(fiber.StatusConflict).JSON(utils.NewCitizenResponse(
			false,
			fmt.Sprintf("App '%s' is already connected to repository '%s'. Disconnect first to connect a different repository.", appName, existingConn.FullName),
			fiber.Map{
				"current_repository": existingConn.FullName,
			},
		))
	}

	// Set default branch if not provided
	if connectData.DeployBranch == "" {
		connectData.DeployBranch = "main"
	}

	// Get access token using the service helper
	accessToken, err := githubservices.GetAccessToken(c.Context(), userID.(int))
	if err != nil {
		log.Printf("[GITHUB] Failed to get GitHub access token: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"GitHub not connected or access token not found",
			nil,
		))
	}

	// Parse repository full name
	owner, repoName, err := githubservices.ParseRepositoryFullName(connectData.FullName)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			err.Error(),
			nil,
		))
	}

	githubRepo, err := githubservices.GetRepositoryInfo(accessToken, owner, repoName)
	if err != nil {
		log.Printf("[GITHUB] Failed to get repository info: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to get repository information from GitHub",
			nil,
		))
	}

	// Create webhook if auto deploy is enabled
	var webhookID *int64
	webhookURL := githubservices.GetWebhookURL(c.BaseURL())

	if connectData.AutoDeploy {
		webhookID, err = githubservices.CreateOrFindWebhook(accessToken, owner, repoName, webhookURL)
		if err != nil {
			log.Printf("[GITHUB] Failed to create webhook: %v", err)
			// Disable auto deploy on error
			connectData.AutoDeploy = false
		} else if webhookID != nil {
			log.Printf("[GITHUB] Webhook created/found with ID: %d", *webhookID)
		}
	}

	// Save repository connection to database
	err = api.GitHub.ConnectGitHubRepository(
		c.Context(),
		userID.(int),
		appName,
		connectData.RepositoryID,
		connectData.FullName,
		githubRepo.Name,
		githubRepo.Owner.Login,
		githubRepo.CloneURL,
		githubRepo.HTMLURL,
		githubRepo.Private,
		githubRepo.DefaultBranch,
		connectData.AutoDeploy,
		connectData.DeployBranch,
		webhookID,
	)

	if err != nil {
		log.Printf("[GITHUB] Failed to save repository connection: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to save repository connection",
			nil,
		))
	}

	log.Printf("[GITHUB] ✅ Existing app '%s' connected to repository '%s'", appName, connectData.FullName)

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Repository connected to existing app successfully",
		fiber.Map{
			"app_name":       appName,
			"repository":     githubRepo,
			"auto_deploy":    connectData.AutoDeploy,
			"deploy_branch":  connectData.DeployBranch,
			"webhook_id":     webhookID,
			"webhook_active": webhookID != nil,
		},
	))
}
