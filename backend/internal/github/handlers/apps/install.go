package apps

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/pkg/logger"
	"backend/pkg/response"
	"fmt"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
)

var logInstall = logger.Default().WithComponent("github-apps")

// StartGitHubInstall generates an install URL for existing app
func StartGitHubInstall(c *fiber.Ctx) error {
	appID, appSlug, _, _, _ := githubservices.GetGitHubAppConfig()

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
		return response.BadRequest(c, "GitHub App not configured yet")
	}
	stateService := githubservices.GetStateService()
	state := stateService.GenerateInstallState()
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s&redirect_url=%s",
		url.QueryEscape(*appSlug), url.QueryEscape(state), url.QueryEscape(githubservices.GetAppInstallCallbackURL(c.BaseURL())))

	return response.SuccessWithMessage(c, "GitHub App install URL generated", fiber.Map{
		"install_url": installURL,
		"state":       state,
	})
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
	logInstall.WithFields(map[string]interface{}{
		"installation_id": installationID,
		"state":           statePreview,
		"setup_action":    setupAction,
	}).Info("Install callback received")

	// State validation: either valid state from our store, or GitHub's setup_url redirect (has setup_action)
	// GitHub's setup_url redirect doesn't preserve our state, so we accept requests with setup_action
	stateService := githubservices.GetStateService()
	if state != "" && !stateService.ValidateInstallState(state, 30*time.Minute) {
		logInstall.Warn("Invalid state provided in install callback")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid state - please try again")
	}

	// If no state and no setup_action, this is an invalid request
	if state == "" && setupAction == "" {
		logInstall.Warn("Missing both state and setup_action in install callback - rejecting")
		return c.Status(fiber.StatusBadRequest).SendString("Invalid request - missing state or setup_action")
	}

	if installationID == 0 {
		logInstall.Warn("Missing installation_id in callback")
		return c.Status(fiber.StatusBadRequest).SendString("Missing installation_id")
	}

	if err := api.GitHub.UpdateGitHubInstallationID(c.Context(), int64(installationID)); err != nil {
		logInstall.WithField("error", err.Error()).Error("Failed to store installation ID")
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save installation")
	}

	// Update in-memory config with latest installation
	appID, appSlug, appName, _, _ := githubservices.GetGitHubAppConfig()
	if appID != nil && githubservices.HasPrivateKey() {
		inst := int64(installationID)
		// Pass nil for privateKey - it's stored in DB, not in memory
		githubservices.SetupGitHubApp(*appID, appSlug, nil, &inst, appName)
		logInstall.WithFields(map[string]interface{}{
			"app_id":          *appID,
			"installation_id": installationID,
		}).Info("GitHub App installed successfully")
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
  (function() {
    var targetOrigin = '` + c.BaseURL() + `';
    setTimeout(function() {
      if (window.opener) {
        try {
          window.opener.postMessage({ 
            type: 'github-app-install-success',
            installation_id: ` + fmt.Sprintf("%d", installationID) + `
          }, targetOrigin);
        } catch(e) { console.error('postMessage failed:', e); }
        window.close();
      }
    }, 1500);
  })();
  
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
