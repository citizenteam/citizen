package handlers

import (
	authservices "backend/internal/auth/services"
	"backend/internal/database"
	"backend/internal/database/api"
	"backend/internal/rbac/domain"
	rbacservice "backend/internal/rbac/service"
	"backend/internal/tokens"
	"backend/internal/utils"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

var rbacSvc = rbacservice.Default()

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
		// Check if it's a device session token (cds_ prefix)
		if strings.HasPrefix(bearerToken, "cds_") {
			log.Printf("🔐 [VALIDATE] Device token detected, validating with CitizenAuth...")

			// Validate device token with CitizenAuth
			validationResult, err := authservices.ValidateDeviceToken(c.Context(), bearerToken)
			if err == nil && validationResult != nil {
				log.Printf("✅ [VALIDATE] Authenticated via device token: %s (%s) org=%s",
					validationResult.UserID, validationResult.Email, validationResult.OrganizationID)
				utils.AuthDebugLog("Device token validated: %s", validationResult.Email)

				// Set user context for downstream services
				c.Locals("citizenauth_user_id", validationResult.UserID)
				c.Locals("organization_id", validationResult.OrganizationID)
				c.Locals("user_role", validationResult.Role)

				return c.SendStatus(fiber.StatusOK)
			}
			log.Printf("⚠️  [VALIDATE] Device token validation failed: %v", err)
		}

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
	// NOTE: Query parameter token support removed for security
	if appName != "" {
		if token := tokens.ExtractAPIToken(authHeader); token != "" && appName != "" {
			// Check if app has API access enabled (simple on/off check)
			hasAccess, err := api.AppAPIAccess.IsAppAPIAccessEnabled(c.Context(), appName)

			if err == nil && hasAccess {
				// Validate API token with full context for audit logging
				clientIP := c.IP()
				userAgent := c.Get("User-Agent")
				citizenauthUserID, _ := c.Locals("citizenauth_user_id").(string)
				orgID, _ := c.Locals("organization_id").(string)
				user, err := api.APITokens.ValidateAPIToken(c.Context(), token, clientIP, userAgent, appName, citizenauthUserID, orgID)
				if err == nil && user != nil {
					utils.AuthDebugLog("API token validation successful for app: %s, User: %d", appName, user.ID)

					// Update token usage asynchronously with independent context
					// Note: c.Context() cannot be used in goroutines as it becomes invalid after response
					go func(tokenCopy, ipCopy string) {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						api.APITokens.UpdateTokenUsage(ctx, tokenCopy, ipCopy)
					}(token, clientIP)

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

	// SSO session validated - now check app-level RBAC permission
	if appName != "" {
		// Get CitizenAuth user ID and org ID from session
		citizenAuthUserID, orgID, err := getUserContextFromSession(c.Context(), session.UserID)
		if err != nil {
			log.Printf("❌ [VALIDATE] Failed to get user context for RBAC: %v", err)
			return c.SendStatus(fiber.StatusForbidden)
		}

		if citizenAuthUserID != "" && orgID != "" {
			// Check app permission using cached RBAC
			hasPermission, permErr := rbacSvc.CheckAppPermission(
				c.Context(),
				citizenAuthUserID,
				orgID,
				appName,
				domain.RoleViewer,
			)

			if permErr != nil {
				log.Printf("❌ [VALIDATE] RBAC check failed for user %d on app %s: %v", session.UserID, appName, permErr)
				return c.SendStatus(fiber.StatusForbidden)
			}

			if !hasPermission {
				log.Printf("🚫 [VALIDATE] Access denied - user %d has no permission for app %s", session.UserID, appName)
				return c.SendStatus(fiber.StatusForbidden)
			}

			log.Printf("✅ [VALIDATE] App permission verified for user %d on %s", session.UserID, appName)
		}
	}

	// SSO session validated with RBAC
	log.Printf("✅ [VALIDATE] Authenticated via SSO session: user_id=%d (app: %s)", session.UserID, appName)
	utils.AuthDebugLog("SSO session validation successful for host: %s, User: %d", forwardedHost, session.UserID)
	return c.SendStatus(fiber.StatusOK)
}

// getUserContextFromSession retrieves CitizenAuth user ID and org ID from local user
func getUserContextFromSession(ctx context.Context, localUserID int) (string, string, error) {
	query := `
		SELECT citizenauth_user_id, organization_id 
		FROM users 
		WHERE id = $1
	`
	var citizenAuthUserID, orgID string
	err := database.DB.QueryRow(ctx, query, localUserID).Scan(&citizenAuthUserID, &orgID)
	if err != nil {
		return "", "", err
	}
	return citizenAuthUserID, orgID, nil
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
