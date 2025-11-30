package handlers

import (
	"backend/internal/auth/models"
	"backend/internal/auth/services"
	"backend/internal/database/api"
	appmodels "backend/internal/models"
	"backend/internal/utils"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Login function with SSO session creation
func Login(c *fiber.Ctx) error {
	redirectURL := c.Query("redirect")
	utils.RequestDebugLog(c.Method(), "/auth/login", "Redirect: %s", redirectURL)

	// GET request for login page
	if c.Method() == "GET" {
		if session, _ := validateAndGetSSOSession(c, ""); session != nil {
			// Already logged in with SSO
			if redirectURL != "" {
				return c.Redirect(redirectURL)
			}
			return c.Redirect("/")
		}

		return c.SendString("Login sayfası")
	}

	// POST request only
	if c.Method() != "POST" {
		return c.Status(fiber.StatusMethodNotAllowed).JSON(utils.NewCitizenResponse(
			false,
			"Method not allowed",
			nil,
		))
	}

	// Parse login data
	var loginData appmodels.UserLogin
	if err := c.BodyParser(&loginData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Geçersiz istek içeriği",
			nil,
		))
	}

	// Validate required fields
	if loginData.Username == "" || loginData.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Kullanıcı adı ve şifre zorunludur",
			nil,
		))
	}

	// Get user
	user, err := api.Users.GetUserByUsername(c.Context(), loginData.Username)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User not found",
			nil,
		))
	}

	// Check password
	if !utils.CheckPasswordHash(loginData.Password, user.Password) {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"Hatalı şifre",
			nil,
		))
	}

	// Create SSO session directly (no JWT needed)
	userID := int(user.ID)
	deviceID := c.Get("User-Agent")
	var organizationPtr *string
	if org, ok := c.Locals("organization_id").(string); ok && org != "" {
		organizationPtr = &org
	}
	ssoSessionID := services.CreateOrUpdateSSOSession(userID, c.Hostname(), deviceID, organizationPtr)

	currentHost := c.Hostname()
	loginHost := getLoginHost()

	utils.SessionDebugLog(ssoSessionID, "Storing SSO session for User: %d", userID)

	// Always set SSO session cookie for current host first
	cookieDomain := getCookieDomainForHost(currentHost)
	currentHostSameSite := getSameSitePolicy(currentHost)

	c.Cookie(&fiber.Cookie{
		Name:     "sso_session",
		Value:    ssoSessionID,
		Domain:   cookieDomain,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HTTPOnly: true,
		SameSite: currentHostSameSite,
		Secure:   isHttpsRequired(),
	})

	// Always set SSO session cookie for login host (unless we're already on login host)
	if currentHost != loginHost {
		utils.AuthDebugLog("Setting SSO session cookie for login host: %s", loginHost)

		loginCookieDomain := getCookieDomainForHost(loginHost)
		loginSameSitePolicy := getSameSitePolicy(loginHost)
		c.Cookie(&fiber.Cookie{
			Name:     "sso_session",
			Value:    ssoSessionID,
			Domain:   loginCookieDomain,
			Path:     "/",
			Expires:  time.Now().Add(24 * time.Hour),
			HTTPOnly: true,
			SameSite: loginSameSitePolicy, // Use dynamic policy based on host
			Secure:   isHttpsRequired(),
		})
	}

	// If redirect URL is for a custom domain, also set cookie for that domain
	if redirectURL != "" {
		if redirectURLParsed, err := url.Parse(redirectURL); err == nil {
			redirectHost := redirectURLParsed.Host

			// If redirect is to a custom domain (not login host or subdomain) and not current host
			if redirectHost != loginHost && !strings.HasSuffix(redirectHost, "."+loginHost) && redirectHost != currentHost {
				utils.AuthDebugLog("Setting SSO session cookie for custom domain: %s", redirectHost)

				// For custom domains, use domain-specific cookie strategy
				var customCookieDomain string
				var customSameSitePolicy string
				var customIsSecure bool

				// Custom domain - use Lax policy for cross-site compatibility
				customCookieDomain = ""                                                 // No domain set for custom domains
				customSameSitePolicy = "Lax"                                            // Use Lax for cross-site navigation compatibility
				customIsSecure = strings.HasPrefix(c.Get("X-Forwarded-Proto"), "https") // Check actual protocol

				utils.AuthDebugLog("Custom domain redirect detected, using Lax cookie policy for %s", redirectHost)

				// Set cookie for the custom domain as well
				c.Cookie(&fiber.Cookie{
					Name:     "sso_session",
					Value:    ssoSessionID,
					Domain:   customCookieDomain,
					Path:     "/",
					Expires:  time.Now().Add(24 * time.Hour),
					HTTPOnly: true,
					SameSite: customSameSitePolicy,
					Secure:   customIsSecure,
				})
			}
		}
	}

	utils.SecurityLog("User %d LOGIN - SSO Session: %s, Host: %s", userID, ssoSessionID, currentHost)

	// Response
	responseData := fiber.Map{
		"sso_session": ssoSessionID,
		"user": fiber.Map{
			"user_id":  user.ID,
			"username": user.Username,
		},
	}

	if redirectURL != "" {
		responseData["redirect_url"] = redirectURL
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Login successful",
		responseData,
	))
}

// Logout endpoint
func Logout(c *fiber.Ctx) error {
	// Get user ID from session
	var userID int
	if session, _ := validateAndGetSSOSession(c, ""); session != nil {
		userID = session.UserID
	}

	// Clear all SSO sessions for this user
	if userID != 0 {
		services.ClearUserSSOSessions(userID)
		log.Printf("[AUTH] Cleared all SSO sessions for user %d", userID)
	}

	currentHost := c.Hostname()
	loginHost := getLoginHost()

	log.Printf("[AUTH] Logout: Clearing cookies for host: %s", currentHost)

	// Clear cookie for current host
	if getDomainType(currentHost) == models.DomainTypeCustom {
		// For custom domains, use domain-specific policy
		config := getCookieConfig(currentHost, c.Get("X-Forwarded-Proto"))
		// Keep the original SameSite policy for clearing

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
	} else {
		// For login host or subdomain, use standard clearing
		clearSSOCookie(c, currentHost)
	}

	// Clear login host cookie if different
	if currentHost != loginHost {
		utils.AuthDebugLog("Clearing login host cookie during logout")

		// Use special config for login host (always SameSite=None)
		config := getCookieConfigForLoginHost(c.Get("X-Forwarded-Proto"))

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
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Çıkış successful",
		fiber.Map{
			"sso_sessions_cleared": userID != 0,
			"domain_cleared":       currentHost,
		},
	))
}

func redirectToLogin(c *fiber.Ctx, originalURL string) error {
	// Redirect to CitizenAuth SSO Init instead of local login
	citizenAuthURL := os.Getenv("CITIZENAUTH_URL")
	if citizenAuthURL == "" {
		utils.WarnLog("CITIZENAUTH_URL not set, cannot redirect to login")
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "CitizenAuth not configured",
		})
	}

	// Add CORS headers for redirect response
	origin := c.Get("Origin")
	if origin != "" {
		c.Set("Access-Control-Allow-Origin", origin)
		c.Set("Access-Control-Allow-Credentials", "true")
		c.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Origin,Content-Type,Accept,Authorization,X-Requested-With,Cookie")
	}

	ssoInitURL := fmt.Sprintf("%s/sso/init?redirect=%s", citizenAuthURL, url.QueryEscape(originalURL))
	log.Printf("[AUTH] Redirecting to SSO Init: %s", ssoInitURL)
	c.Set("Location", ssoInitURL)
	return c.SendStatus(fiber.StatusTemporaryRedirect)
}
