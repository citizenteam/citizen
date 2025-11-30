package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/utils"

	"github.com/gofiber/fiber/v2"
)

// setupTestApp creates a new Fiber app for testing
func setupTestApp() *fiber.App {
	app := fiber.New()
	return app
}

// mockUserContext middleware that sets a mock user ID
func mockUserContext(userID int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("user_id", userID)
		return c.Next()
	}
}

func TestGetGitHubConfig_NotConfigured(t *testing.T) {
	// Reset GitHub config
	utils.SetupGitHubOAuth("", "", "", "")

	app := setupTestApp()
	app.Get("/config", GetGitHubConfig)

	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data in response")
	}

	if configured, ok := data["configured"].(bool); !ok || configured {
		t.Error("Expected configured to be false")
	}
}

func TestDeleteGitHubConfig_Success(t *testing.T) {
	// This test requires database mock
	// Skip for now - would need to mock api.GitHub.DeleteGitHubConfig
	t.Skip("Requires database mock")
}

func TestConnectWithPrivateKey_MissingAppID(t *testing.T) {
	app := setupTestApp()
	app.Post("/connect", mockUserContext(1), ConnectWithPrivateKey)

	body := map[string]interface{}{
		"private_key": "test-key",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/connect", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	if result["success"] != false {
		t.Error("Expected success to be false")
	}
}

func TestConnectWithPrivateKey_MissingPrivateKey(t *testing.T) {
	app := setupTestApp()
	app.Post("/connect", mockUserContext(1), ConnectWithPrivateKey)

	body := map[string]interface{}{
		"app_id": 12345,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/connect", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestConnectWithPrivateKey_Unauthorized(t *testing.T) {
	app := setupTestApp()
	// No mockUserContext - simulates unauthorized request
	app.Post("/connect", ConnectWithPrivateKey)

	body := map[string]interface{}{
		"app_id":      12345,
		"private_key": "test-key",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/connect", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}
}

func TestConnectWithPrivateKey_InvalidPrivateKey(t *testing.T) {
	app := setupTestApp()
	app.Post("/connect", mockUserContext(1), ConnectWithPrivateKey)

	body := map[string]interface{}{
		"app_id":      12345,
		"private_key": "invalid-not-a-pem-key",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/connect", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	if result["message"] == nil {
		t.Error("Expected error message in response")
	}
}

func TestGitHubAuthCallback_MissingCode(t *testing.T) {
	app := setupTestApp()
	app.Get("/callback", GitHubAuthCallback)

	req := httptest.NewRequest(http.MethodGet, "/callback?state=test", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestGitHubAuthCallback_MissingState(t *testing.T) {
	app := setupTestApp()
	app.Get("/callback", GitHubAuthCallback)

	req := httptest.NewRequest(http.MethodGet, "/callback?code=test-code", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestGitHubAuthCallback_InvalidStateFormat(t *testing.T) {
	app := setupTestApp()
	app.Get("/callback", GitHubAuthCallback)

	// State should start with "user_"
	req := httptest.NewRequest(http.MethodGet, "/callback?code=test-code&state=invalid", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestGitHubAuthCallback_ExpiredState(t *testing.T) {
	app := setupTestApp()
	app.Get("/callback", GitHubAuthCallback)

	// State with old timestamp (more than 10 minutes ago)
	oldTimestamp := "1600000000" // Sept 2020
	req := httptest.NewRequest(http.MethodGet, "/callback?code=test-code&state=user_1_"+oldTimestamp+"_abcdef1234567890abcdef1234567890", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestStartGitHubManifest(t *testing.T) {
	app := setupTestApp()
	app.Post("/manifest/start", StartGitHubManifest)

	req := httptest.NewRequest(http.MethodPost, "/manifest/start", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if result["success"] != true {
		t.Error("Expected success to be true")
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data in response")
	}

	if data["manifest_url"] == nil {
		t.Error("Expected manifest_url in response")
	}

	if data["state"] == nil {
		t.Error("Expected state in response")
	}
}

func TestGitHubInstallCallback_MissingInstallationID(t *testing.T) {
	app := setupTestApp()
	app.Get("/install/callback", GitHubInstallCallback)

	req := httptest.NewRequest(http.MethodGet, "/install/callback?state=test", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestGenerateSecureSecret(t *testing.T) {
	secret1 := generateSecureSecret()
	secret2 := generateSecureSecret()

	// Should be 64 characters (32 bytes hex encoded)
	if len(secret1) != 64 {
		t.Errorf("Expected secret length 64, got %d", len(secret1))
	}

	// Two secrets should be different
	if secret1 == secret2 {
		t.Error("Expected two generated secrets to be different")
	}
}

func TestStateStore(t *testing.T) {
	store := newStateStore()

	// Add state
	store.add("test-state-1")

	// Check exists
	if !store.exists("test-state-1") {
		t.Error("Expected state to exist after adding")
	}

	// Check non-existent
	if store.exists("non-existent") {
		t.Error("Expected non-existent state to not exist")
	}
}

func TestHtmlEscapeSingleQuotes(t *testing.T) {
	input := "test'string'with'quotes"
	expected := "test&#39;string&#39;with&#39;quotes"
	result := htmlEscapeSingleQuotes(input)

	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}

	// No quotes
	input2 := "no quotes here"
	if htmlEscapeSingleQuotes(input2) != input2 {
		t.Error("Expected string without quotes to remain unchanged")
	}
}
