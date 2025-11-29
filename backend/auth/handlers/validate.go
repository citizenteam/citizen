package handlers

import (
	authservices "backend/auth/services"
	"backend/database/api"
	"backend/tokens"
	"backend/utils"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// ValidateForTraefik - ForwardAuth validation endpoint
func ValidateForTraefik(c *fiber.Ctx) error {
	// Disable caching
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")

	// Handle OPTIONS preflight requests - always allow
	if c.Method() == "OPTIONS" || c.Get("X-Forwarded-Method") == "OPTIONS" {
		origin := c.Get("Origin")
		if origin != "" {
			c.Set("Access-Control-Allow-Origin", origin)
			c.Set("Access-Control-Allow-Credentials", "true")
			c.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			c.Set("Access-Control-Allow-Headers", "Origin,Content-Type,Accept,Authorization,X-Requested-With,Cookie")
			c.Set("Access-Control-Max-Age", "86400")
		}
		return c.SendStatus(fiber.StatusOK)
	}

	// Get forwarded headers
	forwardedHost := c.Get("X-Forwarded-Host")
	forwardedUri := c.Get("X-Forwarded-Uri")

	// Get Authorization header (Traefik forwards it)
	authHeader := strings.TrimSpace(c.Get("Authorization"))

	if authHeader == "" {
		authHeader = strings.TrimSpace(c.Get("X-Forwarded-Authorization"))
	}

	// Normalize common empty placeholders
	if strings.EqualFold(authHeader, "Bearer null") || strings.EqualFold(authHeader, "Bearer") {
		authHeader = ""
	}

	var bearerToken string
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		bearerToken = strings.TrimSpace(authHeader[len("Bearer "):])
		if bearerToken == "" || strings.EqualFold(bearerToken, "null") {
			bearerToken = ""
			authHeader = ""
		}
	}

	// Log all headers for debugging
	log.Printf("🔍 [VALIDATE] URI: %s, Authorization: %v, Cookie: %v",
		forwardedUri,
		authHeader != "",
		c.Get("Cookie") != "")

	if authHeader != "" {
		headerPreview := authHeader
		if bearerToken != "" {
			if len(bearerToken) > 8 {
				headerPreview = fmt.Sprintf("Bearer %s…%s", bearerToken[:4], bearerToken[len(bearerToken)-4:])
			} else {
				headerPreview = fmt.Sprintf("Bearer (len=%d)", len(bearerToken))
			}
		} else if len(authHeader) > 30 {
			headerPreview = authHeader[:30] + "..."
		}
		log.Printf("🔑 [VALIDATE] Auth header: %s", headerPreview)
	} else {
		log.Printf("🔑 [VALIDATE] Auth header: (missing)")
	}

	utils.RequestDebugLog("VALIDATE", forwardedUri, "Host: %s, Auth: %v", forwardedHost, authHeader != "")

	// Check public paths
	if IsPublicPath(forwardedUri) ||
		strings.HasPrefix(forwardedUri, "/login") ||
		strings.HasPrefix(forwardedUri, "/sso/") ||
		strings.HasPrefix(forwardedUri, "/api/v1/auth/validate") {
		utils.AuthDebugLog("Public path accessed, allowing. URI: %s", forwardedUri)
		return c.SendStatus(fiber.StatusOK)
	}

	// Check public apps
	appName := extractAppNameFromHost(forwardedHost)
	if appName != "" && isAppPublic(appName) {
		utils.AuthDebugLog("Public app accessed, allowing. App: %s", appName)
		return c.SendStatus(fiber.StatusOK)
	}

	// Get Authorization header (already extracted above)
	if bearerToken != "" {
		// This is a JWT token from CitizenAuth - validate it directly
		// Try JWT validation first
		if jwtValidator := authservices.GetJWTValidator(); jwtValidator != nil {
			claims, err := jwtValidator.ValidateToken(bearerToken)
			if err == nil {
				log.Printf("✅ [VALIDATE] Authenticated via JWT: %s (%s)", claims.UserID, claims.Email)
				utils.AuthDebugLog("JWT validated via ForwardAuth: %s", claims.Email)
				return c.SendStatus(fiber.StatusOK)
			}
		}
	}

	// Try API Token authentication (if app has API access enabled)
	if appName != "" {
		queryToken := c.Query("token")

		if token := tokens.ExtractAPIToken(authHeader, queryToken); token != "" && appName != "" {
			// Check if app has API access enabled (simple on/off check)
			hasAccess, err := api.AppAPIAccess.IsAppAPIAccessEnabled(c.Context(), appName)

			if err == nil && hasAccess {
				// Validate API token
				user, err := api.APITokens.ValidateAPIToken(c.Context(), token)
				if err == nil && user != nil {
					utils.AuthDebugLog("API token validation successful for app: %s, User: %d", appName, user.ID)

					// Update token usage asynchronously
					go func() {
						api.APITokens.UpdateTokenUsage(c.Context(), token, c.IP())
					}()

					return c.SendStatus(fiber.StatusOK)
				}
				utils.AuthDebugLog("API token validation failed: %v", err)
			}
		}
	}

	// Fallback to SSO session validation
	session, _ := validateAndGetSSOSession(c, forwardedUri)

	if session == nil {
		utils.AuthDebugLog("No valid authentication found for host: %s", forwardedHost)

		originalURL := c.Get("X-Forwarded-Proto") + "://" + forwardedHost + forwardedUri

		// Always redirect to CitizenAuth SSO Init
		return redirectToLogin(c, originalURL)
	}

	// SSO session validated
	log.Printf("✅ [VALIDATE] Authenticated via SSO session: user_id=%d", session.UserID)
	utils.AuthDebugLog("SSO session validation successful for host: %s, User: %d", forwardedHost, session.UserID)
	return c.SendStatus(fiber.StatusOK)
}

// ValidateSessionEndpoint - API endpoint for SSO session validation (keeping token-validate path for compatibility)
func ValidateSessionEndpoint(c *fiber.Ctx) error {
	log.Printf("[AUTH] ValidateSessionEndpoint called from IP: %s", c.IP())

	session, _ := validateAndGetSSOSession(c, "")
	if session == nil {
		log.Printf("[AUTH] ValidateSessionEndpoint - No valid SSO session found")
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"SSO session bulunamadı",
			nil,
		))
	}

	log.Printf("[AUTH] ValidateSessionEndpoint - Valid SSO session found for user: %d", session.UserID)

	// Get user details
	user, err := api.Users.GetUserByID(c.Context(), session.UserID)
	if err != nil {
		log.Printf("[AUTH] ValidateSessionEndpoint - User not found: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not found",
			nil,
		))
	}

	log.Printf("[AUTH] ValidateSessionEndpoint - Success for user: %s", user.Username)
	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"SSO session geçerli",
		fiber.Map{
			"user_id":  session.UserID,
			"username": user.Username,
		},
	))
}
