package api

import (
	"context"
	"fmt"
	"time"
)

// UpdateGitHubInfo updates user's GitHub information
func (g *GitHubAPI) UpdateGitHubInfo(ctx context.Context, userID int, githubID int64, githubUsername, accessToken string) error {
	if err := ValidateArgs(userID, githubID, githubUsername, accessToken); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		UPDATE users SET 
			github_connected = $1,
			github_id = $2,
			github_username = $3,
			github_access_token = $4,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $5`

	_, err := Exec(ctx, query, true, githubID, githubUsername, accessToken, userID)
	if err != nil {
		return fmt.Errorf("failed to update GitHub info: %w", err)
	}

	return nil
}

// GetUserGitHubAccessToken retrieves user's GitHub access token
func (g *GitHubAPI) GetUserGitHubAccessToken(ctx context.Context, userID int) (string, error) {
	if err := ValidateArgs(userID); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	query := `SELECT github_access_token FROM users WHERE id = $1 AND github_connected = true`
	
	var accessToken string
	err := QueryRow(ctx, query, userID).Scan(&accessToken)
	if err != nil {
		return "", fmt.Errorf("failed to get GitHub access token: %w", err)
	}

	return accessToken, nil
}

// ConnectGitHubRepository connects a GitHub repository to an app
func (g *GitHubAPI) ConnectGitHubRepository(ctx context.Context, userID int, appName string, repositoryID int64, fullName, name, owner, cloneURL, htmlURL string, private bool, defaultBranch string, autoDeployEnabled bool, deployBranch string, webhookID *int64) error {
	if err := ValidateArgs(userID, appName, repositoryID, fullName); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		INSERT INTO github_repositories 
		(user_id, app_name, github_id, full_name, name, owner, clone_url, html_url, private, default_branch, auto_deploy_enabled, deploy_branch, webhook_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, CURRENT_TIMESTAMP)
		ON CONFLICT (app_name) DO UPDATE SET
			github_id = EXCLUDED.github_id,
			full_name = EXCLUDED.full_name,
			name = EXCLUDED.name,
			owner = EXCLUDED.owner,
			clone_url = EXCLUDED.clone_url,
			html_url = EXCLUDED.html_url,
			private = EXCLUDED.private,
			default_branch = EXCLUDED.default_branch,
			auto_deploy_enabled = EXCLUDED.auto_deploy_enabled,
			deploy_branch = EXCLUDED.deploy_branch,
			webhook_id = EXCLUDED.webhook_id,
			updated_at = CURRENT_TIMESTAMP`

	_, err := Exec(ctx, query, userID, appName, repositoryID, fullName, name, owner, cloneURL, htmlURL, private, defaultBranch, autoDeployEnabled, deployBranch, webhookID)
	if err != nil {
		return fmt.Errorf("failed to connect GitHub repository: %w", err)
	}

	return nil
}

// GitHubRepositoryConnection represents a repository connection
type GitHubRepositoryConnection struct {
	UserID    int
	WebhookID *int64
	FullName  string
}

// GetGitHubRepositoryConnection retrieves a repository connection by user and app
func (g *GitHubAPI) GetGitHubRepositoryConnection(ctx context.Context, userID int, appName string) (*GitHubRepositoryConnection, error) {
	if err := ValidateArgs(userID, appName); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT user_id, webhook_id, full_name FROM github_repositories gr
		JOIN users u ON gr.user_id = u.id
		WHERE gr.app_name = $1 AND gr.user_id = $2 AND gr.deleted_at IS NULL`

	var userIDResult int
	var webhookID *int64
	var fullName string
	
	err := QueryRow(ctx, query, appName, userID).Scan(&userIDResult, &webhookID, &fullName)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository connection: %w", err)
	}

	return &GitHubRepositoryConnection{
		UserID:    userIDResult,
		WebhookID: webhookID,
		FullName:  fullName,
	}, nil
}

// GetGitHubRepositoryConnectionByAppName retrieves a repository connection by app name only (for webhooks)
func (g *GitHubAPI) GetGitHubRepositoryConnectionByAppName(ctx context.Context, appName string) (*GitHubRepositoryConnection, error) {
	if err := ValidateArgs(appName); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT user_id, webhook_id, full_name FROM github_repositories
		WHERE app_name = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT 1`

	var userID int
	var webhookID *int64
	var fullName string
	
	err := QueryRow(ctx, query, appName).Scan(&userID, &webhookID, &fullName)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository connection: %w", err)
	}

	return &GitHubRepositoryConnection{
		UserID:    userID,
		WebhookID: webhookID,
		FullName:  fullName,
	}, nil
}

// DisconnectGitHubRepository soft deletes a repository connection
func (g *GitHubAPI) DisconnectGitHubRepository(ctx context.Context, userID int, appName string) error {
	if err := ValidateArgs(userID, appName); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		UPDATE github_repositories 
		SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE app_name = $1 AND user_id = $2`

	_, err := Exec(ctx, query, appName, userID)
	if err != nil {
		return fmt.Errorf("failed to disconnect repository: %w", err)
	}

	return nil
}

// GitHubRepository represents a GitHub repository with deployment info
type GitHubRepository struct {
	AppName           string
	AutoDeployEnabled bool
	DeployBranch      string
}

// GetGitHubRepositoryByID retrieves a repository by GitHub ID
func (g *GitHubAPI) GetGitHubRepositoryByID(ctx context.Context, githubID int64) (*GitHubRepository, error) {
	if err := ValidateArgs(githubID); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT app_name, auto_deploy_enabled, deploy_branch 
		FROM github_repositories 
		WHERE github_id = $1 AND deleted_at IS NULL`

	var appName, deployBranch string
	var autoDeployEnabled bool
	
	err := QueryRow(ctx, query, githubID).Scan(&appName, &autoDeployEnabled, &deployBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	return &GitHubRepository{
		AppName:           appName,
		AutoDeployEnabled: autoDeployEnabled,
		DeployBranch:      deployBranch,
	}, nil
}

// GetGitHubRepositoryConnections retrieves all repository connections for a user
func (g *GitHubAPI) GetGitHubRepositoryConnections(ctx context.Context, userID int) ([]map[string]interface{}, error) {
	if err := ValidateArgs(userID); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT app_name, github_id, full_name, name, owner, clone_url, html_url, private, 
		       default_branch, auto_deploy_enabled, deploy_branch, webhook_id, 
		       connected_at, last_deploy, created_at 
		FROM github_repositories 
		WHERE user_id = $1 AND deleted_at IS NULL`

	rows, err := Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query repository connections: %w", err)
	}
	defer rows.Close()

	var connections []map[string]interface{}
	for rows.Next() {
		var appName, fullName, name, owner, cloneURL, htmlURL, defaultBranch, deployBranch string
		var githubID *int64
		var private, autoDeploy bool
		var webhookID *int64
		var connectedAt, lastDeploy, createdAt interface{}

		err := rows.Scan(&appName, &githubID, &fullName, &name, &owner, &cloneURL, &htmlURL, &private, 
			&defaultBranch, &autoDeploy, &deployBranch, &webhookID, &connectedAt, &lastDeploy, &createdAt)
		if err != nil {
			continue
		}

		connections = append(connections, map[string]interface{}{
			"app_name":        appName,
			"github_id":       githubID,
			"full_name":       fullName,
			"name":           name,
			"owner":          owner,
			"clone_url":       cloneURL,
			"html_url":        htmlURL,
			"private":        private,
			"default_branch":  defaultBranch,
			"auto_deploy":     autoDeploy,
			"deploy_branch":   deployBranch,
			"webhook_id":      webhookID,
			"connected_at":    connectedAt,
			"last_deploy":     lastDeploy,
			"created_at":      createdAt,
		})
	}

	return connections, nil
}

// GitHubConfig represents GitHub OAuth configuration
type GitHubConfig struct {
	ClientID      string
	ClientSecret  string
	WebhookSecret string
	RedirectURI   string
	CreatedAt     time.Time
}

// GetGitHubConfig retrieves GitHub config (without secrets)
func (g *GitHubAPI) GetGitHubConfig(ctx context.Context) (*GitHubConfig, error) {
	query := `
		SELECT client_id, redirect_uri, created_at
		FROM github_config
		WHERE is_active = true
		ORDER BY updated_at DESC
		LIMIT 1`

	var clientID, redirectURI string
	var createdAt time.Time

	err := QueryRow(ctx, query).Scan(&clientID, &redirectURI, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub config: %w", err)
	}

	return &GitHubConfig{
		ClientID:    clientID,
		RedirectURI: redirectURI,
		CreatedAt:   createdAt,
	}, nil
}

// GetGitHubConfigFull retrieves full GitHub config (with secrets)
func (g *GitHubAPI) GetGitHubConfigFull(ctx context.Context) (*GitHubConfig, error) {
	query := `
		SELECT client_id, client_secret, webhook_secret, redirect_uri
		FROM github_config
		WHERE is_active = true
		ORDER BY updated_at DESC
		LIMIT 1`

	var clientID, clientSecret, webhookSecret, redirectURI string

	err := QueryRow(ctx, query).Scan(&clientID, &clientSecret, &webhookSecret, &redirectURI)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub config: %w", err)
	}

	return &GitHubConfig{
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		WebhookSecret: webhookSecret,
		RedirectURI:   redirectURI,
	}, nil
}

// SaveGitHubConfig saves GitHub configuration to database
func (g *GitHubAPI) SaveGitHubConfig(ctx context.Context, clientID, clientSecret, webhookSecret, redirectURI string) error {
	if err := ValidateArgs(clientID, clientSecret, webhookSecret, redirectURI); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		WITH deactivated AS (
			UPDATE github_config SET is_active = false WHERE is_active = true
		)
		INSERT INTO github_config (client_id, client_secret, webhook_secret, redirect_uri, is_active)
		VALUES ($1, $2, $3, $4, true)`

	_, err := Exec(ctx, query, clientID, clientSecret, webhookSecret, redirectURI)
	if err != nil {
		return fmt.Errorf("failed to save GitHub config: %w", err)
	}

	return nil
}

// DeleteGitHubConfig soft deletes GitHub configuration
func (g *GitHubAPI) DeleteGitHubConfig(ctx context.Context) error {
	query := `
		UPDATE github_config
		SET is_active = false, updated_at = CURRENT_TIMESTAMP
		WHERE is_active = true`

	_, err := Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to delete GitHub config: %w", err)
	}

	return nil
}

// GetGitHubRepositoryDeployBranch retrieves deploy branch for an app
func (g *GitHubAPI) GetGitHubRepositoryDeployBranch(ctx context.Context, appName string) (string, error) {
	if err := ValidateArgs(appName); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT deploy_branch FROM github_repositories 
		WHERE app_name = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT 1`

	var deployBranch string
	err := QueryRow(ctx, query, appName).Scan(&deployBranch)
	if err != nil {
		return "", fmt.Errorf("failed to get deploy branch: %w", err)
	}

	return deployBranch, nil
} 