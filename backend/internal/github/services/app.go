package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"backend/internal/github/models"
	"backend/pkg/errors"
	"backend/pkg/logger"

	"github.com/golang-jwt/jwt/v5"
)

// generateGitHubAppJWT creates a short-lived JWT for GitHub App authentication
func generateGitHubAppJWT(appID int64, pemKey string) (string, error) {
	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(pemKey))
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeInternal, "parse private key")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Add(-1 * time.Minute).Unix(), // backdate 60s to avoid clock skew
		"exp": now.Add(9 * time.Minute).Unix(),  // GitHub requires <= 10 minutes
		"iss": appID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeInternal, "sign jwt")
	}
	return signed, nil
}

// GenerateGitHubAppJWTWithKey creates a JWT using provided app ID and private key
func GenerateGitHubAppJWTWithKey(appID int64, pemKey string) (string, error) {
	return generateGitHubAppJWT(appID, pemKey)
}

// GetGitHubAppInfoWithJWT gets app info using JWT authentication
func GetGitHubAppInfoWithJWT(jwtToken string) (*models.AppInfo, error) {
	url := "https://api.github.com/app"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 30 * time.Second}
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
		return nil, errors.InternalErrorf("failed to get app info: %s", string(body))
	}

	var appInfo models.AppInfo
	if err := json.Unmarshal(body, &appInfo); err != nil {
		return nil, err
	}
	return &appInfo, nil
}

// GetAppInstallationsWithJWT lists all installations for a GitHub App using JWT
func GetAppInstallationsWithJWT(jwtToken string) ([]models.Installation, error) {
	url := "https://api.github.com/app/installations"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 30 * time.Second}
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
		return nil, errors.InternalErrorf("failed to get installations: %s", string(body))
	}

	var installations []models.Installation
	if err := json.Unmarshal(body, &installations); err != nil {
		return nil, err
	}
	return installations, nil
}

// UpdateGitHubAppURLs updates the GitHub App webhook URL
// Note: GitHub API only supports updating webhook URL via PATCH /app/hook/config
// Other URLs (homepage, callback, setup) can only be changed via GitHub UI
func UpdateGitHubAppURLs(jwtToken string, updates models.AppURLUpdate) error {
	log := logger.Default().WithComponent("github-app")
	log.WithFields(map[string]interface{}{
		"homepage_url":  updates.HomepageURL,
		"webhook_url":   updates.WebhookURL,
		"callback_urls": updates.CallbackURLs,
		"setup_url":     updates.SetupURL,
	}).Info("UpdateGitHubAppURLs called")

	// GitHub API only supports updating webhook URL programmatically
	// Homepage, callback URLs, and setup URL can only be changed via GitHub UI
	if updates.WebhookURL == "" {
		log.Info("No webhook URL provided, skipping update")
		return nil
	}

	// Update webhook configuration: PATCH /app/hook/config
	apiURL := "https://api.github.com/app/hook/config"

	requestBody := map[string]interface{}{
		"url":          updates.WebhookURL,
		"content_type": "json",
	}

	// Include webhook secret if provided
	if updates.WebhookSecret != "" {
		requestBody["secret"] = updates.WebhookSecret
		log.Info("Including new webhook secret in update")
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to marshal request")
	}

	log.WithField("config", string(jsonBody)).Debug("Updating webhook config")

	req, err := http.NewRequest("PATCH", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to create request")
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "request failed")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.WithFields(map[string]interface{}{
		"status_code": resp.StatusCode,
		"response":    string(body),
	}).Debug("UpdateGitHubAppURLs response")

	if resp.StatusCode != http.StatusOK {
		return errors.InternalErrorf("GitHub API error (%d): %s", resp.StatusCode, string(body))
	}

	if updates.WebhookSecret != "" {
		log.WithField("webhook_url", updates.WebhookURL).Info("Webhook URL and secret updated successfully")
	} else {
		log.WithField("webhook_url", updates.WebhookURL).Info("Webhook URL updated successfully")
	}

	// Warn about URLs that need manual update
	if updates.HomepageURL != "" || len(updates.CallbackURLs) > 0 || updates.SetupURL != "" {
		log.Warn("Note: Homepage, Callback, and Setup URLs must be updated manually in GitHub App settings")
	}

	return nil
}

// GetGitHubInstallationToken returns an installation access token using stored app config
// Private key is fetched from DB and decrypted only when needed (not cached in memory)
func GetGitHubInstallationToken() (*models.InstallationTokenResponse, error) {
	appID, _, _, _, installationID := GetGitHubAppConfig()
	if appID == nil || !HasPrivateKey() {
		return nil, errors.BadRequest("github app config missing")
	}
	if installationID == nil {
		return nil, errors.BadRequest("github app installation missing")
	}

	// Fetch private key from DB (not cached in memory for security)
	privateKey, err := GetPrivateKeyFromDB()
	if err != nil {
		return nil, err
	}

	jwtToken, err := generateGitHubAppJWT(*appID, privateKey)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", *installationID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
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

	if resp.StatusCode >= 300 {
		return nil, errors.InternalErrorf("failed to get installation token: %s", string(body))
	}

	var tokenResp models.InstallationTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

// ConvertManifestCode converts a GitHub App manifest code to app credentials
func ConvertManifestCode(code string) (*models.ManifestConversionResponse, error) {
	log := logger.Default().WithComponent("github-app")
	log.WithField("code", code[:min(8, len(code))]+"...").Debug("Converting manifest code")

	url := fmt.Sprintf("https://api.github.com/app-manifests/%s/conversions", code)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to build manifest conversion request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to call GitHub API for manifest conversion")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to read manifest conversion response")
	}

	if resp.StatusCode >= 300 {
		// Log full error details internally for debugging
		log.WithFields(map[string]interface{}{
			"status_code": resp.StatusCode,
			"response":    string(body),
		}).Error("Manifest conversion failed")
		// Return generic error to user - don't expose GitHub API internals
		return nil, errors.InternalError("Failed to create GitHub App. Please try again.")
	}

	var manifestResp models.ManifestConversionResponse
	if err := json.Unmarshal(body, &manifestResp); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to parse manifest conversion response")
	}

	log.WithFields(map[string]interface{}{
		"app_id":   manifestResp.ID,
		"app_slug": manifestResp.Slug,
		"app_name": manifestResp.Name,
	}).Info("Manifest conversion successful")

	return &manifestResp, nil
}
