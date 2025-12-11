package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"backend/internal/github/models"
	"backend/pkg/errors"
	"backend/pkg/logger"
)

// CreateWebhook creates a GitHub webhook for repository
func CreateWebhook(accessToken, owner, repo, webhookURL string) (*models.Webhook, error) {
	webhookSecret := GetWebhookSecret()

	log := logger.Default().WithComponent("github-webhook")
	log.Debug("CreateWebhook called")

	if webhookSecret == "" {
		// If webhook secret is empty, generate one and save it
		log.Info("Webhook secret is empty, generating new one")
		webhookSecret = GenerateSecureSecret()
		SetupGitHubWebhookSecret(webhookSecret)
		log.Info("Generated and saved new webhook secret")
	}

	webhook := map[string]interface{}{
		"name":   "web",
		"active": true,
		"events": []string{"push", "pull_request"},
		"config": map[string]interface{}{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       webhookSecret,
			"insecure_ssl": "0",
		},
	}

	jsonData, err := json.Marshal(webhook)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/hooks", owner, repo)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, errors.InternalErrorf("failed to create webhook: %s", string(body))
	}

	var createdWebhook models.Webhook
	if err := json.Unmarshal(body, &createdWebhook); err != nil {
		return nil, err
	}

	return &createdWebhook, nil
}

// ListWebhooks lists all webhooks for a repository
func ListWebhooks(accessToken, owner, repo string) ([]models.Webhook, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/hooks", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.InternalErrorf("failed to list webhooks: %s", string(body))
	}

	var webhooks []models.Webhook
	if err := json.NewDecoder(resp.Body).Decode(&webhooks); err != nil {
		return nil, err
	}

	return webhooks, nil
}

// FindExistingWebhook finds an existing webhook for the given URL
func FindExistingWebhook(accessToken, owner, repo, webhookURL string) (*models.Webhook, error) {
	webhooks, err := ListWebhooks(accessToken, owner, repo)
	if err != nil {
		return nil, err
	}

	for _, webhook := range webhooks {
		if webhook.Config.URL == webhookURL {
			return &webhook, nil
		}
	}

	return nil, nil // Not found
}

// CreateOrFindWebhook creates a webhook or finds an existing one
// This handles the common pattern of creating a webhook and handling "Hook already exists" error
func CreateOrFindWebhook(accessToken, owner, repo, webhookURL string) (*int64, error) {
	webhook, err := CreateWebhook(accessToken, owner, repo, webhookURL)
	if err != nil {
		// Check if webhook already exists
		if strings.Contains(err.Error(), "Hook already exists") {
			existingWebhook, findErr := FindExistingWebhook(accessToken, owner, repo, webhookURL)
			if findErr == nil && existingWebhook != nil {
				return &existingWebhook.ID, nil
			}
			// If we can't find existing webhook, return nil but no error
			// (webhook exists but we don't have the ID)
			return nil, nil
		}
		return nil, err
	}
	return &webhook.ID, nil
}

// DeleteWebhook deletes a GitHub webhook
func DeleteWebhook(accessToken, owner, repo string, webhookID int64) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/hooks/%d", owner, repo, webhookID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return errors.InternalErrorf("failed to delete webhook: %s", string(body))
	}

	return nil
}

// ValidateGitHubSignature validates GitHub webhook signature using constant-time comparison
// to prevent timing attacks
func ValidateGitHubSignature(payload []byte, signature string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	webhookSecret := GetWebhookSecret()
	if webhookSecret == "" {
		return false
	}

	expectedSignature := "sha256=" + generateHMACSignature(payload, webhookSecret)
	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expectedSignature)) == 1
}

// generateHMACSignature generates HMAC SHA256 signature
func generateHMACSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// ProcessPushWebhook processes a GitHub push webhook event
// This function handles the business logic for push events including deployment triggering
// Note: This function is designed to be called from a handler and handles deployment asynchronously
// Callbacks are provided by the handler to avoid circular dependencies
func ProcessPushWebhook(
	pushEvent *models.PushEvent,
	appName string,
	autoDeploy bool,
	deployBranch string,
	getAuthenticatedGitURL func() (string, *int, error),
	triggerDeployment func(gitURL string, branch string, userID *int) (string, error),
	logDeployment func(appName, gitURL, branch, commitID, commitMessage, authorName string) (interface{}, error),
	updateActivity func(activityID interface{}, status string, errorMsg *string),
	updateDeploymentStatus func(appName, commitID, status string, output, errorOutput *string),
) error {
	log := logger.Default().WithComponent("github-webhook")

	// Extract branch name from ref (refs/heads/main -> main)
	branch := strings.TrimPrefix(pushEvent.Ref, "refs/heads/")

	log.WithFields(map[string]interface{}{
		"repository": pushEvent.Repository.FullName,
		"branch":     branch,
		"commit":     pushEvent.HeadCommit.ID,
		"app_name":   appName,
	}).Info("Processing push webhook")

	// Check if auto deploy is enabled
	if !autoDeploy {
		log.WithField("app_name", appName).Debug("Auto deploy disabled, ignoring")
		return errors.BadRequest("auto deploy disabled")
	}

	// Check if this is the correct branch for deployment
	if branch != deployBranch {
		log.WithFields(map[string]interface{}{
			"branch":        branch,
			"deploy_branch": deployBranch,
			"app_name":      appName,
		}).Debug("Branch does not match deploy branch, ignoring")
		return errors.BadRequestf("branch %s does not match deploy branch %s", branch, deployBranch)
	}

	log.WithFields(map[string]interface{}{
		"app_name":   appName,
		"repository": pushEvent.Repository.FullName,
		"branch":     branch,
	}).Info("Triggering deployment")

	// Trigger deployment asynchronously
	go func() {
		// Create Git URL from repository full name
		gitURL := fmt.Sprintf("https://github.com/%s.git", pushEvent.Repository.FullName)

		// Log webhook deployment start
		deployActivity, activityErr := logDeployment(
			appName,
			gitURL,
			branch,
			pushEvent.HeadCommit.ID,
			pushEvent.HeadCommit.Message,
			pushEvent.HeadCommit.Author.Name,
		)
		if activityErr != nil {
			log.WithField("error", activityErr.Error()).Warn("Failed to log webhook deployment activity")
		}

		// Get authenticated Git URL
		authenticatedGitURL, userID, err := getAuthenticatedGitURL()
		if err != nil {
			log.WithField("error", err.Error()).Error("Failed to get authenticated Git URL")
			if deployActivity != nil {
				errorMsg := err.Error()
				updateActivity(deployActivity, "error", &errorMsg)
			}
			return
		}

		// Trigger deployment
		output, err := triggerDeployment(authenticatedGitURL, branch, userID)
		if err != nil {
			log.WithFields(map[string]interface{}{
				"app_name": appName,
				"error":    err.Error(),
			}).Error("Deployment failed")

			// Update deployment activity as failed
			if deployActivity != nil {
				errorMsg := err.Error()
				updateActivity(deployActivity, "error", &errorMsg)
			}

			// Update GitHub deployment status as failed
			errorOutput := err.Error()
			updateDeploymentStatus(appName, pushEvent.HeadCommit.ID, "failed", &output, &errorOutput)
		} else {
			log.WithFields(map[string]interface{}{
				"app_name": appName,
				"output":   output,
			}).Info("Deployment completed successfully")

			// Update deployment activity as successful
			if deployActivity != nil {
				updateActivity(deployActivity, "success", nil)
			}

			// Update GitHub deployment status as successful
			updateDeploymentStatus(appName, pushEvent.HeadCommit.ID, "success", &output, nil)
		}
	}()

	return nil
}
