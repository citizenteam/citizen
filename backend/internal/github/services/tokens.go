package services

import (
	"context"

	"backend/internal/database/api"
	"backend/pkg/errors"
)

// GetAccessToken retrieves a GitHub access token for the given user.
// It first tries to get an installation token (GitHub App), if that fails,
// it falls back to the user's personal access token.
func GetAccessToken(ctx context.Context, userID int) (string, error) {
	// First, try to get an installation token (GitHub App)
	if tokenResp, err := GetGitHubInstallationToken(); err == nil && tokenResp != nil {
		return tokenResp.Token, nil
	}

	// Fall back to user's personal access token
	token, err := api.GitHub.GetUserGitHubAccessToken(ctx, userID)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeUnauthorized, "failed to get GitHub access token")
	}

	if token == "" {
		return "", errors.Unauthorized("GitHub access token is empty")
	}

	return token, nil
}

// GetAccessTokenOptional retrieves a GitHub access token without requiring user ID.
// Returns empty string and no error if no token is available.
func GetAccessTokenOptional(ctx context.Context, userID *int) (string, error) {
	// First, try to get an installation token (GitHub App)
	if tokenResp, err := GetGitHubInstallationToken(); err == nil && tokenResp != nil {
		return tokenResp.Token, nil
	}

	// Fall back to user's personal access token if userID is provided
	if userID != nil {
		token, err := api.GitHub.GetUserGitHubAccessToken(ctx, *userID)
		if err == nil && token != "" {
			return token, nil
		}
	}

	return "", nil
}
