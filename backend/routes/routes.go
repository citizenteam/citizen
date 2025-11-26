package routes

import (
	"backend/handlers"
	"backend/middleware"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

// SetupRoutes, API routes
func SetupRoutes(app *fiber.App) {

	// Global OPTIONS handler for CORS preflight requests
	app.Options("/*", func(c *fiber.Ctx) error {
		// CORS headers are already set by the CORS middleware in main.go
		return c.SendStatus(fiber.StatusNoContent)
	})

	// Root route - show simple landing page (or app list)
	app.Get("/", func(c *fiber.Ctx) error {
		// Check for SSO session
		ssoSessionID := c.Cookies("sso_session")
		if ssoSessionID != "" {
			// Validate session
			session, err := handlers.GetSSOSession(ssoSessionID)
			if err == nil && session != nil {
				// Logged in - show welcome message
				return c.SendString("✅ Citizen - App Management Platform. You are logged in! (User ID: " + fmt.Sprintf("%d", session.UserID) + ")")
			}
		}

		// Not logged in - redirect to CitizenAuth
		return handlers.RedirectToCitizenAuth(c)
	})

	// SSO endpoints
	sso := app.Group("/sso")
	{
		// SSO init - redirect to CitizenAuth
		sso.Get("/init", handlers.RedirectToCitizenAuth)
		// SSO callback - set cookie for this domain
		sso.Get("/callback", handlers.SSOCallback)
	}

	// Health check endpoints (public)
	app.Get("/health", handlers.HealthCheck)
	app.Get("/redis-status", handlers.RedisStatus)
	app.Post("/clear-test-data", handlers.ClearRedisTestData)

	// API v1 routes
	api := app.Group("/api/v1")

	// Open routes (no auth required)
	auth := api.Group("/auth")

	// Redirect to CitizenAuth for login/logout
	auth.Get("/login", handlers.RedirectToCitizenAuth)
	auth.Post("/login", handlers.RedirectToCitizenAuth)
	auth.Post("/logout", handlers.RedirectToCitizenAuth)

	// Traefik forward auth endpoint (validates SSO session - eski sistem)
	auth.Get("/validate", handlers.ValidateForTraefik)

	// Cross-domain cookie endpoints (removed - not needed)

	// Protected routes (SSO session OR JWT required)
	citizen := api.Group("/citizen")
	citizen.Use(middleware.JWTAuth())   // Try JWT first (CitizenAuth)
	citizen.Use(middleware.Protected()) // Fallback to SSO session

	// User profile
	citizen.Get("/profile", handlers.GetProfile)

	// App management
	citizen.Get("/apps", handlers.ListApps)
	citizen.Get("/apps-info", handlers.GetAllAppsInfo) // Get all apps info
	citizen.Post("/apps", handlers.CreateApp)
	citizen.Get("/apps/:app_name", handlers.GetAppInfo)
	citizen.Delete("/apps/:app_name", handlers.DestroyApp)
	citizen.Post("/apps/:app_name/restart", handlers.RestartApp)

	// Domains
	citizen.Get("/apps/:app_name/domains", handlers.ListDomains)
	citizen.Post("/apps/:app_name/domains", handlers.AddDomain)
	citizen.Post("/apps/:app_name/domain", handlers.AddDomain)
	citizen.Delete("/apps/:app_name/domain", handlers.RemoveDomain)

	// Port settings
	citizen.Post("/apps/:app_name/port", handlers.SetPort)

	// Git deploy
	citizen.Post("/apps/:app_name/git-deploy", handlers.DeployApp)
	citizen.Post("/apps/:app_name/deploy", handlers.DeployApp)

	// Environment variables
	citizen.Get("/apps/:app_name/env", handlers.GetEnv)
	citizen.Post("/apps/:app_name/env", handlers.SetEnv)
	citizen.Delete("/apps/:app_name/env", handlers.RemoveEnv)
	citizen.Post("/apps/:app_name/config", handlers.SetEnv)

	// Custom domain management
	citizen.Post("/apps/:app_name/custom-domain", handlers.SetCustomDomain)
	citizen.Get("/apps/:app_name/custom-domains", handlers.GetCustomDomains)
	citizen.Delete("/apps/:app_name/custom-domain", handlers.RemoveCustomDomain)
	citizen.Get("/custom-domains", handlers.GetAllActiveCustomDomains)

	// Public app settings
	citizen.Post("/apps/:app_name/public-setting", handlers.SetPublicApp)
	citizen.Get("/apps/:app_name/public-setting", handlers.GetPublicAppSetting)

	// API Token management
	citizen.Post("/api-tokens", handlers.CreateAPIToken)
	citizen.Get("/api-tokens", handlers.ListAPITokens)
	citizen.Delete("/api-tokens/:token_id", handlers.DeleteAPIToken)

	// App API Access management
	citizen.Get("/apps/:app_name/api-access", handlers.GetAppAPIAccess)
	citizen.Post("/apps/:app_name/api-access", handlers.SetAppAPIAccess)
	citizen.Get("/api-operations", handlers.GetAvailableOperations)

	// Docker Hub connection endpoints
	citizen.Post("/docker/connection", handlers.CreateDockerConnection)
	citizen.Get("/docker/connection", handlers.GetDockerConnection)
	citizen.Delete("/docker/connection", handlers.DeleteDockerConnection)
	citizen.Post("/docker/test", handlers.TestDockerConnection)

	// Buildpack management
	citizen.Get("/apps/:app_name/buildpacks", handlers.ListBuildpacks)
	citizen.Post("/apps/:app_name/buildpacks", handlers.AddBuildpack)
	citizen.Put("/apps/:app_name/buildpacks", handlers.SetBuildpack)
	citizen.Delete("/apps/:app_name/buildpacks", handlers.RemoveBuildpack)
	citizen.Delete("/apps/:app_name/buildpacks/clear", handlers.ClearBuildpacks)
	citizen.Get("/apps/:app_name/buildpacks/report", handlers.GetBuildpackReport)

	// Builder management
	citizen.Post("/apps/:app_name/builder", handlers.SetBuilder)
	citizen.Get("/apps/:app_name/builder", handlers.GetBuilderReport)

	// App deployment info
	citizen.Get("/deployments", handlers.GetAllAppDeployments)
	citizen.Get("/apps/:app_name/deployment", handlers.GetAppDeployment)
	citizen.Put("/apps/:app_name/deployment", handlers.UpdateAppDeployment)
	citizen.Put("/apps/:app_name/deployment/status", handlers.UpdateAppDeploymentStatus)

	// Log management
	citizen.Get("/apps/:app_name/logs", handlers.GetAppLogs)
	citizen.Get("/apps/:app_name/logs/stream", handlers.StreamAppLogs)
	citizen.Get("/apps/:app_name/logs/info", handlers.GetLogInfo)
	citizen.Get("/apps/:app_name/logs/live-build", handlers.GetLiveBuildLogs)

	// Activities
	citizen.Get("/apps/:app_name/activities", handlers.GetAppActivities)

	// GitHub integration endpoints
	github := api.Group("/github")

	// GitHub endpoints (JWT or SSO session required)
	githubProtected := github.Group("")
	githubProtected.Use(middleware.JWTAuth())   // Try JWT first (CitizenAuth)
	githubProtected.Use(middleware.Protected()) // Fallback to SSO session
	{
		// GitHub config endpoints (admin only)
		githubProtected.Post("/config", handlers.SetupGitHubConfig)
		githubProtected.Get("/config", handlers.GetGitHubConfig)
		githubProtected.Delete("/config", handlers.DeleteGitHubConfig)
		githubProtected.Post("/app/manifest/start", handlers.StartGitHubManifest)

		// GitHub OAuth endpoints
		githubProtected.Get("/auth/init", handlers.GitHubAuthInit)
		githubProtected.Get("/auth/callback", handlers.GitHubAuthCallback)
		githubProtected.Get("/status", handlers.GetGitHubStatus)
		githubProtected.Get("/repositories", handlers.ListGitHubRepositories)
		githubProtected.Get("/connections", handlers.GetRepositoryConnections)
		githubProtected.Post("/connect", handlers.ConnectRepository)
		githubProtected.Delete("/apps/:app_name/disconnect", handlers.DisconnectRepository)
		githubProtected.Put("/apps/:app_name/auto-deploy", handlers.ToggleAutoDeploy)
	}

	// GitHub webhook endpoint (public - no auth required)
	github.Post("/webhook", handlers.GitHubWebhookHandler)
	github.Get("/app/manifest/callback", handlers.GitHubManifestCallback)
	github.Get("/app/install/callback", handlers.GitHubInstallCallback)

	// ===== CITIZENAUTH INTEGRATION ENDPOINTS =====

	// Service endpoints (API key authentication)
	service := api.Group("/service")
	{
		// Webhooks (CitizenAuth → Citizen)
		webhooks := service.Group("/webhooks")
		webhooks.Use(middleware.APIKeyAuth())
		webhooks.Use(middleware.RequireServiceAuth())
		webhooks.Use(middleware.RequireScope("webhooks"))
		{
			webhooks.Post("/permission-update", handlers.WebhookPermissionUpdate)
			webhooks.Post("/session-update", handlers.WebhookSessionUpdate) // Login/Logout events
			webhooks.Post("/instance-lifecycle", handlers.WebhookInstanceLifecycle)
		}

		// Permission API (CitizenAuth reads permissions for UI)
		permissions := service.Group("/permissions")
		permissions.Use(middleware.APIKeyAuth())
		permissions.Use(middleware.RequireServiceAuth())
		permissions.Use(middleware.RequireScope("permissions:read"))
		{
			permissions.Get("/", handlers.GetPermissionsForCitizenAuth)
		}

		// Instance handshake (no middleware - handler does its own auth)
		service.Post("/instances/handshake", handlers.InstanceHandshake)
	}
}
