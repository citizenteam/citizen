package handlers

import (

	"backend/services"
	"backend/utils"
	"log"
	"os"
	"backend/database"
	"github.com/gofiber/fiber/v2"
)

var callbackJWTValidator *services.JWTValidator

// SSOCallback handles SSO callback from CitizenAuth
// Converts CitizenAuth JWT → Citizen SSO session (local Redis)
// GET /sso/callback?token=<jwt>&redirect=<path>
func SSOCallback(c *fiber.Ctx) error {
	token := c.Query("token")
	redirect := c.Query("redirect", "/")
	
	log.Printf("📨 [SSO-CALLBACK] Received callback: redirect=%s", redirect)
	
	if token == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Missing token parameter")
	}
	
	// Initialize validator if not already
	if callbackJWTValidator == nil {
		jwksURL := os.Getenv("CITIZENAUTH_JWKS_URL")
		if jwksURL != "" {
			callbackJWTValidator = services.NewJWTValidator(jwksURL)
		}
	}
	
	// Validate CitizenAuth JWT
	if callbackJWTValidator == nil {
		log.Println("❌ [SSO-CALLBACK] JWT validator not configured")
		return c.Status(fiber.StatusServiceUnavailable).SendString("JWT validator not configured")
	}
	
	claims, err := callbackJWTValidator.ValidateToken(token)
	if err != nil {
		log.Printf("❌ [SSO-CALLBACK] Invalid token: %v", err)
		return c.Status(fiber.StatusUnauthorized).SendString("Invalid token")
	}
	
	log.Printf("✅ [SSO-CALLBACK] JWT validated: %s (%s)", claims.UserID, claims.Email)
	
	// Get or create local user (map CitizenAuth UUID to local user)
	var localUserID int
	query := `SELECT get_or_create_local_user($1, $2, $3)`
	err = database.DB.QueryRow(c.Context(), query, 
		claims.UserID, 
		claims.Email, 
		claims.Name,
	).Scan(&localUserID)
	
	if err != nil {
		log.Printf("❌ [SSO-CALLBACK] Failed to get/create local user: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to create user mapping")
	}
	
	log.Printf("🔗 [SSO-CALLBACK] Mapped CitizenAuth user %s to local user %d", claims.UserID, localUserID)
	
	// Create Citizen SSO session (local Redis)
	deviceID := c.Get("User-Agent")
	ssoSessionID := createOrUpdateSSOSession(localUserID, c.Hostname(), deviceID)
	
	log.Printf("🔄 [SSO-CALLBACK] Created local SSO session: %s for user %d", ssoSessionID, localUserID)
	
	// Set SSO session cookie (Citizen's own cookie)
	setSSOCookie(c, ssoSessionID, c.Hostname())
	
	log.Printf("🍪 [SSO-CALLBACK] SSO cookie set for domain: %s", c.Hostname())
	utils.SecurityLog("User %d LOGIN via CitizenAuth SSO - Session: %s", localUserID, ssoSessionID)
	
	log.Printf("➡️  [SSO-CALLBACK] Redirecting to: %s", redirect)
	
	// Redirect to original destination
	return c.Redirect(redirect, fiber.StatusTemporaryRedirect)
}

func isSecure() bool {
	forceHTTPS := os.Getenv("FORCE_HTTPS")
	return forceHTTPS == "true"
}

