package handlers

import (
	"backend/database"
	"backend/database/api"
	"backend/models"
	"backend/utils"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Removed JWT token denylist - system uses SSO sessions instead

// SSO sessions - for cross-domain authentication
var (
	ssoSessions = make(map[string]*SSOSession)
	ssoMutex    = &sync.RWMutex{}
)

// SSOSession structure
type SSOSession struct {
	SessionID    string
	UserID       int
	MainDomain   string
	DeviceID     string
	CreatedAt    time.Time
	LastActivity time.Time
	ExpiresAt    time.Time
}

// Domain types
type DomainType int

const (
	DomainTypeLogin DomainType = iota
	DomainTypeSubdomain
	DomainTypeCustom
)

// Cookie configuration
type CookieConfig struct {
	Domain   string
	SameSite string
	Secure   bool
}

// Base public paths that are always allowed
var basePublicPaths = []string{
	"/login",
	"/register",
	"/sso/check",
	"/sso/init",
	"/health",
	"/static/",
	"/favicon.ico",
	"/robots.txt",
	"/api/v1/auth/login",
	"/api/v1/auth/register",
	"/api/v1/auth/validate",
	"/.well-known/acme-challenge/",
	"/vite.svg",
	"/assets/",
	".css", ".js", ".mjs", ".ico", ".png", ".jpg", ".jpeg",
	".gif", ".svg", ".woff", ".woff2", ".ttf", ".eot", ".map",
}

// Development-only paths
var developmentPaths = []string{
	"/node_modules/", "/src/", "/@vite/", "/@fs/", "/@id/",
	"/__vite_ping", ".tsx", ".ts", ".jsx", ".vue",
	".scss", ".sass", ".less", ".styl",
}

// ==================== Helper Functions ====================

// getLoginHost returns the login host from env or default
func getLoginHost() string {
	if host := os.Getenv("LOGIN_HOST"); host != "" {
		return host
	}
	return "localhost"
}

// getDomainType determines the type of domain
func getDomainType(host string) DomainType {
	loginHost := getLoginHost()
	
	if host == loginHost || host == "www."+loginHost {
		return DomainTypeLogin
	}
	
	if strings.HasSuffix(host, "."+loginHost) {
		return DomainTypeSubdomain
	}
	
	return DomainTypeCustom
}

// getCookieConfig returns appropriate cookie configuration for a host
func getCookieConfig(host string, forwardedProto string) CookieConfig {
	domainType := getDomainType(host)
	config := CookieConfig{}
	loginHost := getLoginHost()
	
	// Determine domain
	switch domainType {
	case DomainTypeCustom:
		config.Domain = "" // No domain for custom domains
	case DomainTypeLogin, DomainTypeSubdomain:
		if strings.Contains(host, "localhost") {
			config.Domain = "" // No domain for localhost
		} else {
			config.Domain = "." + loginHost
		}
	}
	
	// Determine SameSite and Secure
	isHTTPS := isHttpsRequired()
	
	if strings.Contains(host, "localhost") {
		config.SameSite = "Lax"
		config.Secure = false
	} else if domainType == DomainTypeCustom {
		if isHTTPS {
			config.SameSite = "None"
			config.Secure = true
		} else {
			config.SameSite = "Lax"
			config.Secure = false
		}
	} else {
		// Login host or subdomain
		if isHTTPS {
			config.SameSite = "None"
			config.Secure = true
		} else {
			config.SameSite = "Lax"
			config.Secure = false
		}
	}
	
	// Override secure if protocol indicates HTTPS
	if strings.HasPrefix(forwardedProto, "https") {
		config.Secure = true
	}
	
	utils.AuthDebugLog("getCookieConfig('%s') = domain:'%s', sameSite:'%s', secure:%v", 
		host, config.Domain, config.SameSite, config.Secure)
	
	return config
}

// getCookieConfigForLoginHost returns cookie configuration specifically for login host
// This always uses SameSite=None for cross-domain SSO functionality
func getCookieConfigForLoginHost(forwardedProto string) CookieConfig {
	loginHost := getLoginHost()
	config := CookieConfig{}
	
	if strings.Contains(loginHost, "localhost") {
		config.Domain = ""
		config.SameSite = "Lax"
		config.Secure = false
	} else {
		config.Domain = "." + loginHost
		config.SameSite = "None" // Always None for login host for cross-domain SSO
		config.Secure = isHttpsRequired()
	}
	
	// Override secure if protocol indicates HTTPS
	if strings.HasPrefix(forwardedProto, "https") {
		config.Secure = true
	}
	
	utils.AuthDebugLog("getCookieConfigForLoginHost() = domain:'%s', sameSite:'%s', secure:%v", 
		config.Domain, config.SameSite, config.Secure)
	
	return config
}

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

// extractSSOSessionFromURI removed - using cookie-only approach for security

// buildSSOInitURL builds the SSO initialization URL
func buildSSOInitURL(targetURL string) string {
	protocol := "http://"
	if isHttpsRequired() {
		protocol = "https://"
	}
	
	loginHost := getLoginHost()
	return fmt.Sprintf("%s%s/sso/init?target=%s", protocol, loginHost, url.QueryEscape(targetURL))
}

// buildLoginURL builds the login URL with redirect
func buildLoginURL(targetURL string) string {
	protocol := "http://"
	if isHttpsRequired() {
		protocol = "https://"
	}
	
	loginHost := getLoginHost()
	cleanedURL := cleanViteParams(targetURL)
	
	if isHttpsRequired() && strings.HasPrefix(cleanedURL, "http://") {
		cleanedURL = strings.Replace(cleanedURL, "http://", "https://", 1)
	}
	
	return fmt.Sprintf("%s%s/login?redirect=%s", protocol, loginHost, url.QueryEscape(cleanedURL))
}

// validateAndGetSSOSession validates SSO session from cookie only (secure approach)
func validateAndGetSSOSession(c *fiber.Ctx, forwardedUri string) (*SSOSession, string) {
	// Debug: Log all cookies
	allCookies := c.Get("Cookie")
	utils.AuthDebugLog("All cookies received: '%s'", allCookies)
	
	// Use cookie only for security - no URL parameters that can leak
	if sessionID := c.Cookies("sso_session"); sessionID != "" {
		utils.AuthDebugLog("SSO session cookie found: '%s'", sessionID)
		if session, err := GetSSOSession(sessionID); err == nil && session != nil {
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

// getPublicPaths returns environment-appropriate public paths
func getPublicPaths() []string {
	paths := make([]string, len(basePublicPaths))
	copy(paths, basePublicPaths)
	
	if utils.IsDevelopmentEnvironment() {
		paths = append(paths, developmentPaths...)
	}
	
	return paths
}

// isPublicPath checks if a path is public
func isPublicPath(uri string) bool {
	cleanURI := uri
	if queryIndex := strings.Index(uri, "?"); queryIndex != -1 {
		cleanURI = uri[:queryIndex]
	}
	
	publicPaths := getPublicPaths()
	
	for _, path := range publicPaths {
		if strings.HasPrefix(uri, path) {
			return true
		}
		
		if strings.HasPrefix(path, ".") && strings.HasSuffix(cleanURI, path) {
			return true
		}
	}
	return false
}

// ==================== Core Functions ====================

// Generate secure random ID
func generateSecureID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.URLEncoding.EncodeToString(b)
}

// Create or update SSO session
func createOrUpdateSSOSession(userID int, mainDomain string, deviceID string) string {
	sessionID := generateSecureID()
	
	session := &SSOSession{
		SessionID:    sessionID,
		UserID:       userID,
		MainDomain:   mainDomain,
		DeviceID:     deviceID,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	
	// Store in memory
	ssoMutex.Lock()
	ssoSessions[sessionID] = session
	ssoMutex.Unlock()
	
	// Store in Redis if available
	if data, err := json.Marshal(session); err == nil {
		database.SetWithTTL("sso_session:"+sessionID, string(data), 24*time.Hour)
	}
	
	return sessionID
}

// GetSSOSession retrieves an SSO session by ID
func GetSSOSession(sessionID string) (*SSOSession, error) {
	utils.SessionDebugLog(sessionID, "GetSSOSession called")
	
	// Try Redis first
	if data, err := database.Get("sso_session:" + sessionID); err == nil && data != "" {
		utils.SessionDebugLog(sessionID, "Found session in Redis")
		var session SSOSession
		if err := json.Unmarshal([]byte(data), &session); err == nil {
			if time.Now().After(session.ExpiresAt) {
				utils.SessionDebugLog(sessionID, "Session expired in Redis. ExpiresAt: %v, Now: %v", session.ExpiresAt, time.Now())
				return nil, fmt.Errorf("session expired")
			}
			utils.SessionDebugLog(sessionID, "Valid session found in Redis, UserID: %d", session.UserID)
			return &session, nil
		} else {
			utils.SessionDebugLog(sessionID, "Failed to unmarshal Redis data: %v", err)
		}
	} else {
		utils.SessionDebugLog(sessionID, "Session not found in Redis: %v", err)
	}
	
	// Fallback to memory
	ssoMutex.RLock()
	defer ssoMutex.RUnlock()
	
	session, exists := ssoSessions[sessionID]
	if !exists {
		utils.SessionDebugLog(sessionID, "Session not found in memory")
		return nil, fmt.Errorf("session not found")
	}
	
	if time.Now().After(session.ExpiresAt) {
		utils.SessionDebugLog(sessionID, "Session expired in memory. ExpiresAt: %v, Now: %v", session.ExpiresAt, time.Now())
		return nil, fmt.Errorf("session expired")
	}
	
	utils.SessionDebugLog(sessionID, "Valid session found in memory, UserID: %d", session.UserID)
	return session, nil
}

// Clear all SSO sessions for a user (global logout)
func clearUserSSOSessions(userID int) {
	ssoMutex.Lock()
	defer ssoMutex.Unlock()
	
	for sessionID, session := range ssoSessions {
		if session.UserID == userID {
			delete(ssoSessions, sessionID)
			database.Delete("sso_session:" + sessionID)
		}
	}
}

// ==================== HTTP Handlers ====================

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
	
	// No valid authentication, redirect to login
	loginURL := buildLoginURL(targetURL)
	utils.AuthDebugLog("No authentication found, redirecting to login: %s", loginURL)
	return c.Redirect(loginURL, fiber.StatusTemporaryRedirect)
}

// SSOSetCookie endpoint removed - custom domains now use Traefik redirect instead of iframe cookies

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
			if getDomainType(originHost) == DomainTypeCustom {
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
	var loginData models.UserLogin
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
	ssoSessionID := createOrUpdateSSOSession(userID, c.Hostname(), deviceID)

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
				customCookieDomain = "" // No domain set for custom domains
				customSameSitePolicy = "Lax" // Use Lax for cross-site navigation compatibility
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
	
	utils.SecurityLog("User %s LOGIN - SSO Session: %s, Host: %s", userID, ssoSessionID, currentHost)

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

// ValidateForTraefik - ForwardAuth validation endpoint
func ValidateForTraefik(c *fiber.Ctx) error {
	// Disable caching
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")

	// Get forwarded headers
	forwardedHost := c.Get("X-Forwarded-Host")
	forwardedUri := c.Get("X-Forwarded-Uri")
	utils.RequestDebugLog("VALIDATE", forwardedUri, "Host: %s, IP: %s", forwardedHost, c.IP())

	// Check public paths
	if isPublicPath(forwardedUri) ||
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

	// Validate SSO session
	session, _ := validateAndGetSSOSession(c, forwardedUri)
	
	if session == nil {
		utils.AuthDebugLog("No valid SSO session found for host: %s", forwardedHost)
		
		originalURL := c.Get("X-Forwarded-Proto") + "://" + forwardedHost + forwardedUri
		
		// Check if we need SSO init
		domainType := getDomainType(forwardedHost)
		if domainType == DomainTypeSubdomain || (domainType == DomainTypeCustom && appName != "") {
			ssoInitURL := buildSSOInitURL(originalURL)
			utils.AuthDebugLog("Redirecting to SSO init: %s", ssoInitURL)
			return c.Redirect(ssoInitURL, fiber.StatusTemporaryRedirect)
		}
		
		// Direct login redirect
		return redirectToLogin(c, originalURL)
	}
	
	// Session validated from secure cookie only

	utils.AuthDebugLog("SSO session validation successful for host: %s, User: %d", forwardedHost, session.UserID)
	return c.SendStatus(fiber.StatusOK)
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
		clearUserSSOSessions(userID)
		log.Printf("[AUTH] Cleared all SSO sessions for user %d", userID)
	}

	currentHost := c.Hostname()
	loginHost := getLoginHost()

	log.Printf("[AUTH] Logout: Clearing cookies for host: %s", currentHost)

	// Clear cookie for current host
	if getDomainType(currentHost) == DomainTypeCustom {
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

/*
// Register endpoint
func Register(c *fiber.Ctx) error {
	var user models.UserRegister
	if err := c.BodyParser(&user); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Geçersiz istek içeriği",
			nil,
		))
	}

	if user.Username == "" || user.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Kullanıcı adı ve şifre zorunludur",
			nil,
		))
	}

	// Hash password
	hashedPassword, err := utils.HashPassword(user.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Şifre hashleme error",
			nil,
		))
	}

	// Check if user exists
	exists, err := api.Users.UserExists(c.Context(), user.Username, user.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Veritabanı error",
			nil,
		))
	}
	if exists {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Bu kullanıcı adı zaten kullanılıyor",
			nil,
		))
	}

	// Create user
	newUser := &models.User{
		Username: user.Username,
		Email:    user.Email,
		Password: hashedPassword,
	}
	
	if err := api.Users.CreateUser(c.Context(), newUser); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Kullanıcı oluşturma error",
			nil,
		))
	}

	// Generate token
	token, err := utils.GenerateToken(int(newUser.ID))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Token oluşturulurken hata oluştu",
			nil,
		))
	}

	return c.Status(fiber.StatusCreated).JSON(utils.NewCitizenResponse(
		true,
		"Kullanıcı başarıyla saved",
		fiber.Map{
			"user":  newUser,
			"token": token,
		},
	))
}
*/

// GetProfile endpoint
func GetProfile(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)

	user, err := api.Users.GetUserByID(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(utils.NewCitizenResponse(
			false,
			"User not found",
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Profil başarıyla getirildi",
		user,
	))
}

// getCookieDomainForHost returns the cookie domain for a given host
func getCookieDomainForHost(host string) string {
	loginDomain := getLoginHost()
	
	if strings.Contains(host, "localhost") {
		// For localhost development, set .localhost domain for subdomain sharing
		utils.AuthDebugLog("getCookieDomainForHost('%s') = '.localhost' (localhost subdomain support)", host)
		return ".localhost"
	}
	
	if host == loginDomain || strings.HasSuffix(host, "."+loginDomain) {
		utils.AuthDebugLog("getCookieDomainForHost('%s') = '.%s' (login domain/subdomain)", host, loginDomain)
		return "." + loginDomain
	}
	
	domains, err := getActiveCustomDomainsFromDB()
	if err != nil {
		log.Printf("[AUTH] Error fetching custom domains: %v", err)
		utils.AuthDebugLog("getCookieDomainForHost('%s') = '' (error fetching domains)", host)
		return ""
	}

	for _, domain := range domains {
		if domain.Domain == host {
			// For custom domains, don't set domain - let browser handle it per host
			utils.AuthDebugLog("getCookieDomainForHost('%s') = '' (custom domain)", host)
			return ""
		}
	}

	utils.AuthDebugLog("getCookieDomainForHost('%s') = '' (not found)", host)
	return ""
}

// getSameSitePolicy returns appropriate SameSite policy based on host
func getSameSitePolicy(host string) string {
	if strings.Contains(host, "localhost") {
		utils.AuthDebugLog("getSameSitePolicy('%s') = 'Lax' (localhost)", host)
		return "Lax"
	}
	
	loginDomain := getLoginHost()
	
	// For custom domains, check if HTTPS is required
	if host != loginDomain && !strings.HasSuffix(host, "."+loginDomain) {
		// Custom domain - for cross-domain cookies we need SameSite=None and Secure=true
		// But only if HTTPS is enabled
		if isHttpsRequired() {
			utils.AuthDebugLog("getSameSitePolicy('%s') = 'None' (custom domain, HTTPS)", host)
			return "None"
		} else {
			// In development without HTTPS, use Lax for custom domains
			utils.AuthDebugLog("getSameSitePolicy('%s') = 'Lax' (custom domain, no HTTPS)", host)
			return "Lax"
		}
	}
	
	// For subdomains of login domain, use None for cross-domain functionality (with HTTPS)
	if isHttpsRequired() {
		utils.AuthDebugLog("getSameSitePolicy('%s') = 'None' (production/subdomain, HTTPS)", host)
		return "None"
	} else {
		utils.AuthDebugLog("getSameSitePolicy('%s') = 'Lax' (production/subdomain, no HTTPS)", host)
		return "Lax"
	}
}

func isHttpsRequired() bool {
	forceHttps := os.Getenv("FORCE_HTTPS")
	if forceHttps == "" {
		forceHttps = "true"
	}
	
	result := forceHttps == "true"
	utils.AuthDebugLog("isHttpsRequired() = %v (FORCE_HTTPS='%s')", result, forceHttps)
	return result
}

func extractAppNameFromHost(host string) string {
	if host == "" {
		return ""
	}

	loginHost := getLoginHost()
	domainType := getDomainType(host)

	switch domainType {
	case DomainTypeLogin:
		return ""
	case DomainTypeSubdomain:
		subdomain := strings.TrimSuffix(host, "."+loginHost)
		if !strings.Contains(subdomain, ".") && subdomain != "www" {
			return subdomain
		}
		return ""
	case DomainTypeCustom:
		domains, err := getActiveCustomDomainsFromDB()
		if err != nil {
			log.Printf("[AUTH] Error fetching custom domains: %v", err)
			return ""
		}
		for _, domain := range domains {
			if domain.Domain == host {
				return domain.AppName
			}
		}
		return ""
	}

	return ""
}

func isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	
	host := u.Host
	domainType := getDomainType(host)
	
	// Allow login host and subdomains
	if domainType == DomainTypeLogin || domainType == DomainTypeSubdomain {
		return true
	}
	
	// Check custom domains
	domains, err := getActiveCustomDomainsFromDB()
	if err == nil {
		for _, domain := range domains {
			if domain.Domain == host {
				return true
			}
		}
	}
	
	return false
}

func redirectToLogin(c *fiber.Ctx, originalURL string) error {
	redirectURL := buildLoginURL(originalURL)
	log.Printf("[AUTH] Redirecting to login: %s", redirectURL)
	c.Set("Location", redirectURL)
	return c.SendStatus(fiber.StatusTemporaryRedirect)
}

func cleanViteParams(originalURL string) string {
	viteParams := []string{"?t=", "&t="}
	
	cleanedURL := originalURL
	for _, param := range viteParams {
		if strings.Contains(cleanedURL, param) {
			parts := strings.Split(cleanedURL, param)
			if len(parts) > 1 {
				afterParam := parts[1]
				if ampIndex := strings.Index(afterParam, "&"); ampIndex != -1 {
					cleanedURL = parts[0] + "&" + afterParam[ampIndex+1:]
				} else {
					cleanedURL = parts[0]
				}
			}
		}
	}
	
	cleanedURL = strings.TrimSuffix(cleanedURL, "?")
	cleanedURL = strings.TrimSuffix(cleanedURL, "&")
	
	return cleanedURL
}

// ==================== HTML Templates ====================

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

// ==================== Token Denylist Functions Removed ====================
// JWT token denylist functions removed as system uses SSO sessions instead

// Removed JWT token validation functions as system uses SSO sessions instead

// ==================== Cleanup Functions ====================

func CleanExpiredSSOTokens() {
	ssoMutex.Lock()
	defer ssoMutex.Unlock()
	
	now := time.Now()
	for sessionID, session := range ssoSessions {
		if now.After(session.ExpiresAt) {
			delete(ssoSessions, sessionID)
		}
	}
}

func init() {
	// Start periodic cleanup
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		
		for range ticker.C {
			CleanExpiredSSOTokens()
		}
	}()
}