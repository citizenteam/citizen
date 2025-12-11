package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"backend/internal/github/models"
	"backend/pkg/errors"
)

// GetRepositoryBranchesWithApp fetches branches using GitHub App installation token
func GetRepositoryBranchesWithApp(fullName string) ([]models.Branch, error) {
	if !IsGitHubConfigured() {
		return nil, errors.BadRequest("GitHub App not configured")
	}

	tokenResp, err := GetGitHubInstallationToken()
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to get installation token")
	}

	return fetchBranches(fullName, tokenResp.Token)
}

// GetRepositoryBranchesWithToken fetches branches using user's OAuth token
func GetRepositoryBranchesWithToken(fullName, accessToken string) ([]models.Branch, error) {
	if accessToken == "" {
		return nil, errors.BadRequest("access token is required")
	}
	return fetchBranches(fullName, accessToken)
}

// fetchBranches is the internal function that fetches branches from GitHub API
func fetchBranches(fullName, token string) ([]models.Branch, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/branches?per_page=100", fullName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "request failed")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to read response")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.InternalErrorf("GitHub API error: %s", string(body))
	}

	var branches []models.Branch
	if err := json.Unmarshal(body, &branches); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to parse branches")
	}

	return branches, nil
}
