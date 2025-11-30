package handlers

import (
	"backend/internal/database/api"
	"backend/internal/utils"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// ListGitHubRepositories lists user's GitHub repositories
func ListGitHubRepositories(c *fiber.Ctx) error {
	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	page := c.QueryInt("page", 1)

	// If GitHub App (manifest) is installed, prefer installation token
	appRepos, appErr := utils.GetInstallationRepositories(page)
	if appErr == nil {
		return c.JSON(utils.NewCitizenResponse(
			true,
			"Repositories fetched successfully",
			fiber.Map{
				"repositories": appRepos,
				"page":         page,
				"total":        len(appRepos),
				"github_app":   true,
			},
		))
	}

	// Get user's GitHub access token from database
	accessToken, err := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))

	if err != nil {
		log.Printf("[GITHUB] Failed to get user GitHub access token: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"GitHub not connected or access token not found",
			nil,
		))
	}

	if accessToken == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"GitHub access token is empty",
			nil,
		))
	}

	repos, err := utils.GetUserRepositories(accessToken, page)
	if err != nil {
		log.Printf("[GITHUB] Failed to get repositories: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to fetch repositories",
			nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Repositories fetched successfully",
		fiber.Map{
			"repositories": repos,
			"page":         page,
			"total":        len(repos),
		},
	))
}

// GetRepositoryBranches lists branches for a specific repository
func GetRepositoryBranches(c *fiber.Ctx) error {
	// Get repo full name from params (owner/repo format)
	owner := c.Params("owner")
	repo := c.Params("repo")

	if owner == "" || repo == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Repository owner and name are required",
			nil,
		))
	}

	fullName := owner + "/" + repo
	log.Printf("[GITHUB] GetRepositoryBranches called for: %s", fullName)

	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	// Try GitHub App installation token first
	branches, appErr := utils.GetRepositoryBranchesWithApp(fullName)
	if appErr == nil && len(branches) > 0 {
		return c.JSON(utils.NewCitizenResponse(
			true,
			"Branches fetched",
			fiber.Map{
				"branches":   branches,
				"total":      len(branches),
				"github_app": true,
			},
		))
	}

	// Fall back to user's OAuth token
	accessToken, err := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))
	if err != nil || accessToken == "" {
		log.Printf("[GITHUB] No access token, returning default branches")
		return c.JSON(utils.NewCitizenResponse(
			true,
			"Default branches (no GitHub access)",
			fiber.Map{
				"branches": []string{"main", "master", "develop"},
				"total":    3,
				"default":  true,
			},
		))
	}

	// Fetch branches using user's token
	branches, err = utils.GetRepositoryBranchesWithToken(fullName, accessToken)
	if err != nil {
		log.Printf("[GITHUB] Failed to get branches: %v", err)
		return c.JSON(utils.NewCitizenResponse(
			true,
			"Default branches (fetch failed)",
			fiber.Map{
				"branches": []string{"main", "master", "develop"},
				"total":    3,
				"default":  true,
			},
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Branches fetched",
		fiber.Map{
			"branches": branches,
			"total":    len(branches),
		},
	))
}

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

	// Get token via GitHub App installation if available, otherwise fall back to user token
	var accessToken string
	if tokenResp, err := utils.GetGitHubInstallationToken(); err == nil && tokenResp != nil {
		accessToken = tokenResp.Token
	} else {
		token, err := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))
		if err != nil {
			log.Printf("[GITHUB] Failed to get user GitHub access token: %v", err)
			return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
				false,
				"GitHub not connected or access token not found",
				nil,
			))
		}
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
				false,
				"GitHub access token is empty",
				nil,
			))
		}
		accessToken = token
	}

	// Get repository details from GitHub
	repoParts := strings.Split(connectData.FullName, "/")
	if len(repoParts) != 2 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid repository full name format (should be owner/repo)",
			nil,
		))
	}

	owner, repoName := repoParts[0], repoParts[1]

	githubRepo, err := utils.GetRepositoryInfo(accessToken, owner, repoName)
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
	baseURL := c.BaseURL()
	if strings.HasPrefix(baseURL, "http://") && !strings.Contains(baseURL, "localhost") && !strings.Contains(baseURL, "127.0.0.1") {
		baseURL = strings.Replace(baseURL, "http://", "https://", 1)
	}
	webhookURL := fmt.Sprintf("%s/api/v1/github/webhook", baseURL)

	if connectData.AutoDeploy {
		webhook, err := utils.CreateWebhook(accessToken, owner, repoName, webhookURL)
		if err != nil {
			log.Printf("[GITHUB] Failed to create webhook: %v", err)
			// Check if webhook already exists
			if strings.Contains(err.Error(), "Hook already exists") {
				log.Printf("[GITHUB] Webhook already exists, trying to find existing webhook...")
				existingWebhook, findErr := utils.FindExistingWebhook(accessToken, owner, repoName, webhookURL)
				if findErr == nil && existingWebhook != nil {
					log.Printf("[GITHUB] Found existing webhook with ID: %d", existingWebhook.ID)
					webhookID = &existingWebhook.ID
				} else {
					log.Printf("[GITHUB] Could not find existing webhook, keeping auto deploy enabled anyway")
					// Keep auto deploy enabled - webhook exists but we don't have the ID
				}
			} else {
				// Other error - disable auto deploy
				connectData.AutoDeploy = false
			}
		} else {
			webhookID = &webhook.ID
			log.Printf("[GITHUB] Webhook created with ID: %d", webhook.ID)
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

	// Get user's GitHub access token or fall back to installation token
	accessToken, err := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))
	if err != nil || accessToken == "" {
		if tokenResp, instErr := utils.GetGitHubInstallationToken(); instErr == nil {
			accessToken = tokenResp.Token
			err = nil
		}
	}

	if err == nil && accessToken != "" && webhookID != nil {
		// Delete webhook if exists
		repoParts := strings.Split(fullName, "/")
		if len(repoParts) == 2 {
			owner, repoName := repoParts[0], repoParts[1]
			err = utils.DeleteWebhook(accessToken, owner, repoName, *webhookID)
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

	// Get access token for webhook management (using GitHub App installation token)
	var accessToken string
	if tokenResp, tokenErr := utils.GetGitHubInstallationToken(); tokenErr == nil && tokenResp != nil {
		accessToken = tokenResp.Token
	}
	if accessToken == "" {
		log.Printf("[GITHUB] Failed to get installation token")
		// Continue without webhook management, just update database
	} else if repoConnection.FullName != "" {
		parts := strings.Split(repoConnection.FullName, "/")
		if len(parts) == 2 {
			owner, repoName := parts[0], parts[1]
			baseURL := c.BaseURL()
			if strings.HasPrefix(baseURL, "http://") && !strings.Contains(baseURL, "localhost") && !strings.Contains(baseURL, "127.0.0.1") {
				baseURL = strings.Replace(baseURL, "http://", "https://", 1)
			}
			webhookURL := fmt.Sprintf("%s/api/v1/github/webhook", baseURL)

			if toggleData.AutoDeploy {
				// Create webhook if not exists
				if webhookID == nil {
					webhook, err := utils.CreateWebhook(accessToken, owner, repoName, webhookURL)
					if err != nil {
						if strings.Contains(err.Error(), "Hook already exists") {
							// Find existing webhook
							existingWebhook, _ := utils.FindExistingWebhook(accessToken, owner, repoName, webhookURL)
							if existingWebhook != nil {
								webhookID = &existingWebhook.ID
								log.Printf("[GITHUB] Found existing webhook with ID: %d", existingWebhook.ID)
							}
						} else {
							log.Printf("[GITHUB] Failed to create webhook: %v", err)
						}
					} else {
						webhookID = &webhook.ID
						log.Printf("[GITHUB] Created webhook with ID: %d", webhook.ID)
					}
				}
			} else {
				// Delete webhook if exists
				if webhookID != nil {
					err := utils.DeleteWebhook(accessToken, owner, repoName, *webhookID)
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

	// Get access token
	var accessToken string
	if tokenResp, err := utils.GetGitHubInstallationToken(); err == nil && tokenResp != nil {
		accessToken = tokenResp.Token
	} else {
		token, err := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))
		if err != nil || token == "" {
			log.Printf("[GITHUB] Failed to get GitHub access token: %v", err)
			return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
				false,
				"GitHub not connected or access token not found",
				nil,
			))
		}
		accessToken = token
	}

	// Get repository details from GitHub
	repoParts := strings.Split(connectData.FullName, "/")
	if len(repoParts) != 2 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid repository full name format (should be owner/repo)",
			nil,
		))
	}

	owner, repoName := repoParts[0], repoParts[1]

	githubRepo, err := utils.GetRepositoryInfo(accessToken, owner, repoName)
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
	baseURL := c.BaseURL()
	if strings.HasPrefix(baseURL, "http://") && !strings.Contains(baseURL, "localhost") && !strings.Contains(baseURL, "127.0.0.1") {
		baseURL = strings.Replace(baseURL, "http://", "https://", 1)
	}
	webhookURL := fmt.Sprintf("%s/api/v1/github/webhook", baseURL)

	if connectData.AutoDeploy {
		webhook, err := utils.CreateWebhook(accessToken, owner, repoName, webhookURL)
		if err != nil {
			log.Printf("[GITHUB] Failed to create webhook: %v", err)
			// Check if webhook already exists
			if strings.Contains(err.Error(), "Hook already exists") {
				log.Printf("[GITHUB] Webhook already exists, trying to find existing webhook...")
				existingWebhook, findErr := utils.FindExistingWebhook(accessToken, owner, repoName, webhookURL)
				if findErr == nil && existingWebhook != nil {
					log.Printf("[GITHUB] Found existing webhook with ID: %d", existingWebhook.ID)
					webhookID = &existingWebhook.ID
				} else {
					log.Printf("[GITHUB] Could not find existing webhook, keeping auto deploy enabled anyway")
				}
			} else {
				// Other error - disable auto deploy
				connectData.AutoDeploy = false
			}
		} else {
			webhookID = &webhook.ID
			log.Printf("[GITHUB] Webhook created with ID: %d", webhook.ID)
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
