package repositories

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/internal/utils"
	"log"

	"github.com/gofiber/fiber/v2"
)

// ListGitHubRepositories lists user's GitHub repositories
func ListGitHubRepositories(c *fiber.Ctx) error {
	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	page := c.QueryInt("page", 1)

	// If GitHub App (manifest) is installed, prefer installation token
	appRepos, appErr := githubservices.GetInstallationRepositories(page)
	if appErr == nil {
		return c.JSON(utils.NewCitizenResponse(
			true,
			"Repositories fetched successfully",
			fiber.Map{
				"repositories": appRepos,
				"page":         page,
				"total":        len(appRepos),
				"github_app":   true,
			},
		))
	}

	// Get user's GitHub access token from database
	accessToken, err := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))

	if err != nil {
		log.Printf("[GITHUB] Failed to get user GitHub access token: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"GitHub not connected or access token not found",
			nil,
		))
	}

	if accessToken == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"GitHub access token is empty",
			nil,
		))
	}

	repos, err := githubservices.GetUserRepositories(accessToken, page)
	if err != nil {
		log.Printf("[GITHUB] Failed to get repositories: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to fetch repositories",
			nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Repositories fetched successfully",
		fiber.Map{
			"repositories": repos,
			"page":         page,
			"total":        len(repos),
		},
	))
}

// GetRepositoryBranches lists branches for a specific repository
func GetRepositoryBranches(c *fiber.Ctx) error {
	// Get repo full name from params (owner/repo format)
	owner := c.Params("owner")
	repo := c.Params("repo")

	if owner == "" || repo == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Repository owner and name are required",
			nil,
		))
	}

	fullName := owner + "/" + repo
	log.Printf("[GITHUB] GetRepositoryBranches called for: %s", fullName)

	// Try GitHub App installation token first
	branches, appErr := githubservices.GetRepositoryBranchesWithApp(fullName)
	if appErr == nil && len(branches) > 0 {
		return c.JSON(utils.NewCitizenResponse(
			true,
			"Branches fetched",
			fiber.Map{
				"branches":   branches,
				"total":      len(branches),
				"github_app": true,
			},
		))
	}

	// Fallback to user OAuth token
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not authenticated",
			nil,
		))
	}

	accessToken, err := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))
	if err != nil || accessToken == "" {
		log.Printf("[GITHUB] No access token, returning default branches")
		return c.JSON(utils.NewCitizenResponse(
			true,
			"Default branches (no GitHub access)",
			fiber.Map{
				"branches": []string{"main", "master", "develop"},
				"total":    3,
				"default":  true,
			},
		))
	}

	branches, err = githubservices.GetRepositoryBranchesWithToken(fullName, accessToken)
	if err != nil {
		log.Printf("[GITHUB] Failed to get branches: %v", err)
		return c.JSON(utils.NewCitizenResponse(
			true,
			"Default branches (fetch failed)",
			fiber.Map{
				"branches": []string{"main", "master", "develop"},
				"total":    3,
				"default":  true,
			},
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Branches fetched",
		fiber.Map{
			"branches": branches,
			"total":    len(branches),
		},
	))
}
