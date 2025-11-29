package handlers

import (
	"backend/auth/models"
	"backend/auth/services"
	"backend/utils"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
)

// setSSOCookie sets the SSO session cookie with appropriate configuration
func setSSOCookie(c *fiber.Ctx, sessionID string, host string) {
	config := getCookieConfig(host, c.Get("X-Forwarded-Proto"))

	c.Cookie(&fiber.Cookie{
		Name:     "sso_session",
		Value:    sessionID,
		Domain:   config.Domain,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HTTPOnly: true,
		SameSite: config.SameSite,
		Secure:   config.Secure,
	})

	utils.AuthDebugLog("Set SSO cookie for host %s", host)
}

// clearSSOCookie clears the SSO session cookie
func clearSSOCookie(c *fiber.Ctx, host string) {
	config := getCookieConfig(host, c.Get("X-Forwarded-Proto"))

	c.Cookie(&fiber.Cookie{
		Name:     "sso_session",
		Value:    "",
		Domain:   config.Domain,
		Path:     "/",
		Expires:  time.Now().Add(-24 * time.Hour),
		HTTPOnly: true,
		SameSite: config.SameSite,
		Secure:   config.Secure,
	})

	utils.AuthDebugLog("Cleared SSO cookie for host %s", host)
}

// validateAndGetSSOSession validates SSO session from cookie only (secure approach)
func validateAndGetSSOSession(c *fiber.Ctx, forwardedUri string) (*models.SSOSession, string) {
	// Debug: Log all cookies
	allCookies := c.Get("Cookie")
	utils.AuthDebugLog("All cookies received: '%s'", allCookies)

	// Use cookie only for security - no URL parameters that can leak
	if sessionID := c.Cookies("sso_session"); sessionID != "" {
		utils.AuthDebugLog("SSO session cookie found: '%s'", sessionID)
		if session, err := services.GetSSOSession(sessionID); err == nil && session != nil {
			utils.AuthDebugLog("SSO session valid for user: %d", session.UserID)
			return session, sessionID
		} else {
			utils.AuthDebugLog("SSO session invalid/expired: %v", err)
		}
	} else {
		utils.AuthDebugLog("No sso_session cookie found")
	}

	return nil, ""
}

// SSO Init endpoint - iframe-based cookie setting for custom domains
func SSOInit(c *fiber.Ctx) error {
	targetURL := c.Query("target")
	if targetURL == "" {
		targetURL = "/"
	}

	utils.RequestDebugLog("GET", "/sso/init", "SSO Init page requested for target: %s", targetURL)

	// Check if user is already authenticated on this domain
	if session, _ := validateAndGetSSOSession(c, ""); session != nil {
		// User is authenticated - direct redirect (custom domains now handle redirect at Traefik level)
		utils.AuthDebugLog("User %d authenticated, redirecting to: %s", session.UserID, targetURL)
		return c.Redirect(targetURL, fiber.StatusTemporaryRedirect)
	}

	// No valid authentication, redirect to CitizenAuth SSO Init
	citizenAuthURL := os.Getenv("CITIZENAUTH_URL")
	if citizenAuthURL == "" {
		utils.WarnLog("CITIZENAUTH_URL not set, SSO redirect will fail")
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "CitizenAuth not configured",
		})
	}

	ssoInitURL := fmt.Sprintf("%s/sso/init?redirect=%s", citizenAuthURL, url.QueryEscape(targetURL))
	utils.AuthDebugLog("No authentication found, redirecting to SSO Init: %s", ssoInitURL)
	return c.Redirect(ssoInitURL, fiber.StatusTemporaryRedirect)
}

// SSO Check endpoint - Microsoft style (called by hidden iframe)
func SSOCheck(c *fiber.Ctx) error {
	origin := c.Get("Origin")

	utils.RequestDebugLog("GET", "/sso/check", "Origin: '%s', Host: '%s'", origin, c.Hostname())

	// Validate origin
	if origin != "" && !isAllowedOrigin(origin) {
		utils.SecurityLog("SSO Check - Origin not allowed: %s", origin)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Invalid origin",
		})
	}

	// Get SSO session
	session, sessionID := validateAndGetSSOSession(c, "")

	allowedOrigin := origin
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}

	if session == nil {
		return c.Type("html").SendString(getSSOCheckHTML(false, "", allowedOrigin))
	}

	// Update last activity
	session.LastActivity = time.Now()

	// Set cookie for custom domain if needed
	if origin != "" {
		if parsedOrigin, err := url.Parse(origin); err == nil {
			originHost := parsedOrigin.Host
			if getDomainType(originHost) == models.DomainTypeCustom {
				utils.AuthDebugLog("Setting SSO session cookie for custom domain origin: %s", originHost)

				// For SSO Check, use Lax for custom domains as per original logic
				config := getCookieConfig(originHost, c.Get("X-Forwarded-Proto"))
				config.SameSite = "Lax" // Override to Lax for cross-site iframe compatibility

				c.Cookie(&fiber.Cookie{
					Name:     "sso_session",
					Value:    sessionID,
					Domain:   config.Domain,
					Path:     "/",
					Expires:  time.Now().Add(24 * time.Hour),
					HTTPOnly: true,
					SameSite: config.SameSite,
					Secure:   config.Secure,
				})
			}
		}
	}

	return c.Type("html").SendString(getSSOCheckHTML(true, sessionID, allowedOrigin))
}

// getSSOCheckHTML returns HTML for SSO check iframe
func getSSOCheckHTML(authenticated bool, ssoSessionID string, allowedOrigin string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <script>
    (function() {
        var authenticated = %v;
        var ssoSessionID = "%s";
        var allowedOrigin = "%s";
        
        if (window.parent !== window) {
            var message = {
                type: 'sso-check-result',
                authenticated: authenticated
            };
            
            if (authenticated && ssoSessionID) {
                message.ssoSessionID = ssoSessionID;
            }
            
            window.parent.postMessage(message, allowedOrigin);
        }
    })();
    </script>
</head>
<body></body>
</html>
`, authenticated, ssoSessionID, allowedOrigin)
}
