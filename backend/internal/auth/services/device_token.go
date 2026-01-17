package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
		citizenauthURL = "https://ustun.tech/api/v1"
	}

	// Call CitizenAuth /auth/device/validate endpoint
	reqBody := map[string]string{
		"token": token,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", citizenauthURL+"/auth/device/validate", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to validate device token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device token validation failed: %s (status: %d)", string(body), resp.StatusCode)
	}

	var response struct {
		Success bool                         `json:"success"`
		Data    DeviceTokenValidationResult  `json:"data"`
		Error   string                       `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("validation failed: %s", response.Error)
	}

	return &response.Data, nil
}
