package handlers

import (
	"backend/database/api"
	"backend/utils"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

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
	jwtToken, err := utils.GenerateGitHubAppJWTWithKey(connectData.AppID, connectData.PrivateKey)
	if err != nil {
		log.Printf("[GITHUB] Failed to generate JWT: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid Private Key - could not generate JWT. Make sure it's a valid PEM file.",
			nil,
		))
	}

	// Get app info using JWT
	appInfo, err := utils.GetGitHubAppInfoWithJWT(jwtToken)
	if err != nil {
		log.Printf("[GITHUB] Failed to get app info: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Could not get app info. Check if App ID and Private Key match.",
			nil,
		))
	}

	// Get installations for this app
	installations, err := utils.GetAppInstallationsWithJWT(jwtToken)
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

	// Generate config values
	clientID := fmt.Sprintf("app-%d", connectData.AppID)
	clientSecret := generateSecureSecret()
	webhookSecret := generateSecureSecret()
	baseURL := c.BaseURL()
	if strings.HasPrefix(baseURL, "http://") && !strings.Contains(baseURL, "localhost") && !strings.Contains(baseURL, "127.0.0.1") {
		baseURL = strings.Replace(baseURL, "http://", "https://", 1)
	}
	redirectURI := fmt.Sprintf("%s/api/v1/github/auth/callback", baseURL)

	// Update GitHub App webhook config if requested
	var urlsUpdated bool
	if connectData.UpdateURLs {
		log.Printf("[GITHUB] Updating GitHub App webhook config to: %s", baseURL)

		urlUpdate := utils.GitHubAppURLUpdate{
			HomepageURL:   baseURL,
			WebhookURL:    fmt.Sprintf("%s/api/v1/github/webhook", baseURL),
			WebhookSecret: webhookSecret, // Update webhook secret to match our new one
			CallbackURLs:  []string{redirectURI},
			SetupURL:      fmt.Sprintf("%s/api/v1/github/app/install/callback", baseURL),
		}

		if err := utils.UpdateGitHubAppURLs(jwtToken, urlUpdate); err != nil {
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
	utils.SetupGitHubOAuth(clientID, clientSecret, redirectURI, webhookSecret)
	utils.SetupGitHubApp(connectData.AppID, &appInfo.Slug, &connectData.PrivateKey, &installationID, &appInfo.Name)

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
			"webhook_url":     fmt.Sprintf("%s/api/v1/github/webhook", baseURL),
			"callback_url":    redirectURI,
		},
	))
}

// StartGitHubManifest kicks off GitHub App manifest flow (instance-owned app)
// This returns a URL to the manifest redirect endpoint which will POST the manifest to GitHub
func StartGitHubManifest(c *fiber.Ctx) error {
	// Generate secure state for CSRF protection
	state := generateSecureSecret()
	manifestStates.add(state)

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

// StartGitHubInstall generates an install URL for existing app
func StartGitHubInstall(c *fiber.Ctx) error {
	appID, appSlug, _, _, _ := utils.GetGitHubAppConfig()

	// Allow optional overrides via request body for existing app flow
	var body struct {
		AppID   *int64  `json:"app_id"`
		AppSlug *string `json:"app_slug"`
	}
	_ = c.BodyParser(&body)

	if body.AppID != nil {
		appID = body.AppID
	}
	if body.AppSlug != nil && *body.AppSlug != "" {
		appSlug = body.AppSlug
	}

	if appSlug == nil || appID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"GitHub App not configured yet",
			nil,
		))
	}
	state := generateSecureSecret()
	installStates.add(state)
	baseURL := c.BaseURL()
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s&redirect_url=%s", url.QueryEscape(*appSlug), url.QueryEscape(state), url.QueryEscape(fmt.Sprintf("%s/api/v1/github/app/install/callback", baseURL)))

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub App install URL generated",
		fiber.Map{
			"install_url": installURL,
			"state":       state,
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
	if !manifestStates.exists(state) {
		log.Printf("[GITHUB] Invalid or expired state in manifest redirect: %s", state[:8]+"...")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid or expired state - please try again")
	}

	baseURL := c.BaseURL()
	// Ensure HTTPS for production (GitHub requires HTTPS for redirect_url)
	if strings.HasPrefix(baseURL, "http://") && !strings.Contains(baseURL, "localhost") && !strings.Contains(baseURL, "127.0.0.1") {
		baseURL = strings.Replace(baseURL, "http://", "https://", 1)
	}
	webhookURL := fmt.Sprintf("%s/api/v1/github/webhook", baseURL)
	// redirect_url must NOT have query params - GitHub adds its own (code, state)
	redirectURL := fmt.Sprintf("%s/api/v1/github/app/manifest/callback", baseURL)

	// Generate unique app name based on domain
	appName := fmt.Sprintf("citizen-%d", time.Now().Unix())

	// OAuth callback URL for user authorization
	callbackURL := fmt.Sprintf("%s/api/v1/github/auth/callback", baseURL)

	// Setup URL for post-installation redirect (GitHub adds installation_id query param)
	setupURL := fmt.Sprintf("%s/api/v1/github/app/install/callback", baseURL)

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

// min returns the smaller of two integers (helper for Go versions < 1.21)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

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
	if state == "" || !manifestStates.validate(state, 15*time.Minute) {
		log.Printf("[GITHUB] Invalid or expired state in manifest callback")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid or expired state - please try again")
	}

	// Convert the manifest code to get app credentials
	req, err := http.NewRequest("POST", fmt.Sprintf("https://api.github.com/app-manifests/%s/conversions", code), nil)
	if err != nil {
		log.Printf("[GITHUB] Failed to build manifest conversion request: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to build request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[GITHUB] Failed to call GitHub API for manifest conversion: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to convert manifest code")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		log.Printf("[GITHUB] Manifest conversion failed (status %d): %s", resp.StatusCode, string(body))
		return c.Status(resp.StatusCode).SendString("GitHub manifest conversion failed - please try again")
	}

	var manifestResp struct {
		ID            int64  `json:"id"`
		Slug          string `json:"slug"`
		Name          string `json:"name"`
		ClientID      string `json:"client_id"`
		ClientSecret  string `json:"client_secret"`
		WebhookSecret string `json:"webhook_secret"`
		Pem           string `json:"pem"`
		HTMLURL       string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &manifestResp); err != nil {
		log.Printf("[GITHUB] Failed to parse manifest conversion response: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Invalid manifest response from GitHub")
	}

	log.Printf("[GITHUB] ✅ GitHub App created: id=%d, slug=%s, name=%s",
		manifestResp.ID, manifestResp.Slug, manifestResp.Name)

	baseURL := c.BaseURL()
	redirectURI := fmt.Sprintf("%s/api/v1/github/auth/callback", baseURL)

	appID := manifestResp.ID
	appSlug := manifestResp.Slug
	appName := manifestResp.Name
	privateKey := manifestResp.Pem

	// Save the GitHub App configuration to database (including private key)
	if err := saveGitHubConfigToDB(manifestResp.ClientID, manifestResp.ClientSecret, redirectURI, manifestResp.WebhookSecret, &appID, &appSlug, &appName, &privateKey, nil); err != nil {
		log.Printf("[GITHUB] Failed to save manifest config to database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save GitHub App config")
	}

	// Setup in-memory caches
	_ = utils.SetupGitHubOAuth(manifestResp.ClientID, manifestResp.ClientSecret, redirectURI, manifestResp.WebhookSecret)
	utils.SetupGitHubApp(appID, &appSlug, &privateKey, nil, &appName)

	log.Printf("[GITHUB] GitHub App config saved successfully, redirecting to installation...")

	// Generate install state and redirect to GitHub App installation
	installState := generateSecureSecret()
	installStates.add(installState)
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s&redirect_url=%s",
		url.QueryEscape(appSlug),
		url.QueryEscape(installState),
		url.QueryEscape(fmt.Sprintf("%s/api/v1/github/app/install/callback", baseURL)))

	return c.Redirect(installURL, http.StatusFound)
}

// GitHubInstallCallback stores installation ID after user installs the App
func GitHubInstallCallback(c *fiber.Ctx) error {
	installationID := c.QueryInt("installation_id")
	state := c.Query("state")
	setupAction := c.Query("setup_action") // "install" or "update"

	statePreview := ""
	if len(state) > 8 {
		statePreview = state[:8] + "..."
	} else {
		statePreview = state
	}
	log.Printf("[GITHUB] Install callback received: installation_id=%d, state=%s, setup_action=%s",
		installationID, statePreview, setupAction)

	// State validation: either valid state from our store, or GitHub's setup_url redirect (has setup_action)
	// GitHub's setup_url redirect doesn't preserve our state, so we accept requests with setup_action
	if state != "" && !installStates.validate(state, 30*time.Minute) {
		log.Printf("[GITHUB] Invalid state provided in install callback")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid state - please try again")
	}

	// If no state and no setup_action, this is an invalid request
	if state == "" && setupAction == "" {
		log.Printf("[GITHUB] Missing both state and setup_action in install callback - rejecting")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid request - missing state or setup_action")
	}

	if installationID == 0 {
		log.Printf("[GITHUB] Missing installation_id in callback")
		return c.Status(fiber.StatusBadRequest).SendString("Missing installation_id")
	}

	if err := api.GitHub.UpdateGitHubInstallationID(c.Context(), int64(installationID)); err != nil {
		log.Printf("[GITHUB] Failed to store installation ID: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save installation")
	}

	// Update in-memory config with latest installation
	appID, appSlug, appName, privateKey, _ := utils.GetGitHubAppConfig()
	if appID != nil && privateKey != nil {
		inst := int64(installationID)
		utils.SetupGitHubApp(*appID, appSlug, privateKey, &inst, appName)
		log.Printf("[GITHUB] ✅ GitHub App installed successfully: app_id=%d, installation_id=%d", *appID, installationID)
	}

	// Override CSP for this page to allow postMessage to parent
	c.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")

	// Return a nice success page that notifies the opener and closes
	html := `<!DOCTYPE html>
<html>
<head>
  <title>GitHub App Installed</title>
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
    .container { text-align: center; max-width: 400px; padding: 40px; }
    .success-icon {
      width: 64px;
      height: 64px;
      background: #28a745;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      margin: 0 auto 20px;
    }
    .success-icon svg { width: 32px; height: 32px; }
    h2 { color: #24292e; margin-bottom: 8px; }
    p { color: #586069; margin-bottom: 20px; }
    .closing { color: #0366d6; font-size: 14px; }
  </style>
</head>
<body>
<div class="container">
  <div class="success-icon">
    <svg fill="white" viewBox="0 0 24 24"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>
  </div>
  <h2>GitHub App Installed!</h2>
  <p>Your GitHub App has been successfully connected to this Citizen instance.</p>
  <p class="closing">This window will close automatically...</p>
</div>
<script>
  // Notify parent window and close
  setTimeout(function() {
    if (window.opener) {
      window.opener.postMessage({ 
        type: 'github-app-install-success',
        installation_id: ` + fmt.Sprintf("%d", installationID) + `
      }, '*');
      window.close();
    }
  }, 1500);
  
  // Fallback: show close button after 3 seconds
  setTimeout(function() {
    if (!window.closed) {
      document.querySelector('.closing').innerHTML = 
        '<button onclick="window.close()" style="background:#0366d6;color:white;border:none;padding:10px 20px;border-radius:6px;cursor:pointer;">Close Window</button>';
    }
  }, 3000);
</script>
</body>
</html>`

	return c.Type("html").SendString(html)
}
