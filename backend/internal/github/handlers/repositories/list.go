package repositories

import (
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/pkg/logger"
	"backend/pkg/response"

	"github.com/gofiber/fiber/v2"
)

var logList = logger.Default().WithComponent("github-repos")

// ListGitHubRepositories lists user's GitHub repositories
func ListGitHubRepositories(c *fiber.Ctx) error {
	// Get current user from context
	userID := c.Locals("user_id")
	if userID == nil {
		return response.Unauthorized(c, "User not authenticated")
	}

	page := c.QueryInt("page", 1)

	// If GitHub App (manifest) is installed, prefer installation token
	appRepos, appErr := githubservices.GetInstallationRepositories(page)
	if appErr == nil {
		return response.SuccessWithMessage(c, "Repositories fetched successfully", fiber.Map{
			"repositories": appRepos,
			"page":         page,
			"total":        len(appRepos),
			"github_app":   true,
		})
	}

	// Get user's GitHub access token from database
	accessToken, err := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))

	if err != nil {
		logList.WithField("error", err.Error()).Warn("Failed to get user GitHub access token")
		return response.Unauthorized(c, "GitHub not connected or access token not found")
	}

	if accessToken == "" {
		return response.Unauthorized(c, "GitHub access token is empty")
	}

	repos, err := githubservices.GetUserRepositories(accessToken, page)
	if err != nil {
		logList.WithField("error", err.Error()).Warn("Failed to get repositories")
		return response.InternalServerError(c, "Failed to fetch repositories")
	}

	return response.SuccessWithMessage(c, "Repositories fetched successfully", fiber.Map{
		"repositories": repos,
		"page":         page,
		"total":        len(repos),
	})
}

// GetRepositoryBranches lists branches for a specific repository
func GetRepositoryBranches(c *fiber.Ctx) error {
	// Get repo full name from params (owner/repo format)
	owner := c.Params("owner")
	repo := c.Params("repo")

	if owner == "" || repo == "" {
		return response.BadRequest(c, "Repository owner and name are required")
	}

	fullName := owner + "/" + repo
	logList.WithField("repository", fullName).Debug("GetRepositoryBranches called")

	// Try GitHub App installation token first
	branches, appErr := githubservices.GetRepositoryBranchesWithApp(fullName)
	if appErr == nil && len(branches) > 0 {
		return response.SuccessWithMessage(c, "Branches fetched", fiber.Map{
			"branches":   branches,
			"total":      len(branches),
			"github_app": true,
		})
	}

	// Fallback to user OAuth token
	userID := c.Locals("user_id")
	if userID == nil {
		return response.Unauthorized(c, "User not authenticated")
	}

	accessToken, err := api.GitHub.GetUserGitHubAccessToken(c.Context(), userID.(int))
	if err != nil || accessToken == "" {
		logList.Debug("No access token, returning default branches")
		return response.SuccessWithMessage(c, "Default branches (no GitHub access)", fiber.Map{
			"branches": []string{"main", "master", "develop"},
			"total":    3,
			"default":  true,
		})
	}

	branches, err = githubservices.GetRepositoryBranchesWithToken(fullName, accessToken)
	if err != nil {
		logList.WithField("error", err.Error()).Warn("Failed to get branches")
		return response.SuccessWithMessage(c, "Default branches (fetch failed)", fiber.Map{
			"branches": []string{"main", "master", "develop"},
			"total":    3,
			"default":  true,
		})
	}

	return response.SuccessWithMessage(c, "Branches fetched", fiber.Map{
		"branches": branches,
		"total":    len(branches),
	})
}
