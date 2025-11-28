package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

// generateTestPrivateKey generates a test RSA private key in PEM format
func generateTestPrivateKey(t *testing.T) string {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate test private key: %v", err)
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	return string(pem.EncodeToMemory(pemBlock))
}

func TestSetupGitHubOAuth(t *testing.T) {
	// Reset state
	gitHubConfigured = false
	gitHubClientID = ""
	gitHubClientSecret = ""

	err := SetupGitHubOAuth("test-client-id", "test-secret", "http://localhost/callback", "webhook-secret")
	if err != nil {
		t.Fatalf("SetupGitHubOAuth failed: %v", err)
	}

	if !IsGitHubConfigured() {
		t.Error("Expected GitHub to be configured after SetupGitHubOAuth")
	}

	clientID, clientSecret, redirectURI, webhookSecret := GetGitHubConfig()
	if clientID != "test-client-id" {
		t.Errorf("Expected clientID 'test-client-id', got '%s'", clientID)
	}
	if clientSecret != "test-secret" {
		t.Errorf("Expected clientSecret 'test-secret', got '%s'", clientSecret)
	}
	if redirectURI != "http://localhost/callback" {
		t.Errorf("Expected redirectURI 'http://localhost/callback', got '%s'", redirectURI)
	}
	if webhookSecret != "webhook-secret" {
		t.Errorf("Expected webhookSecret 'webhook-secret', got '%s'", webhookSecret)
	}
}

func TestSetupGitHubApp(t *testing.T) {
	appID := int64(12345)
	appSlug := "test-app"
	appName := "Test App"
	privateKey := "test-private-key"
	installationID := int64(67890)

	SetupGitHubApp(appID, &appSlug, &privateKey, &installationID, &appName)

	gotAppID, gotSlug, gotName, gotKey, gotInstallID := GetGitHubAppConfig()

	if gotAppID == nil || *gotAppID != appID {
		t.Errorf("Expected appID %d, got %v", appID, gotAppID)
	}
	if gotSlug == nil || *gotSlug != appSlug {
		t.Errorf("Expected appSlug '%s', got %v", appSlug, gotSlug)
	}
	if gotName == nil || *gotName != appName {
		t.Errorf("Expected appName '%s', got %v", appName, gotName)
	}
	if gotKey == nil || *gotKey != privateKey {
		t.Errorf("Expected privateKey '%s', got %v", privateKey, gotKey)
	}
	if gotInstallID == nil || *gotInstallID != installationID {
		t.Errorf("Expected installationID %d, got %v", installationID, gotInstallID)
	}
}

func TestIsGitHubConfigured(t *testing.T) {
	// Reset state
	gitHubConfigured = false
	gitHubClientID = ""
	gitHubPrivateKey = nil
	gitHubAppID = nil

	// Should be false initially
	if IsGitHubConfigured() {
		t.Error("Expected GitHub to not be configured initially")
	}

	// Set OAuth config
	gitHubConfigured = true
	if !IsGitHubConfigured() {
		t.Error("Expected GitHub to be configured after setting gitHubConfigured")
	}

	// Reset and test with App config
	gitHubConfigured = false
	appID := int64(123)
	privateKey := "test-key"
	gitHubAppID = &appID
	gitHubPrivateKey = &privateKey

	if !IsGitHubConfigured() {
		t.Error("Expected GitHub to be configured with App ID and Private Key")
	}
}

func TestGenerateGitHubAppJWTWithKey(t *testing.T) {
	privateKey := generateTestPrivateKey(t)
	appID := int64(12345)

	jwt, err := GenerateGitHubAppJWTWithKey(appID, privateKey)
	if err != nil {
		t.Fatalf("GenerateGitHubAppJWTWithKey failed: %v", err)
	}

	if jwt == "" {
		t.Error("Expected non-empty JWT")
	}

	// JWT should have 3 parts separated by dots
	parts := 0
	for _, c := range jwt {
		if c == '.' {
			parts++
		}
	}
	if parts != 2 {
		t.Errorf("Expected JWT with 2 dots (3 parts), got %d dots", parts)
	}
}

func TestGenerateGitHubAppJWTWithKey_InvalidKey(t *testing.T) {
	_, err := GenerateGitHubAppJWTWithKey(12345, "invalid-key")
	if err == nil {
		t.Error("Expected error with invalid private key")
	}
}

func TestValidateGitHubSignature(t *testing.T) {
	// Setup webhook secret
	SetupGitHubOAuth("client", "secret", "redirect", "test-webhook-secret")

	payload := []byte(`{"action": "push"}`)

	// Generate valid signature
	expectedSig := "sha256=" + generateHMACSignature(payload, "test-webhook-secret")

	if !ValidateGitHubSignature(payload, expectedSig) {
		t.Error("Expected valid signature to pass")
	}

	// Test invalid signature
	if ValidateGitHubSignature(payload, "sha256=invalid") {
		t.Error("Expected invalid signature to fail")
	}

	// Test missing prefix
	if ValidateGitHubSignature(payload, "invalid") {
		t.Error("Expected signature without sha256= prefix to fail")
	}
}

func TestGetGitHubOAuthURL(t *testing.T) {
	// Setup config
	SetupGitHubOAuth("test-client-id", "test-secret", "http://localhost/callback", "webhook-secret")

	url, err := GetGitHubOAuthURL("test-state")
	if err != nil {
		t.Fatalf("GetGitHubOAuthURL failed: %v", err)
	}

	// Check URL contains required parameters
	expectedParts := []string{
		"https://github.com/login/oauth/authorize",
		"client_id=test-client-id",
		"redirect_uri=http",
		"state=test-state",
		"scope=read%3Auser",
	}

	for _, part := range expectedParts {
		if !contains(url, part) {
			t.Errorf("Expected URL to contain '%s', got: %s", part, url)
		}
	}
}

func TestGetGitHubOAuthURL_NotConfigured(t *testing.T) {
	// Reset config
	gitHubConfigured = false
	gitHubClientID = ""
	gitHubRedirectURI = ""

	_, err := GetGitHubOAuthURL("test-state")
	if err == nil {
		t.Error("Expected error when GitHub OAuth is not configured")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
