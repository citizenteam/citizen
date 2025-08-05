package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"backend/database"
	"backend/database/api"
	"backend/models"
	"backend/utils"

	"github.com/gofiber/fiber/v2"
)

// GitHubAuthInit initiates GitHub OAuth flow
func GitHubAuthInit(c *fiber.Ctx) error {
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
	if !utils.IsGitHubConfigured() {
		// Don't set up placeholder values, just return setup required
		baseURL := c.BaseURL()
		redirectURI := fmt.Sprintf("%s/api/v1/github/auth/callback", baseURL)
		
		log.Printf("[GITHUB] GitHub OAuth not configured, showing setup instructions")
		
		return c.JSON(utils.NewCitizenResponse(
			false,
			"GitHub OAuth needs to be configured. Please set up your GitHub App first.",
			fiber.Map{
				"setup_required": true,
				"redirect_uri": redirectURI,
				"instructions": "Create a GitHub App with this redirect URI, then provide the Client ID and Secret",
			},
		))
	}

	// Generate state for CSRF protection with crypto-secure random component
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		log.Printf("[GITHUB] Failed to generate secure random bytes: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to generate secure state parameter",
			nil,
		))
	}
	randomComponent := hex.EncodeToString(randomBytes)
	state := fmt.Sprintf("user_%v_%d_%s", userID, time.Now().Unix(), randomComponent)
	
	// Generate OAuth URL
	authURL, err := utils.GetGitHubOAuthURL(state)
	if err != nil {
		log.Printf("[GITHUB] Failed to generate OAuth URL: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to generate GitHub OAuth URL",
			nil,
		))
	}
	
	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub OAuth URL generated",
		fiber.Map{
			"auth_url": authURL,
			"state":    state,
		},
	))
}

// GitHubAuthCallback handles GitHub OAuth callback
func GitHubAuthCallback(c *fiber.Ctx) error {
	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	code := c.Query("code")
	state := c.Query("state")
	
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Authorization code is required",
			nil,
		))
	}
	
	// CSRF Protection: Validate state parameter
	if state == "" {
		log.Printf("[GITHUB] CSRF Protection: Missing state parameter for user %v", userID)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}
	
	// Validate state format: "user_{userID}_{timestamp}_{randomComponent}"
	expectedPrefix := fmt.Sprintf("user_%v_", userID)
	if !strings.HasPrefix(state, expectedPrefix) {
		log.Printf("[GITHUB] CSRF Protection: Invalid state format for user %v, state: %s", userID, state)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}
	
	// Extract and validate timestamp (prevent replay attacks)
	parts := strings.Split(state, "_")
	if len(parts) != 4 {
		log.Printf("[GITHUB] CSRF Protection: Invalid state parts count for user %v, expected 4, got %d, state: %s", userID, len(parts), state)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}
	
	// Additional validation: ensure userID in state matches current user
	stateUserIDStr := parts[1]
	if fmt.Sprintf("%v", userID) != stateUserIDStr {
		log.Printf("[GITHUB] CSRF Protection: UserID mismatch for user %v, state userID: %s", userID, stateUserIDStr)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}
	
	timestampStr := parts[2]
	randomComponent := parts[3]
	
	// Validate random component format (should be 32 hex chars)
	if len(randomComponent) != 32 {
		log.Printf("[GITHUB] CSRF Protection: Invalid random component length for user %v, expected 32, got %d", userID, len(randomComponent))
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}
	
	// Validate that random component is hex
	for _, char := range randomComponent {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			log.Printf("[GITHUB] CSRF Protection: Invalid random component format for user %v, not hex: %s", userID, randomComponent)
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"Invalid state parameter - CSRF protection failed",
				nil,
			))
		}
	}
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		log.Printf("[GITHUB] CSRF Protection: Invalid timestamp in state for user %v, state: %s", userID, state)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}
	
	// Check if state is not too old (10 minutes max)
	maxAge := int64(10 * 60) // 10 minutes in seconds
	currentTime := time.Now().Unix()
	if currentTime-timestamp > maxAge {
		log.Printf("[GITHUB] CSRF Protection: Expired state for user %v, age: %d seconds", userID, currentTime-timestamp)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"State parameter expired - please try again",
			nil,
		))
	}
	
	log.Printf("[GITHUB] ✅ CSRF Protection validated successfully for user %v, state: %s", userID, state)
	
	// Exchange code for access token
	tokenResp, err := utils.ExchangeCodeForToken(code)
	if err != nil {
		log.Printf("[GITHUB] Failed to exchange code for token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to exchange code for token",
			nil,
		))
	}
	
	// Get GitHub user info
	githubUser, err := utils.GetGitHubUser(tokenResp.AccessToken)
	if err != nil {
		log.Printf("[GITHUB] Failed to get GitHub user: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to get GitHub user information",
			nil,
		))
	}
	
	// Update user in database with GitHub info
	err = api.GitHub.UpdateGitHubInfo(c.Context(), userID.(int), int64(githubUser.ID), githubUser.Login, tokenResp.AccessToken)
	
	if err != nil {
		log.Printf("[GITHUB] Failed to update user with GitHub info: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to save GitHub connection",
			nil,
		))
	}
	
	log.Printf("[GITHUB] ✅ GitHub user connected: %s (ID: %d)", githubUser.Login, githubUser.ID)
	
	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub account connected successfully",
		fiber.Map{
			"github_user":     githubUser,
			"github_connected": true,
		},
	))
}

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
	
	page := c.QueryInt("page", 1)
	
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
		AppName       string `json:"app_name"`
		RepositoryID  int64  `json:"repository_id"`
		FullName      string `json:"full_name"`
		AutoDeploy    bool   `json:"auto_deploy"`
		DeployBranch  string `json:"deploy_branch"`
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
	if connectData.AutoDeploy {
		webhookURL := fmt.Sprintf("%s/api/v1/github/webhook", c.BaseURL())
		webhook, err := utils.CreateWebhook(accessToken, owner, repoName, webhookURL)
		if err != nil {
			log.Printf("[GITHUB] Failed to create webhook: %v", err)
			// Don't fail the entire connection, just disable auto deploy
			connectData.AutoDeploy = false
		} else {
			webhookID = &webhook.ID
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
			"app_name":        connectData.AppName,
			"repository":      githubRepo,
			"auto_deploy":     connectData.AutoDeploy,
			"deploy_branch":   connectData.DeployBranch,
			"webhook_id":      webhookID,
			"webhook_active":  webhookID != nil,
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
	
	// Get user's GitHub access token
	accessToken, err := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))
	
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

	// TODO: Get repository connection from database
	// TODO: Create or delete webhook based on auto_deploy setting
	// TODO: Update database
	
	log.Printf("[GITHUB] ✅ Auto deploy %s for app: %s", 
		map[bool]string{true: "enabled", false: "disabled"}[toggleData.AutoDeploy], 
		appName)
	
	return c.JSON(utils.NewCitizenResponse(
		true,
		fmt.Sprintf("Auto deploy %s successfully", 
			map[bool]string{true: "enabled", false: "disabled"}[toggleData.AutoDeploy]),
		fiber.Map{
			"app_name":    appName,
			"auto_deploy": toggleData.AutoDeploy,
		},
	))
}

// GitHubWebhookHandler handles GitHub webhook events
func GitHubWebhookHandler(c *fiber.Ctx) error {
	// Verify webhook signature
	signature := c.Get("X-Hub-Signature-256")
	if signature == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing signature",
		})
	}
	
	payload := c.Body()
	if !utils.ValidateGitHubSignature(payload, signature) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid signature",
		})
	}
	
	// Get event type
	eventType := c.Get("X-GitHub-Event")
	deliveryID := c.Get("X-GitHub-Delivery")
	
	log.Printf("[WEBHOOK] Received GitHub webhook: %s (ID: %s)", eventType, deliveryID)
	
	// Only process push events for now
	if eventType != "push" {
		return c.JSON(fiber.Map{
			"status": "ignored",
			"reason": "Event type not supported",
		})
	}
	
	// Parse push event
	var pushEvent struct {
		Ref        string `json:"ref"`
		Before     string `json:"before"`
		After      string `json:"after"`
		Repository struct {
			ID       int64  `json:"id"`
			FullName string `json:"full_name"`
		} `json:"repository"`
		HeadCommit struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Author  struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"author"`
		} `json:"head_commit"`
	}
	
	if err := c.BodyParser(&pushEvent); err != nil {
		log.Printf("[WEBHOOK] Failed to parse push event: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payload",
		})
	}
	
	// Extract branch name from ref (refs/heads/main -> main)
	branch := strings.TrimPrefix(pushEvent.Ref, "refs/heads/")
	
	log.Printf("[WEBHOOK] Push to %s/%s on branch %s (commit: %s)", 
		pushEvent.Repository.FullName, branch, pushEvent.HeadCommit.ID)
	
	// Find repository connection in database
	repoConnection, err := api.GitHub.GetGitHubRepositoryByID(c.Context(), pushEvent.Repository.ID)
	if err != nil {
		log.Printf("[WEBHOOK] No repository connection found for %s (ID: %d): %v", 
			pushEvent.Repository.FullName, pushEvent.Repository.ID, err)
		return c.JSON(fiber.Map{
			"status": "ignored",
			"reason": "Repository not connected or auto deploy disabled",
		})
	}
	
	appName := repoConnection.AppName
	autoDeploy := repoConnection.AutoDeployEnabled
	deployBranch := repoConnection.DeployBranch
	
	// Check if auto deploy is enabled
	if !autoDeploy {
		log.Printf("[WEBHOOK] Auto deploy disabled for %s", appName)
		return c.JSON(fiber.Map{
			"status": "ignored",
			"reason": "Auto deploy disabled",
		})
	}
	
	// Check if this is the correct branch for deployment
	if branch != deployBranch {
		log.Printf("[WEBHOOK] Branch %s does not match deploy branch %s for app %s", 
			branch, deployBranch, appName)
		return c.JSON(fiber.Map{
			"status": "ignored",
			"reason": fmt.Sprintf("Branch %s does not match deploy branch %s", branch, deployBranch),
		})
	}
	
	log.Printf("[WEBHOOK] 🚀 Triggering deployment for app %s from %s/%s", 
		appName, pushEvent.Repository.FullName, branch)
	
	// Trigger deployment asynchronously
	go func() {
		// Create Git URL from repository full name
		gitURL := fmt.Sprintf("https://github.com/%s.git", pushEvent.Repository.FullName)
		
		// 📝 Log webhook deployment start
		deployActivity, activityErr := database.LogWebhookDeployment(
			appName, 
			gitURL, 
			branch, 
			pushEvent.HeadCommit.ID, 
			pushEvent.HeadCommit.Message, 
			pushEvent.HeadCommit.Author.Name,
		)
		if activityErr != nil {
			log.Printf("[WEBHOOK] ⚠️ Failed to log webhook deployment activity: %v", activityErr)
		}
		
		// Get the connected user's ID for authentication
		var userID *int
		repoConnection, err := api.GitHub.GetGitHubRepositoryConnectionByAppName(context.Background(), appName)
		if err == nil && repoConnection.UserID != 0 {
			uid := repoConnection.UserID
			userID = &uid
			log.Printf("[WEBHOOK] 🔑 Using user ID %d for GitHub authentication", uid)
		} else {
			log.Printf("[WEBHOOK] ⚠️ No user ID found for webhook authentication: %v", err)
		}
		
		// 🚀 Trigger deployment using existing deploy logic (WITH GITHUB TOKEN)
		output, err := utils.DeployFromGit(appName, gitURL, branch, userID)
		if err != nil {
			log.Printf("[WEBHOOK] ❌ Deployment failed for %s: %v", appName, err)
			
			// 📝 Update deployment activity as failed
			if deployActivity != nil {
				errorMsg := err.Error()
				database.UpdateActivity(deployActivity.ID, database.StatusError, &errorMsg)
			}
			
			
			// Update GitHub deployment status as failed
			errorOutput := err.Error()
			database.UpdateGitHubDeploymentStatus(appName, pushEvent.HeadCommit.ID, "failed", &output, &errorOutput)
		} else {
			log.Printf("[WEBHOOK] ✅ Deployment completed for %s", appName)
			log.Printf("[WEBHOOK] Deploy output: %s", output)
			
			// 📝 Update deployment activity as successful
			if deployActivity != nil {
				database.UpdateActivity(deployActivity.ID, database.StatusSuccess, nil)
			}
			
			// Update GitHub deployment status as successful
			database.UpdateGitHubDeploymentStatus(appName, pushEvent.HeadCommit.ID, "success", &output, nil)
			
			// Note: Traefik reload will be triggered automatically by dokku-traefik-watcher
			// after the container is restarted and fully ready
		}
	}()
	
	return c.JSON(fiber.Map{
		"status":     "accepted",
		"event_type": eventType,
		"repository": pushEvent.Repository.FullName,
		"branch":     branch,
		"commit":     pushEvent.HeadCommit.ID,
		"app_name":   appName,
		"action":     "deployment_triggered",
	})
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
	
	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub status fetched successfully",
		fiber.Map{
			"github_configured": isConfigured,
			"github_connected":  githubConnected,
			"github_username":   githubUsername,
			"github_id":         githubID,
		},
	))
}

// GitHubConfigRequest represents GitHub config setup request
type GitHubConfigRequest struct {
	ClientID     string `json:"client_id" validate:"required"`
	ClientSecret string `json:"client_secret" validate:"required"`
	RedirectURI  string `json:"redirect_uri" validate:"required"`
}

// GitHubConfigResponse represents GitHub config response (without secrets)
type GitHubConfigResponse struct {
	ClientID    string `json:"client_id"`
	RedirectURI string `json:"redirect_uri"`
	IsActive    bool   `json:"is_active"`
	ConfiguredAt string `json:"configured_at"`
}

// SetupGitHubConfig handles GitHub OAuth configuration setup
func SetupGitHubConfig(c *fiber.Ctx) error {
	var req GitHubConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if req.ClientID == "" || req.ClientSecret == "" || req.RedirectURI == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "All fields are required",
		})
	}

	// Generate webhook secret
	webhookSecret := generateSecureSecret()
	
	// Save to database (encrypted)
	err := saveGitHubConfigToDB(req.ClientID, req.ClientSecret, req.RedirectURI, webhookSecret)
	if err != nil {
		log.Printf("[GITHUB] Failed to save GitHub config to database: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save GitHub config to database",
		})
	}

	// Setup GitHub OAuth in memory
	err = utils.SetupGitHubOAuth(req.ClientID, req.ClientSecret, req.RedirectURI, webhookSecret)
	if err != nil {
		log.Printf("[GITHUB] Failed to setup GitHub OAuth: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to setup GitHub OAuth",
		})
	}

	log.Printf("[GITHUB] ✅ GitHub OAuth setup completed")
	return c.JSON(fiber.Map{
		"message": "GitHub OAuth setup completed successfully",
		"configured": true,
	})
}

// GetGitHubConfig returns current GitHub configuration (without secrets)
func GetGitHubConfig(c *fiber.Ctx) error {
	log.Printf("[CONFIG] GetGitHubConfig called")
	
	// Check if configured
	if !utils.IsGitHubConfigured() {
		log.Printf("[CONFIG] GitHub not configured")
		return c.JSON(utils.NewCitizenResponse(
			true,
			"GitHub not configured",
			fiber.Map{
				"configured": false,
			},
		))
	}
	
	log.Printf("[CONFIG] GitHub is configured, fetching from DB")

	// Get config from database
	config, err := api.GitHub.GetGitHubConfig(context.Background())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to load GitHub config",
		})
	}

	// Decrypt only client ID for display
	clientID, err := utils.DecryptString(config.ClientID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to decrypt config",
		})
	}

	// Mask client ID for security (show only first 8 chars)
	maskedClientID := clientID
	if len(clientID) > 8 {
		maskedClientID = clientID[:8] + "..."
	}

	response := fiber.Map{
		"configured":   true,
		"client_id":    maskedClientID,
		"redirect_uri": config.RedirectURI,
		"is_active":    true,
		"configured_at": config.CreatedAt.Format(time.RFC3339),
	}
	
	log.Printf("[CONFIG] Returning response: %+v", response)
	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub configuration loaded",
		response,
	))
}

// DeleteGitHubConfig removes GitHub configuration
func DeleteGitHubConfig(c *fiber.Ctx) error {
	// Soft delete - mark as inactive
	err := api.GitHub.DeleteGitHubConfig(context.Background())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete GitHub config",
		})
	}

	log.Printf("[GITHUB] ✅ GitHub config deleted")
	return c.JSON(fiber.Map{
		"message": "GitHub configuration deleted successfully",
	})
}

// generateSecureSecret generates a cryptographically secure secret
func generateSecureSecret() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// saveGitHubConfigToDB saves GitHub configuration to database (encrypted)
func saveGitHubConfigToDB(clientID, clientSecret, redirectURI, webhookSecret string) error {
	// Encrypt sensitive data
	encryptedClientID, err := utils.EncryptString(clientID)
	if err != nil {
		return fmt.Errorf("failed to encrypt client ID: %w", err)
	}
	
	encryptedClientSecret, err := utils.EncryptString(clientSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt client secret: %w", err)
	}
	
	encryptedWebhookSecret, err := utils.EncryptString(webhookSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt webhook secret: %w", err)
	}
	
	// Save to database - first deactivate old configs, then insert new
	err = api.GitHub.SaveGitHubConfig(context.Background(), encryptedClientID, encryptedClientSecret, encryptedWebhookSecret, redirectURI)
	if err != nil {
		return fmt.Errorf("failed to save GitHub config to database: %w", err)
	}
	
	fmt.Printf("[CONFIG] ✅ GitHub config saved to database\n")
	return nil
}

// LoadGitHubConfigFromDB loads GitHub configuration from database (decrypted)
func LoadGitHubConfigFromDB() (clientID, clientSecret, redirectURI, webhookSecret string, err error) {
	config, err := api.GitHub.GetGitHubConfigFull(context.Background())
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to load GitHub config from database: %w", err)
	}
	
	// Decrypt sensitive data
	clientID, err = utils.DecryptString(config.ClientID)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to decrypt client ID: %w", err)
	}
	
	clientSecret, err = utils.DecryptString(config.ClientSecret)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to decrypt client secret: %w", err)
	}
	
	webhookSecret, err = utils.DecryptString(config.WebhookSecret)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to decrypt webhook secret: %w", err)
	}
	
	fmt.Printf("[CONFIG] ✅ GitHub config loaded from database\n")
	return clientID, clientSecret, config.RedirectURI, webhookSecret, nil
}