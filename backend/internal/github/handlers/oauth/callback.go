package oauth

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/internal/utils"
	"log"

	"github.com/gofiber/fiber/v2"
)

// GitHubAuthCallback handles GitHub OAuth callback
// This is a public endpoint - user validation is done via state parameter
func GitHubAuthCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Authorization code is required",
			nil,
		))
	}

	// CSRF Protection: Validate state parameter
	stateService := githubservices.GetStateService()
	userID, err := stateService.ValidateOAuthState(state, 10*60) // 10 minutes max age
	if err != nil {
		log.Printf("[GITHUB] CSRF Protection failed: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			err.Error(),
			nil,
		))
	}

	log.Printf("[GITHUB] ✅ CSRF Protection validated successfully for user %v", userID)

	// Exchange code for access token
	tokenResp, err := githubservices.ExchangeCodeForToken(code)
	if err != nil {
		log.Printf("[GITHUB] Failed to exchange code for token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to exchange code for token",
			nil,
		))
	}

	// Get GitHub user info
	githubUser, err := githubservices.GetGitHubUser(tokenResp.AccessToken)
	if err != nil {
		log.Printf("[GITHUB] Failed to get GitHub user: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to get GitHub user information",
			nil,
		))
	}

	// Update user in database with GitHub info
	err = api.GitHub.UpdateGitHubInfo(c.Context(), userID, int64(githubUser.ID), githubUser.Login, tokenResp.AccessToken)

	if err != nil {
		log.Printf("[GITHUB] Failed to update user with GitHub info: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to save GitHub connection",
			nil,
		))
	}

	log.Printf("[GITHUB] ✅ GitHub user connected: %s (ID: %d)", githubUser.Login, githubUser.ID)

	// Check if this is a popup (has state parameter) - return HTML to close popup
	if state != "" {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(`<!DOCTYPE html>
<html>
<head>
	<title>GitHub Connected</title>
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; background: #f9fafb; }
		.container { text-align: center; padding: 2rem; }
		.success { color: #16a34a; font-size: 3rem; margin-bottom: 1rem; }
		h1 { color: #111827; font-size: 1.5rem; margin-bottom: 0.5rem; }
		p { color: #6b7280; }
	</style>
</head>
<body>
	<div class="container">
		<div class="success">✓</div>
		<h1>GitHub Bağlandı!</h1>
		<p>Bu pencere kapanıyor...</p>
	</div>
	<script>
		if (window.opener) {
			window.opener.postMessage({ type: 'github-oauth-success' }, '*');
		}
		setTimeout(function() { window.close(); }, 1500);
	</script>
</body>
</html>`)
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub account connected successfully",
		fiber.Map{
			"github_user":      githubUser,
			"github_connected": true,
		},
	))
}

