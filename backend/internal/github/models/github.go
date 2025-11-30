package models

import "time"

// OAuthResponse represents GitHub OAuth access token response
type OAuthResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// User represents GitHub user information
type User struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// Repository represents GitHub repository information
type Repository struct {
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

// Webhook represents GitHub webhook information
type Webhook struct {
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

// InstallationTokenResponse represents installation token response
type InstallationTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Installation represents a GitHub App installation
type Installation struct {
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

// AppInfo represents basic GitHub App information
type AppInfo struct {
	ID    int64  `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// AppURLUpdate represents the URLs and webhook config to update on a GitHub App
type AppURLUpdate struct {
	HomepageURL   string   `json:"homepage_url,omitempty"`
	WebhookURL    string   `json:"webhook_url,omitempty"`
	WebhookSecret string   `json:"webhook_secret,omitempty"` // New webhook secret to set
	CallbackURLs  []string `json:"callback_urls,omitempty"`
	SetupURL      string   `json:"setup_url,omitempty"`
	SetupOnUpdate bool     `json:"setup_on_update,omitempty"`
}

// Branch represents a GitHub branch
type Branch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

// ManifestConversionResponse represents GitHub App manifest conversion response
type ManifestConversionResponse struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	WebhookSecret string `json:"webhook_secret"`
	Pem           string `json:"pem"`
	HTMLURL       string `json:"html_url"`
}

// PushEvent represents a GitHub push webhook event
type PushEvent struct {
	Ref        string `json:"ref"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadCommit struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	} `json:"head_commit"`
}

// ConfigRequest represents GitHub config setup request
// Supports either OAuth App (client_id/secret) or GitHub App (app_id + private_key + installation_id)
type ConfigRequest struct {
	ClientID        string  `json:"client_id"`
	ClientSecret    string  `json:"client_secret"`
	RedirectURI     string  `json:"redirect_uri"`
	AppID           *int64  `json:"app_id"`
	AppSlug         *string `json:"app_slug"`
	AppName         *string `json:"app_name"`
	PrivateKey      *string `json:"private_key"`
	InstallationID  *int64  `json:"installation_id"`
	WebhookSecretIn *string `json:"webhook_secret"`
}

// ConfigResponse represents GitHub config response (without secrets)
type ConfigResponse struct {
	ClientID     string `json:"client_id"`
	RedirectURI  string `json:"redirect_uri"`
	IsActive     bool   `json:"is_active"`
	ConfiguredAt string `json:"configured_at"`
}
