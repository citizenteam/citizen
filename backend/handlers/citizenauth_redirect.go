package handlers

import (
	"backend/services"
	"fmt"
	"net/url"
	"os"
	"log"
	"github.com/gofiber/fiber/v2"
)

var (
	jwtValidatorInstance *services.JWTValidator // Will be initialized on first use
	validatorInitialized = false
)

// getOrInitValidator returns JWT validator instance (lazy loading)
func getOrInitValidator() *services.JWTValidator {
	if !validatorInitialized {
		jwksURL := os.Getenv("CITIZENAUTH_JWKS_URL")
		if jwksURL != "" {
			jwtValidatorInstance = services.NewJWTValidator(jwksURL)
		}
		validatorInitialized = true
	}
	return jwtValidatorInstance
}

// RedirectToCitizenAuth redirects auth requests to CitizenAuth
// This is called when user tries to access Citizen without a JWT
func RedirectToCitizenAuth(c *fiber.Ctx) error {
	citizenAuthURL := os.Getenv("CITIZENAUTH_URL")
	if citizenAuthURL == "" {
		citizenAuthURL = "http://localhost:8080" // Fallback
	}

	// Build redirect URL (where to return after login)
	redirectURL := c.Query("redirect")
	if redirectURL == "" {
		// Build from current request
		proto := "http"
		if c.Get("X-Forwarded-Proto") == "https" || os.Getenv("FORCE_HTTPS") == "true" {
			proto = "https"
		}
		redirectURL = fmt.Sprintf("%s://%s%s", proto, c.Hostname(), c.Path())
	}

	// Use SSO Init flow (checks if already logged in on CitizenAuth)
	ssoInitURL := fmt.Sprintf("%s/sso/init?redirect=%s", citizenAuthURL, url.QueryEscape(redirectURL))

	log.Printf("🔄 [REDIRECT] Sending to SSO Init: %s", ssoInitURL)
	return c.Redirect(ssoInitURL, fiber.StatusTemporaryRedirect)
}

// ValidateJWTForTraefik validates JWT from CitizenAuth (ForwardAuth for Traefik)
// This endpoint is called by Traefik for EVERY request to protected resources
func ValidateJWTForTraefik(c *fiber.Ctx) error {
	// Disable caching (critical for security)
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")

	forwardedHost := c.Get("X-Forwarded-Host")
	forwardedURI := c.Get("X-Forwarded-Uri")

	// Allow public paths without authentication
	publicPaths := []string{"/health", "/api/v1/auth/", "/api/v1/service/", "/static/", "/assets/"}
	for _, path := range publicPaths {
		if len(forwardedURI) >= len(path) && forwardedURI[:len(path)] == path {
			return c.SendStatus(fiber.StatusOK)
		}
	}

	// Try to get JWT from cookie or Authorization header
	token := c.Cookies("sso_session")
	if token == "" {
		authHeader := c.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}
	}

	if token == "" {
		// No token found - return 401 (Traefik will handle redirect)
		log.Printf("⚠️  [FORWARDAUTH] No token found for %s%s", forwardedHost, forwardedURI)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	// Get JWT validator (lazy loading)
	validator := getOrInitValidator()
	
	// Validate JWT locally (fast path - no network call)
	if validator != nil {
		claims, err := validator.ValidateToken(token)
		if err == nil && claims != nil {
			// JWT valid - set auth headers for downstream app
			c.Set("X-Auth-User-ID", claims.UserID)
			c.Set("X-Auth-Email", claims.Email)
			c.Set("X-Auth-Name", claims.Name)
			c.Set("X-Auth-Session-ID", claims.SessionID)
			
			if claims.OrganizationID != nil {
				c.Set("X-Auth-Organization-ID", *claims.OrganizationID)
			}
			if claims.Role != "" {
				c.Set("X-Auth-Role", claims.Role)
			}
			if claims.IsSuperAdmin {
				c.Set("X-Auth-Super-Admin", "true")
			}
			
			log.Printf("✅ [FORWARDAUTH] JWT validated for %s", claims.Email)
			return c.SendStatus(fiber.StatusOK)
		}
		
		log.Printf("❌ [FORWARDAUTH] Invalid token: %v", err)
	}

	// Token invalid - return 401 (Traefik will handle redirect)
	return c.SendStatus(fiber.StatusUnauthorized)
}

func redirectToCitizenAuthLogin(c *fiber.Ctx, host, uri string) error {
	citizenAuthURL := os.Getenv("CITIZENAUTH_URL")
	if citizenAuthURL == "" {
		citizenAuthURL = "http://localhost:8080"
	}

	proto := c.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
	}

	originalURL := fmt.Sprintf("%s://%s%s", proto, host, uri)
	loginURL := fmt.Sprintf("%s/login?redirect=%s", citizenAuthURL, url.QueryEscape(originalURL))

	c.Set("Location", loginURL)
	return c.SendStatus(fiber.StatusTemporaryRedirect)
}

