package config

import (
	"backend/internal/database/api"
	githubmodels "backend/internal/github/models"
	githubservices "backend/internal/github/services"
	"backend/internal/utils"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// SetupGitHubConfig handles GitHub OAuth configuration setup
func SetupGitHubConfig(c *fiber.Ctx) error {
	var req githubmodels.ConfigRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("[GITHUB] Failed to parse request body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	log.Printf("[GITHUB] SetupGitHubConfig called")

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
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Provide either Client ID/Secret or App ID",
			nil,
		))
	}

	// Default redirect URI
	if req.RedirectURI == "" {
		req.RedirectURI = githubservices.GetRedirectURI(c.BaseURL())
	}

	// Generate webhook secret if not provided
	webhookSecret := ""
	if req.WebhookSecretIn != nil && *req.WebhookSecretIn != "" {
		webhookSecret = *req.WebhookSecretIn
	} else {
		webhookSecret = githubservices.GenerateSecureSecret()
		log.Printf("[GITHUB] Generated new webhook secret")
	}

	// Save to database (encrypted)
	err := githubservices.SaveGitHubConfigToDB(
		req.ClientID,
		req.ClientSecret,
		req.RedirectURI,
		webhookSecret,
		req.AppID,
		req.AppSlug,
		req.AppName,
		req.PrivateKey,
		req.InstallationID,
	)
	if err != nil {
		log.Printf("[GITHUB] Failed to save GitHub config: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			fmt.Sprintf("Failed to save GitHub configuration: %v", err),
			nil,
		))
	}

	// Setup GitHub OAuth in memory
	if hasOAuth {
		err = githubservices.SetupGitHubOAuth(req.ClientID, req.ClientSecret, req.RedirectURI, webhookSecret)
		if err != nil {
			log.Printf("[GITHUB] Failed to setup GitHub OAuth: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
				false,
				"Failed to setup GitHub OAuth",
				nil,
			))
		}
	}
	if hasApp && req.PrivateKey != nil {
		githubservices.SetupGitHubApp(*req.AppID, req.AppSlug, req.PrivateKey, req.InstallationID, req.AppName)
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
	if !githubservices.IsGitHubConfigured() {
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
