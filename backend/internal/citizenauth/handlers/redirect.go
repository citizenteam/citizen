package handlers

import (
	"backend/internal/database/api"
	appmodels "backend/internal/models"
	"backend/internal/rbac/domain"
	rbacservice "backend/internal/rbac/service"
	"backend/internal/services"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

var rbacSvc = rbacservice.Default()

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
// SECURITY: This function now validates app-level permissions (cross-app access prevention)
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
			// JWT valid - now check app-level permission (CRITICAL SECURITY CHECK)
			appName := extractAppNameFromHost(forwardedHost)

			// If this is an app domain (not main login domain), check app permission
			if appName != "" && claims.OrganizationID != nil {
				// Super admin bypasses app checks
				if !claims.IsSuperAdmin {
					// Check if user has permission for this specific app
					hasPermission, permErr := rbacSvc.CheckAppPermission(
						context.Background(),
						claims.UserID,
						*claims.OrganizationID,
						appName,
						domain.RoleViewer, // Minimum required role to access app
					)

					if permErr != nil {
						log.Printf("❌ [FORWARDAUTH] RBAC check failed for %s on app %s: %v", claims.Email, appName, permErr)
						return c.SendStatus(fiber.StatusForbidden)
					}

					if !hasPermission {
						log.Printf("🚫 [FORWARDAUTH] Access denied - %s has no permission for app %s", claims.Email, appName)
						return c.SendStatus(fiber.StatusForbidden)
					}

					log.Printf("✅ [FORWARDAUTH] App permission verified for %s on %s", claims.Email, appName)
				}
			}

			// Set auth headers for downstream app
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

			log.Printf("✅ [FORWARDAUTH] JWT validated for %s (app: %s)", claims.Email, appName)
			return c.SendStatus(fiber.StatusOK)
		}

		log.Printf("❌ [FORWARDAUTH] Invalid token: %v", err)
	}

	// Token invalid - return 401 (Traefik will handle redirect)
	return c.SendStatus(fiber.StatusUnauthorized)
}

// extractAppNameFromHost extracts app name from the host domain
func extractAppNameFromHost(host string) string {
	if host == "" {
		return ""
	}

	loginHost := os.Getenv("LOGIN_HOST")
	if loginHost == "" {
		loginHost = os.Getenv("APP_HOST")
	}

	// Check if it's the main login domain
	if host == loginHost || host == "www."+loginHost {
		return ""
	}

	// Check if it's a subdomain of main domain
	if strings.HasSuffix(host, "."+loginHost) {
		subdomain := strings.TrimSuffix(host, "."+loginHost)
		if subdomain == "www" {
			return ""
		}
		// Multi-level subdomain support: app2.whimsical-isle.amber-ridge.app.domain.com
		// First part is the app name
		if strings.Contains(subdomain, ".") {
			parts := strings.Split(subdomain, ".")
			return parts[0]
		}
		return subdomain
	}

	// Check custom domains from database
	domains, err := getActiveCustomDomainsFromDB()
	if err != nil {
		log.Printf("[FORWARDAUTH] Error fetching custom domains: %v", err)
		return ""
	}
	for _, d := range domains {
		if d.Domain == host {
			return d.AppName
		}
	}

	return ""
}

// getActiveCustomDomainsFromDB fetches active custom domains
func getActiveCustomDomainsFromDB() ([]appmodels.AppCustomDomain, error) {
	return api.Settings.GetAllActiveCustomDomains(context.Background())
}
