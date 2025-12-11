package services

import (
	"backend/pkg/errors"
)

// GetAccessToken retrieves a GitHub access token using GitHub App installation token.
func GetAccessToken() (string, error) {
	tokenResp, err := GetGitHubInstallationToken()
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeUnauthorized, "failed to get GitHub installation token")
	}
	if tokenResp == nil || tokenResp.Token == "" {
		return "", errors.Unauthorized("GitHub App not configured")
	}
	return tokenResp.Token, nil
}

// GetAccessTokenOptional retrieves a GitHub access token without returning error.
// Returns empty string if no token is available.
func GetAccessTokenOptional() string {
	tokenResp, err := GetGitHubInstallationToken()
	if err != nil || tokenResp == nil {
		return ""
	}
	return tokenResp.Token
}
