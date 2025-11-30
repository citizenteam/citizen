package apps

import (
	handlers "backend/internal/github/handlers"
	githubservices "backend/internal/github/services"
	"backend/internal/utils"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
)

// StartGitHubManifest kicks off GitHub App manifest flow (instance-owned app)
// This returns a URL to the manifest redirect endpoint which will POST the manifest to GitHub
func StartGitHubManifest(c *fiber.Ctx) error {
	// Generate secure state for CSRF protection
	stateService := githubservices.GetStateService()
	state := stateService.GenerateManifestState()

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

// GitHubManifestRedirect renders an auto-submitting form to POST manifest to GitHub
func GitHubManifestRedirect(c *fiber.Ctx) error {
	state := c.Query("state")
	if state == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Missing state parameter")
	}

	// Validate state exists in our store (but don't consume it yet - callback will do that)
	stateService := githubservices.GetStateService()
	if !stateService.ExistsManifestState(state) {
		log.Printf("[GITHUB] Invalid or expired state in manifest redirect: %s", state[:8]+"...")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid or expired state - please try again")
	}

	baseURL := githubservices.NormalizeBaseURL(c.BaseURL())
	webhookURL := githubservices.GetWebhookURL(c.BaseURL())
	// redirect_url must NOT have query params - GitHub adds its own (code, state)
	redirectURL := fmt.Sprintf("%s/api/v1/github/app/manifest/callback", baseURL)

	// Generate unique app name based on domain
	appName := fmt.Sprintf("citizen-%d", time.Now().Unix())

	// OAuth callback URL for user authorization
	callbackURL := githubservices.GetRedirectURI(c.BaseURL())

	// Setup URL for post-installation redirect (GitHub adds installation_id query param)
	setupURL := githubservices.GetAppInstallCallbackURL(c.BaseURL())

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

// Import handlers package helpers
var (
	htmlEscapeSingleQuotes = handlers.HtmlEscapeSingleQuotes
	min                    = handlers.Min
)

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
	stateService := githubservices.GetStateService()
	if state == "" || !stateService.ValidateManifestState(state, 15*time.Minute) {
		log.Printf("[GITHUB] Invalid or expired state in manifest callback")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid or expired state - please try again")
	}

	// Convert the manifest code to get app credentials
	manifestResp, err := githubservices.ConvertManifestCode(code)
	if err != nil {
		log.Printf("[GITHUB] Failed to convert manifest code: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to convert manifest code - please try again")
	}

	log.Printf("[GITHUB] ✅ GitHub App created: id=%d, slug=%s, name=%s",
		manifestResp.ID, manifestResp.Slug, manifestResp.Name)

	redirectURI := githubservices.GetRedirectURI(c.BaseURL())

	appID := manifestResp.ID
	appSlug := manifestResp.Slug
	appName := manifestResp.Name
	privateKey := manifestResp.Pem

	// Save the GitHub App configuration to database (including private key)
	if err := githubservices.SaveGitHubConfigToDB(manifestResp.ClientID, manifestResp.ClientSecret, redirectURI, manifestResp.WebhookSecret, &appID, &appSlug, &appName, &privateKey, nil); err != nil {
		log.Printf("[GITHUB] Failed to save manifest config to database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save GitHub App config")
	}

	// Setup in-memory caches
	_ = githubservices.SetupGitHubOAuth(manifestResp.ClientID, manifestResp.ClientSecret, redirectURI, manifestResp.WebhookSecret)
	githubservices.SetupGitHubApp(appID, &appSlug, &privateKey, nil, &appName)

	log.Printf("[GITHUB] GitHub App config saved successfully, redirecting to installation...")

	// Generate install state and redirect to GitHub App installation
	installStateService := githubservices.GetStateService()
	installState := installStateService.GenerateInstallState()
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s&redirect_url=%s",
		url.QueryEscape(appSlug),
		url.QueryEscape(installState),
		url.QueryEscape(githubservices.GetAppInstallCallbackURL(c.BaseURL())))

	return c.Redirect(installURL, http.StatusFound)
}
