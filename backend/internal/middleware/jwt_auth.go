package middleware

import (
	"backend/internal/services"
	"backend/internal/utils"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

var jwtValidator *services.JWTValidator
var deviceTokenValidator *services.DeviceTokenValidator

// InitJWTValidator initializes the JWT validator with JWKS URL
func InitJWTValidator() error {
	// Get LOGIN_HOST for constructing URLs
	loginHost := os.Getenv("LOGIN_HOST")
	if loginHost == "" {
		return nil // Optional - returns nil if not configured
	}

	// Determine protocol
	protocol := "https"
	if strings.Contains(loginHost, "localhost") {
		protocol = "http"
	}

	// Construct JWKS URL if not explicitly set
	jwksURL := os.Getenv("CITIZENAUTH_JWKS_URL")
	if jwksURL == "" {
		jwksURL = fmt.Sprintf("%s://%s/api/v1/auth/jwks.json", protocol, loginHost)
	}

	jwtValidator = services.NewJWTValidator(jwksURL)
	log.Printf("✅ [JWT] JWT Validator initialized with JWKS URL: %s", jwksURL)

	// Initialize device token validator (uses LOGIN_HOST internally)
	deviceTokenValidator = services.NewDeviceTokenValidator("")
	log.Printf("✅ [DeviceToken] Device Token Validator initialized with LOGIN_HOST: %s", loginHost)

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

		// 🔑 CHECK IF DEVICE TOKEN (cds_ prefix)
		if strings.HasPrefix(token, "cds_") {
			utils.AuthDebugLog("Detected device token (cds_ prefix)")

			// Validate device token via CitizenAuth API
			if deviceTokenValidator == nil {
				utils.AuthDebugLog("Device token validator not initialized")
				return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
					false,
					"Device token authentication not configured",
					nil,
				))
			}

			deviceClaims, err := deviceTokenValidator.ValidateToken(token)
			if err != nil {
				utils.AuthDebugLog("Device token validation failed: %v", err)
				return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
					false,
					"Invalid or expired device token",
					nil,
				))
			}

			utils.AuthDebugLog("Device token validated successfully for user: %s (%s)", deviceClaims.UserID, deviceClaims.Email)

			// Store user info in context (same format as JWT for compatibility)
			c.Locals("auth_type", "device_token")
			c.Locals("user_id", deviceClaims.UserID)
			c.Locals("citizenauth_user_id", deviceClaims.UserID)
			c.Locals("email", deviceClaims.Email)
			c.Locals("name", deviceClaims.Name)
			c.Locals("organization_id", deviceClaims.OrganizationID)
			c.Locals("role", deviceClaims.Role)
			c.Locals("is_super_admin", deviceClaims.IsSuperAdmin)
			c.Locals("device_token_scopes", deviceClaims.Scopes)

			return c.Next()
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
