package config

import (
	"backend/internal/database/api"
	githubmodels "backend/internal/github/models"
	githubservices "backend/internal/github/services"
	"backend/pkg/logger"
	"backend/pkg/response"
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

var log = logger.Default().WithComponent("github-config")

// SetupGitHubConfig handles GitHub App configuration setup
func SetupGitHubConfig(c *fiber.Ctx) error {
	var req githubmodels.ConfigRequest

	if err := c.BodyParser(&req); err != nil {
		log.WithField("error", err.Error()).Warn("Failed to parse request body")
		return response.BadRequest(c, "Invalid request body")
	}

	log.Debug("SetupGitHubConfig called")

	// Normalize/trim user input
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

	// Validate: GitHub App requires app_id and private_key
	if req.AppID == nil {
		return response.BadRequest(c, "App ID is required")
	}
	if req.PrivateKey == nil || *req.PrivateKey == "" {
		return response.BadRequest(c, "Private Key is required")
	}

	// App slug and name are required
	appSlug := ""
	if req.AppSlug != nil {
		appSlug = *req.AppSlug
	}
	appName := ""
	if req.AppName != nil {
		appName = *req.AppName
	}

	// Generate webhook secret if not provided
	webhookSecret := ""
	if req.WebhookSecretIn != nil && *req.WebhookSecretIn != "" {
		webhookSecret = *req.WebhookSecretIn
	} else {
		webhookSecret = githubservices.GenerateSecureSecret()
		log.Debug("Generated new webhook secret")
	}

	// Save to database (encrypted)
	err := githubservices.SaveGitHubAppConfigToDB(
		webhookSecret,
		*req.AppID,
		appSlug,
		appName,
		*req.PrivateKey,
		req.InstallationID,
	)
	if err != nil {
		log.WithField("error", err.Error()).Error("Failed to save GitHub config")
		return response.InternalServerError(c, "Failed to save GitHub configuration")
	}

	// Setup in-memory config
	githubservices.SetupGitHubWebhookSecret(webhookSecret)
	githubservices.SetupGitHubApp(*req.AppID, req.AppSlug, req.PrivateKey, req.InstallationID, req.AppName)

	log.Info("GitHub App setup completed")
	return response.SuccessWithMessage(c, "GitHub App setup completed successfully", fiber.Map{
		"configured": true,
	})
}

// GetGitHubConfig returns current GitHub configuration (without secrets)
func GetGitHubConfig(c *fiber.Ctx) error {
	log.Debug("GetGitHubConfig called")

	// Check if configured
	if !githubservices.IsGitHubConfigured() {
		log.Debug("GitHub not configured")
		return response.Success(c, fiber.Map{"configured": false})
	}

	log.Debug("GitHub is configured, fetching from DB")

	// Get config from database
	config, err := api.GitHub.GetGitHubConfig(context.Background())
	if err != nil {
		log.WithField("error", err.Error()).Debug("Failed to load GitHub config from DB")
		return response.Success(c, fiber.Map{"configured": false})
	}

	configData := fiber.Map{
		"configured":      true,
		"is_active":       true,
		"configured_at":   config.CreatedAt.Format(time.RFC3339),
		"app_id":          config.AppID,
		"app_slug":        config.AppSlug,
		"app_name":        config.AppName,
		"installation_id": config.InstallationID,
	}

	log.WithField("config", configData).Debug("Returning config response")
	return response.Success(c, configData)
}

// DeleteGitHubConfig removes GitHub configuration
func DeleteGitHubConfig(c *fiber.Ctx) error {
	// Soft delete - mark as inactive
	err := api.GitHub.DeleteGitHubConfig(context.Background())
	if err != nil {
		return response.InternalServerError(c, "Failed to delete GitHub config")
	}

	log.Info("GitHub config deleted")
	return response.SuccessWithMessage(c, "GitHub configuration deleted successfully", nil)
}
