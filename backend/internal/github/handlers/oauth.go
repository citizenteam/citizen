package handlers

import (
	"backend/internal/database/api"
	"backend/internal/utils"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GitHubAuthInit initiates GitHub OAuth flow
func GitHubAuthInit(c *fiber.Ctx) error {
	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	// Check if GitHub OAuth is configured
	if !utils.IsGitHubConfigured() {
		// Don't set up placeholder values, just return setup required
		baseURL := c.BaseURL()
		redirectURI := fmt.Sprintf("%s/api/v1/github/auth/callback", baseURL)

		log.Printf("[GITHUB] GitHub OAuth not configured, showing setup instructions")

		return c.JSON(utils.NewCitizenResponse(
			false,
			"GitHub OAuth needs to be configured. Please set up your GitHub App first.",
			fiber.Map{
				"setup_required": true,
				"redirect_uri":   redirectURI,
				"instructions":   "Create a GitHub App with this redirect URI, then provide the Client ID and Secret",
			},
		))
	}

	// Generate state for CSRF protection with crypto-secure random component
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		log.Printf("[GITHUB] Failed to generate secure random bytes: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to generate secure state parameter",
			nil,
		))
	}
	randomComponent := hex.EncodeToString(randomBytes)
	state := fmt.Sprintf("user_%v_%d_%s", userID, time.Now().Unix(), randomComponent)

	// Generate OAuth URL
	authURL, err := utils.GetGitHubOAuthURL(state)
	if err != nil {
		log.Printf("[GITHUB] Failed to generate OAuth URL: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to generate GitHub OAuth URL",
			nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"GitHub OAuth URL generated",
		fiber.Map{
			"auth_url": authURL,
			"state":    state,
		},
	))
}

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
	if state == "" {
		log.Printf("[GITHUB] CSRF Protection: Missing state parameter")
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}

	// Validate state format: "user_{userID}_{timestamp}_{randomComponent}"
	if !strings.HasPrefix(state, "user_") {
		log.Printf("[GITHUB] CSRF Protection: Invalid state format, state: %s", state)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}

	// Extract and validate timestamp (prevent replay attacks)
	parts := strings.Split(state, "_")
	if len(parts) != 4 {
		log.Printf("[GITHUB] CSRF Protection: Invalid state parts count, expected 4, got %d, state: %s", len(parts), state)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}

	// Extract userID from state
	stateUserIDStr := parts[1]
	userID, err := strconv.Atoi(stateUserIDStr)
	if err != nil {
		log.Printf("[GITHUB] CSRF Protection: Invalid userID in state: %s", stateUserIDStr)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}

	timestampStr := parts[2]
	randomComponent := parts[3]

	// Validate random component format (should be 32 hex chars)
	if len(randomComponent) != 32 {
		log.Printf("[GITHUB] CSRF Protection: Invalid random component length for user %v, expected 32, got %d", userID, len(randomComponent))
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}

	// Validate that random component is hex
	for _, char := range randomComponent {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			log.Printf("[GITHUB] CSRF Protection: Invalid random component format for user %v, not hex: %s", userID, randomComponent)
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"Invalid state parameter - CSRF protection failed",
				nil,
			))
		}
	}
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		log.Printf("[GITHUB] CSRF Protection: Invalid timestamp in state for user %v, state: %s", userID, state)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid state parameter - CSRF protection failed",
			nil,
		))
	}

	// Check if state is not too old (10 minutes max)
	maxAge := int64(10 * 60) // 10 minutes in seconds
	currentTime := time.Now().Unix()
	if currentTime-timestamp > maxAge {
		log.Printf("[GITHUB] CSRF Protection: Expired state for user %v, age: %d seconds", userID, currentTime-timestamp)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"State parameter expired - please try again",
			nil,
		))
	}

	log.Printf("[GITHUB] ✅ CSRF Protection validated successfully for user %v, state: %s", userID, state)

	// Exchange code for access token
	tokenResp, err := utils.ExchangeCodeForToken(code)
	if err != nil {
		log.Printf("[GITHUB] Failed to exchange code for token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to exchange code for token",
			nil,
		))
	}

	// Get GitHub user info
	githubUser, err := utils.GetGitHubUser(tokenResp.AccessToken)
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
