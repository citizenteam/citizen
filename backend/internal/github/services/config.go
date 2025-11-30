package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"

	"backend/internal/database/api"
	"backend/internal/utils"
	"backend/pkg/logger"
)

// GitHub OAuth configuration - stored in memory after first setup
var (
	gitHubClientID       string
	gitHubClientSecret   string
	gitHubRedirectURI    string
	gitHubWebhookSecret  string
	gitHubConfigMutex    sync.RWMutex
	gitHubConfigured     bool
	gitHubAppID          *int64
	gitHubAppSlug        *string
	gitHubAppName        *string
	gitHubPrivateKey     *string
	gitHubInstallationID *int64
)

// SetupGitHubOAuth sets up GitHub OAuth configuration in memory
func SetupGitHubOAuth(clientID, clientSecret, redirectURI, webhookSecret string) error {
	gitHubConfigMutex.Lock()
	defer gitHubConfigMutex.Unlock()

	log := logger.Default().WithComponent("github-config")
	log.WithField("client_id", clientID).Info("SetupGitHubOAuth called")

	// Set memory variables
	gitHubClientID = clientID
	gitHubClientSecret = clientSecret
	gitHubRedirectURI = redirectURI
	gitHubWebhookSecret = webhookSecret
	gitHubConfigured = true

	log.WithField("configured", gitHubConfigured).Info("GitHub OAuth configured")

	return nil
}

// SetupGitHubApp stores GitHub App configuration (manifest flow)
func SetupGitHubApp(appID int64, appSlug *string, privateKey *string, installationID *int64, appName *string) {
	gitHubConfigMutex.Lock()
	defer gitHubConfigMutex.Unlock()

	gitHubAppID = &appID
	gitHubAppSlug = appSlug
	gitHubAppName = appName
	gitHubPrivateKey = privateKey
	gitHubInstallationID = installationID

	log := logger.Default().WithComponent("github-config")
	log.WithFields(map[string]interface{}{
		"app_id":          appID,
		"app_slug":        appSlug,
		"installation_id": installationID,
	}).Info("GitHub App configured")
}

// IsGitHubConfigured checks if GitHub OAuth is configured
func IsGitHubConfigured() bool {
	gitHubConfigMutex.RLock()
	defer gitHubConfigMutex.RUnlock()

	// Check memory first
	if gitHubConfigured {
		return true
	}

	// If GitHub App (manifest) is set up with private key, treat as configured
	if gitHubPrivateKey != nil && gitHubAppID != nil {
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

	log := logger.Default().WithComponent("github-config")
	log.Debug("GetGitHubConfig called")

	// Try memory first
	if gitHubConfigured {
		log.WithField("source", "memory").Debug("Using memory config")
		return gitHubClientID, gitHubClientSecret, gitHubRedirectURI, gitHubWebhookSecret
	}

	// Fallback to environment variables
	clientID = os.Getenv("GITHUB_CLIENT_ID")
	clientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
	redirectURI = os.Getenv("GITHUB_REDIRECT_URI")
	webhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")

	log.WithField("source", "env").Debug("Using env vars")

	// Update memory if found in env
	if clientID != "" && clientSecret != "" && redirectURI != "" {
		gitHubClientID = clientID
		gitHubClientSecret = clientSecret
		gitHubRedirectURI = redirectURI
		gitHubWebhookSecret = webhookSecret
		gitHubConfigured = true
		log.Info("Updated memory config from env vars")
	}

	return
}

// GetGitHubAppConfig returns manifest app configuration (if set)
func GetGitHubAppConfig() (appID *int64, appSlug *string, appName *string, privateKey *string, installationID *int64) {
	gitHubConfigMutex.RLock()
	defer gitHubConfigMutex.RUnlock()
	return gitHubAppID, gitHubAppSlug, gitHubAppName, gitHubPrivateKey, gitHubInstallationID
}

// GenerateSecureSecret generates a cryptographically secure secret
func GenerateSecureSecret() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// SaveGitHubConfigToDB saves GitHub configuration to database (encrypted)
func SaveGitHubConfigToDB(clientID, clientSecret, redirectURI, webhookSecret string, appID *int64, appSlug, appName *string, privateKey *string, installationID *int64) error {
	// Encrypt sensitive data
	encryptedClientID, err := utils.EncryptString(clientID)
	if err != nil {
		return fmt.Errorf("failed to encrypt client ID: %w", err)
	}

	encryptedClientSecret, err := utils.EncryptString(clientSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt client secret: %w", err)
	}

	encryptedWebhookSecret, err := utils.EncryptString(webhookSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt webhook secret: %w", err)
	}

	var encryptedPrivateKey *string
	if privateKey != nil && *privateKey != "" {
		encPk, err := utils.EncryptString(*privateKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt private key: %w", err)
		}
		encryptedPrivateKey = &encPk
	}

	// Save to database - first deactivate old configs, then insert new
	err = api.GitHub.SaveGitHubConfig(context.Background(), encryptedClientID, encryptedClientSecret, encryptedWebhookSecret, redirectURI, appID, appSlug, appName, encryptedPrivateKey, installationID)
	if err != nil {
		return fmt.Errorf("failed to save GitHub config to database: %w", err)
	}

	log := logger.Default().WithComponent("github-config")
	log.Info("GitHub config saved to database")
	return nil
}

// LoadGitHubConfigFromDB loads GitHub configuration from database (decrypted)
func LoadGitHubConfigFromDB() (clientID, clientSecret, redirectURI, webhookSecret string, appID *int64, appSlug, appName *string, privateKey *string, installationID *int64, err error) {
	config, err := api.GitHub.GetGitHubConfigFull(context.Background())
	if err != nil {
		return "", "", "", "", nil, nil, nil, nil, nil, fmt.Errorf("failed to load GitHub config from database: %w", err)
	}

	// Decrypt sensitive data
	clientID, err = utils.DecryptString(config.ClientID)
	if err != nil {
		return "", "", "", "", nil, nil, nil, nil, nil, fmt.Errorf("failed to decrypt client ID: %w", err)
	}

	clientSecret, err = utils.DecryptString(config.ClientSecret)
	if err != nil {
		return "", "", "", "", nil, nil, nil, nil, nil, fmt.Errorf("failed to decrypt client secret: %w", err)
	}

	webhookSecret, err = utils.DecryptString(config.WebhookSecret)
	if err != nil {
		return "", "", "", "", nil, nil, nil, nil, nil, fmt.Errorf("failed to decrypt webhook secret: %w", err)
	}

	log := logger.Default().WithComponent("github-config")
	log.Info("GitHub config loaded from database")

	var decryptedPrivateKey *string
	if config.PrivateKey != nil && *config.PrivateKey != "" {
		if pk, decErr := utils.DecryptString(*config.PrivateKey); decErr == nil {
			decryptedPrivateKey = &pk
		} else {
			return "", "", "", "", nil, nil, nil, nil, nil, fmt.Errorf("failed to decrypt private key: %w", decErr)
		}
	}

	return clientID, clientSecret, config.RedirectURI, webhookSecret, config.AppID, config.AppSlug, config.AppName, decryptedPrivateKey, config.InstallationID, nil
}
