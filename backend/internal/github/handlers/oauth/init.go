package oauth

import (
	githubservices "backend/internal/github/services"
	"backend/internal/utils"
	"log"

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
	if !githubservices.IsGitHubConfigured() {
		// Don't set up placeholder values, just return setup required
		redirectURI := githubservices.GetRedirectURI(c.BaseURL())

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

	// Generate state for CSRF protection
	stateService := githubservices.GetStateService()
	state, err := stateService.GenerateOAuthState(userID.(int))
	if err != nil {
		log.Printf("[GITHUB] Failed to generate OAuth state: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to generate secure state parameter",
			nil,
		))
	}

	// Generate OAuth URL
	authURL, err := githubservices.GetGitHubOAuthURL(state)
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
