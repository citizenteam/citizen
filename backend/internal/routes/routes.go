package routes

import (
	apitokenshandlers "backend/internal/api_tokens/handlers"
	appshandlers "backend/internal/apps/handlers"
	authhandlers "backend/internal/auth/handlers"
	authservices "backend/internal/auth/services"
	citizenauthhandlers "backend/internal/citizenauth/handlers"
	deploymenthandlers "backend/internal/deployments/handlers"
	dockerhandlers "backend/internal/docker/handlers"
	githubappshandlers "backend/internal/github/handlers/apps"
	githubconfighandlers "backend/internal/github/handlers/config"
	githubreposhandlers "backend/internal/github/handlers/repositories"
	githubstatushandlers "backend/internal/github/handlers/status"
	githubwebhookhandlers "backend/internal/github/handlers/webhook"
	healthhandlers "backend/internal/health/handlers"
	"backend/internal/middleware"
	ssehandlers "backend/internal/sse/handlers"
	webhookshandlers "backend/internal/webhooks/handlers"
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
			session, err := authservices.GetSSOSession(ssoSessionID)
			if err == nil && session != nil {
				// Logged in - show welcome message
				return c.SendString("✅ Citizen - App Management Platform. You are logged in! (User ID: " + fmt.Sprintf("%d", session.UserID) + ")")
			}
		}

		// Not logged in - redirect to CitizenAuth
		return citizenauthhandlers.RedirectToCitizenAuth(c)
	})

	// SSO endpoints
	sso := app.Group("/sso")
	{
		// SSO init - redirect to CitizenAuth
		sso.Get("/init", citizenauthhandlers.RedirectToCitizenAuth)
		// SSO callback - set cookie for this domain
		sso.Get("/callback", authhandlers.SSOCallback)
	}

	// Health check endpoints (public)
	app.Get("/health", healthhandlers.HealthCheck)
	app.Get("/redis-status", healthhandlers.RedisStatus)
	app.Post("/clear-test-data", healthhandlers.ClearRedisTestData)

	// API v1 routes
	api := app.Group("/api/v1")

	// Open routes (no auth required)
	auth := api.Group("/auth")

	// Redirect to CitizenAuth for login/logout
	auth.Get("/login", citizenauthhandlers.RedirectToCitizenAuth)
	auth.Post("/login", citizenauthhandlers.RedirectToCitizenAuth)
	auth.Post("/logout", citizenauthhandlers.RedirectToCitizenAuth)

	// Traefik forward auth endpoint (validates SSO session - eski sistem)
	auth.Get("/validate", authhandlers.ValidateForTraefik)

	// Cross-domain cookie endpoints (removed - not needed)

	// Protected routes (SSO session OR JWT required)
	citizen := api.Group("/citizen")
	citizen.Use(middleware.JWTAuth())   // Try JWT first (CitizenAuth)
	citizen.Use(middleware.Protected()) // Fallback to SSO session

	// User profile
	citizen.Get("/profile", authhandlers.GetProfile)

	// App management
	citizen.Get("/apps", appshandlers.ListApps)
	citizen.Get("/apps-info", appshandlers.GetAllAppsInfo) // Get all apps info
	citizen.Post("/apps", appshandlers.CreateApp)
	citizen.Get("/apps/:app_name", appshandlers.GetAppInfo)
	citizen.Delete("/apps/:app_name", appshandlers.DestroyApp)
	citizen.Post("/apps/:app_name/restart", appshandlers.RestartApp)

	// Domains
	citizen.Get("/apps/:app_name/domains", appshandlers.ListDomains)
	citizen.Post("/apps/:app_name/domains", appshandlers.AddDomain)
	citizen.Post("/apps/:app_name/domain", appshandlers.AddDomain)
	citizen.Delete("/apps/:app_name/domain", appshandlers.RemoveDomain)

	// Port settings
	citizen.Post("/apps/:app_name/port", appshandlers.SetPort)

	// Build settings
	citizen.Get("/apps/:app_name/build-settings", appshandlers.GetBuildSettings)
	citizen.Post("/apps/:app_name/build-settings", appshandlers.SetBuildSettings)
	citizen.Post("/apps/:app_name/builder", appshandlers.SetBuilderType)

	// Git deploy
	citizen.Post("/apps/:app_name/git-deploy", appshandlers.DeployApp)
	citizen.Post("/apps/:app_name/deploy", appshandlers.DeployApp)

	// Environment variables
	citizen.Get("/apps/:app_name/env", appshandlers.GetEnv)
	citizen.Post("/apps/:app_name/env", appshandlers.SetEnv)
	citizen.Delete("/apps/:app_name/env", appshandlers.RemoveEnv)
	citizen.Post("/apps/:app_name/config", appshandlers.SetEnv)

	// Custom domain management
	citizen.Post("/apps/:app_name/custom-domain", appshandlers.SetCustomDomain)
	citizen.Get("/apps/:app_name/custom-domains", appshandlers.GetCustomDomains)
	citizen.Delete("/apps/:app_name/custom-domain", appshandlers.RemoveCustomDomain)
	citizen.Get("/custom-domains", appshandlers.GetAllActiveCustomDomains)

	// Public app settings
	citizen.Post("/apps/:app_name/public-setting", appshandlers.SetPublicApp)
	citizen.Get("/apps/:app_name/public-setting", appshandlers.GetPublicAppSetting)

	// API Token management
	citizen.Post("/api-tokens", apitokenshandlers.CreateAPIToken)
	citizen.Get("/api-tokens", apitokenshandlers.ListAPITokens)
	citizen.Delete("/api-tokens/:token_id", apitokenshandlers.DeleteAPIToken)

	// App API Access management
	citizen.Get("/apps/:app_name/api-access", apitokenshandlers.GetAppAPIAccess)
	citizen.Post("/apps/:app_name/api-access", apitokenshandlers.SetAppAPIAccess)
	citizen.Get("/api-operations", apitokenshandlers.GetAvailableOperations)

	// Docker Hub connection endpoints
	citizen.Post("/docker/connection", dockerhandlers.CreateDockerConnection)
	citizen.Get("/docker/connection", dockerhandlers.GetDockerConnection)
	citizen.Delete("/docker/connection", dockerhandlers.DeleteDockerConnection)
	citizen.Post("/docker/test", dockerhandlers.TestDockerConnection)

	// Buildpack management
	citizen.Get("/apps/:app_name/buildpacks", appshandlers.ListBuildpacks)
	citizen.Post("/apps/:app_name/buildpacks", appshandlers.AddBuildpack)
	citizen.Put("/apps/:app_name/buildpacks", appshandlers.SetBuildpack)
	citizen.Delete("/apps/:app_name/buildpacks", appshandlers.RemoveBuildpack)
	citizen.Delete("/apps/:app_name/buildpacks/clear", appshandlers.ClearBuildpacks)
	citizen.Get("/apps/:app_name/buildpacks/report", appshandlers.GetBuildpackReport)

	// Builder management
	citizen.Post("/apps/:app_name/builder", appshandlers.SetBuilder)
	citizen.Get("/apps/:app_name/builder", appshandlers.GetBuilderReport)

	// App deployment info
	citizen.Get("/deployments", deploymenthandlers.GetAllAppDeployments)
	citizen.Get("/apps/:app_name/deployment", deploymenthandlers.GetAppDeployment)
	citizen.Put("/apps/:app_name/deployment", deploymenthandlers.UpdateAppDeployment)
	citizen.Put("/apps/:app_name/deployment/status", deploymenthandlers.UpdateAppDeploymentStatus)

	// Deployment runs (K3s only) - Netlify-style deployment tracking
	citizen.Get("/apps/:app_name/runs", deploymenthandlers.GetDeploymentRuns)
	citizen.Get("/apps/:app_name/runs/latest", deploymenthandlers.GetLatestDeploymentRun)
	citizen.Get("/runs/:run_id", deploymenthandlers.GetDeploymentRun)
	citizen.Post("/apps/:app_name/trigger-deploy", deploymenthandlers.TriggerDeployment)

	// Log management
	citizen.Get("/apps/:app_name/logs", appshandlers.GetAppLogs)
	citizen.Get("/apps/:app_name/logs/stream", appshandlers.StreamAppLogs)
	citizen.Get("/apps/:app_name/logs/info", appshandlers.GetLogInfo)
	citizen.Get("/apps/:app_name/logs/live-build", appshandlers.GetLiveBuildLogs)

	// Activities
	citizen.Get("/apps/:app_name/activities", appshandlers.GetAppActivities)

	// GitHub integration endpoints

	// PUBLIC GitHub endpoints (no auth required - have their own security mechanisms)
	// These are registered directly on api group to avoid middleware inheritance issues
	api.Post("/github/webhook", githubwebhookhandlers.GitHubWebhookHandler)             // HMAC signature validation
	api.Get("/github/app/manifest/callback", githubappshandlers.GitHubManifestCallback) // State token validation
	api.Get("/github/app/manifest/redirect", githubappshandlers.GitHubManifestRedirect) // State token validation
	api.Get("/github/app/install/callback", githubappshandlers.GitHubInstallCallback)   // State token validation

	// PROTECTED GitHub endpoints (JWT or SSO session required)
	github := api.Group("/github")
	github.Use(middleware.JWTAuth())   // Try JWT first (CitizenAuth)
	github.Use(middleware.Protected()) // Fallback to SSO session
	{
		// GitHub config endpoints (admin only)
		github.Post("/config", githubconfighandlers.SetupGitHubConfig)
		github.Get("/config", githubconfighandlers.GetGitHubConfig)
		github.Delete("/config", githubconfighandlers.DeleteGitHubConfig)
		github.Post("/app/manifest/start", githubappshandlers.StartGitHubManifest)

		// Existing GitHub App connection (App ID + Private Key)
		github.Post("/app/connect-with-key", githubappshandlers.ConnectWithPrivateKey)

		// GitHub status
		github.Get("/status", githubstatushandlers.GetGitHubStatus)
		github.Delete("/disconnect", githubstatushandlers.DisconnectGitHubAccount) // Disconnect GitHub account
		github.Get("/repositories", githubreposhandlers.ListGitHubRepositories)
		github.Get("/repos/:owner/:repo/branches", githubreposhandlers.GetRepositoryBranches)
		github.Get("/connections", githubreposhandlers.GetRepositoryConnections)
		github.Post("/connect", githubreposhandlers.ConnectRepository)
		github.Post("/apps/:app_name/connect", githubreposhandlers.ConnectExistingAppToRepository) // Connect existing app to repo
		github.Delete("/apps/:app_name/disconnect", githubreposhandlers.DisconnectRepository)
		github.Put("/apps/:app_name/auto-deploy", githubreposhandlers.ToggleAutoDeploy)
		github.Post("/app/install/start", githubappshandlers.StartGitHubInstall)
	}

	// SSE endpoints for real-time streaming (auth via middleware)
	api.Get("/sse/runs/:run_id", ssehandlers.DeploymentLogsSSE)
	api.Get("/sse/apps/:app_name/logs", ssehandlers.PodLogsSSE)

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
			webhooks.Post("/permission-update", webhookshandlers.WebhookPermissionUpdate)
			webhooks.Post("/session-update", webhookshandlers.WebhookSessionUpdate) // Login/Logout events
			webhooks.Post("/instance-lifecycle", webhookshandlers.WebhookInstanceLifecycle)
		}

		// Permission API (CitizenAuth reads permissions for UI)
		permissions := service.Group("/permissions")
		permissions.Use(middleware.APIKeyAuth())
		permissions.Use(middleware.RequireServiceAuth())
		permissions.Use(middleware.RequireScope("permissions:read"))
		{
			permissions.Get("/", webhookshandlers.GetPermissionsForCitizenAuth)
		}

		// Instance handshake (no middleware - handler does its own auth)
		service.Post("/instances/handshake", citizenauthhandlers.InstanceHandshake)
	}
}
