package routes

import (
	"backend/handlers"
	"backend/middleware"

	"github.com/gofiber/fiber/v2"
)

// SetupRoutes, API routes
func SetupRoutes(app *fiber.App) {

	app.Get("/sso/check", handlers.SSOCheck)
	app.Get("/sso/init", handlers.SSOInit)

	// Health check endpoints
	app.Get("/health", handlers.HealthCheck)
	app.Get("/redis-status", handlers.RedisStatus)
	app.Post("/clear-test-data", handlers.ClearRedisTestData)

	// API v1 routes
	api := app.Group("/api/v1")

	// Open routes (no auth required)
	auth := api.Group("/auth")
	// auth.Post("/register", handlers.Register)
	auth.Post("/login", handlers.Login)
	auth.Post("/logout", handlers.Logout)
	auth.Get("/token-validate", handlers.ValidateSessionEndpoint)  // kept path for compatibility
	auth.Post("/validate-token", handlers.ValidateSessionEndpoint) // kept path for compatibility
	// auth.Get("/check-session", handlers.CheckSession) // Old session check, to be removed or updated

	// Traefik forward auth endpoint
	auth.Get("/validate", handlers.ValidateForTraefik)

	// Cross-domain cookie endpoints (removed - not needed)

	// Protected routes (auth required)
	citizen := api.Group("/citizen", middleware.Protected())

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
	
	// GitHub config endpoints (admin only)
	github.Post("/config", middleware.Protected(), handlers.SetupGitHubConfig)
	github.Get("/config", middleware.Protected(), handlers.GetGitHubConfig)
	github.Delete("/config", middleware.Protected(), handlers.DeleteGitHubConfig)
	
	// GitHub OAuth endpoints
	github.Get("/auth/init", middleware.Protected(), handlers.GitHubAuthInit)
	github.Get("/auth/callback", middleware.Protected(), handlers.GitHubAuthCallback)
	github.Get("/status", middleware.Protected(), handlers.GetGitHubStatus)
	github.Get("/repositories", middleware.Protected(), handlers.ListGitHubRepositories)
	github.Get("/connections", middleware.Protected(), handlers.GetRepositoryConnections)
	github.Post("/connect", middleware.Protected(), handlers.ConnectRepository)
	github.Delete("/apps/:app_name/disconnect", middleware.Protected(), handlers.DisconnectRepository)
	github.Put("/apps/:app_name/auto-deploy", middleware.Protected(), handlers.ToggleAutoDeploy)
	
	// GitHub webhook endpoint (public - no auth required)
	github.Post("/webhook", handlers.GitHubWebhookHandler)
}
