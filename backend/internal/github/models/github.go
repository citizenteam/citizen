package models

import "time"

// GitHubOAuthResponse represents GitHub OAuth access token response
type GitHubOAuthResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// GitHubUser represents GitHub user information
type GitHubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// GitHubRepository represents GitHub repository information
type GitHubRepository struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Description   string `json:"description"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
	Permissions struct {
		Admin bool `json:"admin"`
		Push  bool `json:"push"`
		Pull  bool `json:"pull"`
	} `json:"permissions"`
}

// GitHubWebhook represents GitHub webhook information
type GitHubWebhook struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
	Config struct {
		URL         string `json:"url"`
		ContentType string `json:"content_type"`
		Secret      string `json:"secret,omitempty"`
	} `json:"config"`
	Events []string `json:"events"`
}

// GitHubInstallationTokenResponse represents installation token response
type GitHubInstallationTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AppInstallation represents a GitHub App installation
type AppInstallation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
		Type  string `json:"type"`
	} `json:"account"`
	AppID               int64  `json:"app_id"`
	TargetType          string `json:"target_type"`
	RepositorySelection string `json:"repository_selection"`
}

// GitHubAppInfo represents basic GitHub App information
type GitHubAppInfo struct {
	ID    int64  `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// GitHubAppURLUpdate represents the URLs and webhook config to update on a GitHub App
type GitHubAppURLUpdate struct {
	HomepageURL   string   `json:"homepage_url,omitempty"`
	WebhookURL    string   `json:"webhook_url,omitempty"`
	WebhookSecret string   `json:"webhook_secret,omitempty"` // New webhook secret to set
	CallbackURLs  []string `json:"callback_urls,omitempty"`
	SetupURL      string   `json:"setup_url,omitempty"`
	SetupOnUpdate bool     `json:"setup_on_update,omitempty"`
}

// GitHubBranch represents a GitHub branch
type GitHubBranch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}
