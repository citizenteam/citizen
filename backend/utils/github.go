package utils

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// GitHub OAuth configuration - stored in memory after first setup
var (
	gitHubClientID     string
	gitHubClientSecret string
	gitHubRedirectURI  string
	gitHubWebhookSecret string
	gitHubConfigMutex  sync.RWMutex
	gitHubConfigured   bool
)

// SetupGitHubOAuth sets up GitHub OAuth configuration in memory
func SetupGitHubOAuth(clientID, clientSecret, redirectURI, webhookSecret string) error {
	gitHubConfigMutex.Lock()
	defer gitHubConfigMutex.Unlock()
	
	fmt.Printf("[SETUP] SetupGitHubOAuth called with ClientID: %s\n", clientID)
	
	// Set memory variables
	gitHubClientID = clientID
	gitHubClientSecret = clientSecret
	gitHubRedirectURI = redirectURI
	gitHubWebhookSecret = webhookSecret
	gitHubConfigured = true
	
	fmt.Printf("[SETUP] Set memory variables - gitHubConfigured: %t, webhookSecret: %s\n", 
		gitHubConfigured, gitHubWebhookSecret)
	
	return nil
}

// IsGitHubConfigured checks if GitHub OAuth is configured
func IsGitHubConfigured() bool {
	gitHubConfigMutex.RLock()
	defer gitHubConfigMutex.RUnlock()
	
	// Check memory first
	if gitHubConfigured {
		return true
	}
	
	// Check environment variables as fallback
	return os.Getenv("GITHUB_CLIENT_ID") != "" && 
		   os.Getenv("GITHUB_CLIENT_SECRET") != "" &&
		   os.Getenv("GITHUB_REDIRECT_URI") != ""
}

// GetGitHubConfig gets current GitHub configuration
func GetGitHubConfig() (clientID, clientSecret, redirectURI, webhookSecret string) {
	gitHubConfigMutex.RLock()
	defer gitHubConfigMutex.RUnlock()
	
	fmt.Printf("[CONFIG] GetGitHubConfig called - gitHubConfigured: %t\n", gitHubConfigured)
	
	// Try memory first
	if gitHubConfigured {
		fmt.Printf("[CONFIG] Using memory config - ClientID: %s, WebhookSecret: %s\n", 
			gitHubClientID, gitHubWebhookSecret)
		return gitHubClientID, gitHubClientSecret, gitHubRedirectURI, gitHubWebhookSecret
	}
	
	// Fallback to environment variables
	clientID = os.Getenv("GITHUB_CLIENT_ID")
	clientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
	redirectURI = os.Getenv("GITHUB_REDIRECT_URI")
	webhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")
	
	fmt.Printf("[CONFIG] Using env vars - ClientID: %s, WebhookSecret: %s\n", 
		clientID, webhookSecret)
	
	// Update memory if found in env
	if clientID != "" && clientSecret != "" && redirectURI != "" {
		gitHubClientID = clientID
		gitHubClientSecret = clientSecret
		gitHubRedirectURI = redirectURI
		gitHubWebhookSecret = webhookSecret
		gitHubConfigured = true
		fmt.Printf("[CONFIG] Updated memory config from env vars\n")
	}
	
	return
}

// generateSecureSecret generates a cryptographically secure secret
func generateSecureSecret() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GitHub config loading functions are now in handlers/github.go to avoid import cycle

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

// GetGitHubOAuthURL returns the GitHub OAuth authorization URL
func GetGitHubOAuthURL(state string) (string, error) {
	clientID, _, redirectURI, _ := GetGitHubConfig()
	if clientID == "" || redirectURI == "" {
		return "", fmt.Errorf("github oauth not configured")
	}
	
	baseURL := "https://github.com/login/oauth/authorize"
	params := url.Values{}
	params.Add("client_id", clientID)
	params.Add("redirect_uri", redirectURI)
	params.Add("scope", "repo,read:user,user:email")
	params.Add("state", state)
	
	return fmt.Sprintf("%s?%s", baseURL, params.Encode()), nil
}

// ExchangeCodeForToken exchanges OAuth code for access token
func ExchangeCodeForToken(code string) (*GitHubOAuthResponse, error) {
	clientID, clientSecret, _, _ := GetGitHubConfig()
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("github oauth not configured")
	}
	
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)
	
	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	
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
	
	var tokenResp GitHubOAuthResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	
	return &tokenResp, nil
}

// GetGitHubUser gets GitHub user information
func GetGitHubUser(accessToken string) (*GitHubUser, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "token "+accessToken)
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
	
	var user GitHubUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, err
	}
	
	return &user, nil
}

// GetUserRepositories gets user's repositories with push access
func GetUserRepositories(accessToken string, page int) ([]GitHubRepository, error) {
	url := fmt.Sprintf("https://api.github.com/user/repos?sort=updated&per_page=100&page=%d", page)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "token "+accessToken)
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
	
	var repos []GitHubRepository
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, err
	}
	
	// Filter repos where user has push access
	var filteredRepos []GitHubRepository
	for _, repo := range repos {
		if repo.Permissions.Push {
			filteredRepos = append(filteredRepos, repo)
		}
	}
	
	return filteredRepos, nil
}

// CreateWebhook creates a GitHub webhook for repository
func CreateWebhook(accessToken, owner, repo, webhookURL string) (*GitHubWebhook, error) {
	clientID, clientSecret, redirectURI, webhookSecret := GetGitHubConfig()
	
	// Debug log
	fmt.Printf("[WEBHOOK] Debug - ClientID: %s, ClientSecret: %s, RedirectURI: %s, WebhookSecret: %s\n", 
		clientID, clientSecret, redirectURI, webhookSecret)
	
	if webhookSecret == "" {
		// If webhook secret is empty, generate one and save it
		fmt.Printf("[WEBHOOK] Webhook secret is empty, generating new one...\n")
		webhookSecret = generateSecureSecret()
		
		// Update the configuration
		if clientID != "" && clientSecret != "" && redirectURI != "" {
			err := SetupGitHubOAuth(clientID, clientSecret, redirectURI, webhookSecret)
			if err != nil {
				return nil, fmt.Errorf("failed to update GitHub config with webhook secret: %v", err)
			}
			fmt.Printf("[WEBHOOK] Generated and saved new webhook secret\n")
		} else {
			return nil, fmt.Errorf("github oauth not fully configured")
		}
	}
	
	webhook := map[string]interface{}{
		"name":   "web",
		"active": true,
		"events": []string{"push", "pull_request"},
		"config": map[string]interface{}{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       webhookSecret,
			"insecure_ssl": "0",
		},
	}
	
	jsonData, err := json.Marshal(webhook)
	if err != nil {
		return nil, err
	}
	
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/hooks", owner, repo)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
	
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
	
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create webhook: %s", string(body))
	}
	
	var createdWebhook GitHubWebhook
	if err := json.Unmarshal(body, &createdWebhook); err != nil {
		return nil, err
	}
	
	return &createdWebhook, nil
}

// DeleteWebhook deletes a GitHub webhook
func DeleteWebhook(accessToken, owner, repo string, webhookID int64) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/hooks/%d", owner, repo, webhookID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete webhook: %s", string(body))
	}
	
	return nil
}

// GetRepositoryInfo gets detailed repository information
func GetRepositoryInfo(accessToken, owner, repo string) (*GitHubRepository, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "token "+accessToken)
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
		return nil, fmt.Errorf("repository not found: %s", string(body))
	}
	
	var repository GitHubRepository
	if err := json.Unmarshal(body, &repository); err != nil {
		return nil, err
	}
	
	return &repository, nil
}

// ValidateGitHubSignature validates GitHub webhook signature
func ValidateGitHubSignature(payload []byte, signature string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	
	_, _, _, webhookSecret := GetGitHubConfig()
	if webhookSecret == "" {
		return false
	}
	
	expectedSignature := "sha256=" + generateHMACSignature(payload, webhookSecret)
	return signature == expectedSignature
}

// generateHMACSignature generates HMAC SHA256 signature
func generateHMACSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
} 