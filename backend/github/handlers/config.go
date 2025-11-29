package handlers

import (
	"backend/database/api"
	"backend/utils"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

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
