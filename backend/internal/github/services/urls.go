package services

import (
	"backend/pkg/errors"
	"fmt"
	"strings"
)

// NormalizeBaseURL normalizes a base URL to use HTTPS for non-local environments
func NormalizeBaseURL(baseURL string) string {
	if strings.HasPrefix(baseURL, "http://") &&
		!strings.Contains(baseURL, "localhost") &&
		!strings.Contains(baseURL, "127.0.0.1") {
		return strings.Replace(baseURL, "http://", "https://", 1)
	}
	return baseURL
}

// GetWebhookURL returns the GitHub webhook URL for the given base URL
func GetWebhookURL(baseURL string) string {
	return fmt.Sprintf("%s/api/v1/github/webhook", NormalizeBaseURL(baseURL))
}

// GetRedirectURI returns the GitHub OAuth callback URL for the given base URL
func GetRedirectURI(baseURL string) string {
	return fmt.Sprintf("%s/api/v1/github/auth/callback", NormalizeBaseURL(baseURL))
}

// GetAppInstallCallbackURL returns the GitHub App install callback URL
func GetAppInstallCallbackURL(baseURL string) string {
	return fmt.Sprintf("%s/api/v1/github/app/install/callback", NormalizeBaseURL(baseURL))
}

// ParseRepositoryFullName parses a repository full name (owner/repo) into owner and repo parts
func ParseRepositoryFullName(fullName string) (owner, repo string, err error) {
	parts := strings.Split(fullName, "/")
	if len(parts) != 2 {
		return "", "", errors.BadRequestf("invalid repository full name format (should be owner/repo): %s", fullName)
	}
	return parts[0], parts[1], nil
}
