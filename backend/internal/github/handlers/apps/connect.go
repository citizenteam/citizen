package apps

import (
	"backend/internal/database/api"
	githubmodels "backend/internal/github/models"
	githubservices "backend/internal/github/services"
	"backend/internal/utils"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
)

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
		UpdateURLs bool   `json:"update_urls"` // If true, update GitHub App URLs to this server
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
	jwtToken, err := githubservices.GenerateGitHubAppJWTWithKey(connectData.AppID, connectData.PrivateKey)
	if err != nil {
		log.Printf("[GITHUB] Failed to generate JWT: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid Private Key - could not generate JWT. Make sure it's a valid PEM file.",
			nil,
		))
	}

	// Get app info using JWT
	appInfo, err := githubservices.GetGitHubAppInfoWithJWT(jwtToken)
	if err != nil {
		log.Printf("[GITHUB] Failed to get app info: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Could not get app info. Check if App ID and Private Key match.",
			nil,
		))
	}

	// Get installations for this app
	installations, err := githubservices.GetAppInstallationsWithJWT(jwtToken)
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

	// Generate config values using service helpers
	clientID := fmt.Sprintf("app-%d", connectData.AppID)
	clientSecret := githubservices.GenerateSecureSecret()
	webhookSecret := githubservices.GenerateSecureSecret()
	baseURL := githubservices.NormalizeBaseURL(c.BaseURL())
	redirectURI := githubservices.GetRedirectURI(c.BaseURL())

	// Update GitHub App webhook config if requested
	var urlsUpdated bool
	if connectData.UpdateURLs {
		log.Printf("[GITHUB] Updating GitHub App webhook config to: %s", baseURL)

		urlUpdate := githubmodels.AppURLUpdate{
			HomepageURL:   baseURL,
			WebhookURL:    githubservices.GetWebhookURL(c.BaseURL()),
			WebhookSecret: webhookSecret, // Update webhook secret to match our new one
			CallbackURLs:  []string{redirectURI},
			SetupURL:      githubservices.GetAppInstallCallbackURL(c.BaseURL()),
		}

		if err := githubservices.UpdateGitHubAppURLs(jwtToken, urlUpdate); err != nil {
			log.Printf("[GITHUB] ⚠️ Failed to update GitHub App webhook config: %v", err)
			// Don't fail the connection, just warn
		} else {
			urlsUpdated = true
			log.Printf("[GITHUB] ✅ GitHub App webhook URL and secret updated successfully")
		}
	}

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
	githubservices.SetupGitHubOAuth(clientID, clientSecret, redirectURI, webhookSecret)
	githubservices.SetupGitHubApp(connectData.AppID, &appInfo.Slug, &connectData.PrivateKey, &installationID, &appInfo.Name)

	log.Printf("[GITHUB] ✅ GitHub App connected via private key: %s (ID: %d, Installation: %d)",
		appInfo.Name, connectData.AppID, installationID)

	message := "GitHub App connected successfully"
	if urlsUpdated {
		message = "GitHub App connected and URLs updated successfully"
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		message,
		fiber.Map{
			"app_id":          connectData.AppID,
			"app_slug":        appInfo.Slug,
			"app_name":        appInfo.Name,
			"installation_id": installationID,
			"configured":      true,
			"urls_updated":    urlsUpdated,
			"webhook_url":     githubservices.GetWebhookURL(c.BaseURL()),
			"callback_url":    redirectURI,
		},
	))
}
