package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"

	"backend/internal/database/api"
	"backend/internal/utils"
	"backend/pkg/errors"
	"backend/pkg/logger"
)

// GitHub App configuration - stored in memory after first setup
// NOTE: Private key is NOT stored in memory for security reasons.
// It is fetched from DB and decrypted only when needed (JWT generation).
var (
	gitHubWebhookSecret    string
	gitHubConfigMutex      sync.RWMutex
	gitHubConfigured       bool
	gitHubAppID            *int64
	gitHubAppSlug          *string
	gitHubAppName          *string
	gitHubPrivateKeyExists bool // Flag only - actual key fetched from DB when needed
	gitHubInstallationID   *int64
)

// SetupGitHubWebhookSecret sets up GitHub webhook secret in memory
func SetupGitHubWebhookSecret(webhookSecret string) {
	gitHubConfigMutex.Lock()
	defer gitHubConfigMutex.Unlock()

	gitHubWebhookSecret = webhookSecret

	log := logger.Default().WithComponent("github-config")
	log.Info("GitHub webhook secret configured")
}

// SetupGitHubApp stores GitHub App configuration (manifest flow)
// NOTE: Private key is stored in DB only, not in memory, for security.
func SetupGitHubApp(appID int64, appSlug *string, privateKey *string, installationID *int64, appName *string) {
	gitHubConfigMutex.Lock()
	defer gitHubConfigMutex.Unlock()

	gitHubAppID = &appID
	gitHubAppSlug = appSlug
	gitHubAppName = appName
	gitHubPrivateKeyExists = privateKey != nil && *privateKey != ""
	gitHubInstallationID = installationID

	log := logger.Default().WithComponent("github-config")
	log.WithFields(map[string]interface{}{
		"app_id":             appID,
		"app_slug":           appSlug,
		"installation_id":    installationID,
		"private_key_exists": gitHubPrivateKeyExists,
	}).Info("GitHub App configured")
}

// IsGitHubConfigured checks if GitHub App is configured
func IsGitHubConfigured() bool {
	gitHubConfigMutex.RLock()
	defer gitHubConfigMutex.RUnlock()

	// GitHub App is configured if we have app ID and private key
	return gitHubPrivateKeyExists && gitHubAppID != nil
}

// GetWebhookSecret gets the webhook secret for signature validation
func GetWebhookSecret() string {
	gitHubConfigMutex.RLock()
	defer gitHubConfigMutex.RUnlock()

	if gitHubWebhookSecret != "" {
		return gitHubWebhookSecret
	}

	// Fallback to environment variable
	return os.Getenv("GITHUB_WEBHOOK_SECRET")
}

// GetGitHubAppConfig returns manifest app configuration (if set)
// NOTE: This returns nil for privateKey. Use GetPrivateKeyFromDB() to get the actual key.
func GetGitHubAppConfig() (appID *int64, appSlug *string, appName *string, privateKey *string, installationID *int64) {
	gitHubConfigMutex.RLock()
	defer gitHubConfigMutex.RUnlock()
	// Private key is intentionally NOT returned here for security
	// Use GetPrivateKeyFromDB() when you actually need the key
	return gitHubAppID, gitHubAppSlug, gitHubAppName, nil, gitHubInstallationID
}

// HasPrivateKey checks if a private key exists without exposing it
func HasPrivateKey() bool {
	gitHubConfigMutex.RLock()
	defer gitHubConfigMutex.RUnlock()
	return gitHubPrivateKeyExists
}

// GetPrivateKeyFromDB fetches and decrypts the private key from DB
// This should only be called when the key is actually needed (e.g., JWT generation)
// The key is NOT cached in memory for security reasons
func GetPrivateKeyFromDB() (string, error) {
	config, err := api.GitHub.GetGitHubConfigFull(context.Background())
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeInternal, "failed to load GitHub config from database")
	}

	if config.PrivateKey == nil || *config.PrivateKey == "" {
		return "", errors.BadRequest("private key not configured")
	}

	privateKey, err := utils.DecryptString(*config.PrivateKey)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeInternal, "failed to decrypt private key")
	}

	return privateKey, nil
}

// GenerateSecureSecret generates a cryptographically secure secret
func GenerateSecureSecret() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// maskSensitiveValue masks a sensitive value for safe logging
// Shows first 4 chars and last 2 chars only
func maskSensitiveValue(value string) string {
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "..." + value[len(value)-2:]
}

// SaveGitHubAppConfigToDB saves GitHub App configuration to database (encrypted)
func SaveGitHubAppConfigToDB(webhookSecret string, appID int64, appSlug, appName string, privateKey string, installationID *int64) error {
	log := logger.Default().WithComponent("github-config")

	// Encrypt sensitive data
	encryptedWebhookSecret, err := utils.EncryptString(webhookSecret)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to encrypt webhook secret")
	}

	encryptedPrivateKey, err := utils.EncryptString(privateKey)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to encrypt private key")
	}

	// Save to database
	err = api.GitHub.SaveGitHubAppConfig(context.Background(), encryptedWebhookSecret, appID, appSlug, appName, encryptedPrivateKey, installationID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to save GitHub config to database")
	}

	log.WithField("app_id", appID).Info("GitHub App config saved to database")
	return nil
}

// LoadGitHubConfigFromDB loads GitHub App configuration from database
func LoadGitHubConfigFromDB() (webhookSecret string, appID *int64, appSlug, appName *string, installationID *int64, err error) {
	config, err := api.GitHub.GetGitHubConfigFull(context.Background())
	if err != nil {
		return "", nil, nil, nil, nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to load GitHub config from database")
	}

	// Decrypt webhook secret
	webhookSecret, err = utils.DecryptString(config.WebhookSecret)
	if err != nil {
		return "", nil, nil, nil, nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to decrypt webhook secret")
	}

	log := logger.Default().WithComponent("github-config")
	log.Info("GitHub config loaded from database")

	return webhookSecret, config.AppID, config.AppSlug, config.AppName, config.InstallationID, nil
}
