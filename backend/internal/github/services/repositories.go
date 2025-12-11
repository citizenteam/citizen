package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"backend/internal/github/models"
	"backend/pkg/errors"
)

// GetUserRepositories gets user's repositories with push access
func GetUserRepositories(accessToken string, page int) ([]models.Repository, error) {
	url := fmt.Sprintf("https://api.github.com/user/repos?sort=updated&per_page=100&page=%d", page)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var repos []models.Repository
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, err
	}

	// Filter repos where user has push access
	var filteredRepos []models.Repository
	for _, repo := range repos {
		if repo.Permissions.Push {
			filteredRepos = append(filteredRepos, repo)
		}
	}

	return filteredRepos, nil
}

// GetRepositoryInfo gets detailed repository information
func GetRepositoryInfo(accessToken, owner, repo string) (*models.Repository, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.NotFound("repository")
	}

	var repository models.Repository
	if err := json.Unmarshal(body, &repository); err != nil {
		return nil, err
	}

	return &repository, nil
}

// GetInstallationRepositories lists repositories accessible to installation
func GetInstallationRepositories(page int) ([]models.Repository, error) {
	tokenResp, err := GetGitHubInstallationToken()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.github.com/installation/repositories?per_page=100&page=%d", page)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.InternalErrorf("failed to list repositories: %s", string(body))
	}

	var parsed struct {
		TotalCount   int                 `json:"total_count"`
		Repositories []models.Repository `json:"repositories"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.Repositories, nil
}
