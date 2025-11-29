package handlers

import (
	"backend/database"
	"backend/database/api"
	"backend/platform"
	"backend/utils"
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
)

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

		// Get GitHub token for private repo access
		var userID *int
		authenticatedGitURL := gitURL

		// Try GitHub App installation token first (preferred)
		if tokenResp, tokenErr := utils.GetGitHubInstallationToken(); tokenErr == nil && tokenResp != nil {
			log.Printf("[WEBHOOK] 🔑 Using GitHub App installation token for authentication")
			// Convert: https://github.com/user/repo.git → https://x-access-token:TOKEN@github.com/user/repo.git
			authenticatedGitURL = strings.Replace(gitURL, "https://github.com/",
				fmt.Sprintf("https://x-access-token:%s@github.com/", tokenResp.Token), 1)
		} else {
			// Fall back to user's OAuth token
			repoConnection, connErr := api.GitHub.GetGitHubRepositoryConnectionByAppName(context.Background(), appName)
			if connErr == nil && repoConnection.UserID != 0 {
				uid := repoConnection.UserID
				userID = &uid

				// Get user's GitHub access token
				accessToken, tokenErr := api.GitHub.GetUserGitHubAccessToken(context.Background(), uid)
				if tokenErr == nil && accessToken != "" {
					log.Printf("[WEBHOOK] 🔑 Using user OAuth token for authentication (user: %d)", uid)
					authenticatedGitURL = strings.Replace(gitURL, "https://github.com/",
						fmt.Sprintf("https://x-access-token:%s@github.com/", accessToken), 1)
				} else {
					log.Printf("[WEBHOOK] ⚠️ No access token found for user %d: %v", uid, tokenErr)
				}
			} else {
				log.Printf("[WEBHOOK] ⚠️ No user ID found for webhook authentication: %v", connErr)
			}
		}

		// 🚀 Trigger deployment using existing deploy logic (WITH AUTHENTICATED GIT URL)
		output, err := platform.GetAdapter().DeployFromGit(appName, authenticatedGitURL, branch, userID)
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

			// Note: Traefik reload will be triggered automatically by traefik-watcher
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
