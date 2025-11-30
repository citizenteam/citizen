package repositories

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/pkg/logger"
	"backend/pkg/response"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

var logConnect = logger.Default().WithComponent("github-repos")

// ConnectRepository connects a GitHub repository to Citizen app
func ConnectRepository(c *fiber.Ctx) error {
	logConnect.Debug("ConnectRepository called")

	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		logConnect.Warn("User not authenticated")
		return response.Unauthorized(c, "User not authenticated")
	}

	logConnect.WithField("user_id", userID).Debug("User authenticated")

	var connectData struct {
		AppName      string `json:"app_name"`
		RepositoryID int64  `json:"repository_id"`
		FullName     string `json:"full_name"`
		AutoDeploy   bool   `json:"auto_deploy"`
		DeployBranch string `json:"deploy_branch"`
	}

	if err := c.BodyParser(&connectData); err != nil {
		logConnect.WithField("error", err.Error()).Warn("Failed to parse request body")
		return response.BadRequest(c, "Invalid request body")
	}

	logConnect.WithField("connect_data", connectData).Debug("Connect data received")

	if connectData.AppName == "" || connectData.RepositoryID == 0 || connectData.FullName == "" {
		return response.BadRequest(c, "App name, repository ID, and full name are required")
	}

	// Set default branch if not provided
	if connectData.DeployBranch == "" {
		connectData.DeployBranch = "main"
	}

	// Get access token using the service helper
	accessToken, err := githubservices.GetAccessToken(c.Context(), userID.(int))
	if err != nil {
		logConnect.WithField("error", err.Error()).Error("Failed to get GitHub access token")
		return response.Unauthorized(c, "GitHub not connected or access token not found")
	}

	// Parse repository full name
	owner, repoName, err := githubservices.ParseRepositoryFullName(connectData.FullName)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	githubRepo, err := githubservices.GetRepositoryInfo(accessToken, owner, repoName)
	if err != nil {
		logConnect.WithField("error", err.Error()).Error("Failed to get repository info")
		return response.InternalServerError(c, "Failed to get repository information")
	}

	// Create webhook if auto deploy is enabled
	var webhookID *int64
	webhookURL := githubservices.GetWebhookURL(c.BaseURL())

	if connectData.AutoDeploy {
		webhookID, err = githubservices.CreateOrFindWebhook(accessToken, owner, repoName, webhookURL)
		if err != nil {
			logConnect.WithField("error", err.Error()).Warn("Failed to create webhook")
			// Disable auto deploy on error
			connectData.AutoDeploy = false
		} else if webhookID != nil {
			logConnect.WithField("webhook_id", *webhookID).Info("Webhook created/found")
		}
	}

	// Save repository connection to database
	logConnect.WithFields(map[string]interface{}{
		"user_id":       userID,
		"app_name":      connectData.AppName,
		"repository_id": connectData.RepositoryID,
		"full_name":     connectData.FullName,
		"auto_deploy":   connectData.AutoDeploy,
		"deploy_branch": connectData.DeployBranch,
		"webhook_id":    webhookID,
	}).Debug("Saving repository connection to database")

	err = api.GitHub.ConnectGitHubRepository(c.Context(), userID.(int), connectData.AppName, connectData.RepositoryID, connectData.FullName, githubRepo.Name, githubRepo.Owner.Login, githubRepo.CloneURL, githubRepo.HTMLURL, githubRepo.Private, githubRepo.DefaultBranch, connectData.AutoDeploy, connectData.DeployBranch, webhookID)

	if err != nil {
		logConnect.WithField("error", err.Error()).Error("Failed to save repository connection")
		// Don't fail the entire connection, just log the error
	} else {
		logConnect.Info("Repository connection saved successfully")
	}

	logConnect.WithFields(map[string]interface{}{
		"full_name": connectData.FullName,
		"app_name":  connectData.AppName,
	}).Info("Repository connected")

	return response.SuccessWithMessage(c, "Repository connected successfully", fiber.Map{
		"app_name":       connectData.AppName,
		"repository":     githubRepo,
		"auto_deploy":    connectData.AutoDeploy,
		"deploy_branch":  connectData.DeployBranch,
		"webhook_id":     webhookID,
		"webhook_active": webhookID != nil,
	})
}

// ConnectExistingAppToRepository connects an existing Citizen app to a GitHub repository
// This is for apps that were created without a repository and need to be linked later
func ConnectExistingAppToRepository(c *fiber.Ctx) error {
	logConnect.Debug("ConnectExistingAppToRepository called")

	appName := c.Params("app_name")
	if appName == "" {
		return response.BadRequest(c, "App name is required")
	}

	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		logConnect.Warn("User not authenticated")
		return response.Unauthorized(c, "User not authenticated")
	}

	var connectData struct {
		RepositoryID int64  `json:"repository_id"`
		FullName     string `json:"full_name"`
		AutoDeploy   bool   `json:"auto_deploy"`
		DeployBranch string `json:"deploy_branch"`
	}

	if err := c.BodyParser(&connectData); err != nil {
		logConnect.WithField("error", err.Error()).Warn("Failed to parse request body")
		return response.BadRequest(c, "Invalid request body")
	}

	if connectData.RepositoryID == 0 || connectData.FullName == "" {
		return response.BadRequest(c, "Repository ID and full name are required")
	}

	// Check if app already has a repository connection
	existingConn, err := api.GitHub.GetGitHubRepositoryConnection(c.Context(), userID.(int), appName)
	if err == nil && existingConn != nil {
		return response.Conflict(c, fmt.Sprintf("App '%s' is already connected to repository '%s'. Disconnect first to connect a different repository.", appName, existingConn.FullName))
	}

	// Set default branch if not provided
	if connectData.DeployBranch == "" {
		connectData.DeployBranch = "main"
	}

	// Get access token using the service helper
	accessToken, err := githubservices.GetAccessToken(c.Context(), userID.(int))
	if err != nil {
		logConnect.WithField("error", err.Error()).Warn("Failed to get GitHub access token")
		return response.Unauthorized(c, "GitHub not connected or access token not found")
	}

	// Parse repository full name
	owner, repoName, err := githubservices.ParseRepositoryFullName(connectData.FullName)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	githubRepo, err := githubservices.GetRepositoryInfo(accessToken, owner, repoName)
	if err != nil {
		logConnect.WithField("error", err.Error()).Error("Failed to get repository info from GitHub")
		return response.InternalServerError(c, "Failed to get repository information from GitHub")
	}

	// Create webhook if auto deploy is enabled
	var webhookID *int64
	webhookURL := githubservices.GetWebhookURL(c.BaseURL())

	if connectData.AutoDeploy {
		webhookID, err = githubservices.CreateOrFindWebhook(accessToken, owner, repoName, webhookURL)
		if err != nil {
			logConnect.WithField("error", err.Error()).Warn("Failed to create webhook")
			// Disable auto deploy on error
			connectData.AutoDeploy = false
		} else if webhookID != nil {
			logConnect.WithField("webhook_id", *webhookID).Info("Webhook created/found")
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
		logConnect.WithField("error", err.Error()).Error("Failed to save repository connection")
		return response.InternalServerError(c, "Failed to save repository connection")
	}

	logConnect.WithFields(map[string]interface{}{
		"app_name":  appName,
		"full_name": connectData.FullName,
	}).Info("Existing app connected to repository")

	return response.SuccessWithMessage(c, "Repository connected to existing app successfully", fiber.Map{
		"app_name":       appName,
		"repository":     githubRepo,
		"auto_deploy":    connectData.AutoDeploy,
		"deploy_branch":  connectData.DeployBranch,
		"webhook_id":     webhookID,
		"webhook_active": webhookID != nil,
	})
}
