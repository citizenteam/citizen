package middleware

import (
	"backend/database"
	"backend/utils"
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
)

// ServiceAccount represents a validated service account
type ServiceAccount struct {
	ID             string
	InstanceID     string
	OrganizationID string
	Name           string
	InstanceDomain string
	CitizenauthURL string
	Scopes         []string
	RateLimit      int
	WebhookSecret  string
	APIKeyPrefix   string
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
		c.Locals("citizenauth_instance_id", serviceAccount.InstanceID)
		if serviceAccount.OrganizationID != "" {
			c.Locals("organization_id", serviceAccount.OrganizationID)
		}
		if serviceAccount.WebhookSecret != "" {
			c.Locals("webhook_secret", serviceAccount.WebhookSecret)
		}
		if serviceAccount.InstanceDomain != "" {
			c.Locals("service_domain", serviceAccount.InstanceDomain)
		}

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
func validateAPIKey(ctx context.Context, apiKey string) (*ServiceAccount, error) {
	if len(apiKey) < 8 {
		return nil, fmt.Errorf("invalid API key")
	}

	prefix := apiKey[:8]

	instance, err := database.GetCitizenauthInstanceByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, database.ErrCitizenauthInstanceNotFound) {
			log.Printf("❌ [API-KEY] Unknown API key prefix: %s", prefix)
			return nil, fmt.Errorf("invalid API key")
		}
		log.Printf("❌ [API-KEY] Failed to lookup prefix %s: %v", prefix, err)
		return nil, fmt.Errorf("API key lookup failed")
	}

	if instance.APIKeyHash == nil || *instance.APIKeyHash == "" {
		log.Printf("❌ [API-KEY] Missing stored hash for instance %s", instance.InstanceUUID)
		return nil, fmt.Errorf("invalid API key")
	}

	if !utils.CheckPasswordHash(apiKey, *instance.APIKeyHash) {
		log.Printf("❌ [API-KEY] Hash mismatch for prefix %s", prefix)
		return nil, fmt.Errorf("invalid API key")
	}

	var webhookSecret string
	if instance.WebhookSecretEncrypted != nil && *instance.WebhookSecretEncrypted != "" {
		secret, err := utils.DecryptString(*instance.WebhookSecretEncrypted)
		if err != nil {
			log.Printf("❌ [API-KEY] Failed to decrypt webhook secret for %s: %v", instance.InstanceUUID, err)
			return nil, fmt.Errorf("invalid API key")
		}
		webhookSecret = secret
	}

	var domain string
	if instance.Domain != nil {
		domain = *instance.Domain
	}

	var citizenAuthURL string
	if instance.CitizenauthURL != nil {
		citizenAuthURL = *instance.CitizenauthURL
	}

	serviceAccount := &ServiceAccount{
		ID:             instance.InstanceUUID.String(),
		InstanceID:     instance.InstanceUUID.String(),
		OrganizationID: instance.OrganizationID.String(),
		Name:           fmt.Sprintf("CitizenAuth Instance %s", instance.InstanceUUID),
		InstanceDomain: domain,
		CitizenauthURL: citizenAuthURL,
		Scopes:         []string{"permissions:read", "permissions:write", "webhooks", "instance:import", "instance:lifecycle"},
		RateLimit:      1000,
		WebhookSecret:  webhookSecret,
		APIKeyPrefix:   prefix,
	}

	return serviceAccount, nil
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
