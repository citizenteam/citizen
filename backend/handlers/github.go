package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"backend/database"
	"backend/database/api"
	"backend/models"
	"backend/platform"
	"backend/utils"

	"github.com/gofiber/fiber/v2"
)

var manifestStates = newStateStore()
var installStates = newStateStore()

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
				"redirect_uri":   redirectURI,
				"instructions":   "Create a GitHub App with this redirect URI, then provide the Client ID and Secret",
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
// This is a public endpoint - user validation is done via state parameter
func GitHubAuthCallback(c *fiber.Ctx) error {
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
		log.Printf("[GITHUB] CSRF Protection: Missing state parameter")
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}

	// Validate state format: "user_{userID}_{timestamp}_{randomComponent}"
	if !strings.HasPrefix(state, "user_") {
		log.Printf("[GITHUB] CSRF Protection: Invalid state format, state: %s", state)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}

	// Extract and validate timestamp (prevent replay attacks)
	parts := strings.Split(state, "_")
	if len(parts) != 4 {
		log.Printf("[GITHUB] CSRF Protection: Invalid state parts count, expected 4, got %d, state: %s", len(parts), state)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}

	// Extract userID from state
	stateUserIDStr := parts[1]
	userID, err := strconv.Atoi(stateUserIDStr)
	if err != nil {
		log.Printf("[GITHUB] CSRF Protection: Invalid userID in state: %s", stateUserIDStr)
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
	err = api.GitHub.UpdateGitHubInfo(c.Context(), userID, int64(githubUser.ID), githubUser.Login, tokenResp.AccessToken)

	if err != nil {
		log.Printf("[GITHUB] Failed to update user with GitHub info: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to save GitHub connection",
			nil,
		))
	}

	log.Printf("[GITHUB] ✅ GitHub user connected: %s (ID: %d)", githubUser.Login, githubUser.ID)

	// Check if this is a popup (has state parameter) - return HTML to close popup
	if state != "" {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(`<!DOCTYPE html>
<html>
<head>
	<title>GitHub Connected</title>
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; background: #f9fafb; }
		.container { text-align: center; padding: 2rem; }
		.success { color: #16a34a; font-size: 3rem; margin-bottom: 1rem; }
		h1 { color: #111827; font-size: 1.5rem; margin-bottom: 0.5rem; }
		p { color: #6b7280; }
	</style>
</head>
<body>
	<div class="container">
		<div class="success">✓</div>
		<h1>GitHub Bağlandı!</h1>
		<p>Bu pencere kapanıyor...</p>
	</div>
	<script>
		if (window.opener) {
			window.opener.postMessage({ type: 'github-oauth-success' }, '*');
		}
		setTimeout(function() { window.close(); }, 1500);
	</script>
</body>
</html>`)
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub account connected successfully",
		fiber.Map{
			"github_user":      githubUser,
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

	// Repository webhook management:
	// 1. Get repository connection from database (api.GitHub)
	// 2. Create or delete webhook based on auto_deploy setting
	// 3. Update database with webhook status and URL

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

	log.Printf("[WEBHOOK] Push to %s on branch %s (commit: %s)",
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
		output, err := platform.GetAdapter().DeployFromGit(appName, gitURL, branch, userID)
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
	if connectData.AutoDeploy {
		baseURL := c.BaseURL()
		// Ensure HTTPS for production
		if strings.HasPrefix(baseURL, "http://") && !strings.Contains(baseURL, "localhost") && !strings.Contains(baseURL, "127.0.0.1") {
			baseURL = strings.Replace(baseURL, "http://", "https://", 1)
		}
		webhookURL := fmt.Sprintf("%s/api/v1/github/webhook", baseURL)
		webhook, err := utils.CreateWebhook(accessToken, owner, repoName, webhookURL)
		if err != nil {
			log.Printf("[GITHUB] Failed to create webhook: %v", err)
			// Don't fail, just disable auto deploy
			connectData.AutoDeploy = false
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

// ListUserGitHubAppInstallations lists GitHub App installations accessible to user
// This allows users to select an existing installed app instead of creating a new one
func ListUserGitHubAppInstallations(c *fiber.Ctx) error {
	log.Printf("[GITHUB] ListUserGitHubAppInstallations called")

	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	// Get user's GitHub access token
	accessToken, err := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))
	if err != nil || accessToken == "" {
		log.Printf("[GITHUB] No GitHub access token found for user: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"GitHub not connected. Please connect your GitHub account first.",
			fiber.Map{
				"needs_github_auth": true,
			},
		))
	}

	// Get installations from GitHub
	installations, err := utils.GetUserAppInstallations(accessToken)
	if err != nil {
		log.Printf("[GITHUB] Failed to get user installations: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to fetch GitHub App installations",
			nil,
		))
	}

	log.Printf("[GITHUB] Found %d app installations for user", len(installations))

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub App installations fetched successfully",
		fiber.Map{
			"installations": installations,
			"total":         len(installations),
		},
	))
}

// ConnectExistingGitHubApp connects an existing GitHub App to this Citizen instance
// User provides app_id, app_slug, and private_key - we find the installation automatically
func ConnectExistingGitHubApp(c *fiber.Ctx) error {
	log.Printf("[GITHUB] ConnectExistingGitHubApp called")

	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	var connectData struct {
		InstallationID int64  `json:"installation_id"`
		AppID          int64  `json:"app_id"`
		AppSlug        string `json:"app_slug"`
		AppName        string `json:"app_name"`
		PrivateKey     string `json:"private_key"`
	}

	if err := c.BodyParser(&connectData); err != nil {
		log.Printf("[GITHUB] Failed to parse request body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	// Validate required fields
	if connectData.AppID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App ID is required",
			nil,
		))
	}

	if connectData.PrivateKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Private Key is required to authenticate with GitHub App",
			nil,
		))
	}

	// If installation_id not provided, try to find it using JWT
	if connectData.InstallationID == 0 {
		log.Printf("[GITHUB] Installation ID not provided, trying to find it using JWT...")

		// Generate JWT using the provided private key
		jwtToken, err := utils.GenerateGitHubAppJWTWithKey(connectData.AppID, connectData.PrivateKey)
		if err != nil {
			log.Printf("[GITHUB] Failed to generate JWT with provided key: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"Invalid Private Key - could not generate JWT",
				nil,
			))
		}

		// Get installations for this app
		installations, err := utils.GetAppInstallationsWithJWT(jwtToken)
		if err != nil {
			log.Printf("[GITHUB] Failed to get installations: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"Could not find installations. Make sure the app is installed on your account.",
				nil,
			))
		}

		if len(installations) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"No installations found. Please install the app on GitHub first.",
				nil,
			))
		}

		// Use the first installation (typically there's only one per user/org)
		connectData.InstallationID = installations[0].ID
		log.Printf("[GITHUB] Found installation ID: %d", connectData.InstallationID)
	}

	if connectData.AppSlug == "" {
		log.Printf("[GITHUB] App slug not provided, this may cause issues with install flow")
	}

	// Generate placeholder client_id and secret (not used for App auth, but needed for config)
	// GitHub Apps use JWT + Installation tokens, not OAuth
	clientID := fmt.Sprintf("app-%d", connectData.AppID)
	clientSecret := generateSecureSecret()
	webhookSecret := generateSecureSecret()
	baseURL := c.BaseURL()
	if strings.HasPrefix(baseURL, "http://") && !strings.Contains(baseURL, "localhost") && !strings.Contains(baseURL, "127.0.0.1") {
		baseURL = strings.Replace(baseURL, "http://", "https://", 1)
	}
	redirectURI := fmt.Sprintf("%s/api/v1/github/auth/callback", baseURL)

	// Save to database
	err := api.GitHub.SaveGitHubConfig(
		c.Context(),
		clientID,
		clientSecret,
		webhookSecret,
		redirectURI,
		&connectData.AppID,
		&connectData.AppSlug,
		&connectData.AppName,
		&connectData.PrivateKey,
		&connectData.InstallationID,
	)
	if err != nil {
		log.Printf("[GITHUB] Failed to save GitHub config: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to save GitHub App configuration",
			nil,
		))
	}

	// Setup in-memory config
	utils.SetupGitHubOAuth(clientID, clientSecret, redirectURI, webhookSecret)
	utils.SetupGitHubApp(connectData.AppID, &connectData.AppSlug, &connectData.PrivateKey, &connectData.InstallationID, &connectData.AppName)

	log.Printf("[GITHUB] ✅ Existing GitHub App connected: %s (ID: %d, Installation: %d)",
		connectData.AppName, connectData.AppID, connectData.InstallationID)

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub App connected successfully",
		fiber.Map{
			"app_id":          connectData.AppID,
			"app_slug":        connectData.AppSlug,
			"app_name":        connectData.AppName,
			"installation_id": connectData.InstallationID,
			"configured":      true,
		},
	))
}

// ConnectWithPrivateKey connects an existing GitHub App using only App ID and Private Key
// It automatically fetches app info and finds the installation
func ConnectWithPrivateKey(c *fiber.Ctx) error {
	log.Printf("[GITHUB] ConnectWithPrivateKey called")

	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	var connectData struct {
		AppID      int64  `json:"app_id"`
		PrivateKey string `json:"private_key"`
	}

	if err := c.BodyParser(&connectData); err != nil {
		log.Printf("[GITHUB] Failed to parse request body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	if connectData.AppID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App ID is required",
			nil,
		))
	}

	if connectData.PrivateKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Private Key is required",
			nil,
		))
	}

	// Generate JWT using the provided private key
	jwtToken, err := utils.GenerateGitHubAppJWTWithKey(connectData.AppID, connectData.PrivateKey)
	if err != nil {
		log.Printf("[GITHUB] Failed to generate JWT: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid Private Key - could not generate JWT. Make sure it's a valid PEM file.",
			nil,
		))
	}

	// Get app info using JWT
	appInfo, err := utils.GetGitHubAppInfoWithJWT(jwtToken)
	if err != nil {
		log.Printf("[GITHUB] Failed to get app info: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Could not get app info. Check if App ID and Private Key match.",
			nil,
		))
	}

	// Get installations for this app
	installations, err := utils.GetAppInstallationsWithJWT(jwtToken)
	if err != nil {
		log.Printf("[GITHUB] Failed to get installations: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Could not get installations. Make sure the app is installed on your GitHub account.",
			nil,
		))
	}

	if len(installations) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"No installations found. Please install the app on GitHub first.",
			fiber.Map{
				"install_url": fmt.Sprintf("https://github.com/apps/%s/installations/new", appInfo.Slug),
			},
		))
	}

	// Use the first installation
	installationID := installations[0].ID
	log.Printf("[GITHUB] Found app: %s (slug: %s), installation: %d", appInfo.Name, appInfo.Slug, installationID)

	// Generate config values
	clientID := fmt.Sprintf("app-%d", connectData.AppID)
	clientSecret := generateSecureSecret()
	webhookSecret := generateSecureSecret()
	baseURL := c.BaseURL()
	if strings.HasPrefix(baseURL, "http://") && !strings.Contains(baseURL, "localhost") && !strings.Contains(baseURL, "127.0.0.1") {
		baseURL = strings.Replace(baseURL, "http://", "https://", 1)
	}
	redirectURI := fmt.Sprintf("%s/api/v1/github/auth/callback", baseURL)

	// Encrypt sensitive values before saving
	encryptedClientID, err := utils.EncryptString(clientID)
	if err != nil {
		log.Printf("[GITHUB] Failed to encrypt client ID: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to encrypt configuration",
			nil,
		))
	}
	encryptedClientSecret, err := utils.EncryptString(clientSecret)
	if err != nil {
		log.Printf("[GITHUB] Failed to encrypt client secret: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to encrypt configuration",
			nil,
		))
	}
	encryptedWebhookSecret, err := utils.EncryptString(webhookSecret)
	if err != nil {
		log.Printf("[GITHUB] Failed to encrypt webhook secret: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to encrypt configuration",
			nil,
		))
	}
	encryptedPrivateKey, err := utils.EncryptString(connectData.PrivateKey)
	if err != nil {
		log.Printf("[GITHUB] Failed to encrypt private key: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to encrypt configuration",
			nil,
		))
	}

	// Save to database with encrypted values
	err = api.GitHub.SaveGitHubConfig(
		c.Context(),
		encryptedClientID,
		encryptedClientSecret,
		encryptedWebhookSecret,
		redirectURI,
		&connectData.AppID,
		&appInfo.Slug,
		&appInfo.Name,
		&encryptedPrivateKey,
		&installationID,
	)
	if err != nil {
		log.Printf("[GITHUB] Failed to save GitHub config: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to save GitHub App configuration",
			nil,
		))
	}

	// Setup in-memory config
	utils.SetupGitHubOAuth(clientID, clientSecret, redirectURI, webhookSecret)
	utils.SetupGitHubApp(connectData.AppID, &appInfo.Slug, &connectData.PrivateKey, &installationID, &appInfo.Name)

	log.Printf("[GITHUB] ✅ GitHub App connected via private key: %s (ID: %d, Installation: %d)",
		appInfo.Name, connectData.AppID, installationID)

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub App connected successfully",
		fiber.Map{
			"app_id":          connectData.AppID,
			"app_slug":        appInfo.Slug,
			"app_name":        appInfo.Name,
			"installation_id": installationID,
			"configured":      true,
		},
	))
}

// StartGitHubManifest kicks off GitHub App manifest flow (instance-owned app)
// This returns a URL to the manifest redirect endpoint which will POST the manifest to GitHub
func StartGitHubManifest(c *fiber.Ctx) error {
	// Generate secure state for CSRF protection
	state := generateSecureSecret()
	manifestStates.add(state)

	baseURL := c.BaseURL()

	// Return URL to our manifest redirect endpoint which will handle the POST
	manifestURL := fmt.Sprintf("%s/api/v1/github/app/manifest/redirect?state=%s", baseURL, url.QueryEscape(state))

	log.Printf("[GITHUB] Starting manifest flow with state: %s", state[:8]+"...")

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub App manifest URL generated",
		fiber.Map{
			"manifest_url": manifestURL,
			"state":        state,
		},
	))
}

// StartGitHubInstall generates an install URL for existing app
func StartGitHubInstall(c *fiber.Ctx) error {
	appID, appSlug, _, _, _ := utils.GetGitHubAppConfig()

	// Allow optional overrides via request body for existing app flow
	var body struct {
		AppID   *int64  `json:"app_id"`
		AppSlug *string `json:"app_slug"`
	}
	_ = c.BodyParser(&body)

	if body.AppID != nil {
		appID = body.AppID
	}
	if body.AppSlug != nil && *body.AppSlug != "" {
		appSlug = body.AppSlug
	}

	if appSlug == nil || appID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"GitHub App not configured yet",
			nil,
		))
	}
	state := generateSecureSecret()
	installStates.add(state)
	baseURL := c.BaseURL()
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s&redirect_url=%s", url.QueryEscape(*appSlug), url.QueryEscape(state), url.QueryEscape(fmt.Sprintf("%s/api/v1/github/app/install/callback", baseURL)))

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub App install URL generated",
		fiber.Map{
			"install_url": installURL,
			"state":       state,
		},
	))
}

// GitHubManifestRedirect renders an auto-submitting form to POST manifest to GitHub
func GitHubManifestRedirect(c *fiber.Ctx) error {
	state := c.Query("state")
	if state == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Missing state parameter")
	}

	// Validate state exists in our store (but don't consume it yet - callback will do that)
	if !manifestStates.exists(state) {
		log.Printf("[GITHUB] Invalid or expired state in manifest redirect: %s", state[:8]+"...")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid or expired state - please try again")
	}

	baseURL := c.BaseURL()
	// Ensure HTTPS for production (GitHub requires HTTPS for redirect_url)
	if strings.HasPrefix(baseURL, "http://") && !strings.Contains(baseURL, "localhost") && !strings.Contains(baseURL, "127.0.0.1") {
		baseURL = strings.Replace(baseURL, "http://", "https://", 1)
	}
	webhookURL := fmt.Sprintf("%s/api/v1/github/webhook", baseURL)
	// redirect_url must NOT have query params - GitHub adds its own (code, state)
	redirectURL := fmt.Sprintf("%s/api/v1/github/app/manifest/callback", baseURL)

	// Generate unique app name based on domain
	appName := fmt.Sprintf("citizen-%d", time.Now().Unix())

	// OAuth callback URL for user authorization
	callbackURL := fmt.Sprintf("%s/api/v1/github/auth/callback", baseURL)

	// Setup URL for post-installation redirect (GitHub adds installation_id query param)
	setupURL := fmt.Sprintf("%s/api/v1/github/app/install/callback", baseURL)

	manifest := map[string]interface{}{
		"name":                     appName,
		"description":              "Citizen PaaS deployment integration",
		"url":                      baseURL,
		"redirect_url":             redirectURL,
		"callback_urls":            []string{callbackURL},
		"setup_url":                setupURL, // Post-installation redirect URL
		"public":                   false,
		"request_oauth_on_install": false, // Don't request OAuth during install - we use installation tokens
		"setup_on_update":          true,
		"default_events": []string{
			"push",
			"pull_request",
		},
		"default_permissions": map[string]string{
			"contents":      "read",
			"metadata":      "read",
			"pull_requests": "read",
		},
		"hook_attributes": map[string]interface{}{
			"url":    webhookURL,
			"active": true,
		},
	}

	body, err := json.Marshal(manifest)
	if err != nil {
		log.Printf("[GITHUB] Failed to marshal manifest: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to create manifest")
	}

	action := fmt.Sprintf("https://github.com/settings/apps/new?state=%s", url.QueryEscape(state))

	log.Printf("[GITHUB] Rendering manifest redirect form for app: %s", appName)

	// Override CSP for this page to allow form submission to GitHub
	c.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; form-action 'self' https://github.com")

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <title>Creating GitHub App...</title>
  <style>
    body { 
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      display: flex; 
      justify-content: center; 
      align-items: center; 
      height: 100vh; 
      margin: 0;
      background: #f6f8fa;
    }
    .container { text-align: center; }
    .spinner {
      border: 3px solid #e1e4e8;
      border-top: 3px solid #0366d6;
      border-radius: 50%%;
      width: 40px;
      height: 40px;
      animation: spin 1s linear infinite;
      margin: 0 auto 16px;
    }
    @keyframes spin { 0%% { transform: rotate(0deg); } 100%% { transform: rotate(360deg); } }
    h2 { color: #24292e; margin-bottom: 8px; }
    p { color: #586069; }
    button { 
      background: #0366d6; 
      color: white; 
      border: none; 
      padding: 12px 24px; 
      border-radius: 6px; 
      font-size: 14px;
      cursor: pointer;
      margin-top: 16px;
    }
    button:hover { background: #0256cc; }
  </style>
</head>
<body onload="document.forms[0].submit()">
<div class="container">
  <div class="spinner"></div>
  <h2>Creating GitHub App</h2>
  <p>Redirecting to GitHub...</p>
  <form action="%s" method="post">
    <input type="hidden" name="manifest" value='%s'>
    <noscript>
      <p style="color: #cb2431;">JavaScript is disabled.</p>
      <button type="submit">Click to Continue</button>
    </noscript>
  </form>
</div>
</body>
</html>`, action, htmlEscapeSingleQuotes(string(body)))

	return c.Type("html").SendString(html)
}

// GitHubManifestCallback handles GitHub redirect after App creation
func GitHubManifestCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")

	log.Printf("[GITHUB] Manifest callback received: code=%s, state=%s",
		code[:min(8, len(code))]+"...", state[:min(8, len(state))]+"...")

	if code == "" {
		log.Printf("[GITHUB] Missing manifest code in callback")
		return c.Status(fiber.StatusBadRequest).SendString("Missing manifest code - GitHub did not return a code")
	}
	if state == "" || !manifestStates.validate(state, 15*time.Minute) {
		log.Printf("[GITHUB] Invalid or expired state in manifest callback")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid or expired state - please try again")
	}

	// Convert the manifest code to get app credentials
	req, err := http.NewRequest("POST", fmt.Sprintf("https://api.github.com/app-manifests/%s/conversions", code), nil)
	if err != nil {
		log.Printf("[GITHUB] Failed to build manifest conversion request: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to build request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[GITHUB] Failed to call GitHub API for manifest conversion: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to convert manifest code")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		log.Printf("[GITHUB] Manifest conversion failed (status %d): %s", resp.StatusCode, string(body))
		return c.Status(resp.StatusCode).SendString("GitHub manifest conversion failed - please try again")
	}

	var manifestResp struct {
		ID            int64  `json:"id"`
		Slug          string `json:"slug"`
		Name          string `json:"name"`
		ClientID      string `json:"client_id"`
		ClientSecret  string `json:"client_secret"`
		WebhookSecret string `json:"webhook_secret"`
		Pem           string `json:"pem"`
		HTMLURL       string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &manifestResp); err != nil {
		log.Printf("[GITHUB] Failed to parse manifest conversion response: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Invalid manifest response from GitHub")
	}

	log.Printf("[GITHUB] ✅ GitHub App created: id=%d, slug=%s, name=%s",
		manifestResp.ID, manifestResp.Slug, manifestResp.Name)

	baseURL := c.BaseURL()
	redirectURI := fmt.Sprintf("%s/api/v1/github/auth/callback", baseURL)

	appID := manifestResp.ID
	appSlug := manifestResp.Slug
	appName := manifestResp.Name
	privateKey := manifestResp.Pem

	// Save the GitHub App configuration to database (including private key)
	if err := saveGitHubConfigToDB(manifestResp.ClientID, manifestResp.ClientSecret, redirectURI, manifestResp.WebhookSecret, &appID, &appSlug, &appName, &privateKey, nil); err != nil {
		log.Printf("[GITHUB] Failed to save manifest config to database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save GitHub App config")
	}

	// Setup in-memory caches
	_ = utils.SetupGitHubOAuth(manifestResp.ClientID, manifestResp.ClientSecret, redirectURI, manifestResp.WebhookSecret)
	utils.SetupGitHubApp(appID, &appSlug, &privateKey, nil, &appName)

	log.Printf("[GITHUB] GitHub App config saved successfully, redirecting to installation...")

	// Generate install state and redirect to GitHub App installation
	installState := generateSecureSecret()
	installStates.add(installState)
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s&redirect_url=%s",
		url.QueryEscape(appSlug),
		url.QueryEscape(installState),
		url.QueryEscape(fmt.Sprintf("%s/api/v1/github/app/install/callback", baseURL)))

	return c.Redirect(installURL, http.StatusFound)
}

// min returns the smaller of two integers (helper for Go versions < 1.21)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GitHubInstallCallback stores installation ID after user installs the App
func GitHubInstallCallback(c *fiber.Ctx) error {
	installationID := c.QueryInt("installation_id")
	state := c.Query("state")
	setupAction := c.Query("setup_action") // "install" or "update"

	statePreview := ""
	if len(state) > 8 {
		statePreview = state[:8] + "..."
	} else {
		statePreview = state
	}
	log.Printf("[GITHUB] Install callback received: installation_id=%d, state=%s, setup_action=%s",
		installationID, statePreview, setupAction)

	// State validation: either valid state from our store, or GitHub's setup_url redirect (has setup_action)
	// GitHub's setup_url redirect doesn't preserve our state, so we accept requests with setup_action
	if state != "" && !installStates.validate(state, 30*time.Minute) {
		log.Printf("[GITHUB] Invalid state provided in install callback")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid state - please try again")
	}

	// If no state and no setup_action, this is an invalid request
	if state == "" && setupAction == "" {
		log.Printf("[GITHUB] Missing both state and setup_action in install callback - rejecting")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid request - missing state or setup_action")
	}

	if installationID == 0 {
		log.Printf("[GITHUB] Missing installation_id in callback")
		return c.Status(fiber.StatusBadRequest).SendString("Missing installation_id")
	}

	if err := api.GitHub.UpdateGitHubInstallationID(c.Context(), int64(installationID)); err != nil {
		log.Printf("[GITHUB] Failed to store installation ID: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save installation")
	}

	// Update in-memory config with latest installation
	appID, appSlug, appName, privateKey, _ := utils.GetGitHubAppConfig()
	if appID != nil && privateKey != nil {
		inst := int64(installationID)
		utils.SetupGitHubApp(*appID, appSlug, privateKey, &inst, appName)
		log.Printf("[GITHUB] ✅ GitHub App installed successfully: app_id=%d, installation_id=%d", *appID, installationID)
	}

	// Override CSP for this page to allow postMessage to parent
	c.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")

	// Return a nice success page that notifies the opener and closes
	html := `<!DOCTYPE html>
<html>
<head>
  <title>GitHub App Installed</title>
  <style>
    body { 
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      display: flex; 
      justify-content: center; 
      align-items: center; 
      height: 100vh; 
      margin: 0;
      background: #f6f8fa;
    }
    .container { text-align: center; max-width: 400px; padding: 40px; }
    .success-icon {
      width: 64px;
      height: 64px;
      background: #28a745;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      margin: 0 auto 20px;
    }
    .success-icon svg { width: 32px; height: 32px; }
    h2 { color: #24292e; margin-bottom: 8px; }
    p { color: #586069; margin-bottom: 20px; }
    .closing { color: #0366d6; font-size: 14px; }
  </style>
</head>
<body>
<div class="container">
  <div class="success-icon">
    <svg fill="white" viewBox="0 0 24 24"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>
  </div>
  <h2>GitHub App Installed!</h2>
  <p>Your GitHub App has been successfully connected to this Citizen instance.</p>
  <p class="closing">This window will close automatically...</p>
</div>
<script>
  // Notify parent window and close
  setTimeout(function() {
    if (window.opener) {
      window.opener.postMessage({ 
        type: 'github-app-install-success',
        installation_id: ` + fmt.Sprintf("%d", installationID) + `
      }, '*');
      window.close();
    }
  }, 1500);
  
  // Fallback: show close button after 3 seconds
  setTimeout(function() {
    if (!window.closed) {
      document.querySelector('.closing').innerHTML = 
        '<button onclick="window.close()" style="background:#0366d6;color:white;border:none;padding:10px 20px;border-radius:6px;cursor:pointer;">Close Window</button>';
    }
  }, 3000);
</script>
</body>
</html>`

	return c.Type("html").SendString(html)
}

// GitHubConfigRequest represents GitHub config setup request
// Supports either OAuth App (client_id/secret) or GitHub App (app_id + private_key + installation_id)
type GitHubConfigRequest struct {
	ClientID        string  `json:"client_id"`
	ClientSecret    string  `json:"client_secret"`
	RedirectURI     string  `json:"redirect_uri"`
	AppID           *int64  `json:"app_id"`
	AppSlug         *string `json:"app_slug"`
	AppName         *string `json:"app_name"`
	PrivateKey      *string `json:"private_key"`
	InstallationID  *int64  `json:"installation_id"`
	WebhookSecretIn *string `json:"webhook_secret"`
}

// GitHubConfigResponse represents GitHub config response (without secrets)
type GitHubConfigResponse struct {
	ClientID     string `json:"client_id"`
	RedirectURI  string `json:"redirect_uri"`
	IsActive     bool   `json:"is_active"`
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

	// Normalize/trim user input to avoid hidden whitespace issues
	req.ClientID = strings.TrimSpace(req.ClientID)
	req.ClientSecret = strings.TrimSpace(req.ClientSecret)
	req.RedirectURI = strings.TrimSpace(req.RedirectURI)
	if req.AppSlug != nil {
		s := strings.TrimSpace(*req.AppSlug)
		req.AppSlug = &s
	}
	if req.AppName != nil {
		n := strings.TrimSpace(*req.AppName)
		req.AppName = &n
	}
	if req.PrivateKey != nil {
		pk := strings.TrimSpace(*req.PrivateKey)
		req.PrivateKey = &pk
	}

	// Validate: either OAuth App (client_id/secret) or GitHub App (app_id)
	hasOAuth := req.ClientID != "" && req.ClientSecret != ""
	hasApp := req.AppID != nil
	if !hasOAuth && !hasApp {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Provide either Client ID/Secret or App ID",
		})
	}

	// Default redirect URI
	if req.RedirectURI == "" {
		req.RedirectURI = fmt.Sprintf("%s/api/v1/github/auth/callback", c.BaseURL())
	}

	// Generate webhook secret
	webhookSecret := generateSecureSecret()
	if req.WebhookSecretIn != nil && *req.WebhookSecretIn != "" {
		webhookSecret = *req.WebhookSecretIn
	}

	// Save to database (encrypted)
	err := saveGitHubConfigToDB(req.ClientID, req.ClientSecret, req.RedirectURI, webhookSecret, req.AppID, req.AppSlug, req.AppName, req.PrivateKey, req.InstallationID)
	if err != nil {
		log.Printf("[GITHUB] Failed to save GitHub config to database: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save GitHub config to database",
		})
	}

	// Setup GitHub OAuth in memory
	if hasOAuth {
		err = utils.SetupGitHubOAuth(req.ClientID, req.ClientSecret, req.RedirectURI, webhookSecret)
		if err != nil {
			log.Printf("[GITHUB] Failed to setup GitHub OAuth: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to setup GitHub OAuth",
			})
		}
	}
	if hasApp {
		if req.PrivateKey != nil {
			utils.SetupGitHubApp(*req.AppID, req.AppSlug, req.PrivateKey, req.InstallationID, req.AppName)
		}
	}

	log.Printf("[GITHUB] ✅ GitHub OAuth setup completed")
	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub OAuth setup completed successfully",
		fiber.Map{
			"configured": true,
		},
	))
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
		log.Printf("[CONFIG] Failed to load GitHub config from DB: %v", err)
		// Config doesn't exist in DB, return not configured
		return c.JSON(utils.NewCitizenResponse(
			true,
			"GitHub not configured",
			fiber.Map{
				"configured": false,
			},
		))
	}

	// Decrypt only client ID for display
	clientID, err := utils.DecryptString(config.ClientID)
	if err != nil {
		log.Printf("[CONFIG] Failed to decrypt config: %v", err)
		// Decryption failed, config is corrupted - return not configured
		return c.JSON(utils.NewCitizenResponse(
			true,
			"GitHub not configured",
			fiber.Map{
				"configured": false,
			},
		))
	}

	// Mask client ID for security (show only first 8 chars)
	maskedClientID := clientID
	if len(clientID) > 8 {
		maskedClientID = clientID[:8] + "..."
	}

	response := fiber.Map{
		"configured":      true,
		"client_id":       maskedClientID,
		"redirect_uri":    config.RedirectURI,
		"is_active":       true,
		"configured_at":   config.CreatedAt.Format(time.RFC3339),
		"app_id":          config.AppID,
		"app_slug":        config.AppSlug,
		"app_name":        config.AppName,
		"installation_id": config.InstallationID,
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

// simple in-memory state store for manifest/install flows
type stateStore struct {
	mu    sync.Mutex
	items map[string]time.Time
}

func newStateStore() *stateStore {
	return &stateStore{
		items: make(map[string]time.Time),
	}
}

func (s *stateStore) add(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[state] = time.Now()
}

func (s *stateStore) validate(state string, maxAge time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	created, ok := s.items[state]
	if !ok {
		return false
	}
	if time.Since(created) > maxAge {
		delete(s.items, state)
		return false
	}
	delete(s.items, state)
	return true
}

func (s *stateStore) exists(state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.items[state]
	return ok
}

// generateSecureSecret generates a cryptographically secure secret
func generateSecureSecret() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// htmlEscapeSingleQuotes escapes single quotes for embedding JSON in HTML attribute
func htmlEscapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "&#39;")
}

// saveGitHubConfigToDB saves GitHub configuration to database (encrypted)
func saveGitHubConfigToDB(clientID, clientSecret, redirectURI, webhookSecret string, appID *int64, appSlug, appName *string, privateKey *string, installationID *int64) error {
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

	var encryptedPrivateKey *string
	if privateKey != nil && *privateKey != "" {
		encPk, err := utils.EncryptString(*privateKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt private key: %w", err)
		}
		encryptedPrivateKey = &encPk
	}

	// Save to database - first deactivate old configs, then insert new
	err = api.GitHub.SaveGitHubConfig(context.Background(), encryptedClientID, encryptedClientSecret, encryptedWebhookSecret, redirectURI, appID, appSlug, appName, encryptedPrivateKey, installationID)
	if err != nil {
		return fmt.Errorf("failed to save GitHub config to database: %w", err)
	}

	fmt.Printf("[CONFIG] ✅ GitHub config saved to database\n")
	return nil
}

// LoadGitHubConfigFromDB loads GitHub configuration from database (decrypted)
func LoadGitHubConfigFromDB() (clientID, clientSecret, redirectURI, webhookSecret string, appID *int64, appSlug, appName *string, privateKey *string, installationID *int64, err error) {
	config, err := api.GitHub.GetGitHubConfigFull(context.Background())
	if err != nil {
		return "", "", "", "", nil, nil, nil, nil, nil, fmt.Errorf("failed to load GitHub config from database: %w", err)
	}

	// Decrypt sensitive data
	clientID, err = utils.DecryptString(config.ClientID)
	if err != nil {
		return "", "", "", "", nil, nil, nil, nil, nil, fmt.Errorf("failed to decrypt client ID: %w", err)
	}

	clientSecret, err = utils.DecryptString(config.ClientSecret)
	if err != nil {
		return "", "", "", "", nil, nil, nil, nil, nil, fmt.Errorf("failed to decrypt client secret: %w", err)
	}

	webhookSecret, err = utils.DecryptString(config.WebhookSecret)
	if err != nil {
		return "", "", "", "", nil, nil, nil, nil, nil, fmt.Errorf("failed to decrypt webhook secret: %w", err)
	}

	fmt.Printf("[CONFIG] ✅ GitHub config loaded from database\n")
	var decryptedPrivateKey *string
	if config.PrivateKey != nil && *config.PrivateKey != "" {
		if pk, decErr := utils.DecryptString(*config.PrivateKey); decErr == nil {
			decryptedPrivateKey = &pk
		} else {
			return "", "", "", "", nil, nil, nil, nil, nil, fmt.Errorf("failed to decrypt private key: %w", decErr)
		}
	}

	return clientID, clientSecret, config.RedirectURI, webhookSecret, config.AppID, config.AppSlug, config.AppName, decryptedPrivateKey, config.InstallationID, nil
}
