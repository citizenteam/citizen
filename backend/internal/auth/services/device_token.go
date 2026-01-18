package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DeviceTokenValidationResult represents the response from CitizenAuth device token validation
type DeviceTokenValidationResult struct {
	UserID         string   `json:"user_id"`
	Email          string   `json:"email"`
	Name           string   `json:"name"`
	OrganizationID string   `json:"organization_id"`
	Role           string   `json:"role"`
	IsSuperAdmin   bool     `json:"is_super_admin"`
	Scopes         []string `json:"scopes"`
}

// ValidateDeviceToken validates a device session token with CitizenAuth
func ValidateDeviceToken(ctx context.Context, token string) (*DeviceTokenValidationResult, error) {
	citizenauthURL := os.Getenv("CITIZENAUTH_URL")
	if citizenauthURL == "" {
		citizenauthURL = "https://ustun.tech"
	}
	// Ensure we have /api/v1 prefix
	if !strings.HasSuffix(citizenauthURL, "/api/v1") {
		citizenauthURL = citizenauthURL + "/api/v1"
	}

	// Call CitizenAuth /auth/device/validate endpoint
	reqBody := map[string]string{
		"token": token,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	fullURL := citizenauthURL + "/auth/device/validate"
	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	fmt.Printf("🌐 [DEVICE-TOKEN] Sending validation request to: %s\n", fullURL)
	fmt.Printf("📦 [DEVICE-TOKEN] Request body: %s\n", string(jsonData))

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ [DEVICE-TOKEN] Request failed: %v\n", err)
		return nil, fmt.Errorf("failed to validate device token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("📨 [DEVICE-TOKEN] Response status: %d\n", resp.StatusCode)
	fmt.Printf("📨 [DEVICE-TOKEN] Response body: %s\n", string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device token validation failed: %s (status: %d)", string(body), resp.StatusCode)
	}

	// Decode response body
	var response struct {
		Success bool                        `json:"success"`
		Data    DeviceTokenValidationResult `json:"data"`
		Error   string                      `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("validation failed: %s", response.Error)
	}

	return &response.Data, nil
}
