package middleware

import (
	"backend/services"
	"backend/utils"
	"log"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

var jwtValidator *services.JWTValidator

// InitJWTValidator initializes the JWT validator with JWKS URL
func InitJWTValidator() error {
	jwksURL := os.Getenv("CITIZENAUTH_JWKS_URL")
	if jwksURL == "" {
		return nil // Optional - returns nil if not configured
	}

	jwtValidator = services.NewJWTValidator(jwksURL)
	log.Printf("✅ [JWT] JWT Validator initialized with JWKS URL: %s", jwksURL)
	return nil
}

// JWTAuth validates JWT tokens from CitizenAuth (RS256 signature verification)
// This middleware performs LOCAL validation - no network calls
func JWTAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip if JWT validator not initialized
		if jwtValidator == nil {
			utils.AuthDebugLog("JWT validator not initialized, skipping JWT auth")
			return c.Next()
		}

		// Extract token from Authorization header (Bearer <token>)
		authHeader := strings.TrimSpace(c.Get("Authorization"))
		if authHeader == "" {
			utils.AuthDebugLog("No Authorization header found, skipping JWT auth")
			return c.Next()
		}

		fields := strings.Fields(authHeader)
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			utils.AuthDebugLog("Authorization header present but not Bearer format")
			return c.Next()
		}

		token := strings.TrimSpace(fields[1])
		if token == "" {
			utils.AuthDebugLog("Bearer token is empty, skipping JWT auth")
			return c.Next() // Allow other auth methods to try
		}

		// Validate JWT (LOCAL - signature verification only)
		claims, err := jwtValidator.ValidateToken(token)
		if err != nil {
			utils.AuthDebugLog("JWT validation failed: %v", err)
			return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
				false,
				"Invalid or expired JWT token",
				nil,
			))
		}

		utils.AuthDebugLog("JWT validated successfully for user: %s (%s)", claims.UserID, claims.Email)

		// Store user info in context
		c.Locals("auth_type", "jwt")
		c.Locals("user_id", claims.UserID)
		c.Locals("citizenauth_user_id", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("name", claims.Name)
		c.Locals("session_id", claims.SessionID)
		c.Locals("is_super_admin", claims.IsSuperAdmin)

		if claims.OrganizationID != nil {
			c.Locals("organization_id", *claims.OrganizationID)
		}

		if claims.Role != "" {
			c.Locals("role", claims.Role)
		}

		return c.Next()
	}
}

// RequireJWTAuth ensures JWT authentication is present
func RequireJWTAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authType := c.Locals("auth_type")

		if authType != "jwt" {
			return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
				false,
				"JWT authentication required",
				nil,
			))
		}

		return c.Next()
	}
}
