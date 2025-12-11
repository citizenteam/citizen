package apps

import (
	githubmodels "backend/internal/github/models"
	githubservices "backend/internal/github/services"
	"backend/pkg/logger"
	"backend/pkg/response"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

var logConnect = logger.Default().WithComponent("github-apps")

// ConnectWithPrivateKey connects an existing GitHub App using only App ID and Private Key
// It automatically fetches app info and finds the installation
func ConnectWithPrivateKey(c *fiber.Ctx) error {
	logConnect.Debug("ConnectWithPrivateKey called")

	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return response.Unauthorized(c, "User not authenticated")
	}

	var connectData struct {
		AppID      int64  `json:"app_id"`
		PrivateKey string `json:"private_key"`
		UpdateURLs bool   `json:"update_urls"` // If true, update GitHub App URLs to this server
	}

	if err := c.BodyParser(&connectData); err != nil {
		logConnect.WithField("error", err.Error()).Warn("Failed to parse request body")
		return response.BadRequest(c, "Invalid request body")
	}

	if connectData.AppID == 0 {
		return response.BadRequest(c, "App ID is required")
	}

	if connectData.PrivateKey == "" {
		return response.BadRequest(c, "Private Key is required")
	}

	// Generate JWT using the provided private key
	jwtToken, err := githubservices.GenerateGitHubAppJWTWithKey(connectData.AppID, connectData.PrivateKey)
	if err != nil {
		logConnect.WithField("error", err.Error()).Warn("Failed to generate JWT")
		return response.BadRequest(c, "Invalid Private Key - could not generate JWT. Make sure it's a valid PEM file.")
	}

	// Get app info using JWT
	appInfo, err := githubservices.GetGitHubAppInfoWithJWT(jwtToken)
	if err != nil {
		logConnect.WithField("error", err.Error()).Warn("Failed to get app info")
		return response.BadRequest(c, "Could not get app info. Check if App ID and Private Key match.")
	}

	// Get installations for this app
	installations, err := githubservices.GetAppInstallationsWithJWT(jwtToken)
	if err != nil {
		logConnect.WithField("error", err.Error()).Warn("Failed to get installations")
		return response.BadRequest(c, "Could not get installations. Make sure the app is installed on your GitHub account.")
	}

	if len(installations) == 0 {
		return response.ErrorWithCode(c, fiber.StatusBadRequest, "NO_INSTALLATIONS", fmt.Sprintf("No installations found. Install at: https://github.com/apps/%s/installations/new", appInfo.Slug))
	}

	// Use the first installation
	installationID := installations[0].ID
	logConnect.WithFields(map[string]interface{}{
		"app_name":        appInfo.Name,
		"app_slug":        appInfo.Slug,
		"installation_id": installationID,
	}).Info("Found app")

	// Generate config values using service helpers
	webhookSecret := githubservices.GenerateSecureSecret()
	baseURL := githubservices.NormalizeBaseURL(c.BaseURL())

	// Update GitHub App webhook config if requested
	var urlsUpdated bool
	if connectData.UpdateURLs {
		logConnect.WithField("base_url", baseURL).Info("Updating GitHub App webhook config")

		urlUpdate := githubmodels.AppURLUpdate{
			HomepageURL:   baseURL,
			WebhookURL:    githubservices.GetWebhookURL(c.BaseURL()),
			WebhookSecret: webhookSecret,
			CallbackURLs:  []string{githubservices.GetAppInstallCallbackURL(c.BaseURL())},
			SetupURL:      githubservices.GetAppInstallCallbackURL(c.BaseURL()),
		}

		if err := githubservices.UpdateGitHubAppURLs(jwtToken, urlUpdate); err != nil {
			logConnect.WithField("error", err.Error()).Warn("Failed to update GitHub App webhook config")
			// Don't fail the connection, just warn
		} else {
			urlsUpdated = true
			logConnect.Info("GitHub App webhook URL and secret updated successfully")
		}
	}

	// Save to database
	err = githubservices.SaveGitHubAppConfigToDB(
		webhookSecret,
		connectData.AppID,
		appInfo.Slug,
		appInfo.Name,
		connectData.PrivateKey,
		&installationID,
	)
	if err != nil {
		logConnect.WithField("error", err.Error()).Error("Failed to save GitHub config")
		return response.InternalServerError(c, "Failed to save GitHub App configuration")
	}

	// Setup in-memory config
	githubservices.SetupGitHubWebhookSecret(webhookSecret)
	githubservices.SetupGitHubApp(connectData.AppID, &appInfo.Slug, &connectData.PrivateKey, &installationID, &appInfo.Name)

	logConnect.WithFields(map[string]interface{}{
		"app_name":        appInfo.Name,
		"app_id":          connectData.AppID,
		"installation_id": installationID,
	}).Info("GitHub App connected via private key")

	message := "GitHub App connected successfully"
	if urlsUpdated {
		message = "GitHub App connected and URLs updated successfully"
	}

	return response.SuccessWithMessage(c, message, fiber.Map{
		"app_id":          connectData.AppID,
		"app_slug":        appInfo.Slug,
		"app_name":        appInfo.Name,
		"installation_id": installationID,
		"configured":      true,
		"urls_updated":    urlsUpdated,
		"webhook_url":     githubservices.GetWebhookURL(c.BaseURL()),
		"callback_url":    githubservices.GetAppInstallCallbackURL(c.BaseURL()),
	})
}
