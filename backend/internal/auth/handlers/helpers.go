package handlers

import (
	"backend/internal/auth/models"
	"backend/internal/database/api"
	appmodels "backend/internal/models"
	"backend/internal/utils"
	"context"
	"log"
	"net/url"
	"os"
	"strings"
)

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
	// GitHub public endpoints (have their own security: HMAC signature / state token)
	"/api/v1/github/webhook",
	"/api/v1/github/auth/callback",
	"/api/v1/github/app/manifest/callback",
	"/api/v1/github/app/manifest/redirect",
	"/api/v1/github/app/install/callback",
}

// Development-only paths
var developmentPaths = []string{
	"/node_modules/", "/src/", "/@vite/", "/@fs/", "/@id/",
	"/__vite_ping", ".tsx", ".ts", ".jsx", ".vue",
	".scss", ".sass", ".less", ".styl",
}

// getLoginHost returns the login host from env or default
func getLoginHost() string {
	if host := os.Getenv("LOGIN_HOST"); host != "" {
		return host
	}
	return "localhost"
}

// getDomainType determines the type of domain
func getDomainType(host string) models.DomainType {
	loginHost := getLoginHost()

	if host == loginHost || host == "www."+loginHost {
		return models.DomainTypeLogin
	}

	if strings.HasSuffix(host, "."+loginHost) {
		return models.DomainTypeSubdomain
	}

	return models.DomainTypeCustom
}

// getCookieConfig returns appropriate cookie configuration for a host
func getCookieConfig(host string, forwardedProto string) models.CookieConfig {
	domainType := getDomainType(host)
	config := models.CookieConfig{}
	loginHost := getLoginHost()

	// Determine domain
	switch domainType {
	case models.DomainTypeCustom:
		config.Domain = "" // No domain for custom domains
	case models.DomainTypeLogin, models.DomainTypeSubdomain:
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
	} else if domainType == models.DomainTypeCustom {
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
func getCookieConfigForLoginHost(forwardedProto string) models.CookieConfig {
	loginHost := getLoginHost()
	config := models.CookieConfig{}

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

func isHttpsRequired() bool {
	forceHttps := os.Getenv("FORCE_HTTPS")
	if forceHttps == "" {
		forceHttps = "true"
	}

	result := forceHttps == "true"
	utils.AuthDebugLog("isHttpsRequired() = %v (FORCE_HTTPS='%s')", result, forceHttps)
	return result
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

// IsPublicPath checks if a path is public (exported for use in middleware)
func IsPublicPath(uri string) bool {
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

func extractAppNameFromHost(host string) string {
	if host == "" {
		return ""
	}

	loginHost := getLoginHost()
	domainType := getDomainType(host)

	switch domainType {
	case models.DomainTypeLogin:
		return ""
	case models.DomainTypeSubdomain:
		subdomain := strings.TrimSuffix(host, "."+loginHost)
		if subdomain == "www" {
			return ""
		}
		// Multi-level subdomain desteği: app2.whimsical-isle.amber-ridge.app.domain.com
		// İlk parça app adıdır
		if strings.Contains(subdomain, ".") {
			parts := strings.Split(subdomain, ".")
			return parts[0]
		}
		return subdomain
	case models.DomainTypeCustom:
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
	if domainType == models.DomainTypeLogin || domainType == models.DomainTypeSubdomain {
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

// Helper functions that need to be imported from other packages
func getActiveCustomDomainsFromDB() ([]appmodels.AppCustomDomain, error) {
	return api.Settings.GetAllActiveCustomDomains(context.Background())
}

func isAppPublic(appName string) bool {
	isPublic, err := api.Settings.IsAppPublic(context.Background(), appName)
	if err != nil {
		return false
	}
	return isPublic
}
