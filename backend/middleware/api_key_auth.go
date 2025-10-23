package middleware

import (
	"backend/utils"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

// ServiceAccount represents a validated service account
type ServiceAccount struct {
	ID          string
	InstanceID  string
	Name        string
	Scopes      []string
	RateLimit   int
}

// APIKeyAuth validates API keys from CitizenAuth service
func APIKeyAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract API key from header
		apiKey := c.Get("X-API-Key")
		
		if apiKey == "" {
			// No API key provided, skip to next auth method
			return c.Next()
		}

		// Validate API key
		serviceAccount, err := validateAPIKey(c.Context(), apiKey)
		if err != nil {
			utils.AuthDebugLog("API key validation failed: %v", err)
			return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
				false,
				"Invalid API key",
				nil,
			))
		}

		utils.AuthDebugLog("API key validated successfully: %s", serviceAccount.Name)

		// Store service account info in context
		c.Locals("auth_type", "service")
		c.Locals("service_id", serviceAccount.ID)
		c.Locals("service_name", serviceAccount.Name)
		c.Locals("service_scopes", serviceAccount.Scopes)
		c.Locals("service_rate_limit", serviceAccount.RateLimit)

		return c.Next()
	}
}

// RequireServiceAuth ensures service authentication is present
func RequireServiceAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authType := c.Locals("auth_type")
		
		if authType != "service" {
			return c.Status(fiber.StatusForbidden).JSON(utils.NewCitizenResponse(
				false,
				"Service authentication required",
				nil,
			))
		}

		return c.Next()
	}
}

// RequireScope ensures service has required scope
func RequireScope(requiredScope string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		scopes, ok := c.Locals("service_scopes").([]string)
		if !ok {
			return c.Status(fiber.StatusForbidden).JSON(utils.NewCitizenResponse(
				false,
				"No service scopes available",
				nil,
			))
		}

		// Check if service has required scope or admin scope
		hasScope := false
		for _, scope := range scopes {
			if scope == requiredScope || scope == "admin" || scope == "*" {
				hasScope = true
				break
			}
		}

		if !hasScope {
			utils.SecurityLog("Insufficient scope: required=%s, provided=%v", requiredScope, scopes)
			return c.Status(fiber.StatusForbidden).JSON(utils.NewCitizenResponse(
				false,
				"Insufficient scope: "+requiredScope+" required",
				nil,
			))
		}

		return c.Next()
	}
}

// validateAPIKey validates API key against environment variable
// In production, this should query a database
func validateAPIKey(ctx context.Context, apiKey string) (*ServiceAccount, error) {
	// Get stored API key hash from environment
	storedKeyHash := os.Getenv("CITIZENAUTH_API_KEY_HASH")
	
	// If no hash stored, use plaintext comparison (development only)
	if storedKeyHash == "" {
		expectedKey := os.Getenv("CITIZENAUTH_API_KEY")
		if expectedKey != "" && apiKey == expectedKey {
			return &ServiceAccount{
				ID:         "citizenauth-service",
				InstanceID: "main",
				Name:       "CitizenAuth Service",
				Scopes:     []string{"permissions:read", "permissions:write", "webhooks"},
				RateLimit:  1000,
			}, nil
		}
		
		// No API key configured
		log.Println("⚠️  [API-KEY] No API key configured in environment")
		return nil, fmt.Errorf("API key not configured")
	}

	// Validate hash (production)
	err := bcrypt.CompareHashAndPassword([]byte(storedKeyHash), []byte(apiKey))
	if err != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	// TODO: In future, query database for service account details and scopes
	// For now, return default service account
	return &ServiceAccount{
		ID:         "citizenauth-service",
		InstanceID: "main",
		Name:       "CitizenAuth Service",
		Scopes:     []string{"permissions:read", "permissions:write", "webhooks"},
		RateLimit:  1000,
	}, nil
}

// CombinedAuth tries JWT first, then API key
func CombinedAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Try JWT auth first
		if err := JWTAuth()(c); err == nil {
			if c.Locals("auth_type") == "jwt" {
				return c.Next()
			}
		}

		// Try API key auth
		if err := APIKeyAuth()(c); err == nil {
			if c.Locals("auth_type") == "service" {
				return c.Next()
			}
		}

		// Neither auth method succeeded
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"Authentication required (JWT or API Key)",
			nil,
		))
	}
}

