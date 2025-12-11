package webhook

import (
	"backend/internal/database"
	"backend/internal/database/api"
	githubmodels "backend/internal/github/models"
	githubservices "backend/internal/github/services"
	"backend/internal/platform"
	"backend/pkg/logger"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

var log = logger.Default().WithComponent("github-webhook")

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

	log.WithFields(map[string]interface{}{
		"event_type":  eventType,
		"delivery_id": deliveryID,
	}).Info("Received GitHub webhook")

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
		log.WithField("error", err.Error()).Warn("Failed to parse push event")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payload",
		})
	}

	// Extract branch name from ref (refs/heads/main -> main)
	branch := strings.TrimPrefix(pushEvent.Ref, "refs/heads/")

	log.WithFields(map[string]interface{}{
		"repository": pushEvent.Repository.FullName,
		"branch":     branch,
		"commit":     pushEvent.HeadCommit.ID,
	}).Info("Push event received")

	// Find repository connection in database
	repoConnection, err := api.GitHub.GetGitHubRepositoryByID(c.Context(), pushEvent.Repository.ID)
	if err != nil {
		log.WithFields(map[string]interface{}{
			"repository":    pushEvent.Repository.FullName,
			"repository_id": pushEvent.Repository.ID,
			"error":         err.Error(),
		}).Debug("No repository connection found")
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
		// Callback: Get authenticated Git URL using GitHub App installation token
		func() (string, *int, error) {
			gitURL := fmt.Sprintf("https://github.com/%s.git", pushEvent.Repository.FullName)

			// Get GitHub App installation token
			tokenResp, tokenErr := githubservices.GetGitHubInstallationToken()
			if tokenErr != nil || tokenResp == nil {
				log.WithField("error", tokenErr).Error("Failed to get GitHub App installation token")
				return "", nil, fmt.Errorf("GitHub App not configured or installation token failed")
			}

			log.Debug("Using GitHub App installation token for authentication")
			authenticatedGitURL := strings.Replace(gitURL, "https://github.com/",
				fmt.Sprintf("https://x-access-token:%s@github.com/", tokenResp.Token), 1)

			return authenticatedGitURL, nil, nil
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
		log.WithField("error", err.Error()).Debug("Failed to process webhook")
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
