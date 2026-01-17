package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// DeviceTokenValidator validates device session tokens with CitizenAuth
type DeviceTokenValidator struct {
	apiURL     string
	httpClient *http.Client
	cache      sync.Map // token -> *DeviceTokenClaims with TTL
}

// DeviceTokenClaims represents claims from a device token validation response
type DeviceTokenClaims struct {
	UserID         string   `json:"user_id"`
	Email          string   `json:"email"`
	Name           string   `json:"name"`
	OrganizationID string   `json:"organization_id"`
	Role           string   `json:"role"`
	IsSuperAdmin   bool     `json:"is_super_admin"`
	Scopes         []string `json:"scopes"`
	cachedAt       time.Time
}

// DeviceTokenValidateRequest is the request to CitizenAuth's validate endpoint
type DeviceTokenValidateRequest struct {
	Token string `json:"token"`
}

// DeviceTokenValidateResponse is the response from CitizenAuth
type DeviceTokenValidateResponse struct {
	Success bool `json:"success"`
	Data    struct {
		UserID         string   `json:"user_id"`
		Email          string   `json:"email"`
		Name           string   `json:"name"`
		OrganizationID string   `json:"organization_id"`
		Role           string   `json:"role"`
		IsSuperAdmin   bool     `json:"is_super_admin"`
		Scopes         []string `json:"scopes"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

// NewDeviceTokenValidator creates a new device token validator instance
func NewDeviceTokenValidator(apiURL string) *DeviceTokenValidator {
	if apiURL == "" {
		// Use LOGIN_HOST to construct CitizenAuth API URL
		loginHost := os.Getenv("LOGIN_HOST")
		if loginHost == "" {
			loginHost = "localhost:3000"
		}

		// Determine protocol
		protocol := "https"
		if strings.Contains(loginHost, "localhost") {
			protocol = "http"
		}

		apiURL = fmt.Sprintf("%s://%s", protocol, loginHost)
	}

	return &DeviceTokenValidator{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// ValidateToken validates a device token with CitizenAuth API
func (dtv *DeviceTokenValidator) ValidateToken(tokenString string) (*DeviceTokenClaims, error) {
	// Check cache first (5 minute TTL like CitizenAuth's Redis cache)
	if cached, ok := dtv.cache.Load(tokenString[:min(20, len(tokenString))]); ok {
		claims := cached.(*DeviceTokenClaims)
		if time.Since(claims.cachedAt) < 5*time.Minute {
			return claims, nil
		}
		// Cache expired, remove it
		dtv.cache.Delete(tokenString[:min(20, len(tokenString))])
	}

	// Call CitizenAuth API
	reqBody := DeviceTokenValidateRequest{
		Token: tokenString,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", dtv.apiURL+"/api/v1/auth/device/validate", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := dtv.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call CitizenAuth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var errorResp DeviceTokenValidateResponse
		json.NewDecoder(resp.Body).Decode(&errorResp)
		return nil, fmt.Errorf("device token validation failed: %s", errorResp.Error)
	}

	var validateResp DeviceTokenValidateResponse
	if err := json.NewDecoder(resp.Body).Decode(&validateResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !validateResp.Success {
		return nil, fmt.Errorf("device token invalid: %s", validateResp.Error)
	}

	claims := &DeviceTokenClaims{
		UserID:         validateResp.Data.UserID,
		Email:          validateResp.Data.Email,
		Name:           validateResp.Data.Name,
		OrganizationID: validateResp.Data.OrganizationID,
		Role:           validateResp.Data.Role,
		IsSuperAdmin:   validateResp.Data.IsSuperAdmin,
		Scopes:         validateResp.Data.Scopes,
		cachedAt:       time.Now(),
	}

	// Cache for 5 minutes
	dtv.cache.Store(tokenString[:min(20, len(tokenString))], claims)

	return claims, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
