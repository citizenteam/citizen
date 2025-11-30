package webhook

import (
	"backend/internal/database"
	"backend/internal/database/api"
	githubmodels "backend/internal/github/models"
	githubservices "backend/internal/github/services"
	"backend/internal/platform"
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
	if !githubservices.ValidateGitHubSignature(payload, signature) {
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
	var pushEvent githubmodels.PushEvent
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

	// Process webhook using service
	err = githubservices.ProcessPushWebhook(
		&pushEvent,
		appName,
		autoDeploy,
		deployBranch,
		// Callback: Get authenticated Git URL
		func() (string, *int, error) {
			gitURL := fmt.Sprintf("https://github.com/%s.git", pushEvent.Repository.FullName)
			var userID *int
			authenticatedGitURL := gitURL

			// Try GitHub App installation token first (preferred)
			if tokenResp, tokenErr := githubservices.GetGitHubInstallationToken(); tokenErr == nil && tokenResp != nil {
				log.Printf("[WEBHOOK] 🔑 Using GitHub App installation token for authentication")
				authenticatedGitURL = strings.Replace(gitURL, "https://github.com/",
					fmt.Sprintf("https://x-access-token:%s@github.com/", tokenResp.Token), 1)
			} else {
				// Fall back to user's OAuth token
				repoConn, connErr := api.GitHub.GetGitHubRepositoryConnectionByAppName(context.Background(), appName)
				if connErr == nil && repoConn.UserID != 0 {
					uid := repoConn.UserID
					userID = &uid

					accessToken, tokenErr := api.GitHub.GetUserGitHubAccessToken(context.Background(), uid)
					if tokenErr == nil && accessToken != "" {
						log.Printf("[WEBHOOK] 🔑 Using user OAuth token for authentication (user: %d)", uid)
						authenticatedGitURL = strings.Replace(gitURL, "https://github.com/",
							fmt.Sprintf("https://x-access-token:%s@github.com/", accessToken), 1)
					}
				}
			}
			return authenticatedGitURL, userID, nil
		},
		// Callback: Trigger deployment
		func(gitURL string, branch string, userID *int) (string, error) {
			return platform.GetAdapter().DeployFromGit(appName, gitURL, branch, userID)
		},
		// Callback: Log deployment
		func(appName, gitURL, branch, commitID, commitMessage, authorName string) (interface{}, error) {
			return database.LogWebhookDeployment(appName, gitURL, branch, commitID, commitMessage, authorName)
		},
		// Callback: Update activity
		func(activity interface{}, status string, errorMsg *string) {
			if activity != nil {
				if act, ok := activity.(*database.Activity); ok {
					var activityStatus database.ActivityStatus
					if status == "success" {
						activityStatus = database.StatusSuccess
					} else if status == "error" {
						activityStatus = database.StatusError
					} else {
						activityStatus = database.StatusPending
					}
					database.UpdateActivity(act.ID, activityStatus, errorMsg)
				}
			}
		},
		// Callback: Update deployment status
		func(appName, commitID, status string, output, errorOutput *string) {
			database.UpdateGitHubDeploymentStatus(appName, commitID, status, output, errorOutput)
		},
	)

	if err != nil {
		log.Printf("[WEBHOOK] Failed to process webhook: %v", err)
		return c.JSON(fiber.Map{
			"status": "ignored",
			"reason": err.Error(),
		})
	}

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
