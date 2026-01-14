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
	rbacmw "backend/internal/rbac/middleware"
	ssehandlers "backend/internal/sse/handlers"
	systemhandlers "backend/internal/system/handlers"
	webhookshandlers "backend/internal/webhooks/handlers"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

// SetupRoutes, API routes
func SetupRoutes(app *fiber.App) {

	// Global OPTIONS handler for CORS preflight requests
	app.Options("/*", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	// Root route
	app.Get("/", func(c *fiber.Ctx) error {
		ssoSessionID := c.Cookies("sso_session")
		if ssoSessionID != "" {
			session, err := authservices.GetSSOSession(ssoSessionID)
			if err == nil && session != nil {
				return c.SendString("✅ Citizen - App Management Platform. You are logged in! (User ID: " + fmt.Sprintf("%d", session.UserID) + ")")
			}
		}
		return citizenauthhandlers.RedirectToCitizenAuth(c)
	})

	// SSO endpoints
	sso := app.Group("/sso")
	sso.Get("/init", citizenauthhandlers.RedirectToCitizenAuth)
	sso.Get("/callback", authhandlers.SSOCallback)

	// Health check endpoints (public)
	app.Get("/health", healthhandlers.HealthCheck)
	app.Get("/redis-status", healthhandlers.RedisStatus)
	app.Post("/clear-test-data", healthhandlers.ClearRedisTestData)

	// API v1 routes
	api := app.Group("/api/v1")

	// Auth routes (public)
	auth := api.Group("/auth")
	auth.Get("/login", citizenauthhandlers.RedirectToCitizenAuth)
	auth.Post("/login", citizenauthhandlers.RedirectToCitizenAuth)
	auth.Post("/logout", citizenauthhandlers.RedirectToCitizenAuth)
	auth.Get("/validate", authhandlers.ValidateForTraefik)

	// Protected routes (JWT or SSO required)
	citizen := api.Group("/citizen")
	citizen.Use(middleware.JWTAuth())
	citizen.Use(middleware.Protected())
	citizen.Use(rbacmw.LoadRBACContext())

	// =====================
	// VIEWER+ ENDPOINTS (RLS ile - dashboard gerekmez)
	// =====================

	// User profile (everyone)
	citizen.Get("/profile", authhandlers.GetProfile)

	// App list - RLS filters based on permissions
	citizen.Get("/apps", appshandlers.ListApps)
	citizen.Get("/apps-info", appshandlers.GetAllAppsInfo)

	// App info (viewer can see assigned apps via RLS)
	citizen.Get("/apps/:app_name", rbacmw.RequireAppViewer(), appshandlers.GetAppInfo)
	citizen.Get("/apps/:app_name/domains", rbacmw.RequireAppViewer(), appshandlers.ListDomains)
	citizen.Get("/apps/:app_name/custom-domains", rbacmw.RequireAppViewer(), appshandlers.GetCustomDomains)
	citizen.Get("/apps/:app_name/public-setting", rbacmw.RequireAppViewer(), appshandlers.GetPublicAppSetting)

	// =====================
	// MEMBER+ ENDPOINTS (Dashboard required)
	// =====================
	citizen.Use(rbacmw.RequireDashboardAccess())

	// App READ - Member+ (sensitive data)
	citizen.Get("/apps/:app_name/env", rbacmw.RequireAppMember(), appshandlers.GetEnv)
	citizen.Get("/apps/:app_name/build-settings", rbacmw.RequireAppMember(), appshandlers.GetBuildSettings)
	citizen.Get("/apps/:app_name/buildpacks", rbacmw.RequireAppMember(), appshandlers.ListBuildpacks)
	citizen.Get("/apps/:app_name/buildpacks/report", rbacmw.RequireAppMember(), appshandlers.GetBuildpackReport)
	citizen.Get("/apps/:app_name/builder", rbacmw.RequireAppMember(), appshandlers.GetBuilderReport)
	citizen.Get("/apps/:app_name/api-access", rbacmw.RequireAppMember(), apitokenshandlers.GetAppAPIAccess)
	citizen.Get("/apps/:app_name/deployment", rbacmw.RequireAppMember(), deploymenthandlers.GetAppDeployment)
	citizen.Get("/apps/:app_name/runs", rbacmw.RequireAppMember(), deploymenthandlers.GetDeploymentRuns)
	citizen.Get("/apps/:app_name/runs/latest", rbacmw.RequireAppMember(), deploymenthandlers.GetLatestDeploymentRun)
	citizen.Get("/apps/:app_name/logs", rbacmw.RequireAppMember(), appshandlers.GetAppLogs)
	citizen.Get("/apps/:app_name/logs/stream", rbacmw.RequireAppMember(), appshandlers.StreamAppLogs)
	citizen.Get("/apps/:app_name/logs/info", rbacmw.RequireAppMember(), appshandlers.GetLogInfo)
	citizen.Get("/apps/:app_name/logs/live-build", rbacmw.RequireAppMember(), appshandlers.GetLiveBuildLogs)
	citizen.Get("/apps/:app_name/activities", rbacmw.RequireAppMember(), appshandlers.GetAppActivities)

	// App WRITE - Member+
	citizen.Post("/apps/:app_name/deploy", rbacmw.RequireAppMember(), appshandlers.DeployApp)
	citizen.Post("/apps/:app_name/git-deploy", rbacmw.RequireAppMember(), appshandlers.DeployApp)
	citizen.Post("/apps/:app_name/restart", rbacmw.RequireAppMember(), appshandlers.RestartApp)
	citizen.Post("/apps/:app_name/trigger-deploy", rbacmw.RequireAppMember(), deploymenthandlers.TriggerDeployment)
	citizen.Post("/apps/:app_name/env", rbacmw.RequireAppMember(), appshandlers.SetEnv)
	citizen.Delete("/apps/:app_name/env", rbacmw.RequireAppMember(), appshandlers.RemoveEnv)
	citizen.Post("/apps/:app_name/config", rbacmw.RequireAppMember(), appshandlers.SetEnv)
	citizen.Post("/apps/:app_name/domains", rbacmw.RequireAppMember(), appshandlers.AddDomain)
	citizen.Post("/apps/:app_name/domain", rbacmw.RequireAppMember(), appshandlers.AddDomain)
	citizen.Delete("/apps/:app_name/domain", rbacmw.RequireAppMember(), appshandlers.RemoveDomain)
	citizen.Post("/apps/:app_name/custom-domain", rbacmw.RequireAppMember(), appshandlers.SetCustomDomain)
	citizen.Delete("/apps/:app_name/custom-domain", rbacmw.RequireAppMember(), appshandlers.RemoveCustomDomain)
	citizen.Post("/apps/:app_name/port", rbacmw.RequireAppMember(), appshandlers.SetPort)
	citizen.Post("/apps/:app_name/build-settings", rbacmw.RequireAppMember(), appshandlers.SetBuildSettings)
	citizen.Post("/apps/:app_name/builder", rbacmw.RequireAppMember(), appshandlers.SetBuilderType)
	citizen.Post("/apps/:app_name/buildpacks", rbacmw.RequireAppMember(), appshandlers.AddBuildpack)
	citizen.Put("/apps/:app_name/buildpacks", rbacmw.RequireAppMember(), appshandlers.SetBuildpack)
	citizen.Delete("/apps/:app_name/buildpacks", rbacmw.RequireAppMember(), appshandlers.RemoveBuildpack)
	citizen.Delete("/apps/:app_name/buildpacks/clear", rbacmw.RequireAppMember(), appshandlers.ClearBuildpacks)
	citizen.Post("/apps/:app_name/builder", rbacmw.RequireAppMember(), appshandlers.SetBuilder)
	citizen.Post("/apps/:app_name/public-setting", rbacmw.RequireAppMember(), appshandlers.SetPublicApp)
	citizen.Post("/apps/:app_name/api-access", rbacmw.RequireAppMember(), apitokenshandlers.SetAppAPIAccess)
	citizen.Put("/apps/:app_name/deployment", rbacmw.RequireAppMember(), deploymenthandlers.UpdateAppDeployment)
	citizen.Put("/apps/:app_name/deployment/status", rbacmw.RequireAppMember(), deploymenthandlers.UpdateAppDeploymentStatus)

	// =====================
	// ADMIN ONLY ENDPOINTS
	// =====================

	// App create/delete - Instance Admin only
	citizen.Post("/apps", rbacmw.RequireAdmin(), appshandlers.CreateApp)
	citizen.Delete("/apps/:app_name", rbacmw.RequireAdmin(), appshandlers.DestroyApp)

	// Docker connection - Instance Admin only
	citizen.Post("/docker/connection", rbacmw.RequireAdmin(), dockerhandlers.CreateDockerConnection)
	citizen.Delete("/docker/connection", rbacmw.RequireAdmin(), dockerhandlers.DeleteDockerConnection)

	// Admin audit logs - Instance Admin only
	citizen.Get("/admin/api-tokens/failed-attempts", rbacmw.RequireAdmin(), apitokenshandlers.GetFailedLoginAttempts)

	// System settings - Instance Admin only
	citizen.Get("/system/settings", rbacmw.RequireAdmin(), systemhandlers.GetSystemSettings)
	citizen.Get("/system/build-settings", rbacmw.RequireAdmin(), systemhandlers.GetBuildSettings)
	citizen.Put("/system/build-settings", rbacmw.RequireAdmin(), systemhandlers.UpdateBuildSettings)
	citizen.Get("/system/queue-status", rbacmw.RequireAdmin(), systemhandlers.GetQueueStatus)

	// =====================
	// GENERAL MEMBER+ ENDPOINTS
	// =====================

	// General reads
	citizen.Get("/custom-domains", appshandlers.GetAllActiveCustomDomains)
	citizen.Get("/deployments", deploymenthandlers.GetAllAppDeployments)
	citizen.Get("/runs/:run_id", deploymenthandlers.GetDeploymentRun)
	citizen.Get("/docker/connection", dockerhandlers.GetDockerConnection)
	citizen.Post("/docker/test", dockerhandlers.TestDockerConnection)
	citizen.Get("/api-operations", apitokenshandlers.GetAvailableOperations)

	// Audit logs (RLS filters based on role)
	citizen.Get("/admin/api-tokens/audit-logs", apitokenshandlers.GetTokenAuditLogs)

	// API Token management (member+ required)
	tokens := citizen.Group("/api-tokens")
	tokens.Use(rbacmw.RequireMember())
	{
		tokens.Post("", apitokenshandlers.CreateAPIToken)
		tokens.Get("", apitokenshandlers.ListAPITokens)
		tokens.Delete("/:token_id", apitokenshandlers.DeleteAPIToken)
		tokens.Get("/:token_id/stats", apitokenshandlers.GetTokenUsageStats)
	}

	// =====================
	// GitHub Integration
	// =====================

	// PUBLIC GitHub endpoints (no auth - have their own security)
	api.Post("/github/webhook", githubwebhookhandlers.GitHubWebhookHandler)
	api.Get("/github/app/manifest/callback", githubappshandlers.GitHubManifestCallback)
	api.Get("/github/app/manifest/redirect", githubappshandlers.GitHubManifestRedirect)
	api.Get("/github/app/install/callback", githubappshandlers.GitHubInstallCallback)

	// PROTECTED GitHub endpoints
	github := api.Group("/github")
	github.Use(middleware.JWTAuth())
	github.Use(middleware.Protected())
	github.Use(rbacmw.LoadRBACContext())
	github.Use(rbacmw.RequireDashboardAccess())
	{
		// GitHub config - Instance Admin only
		github.Post("/config", rbacmw.RequireAdmin(), githubconfighandlers.SetupGitHubConfig)
		github.Delete("/config", rbacmw.RequireAdmin(), githubconfighandlers.DeleteGitHubConfig)
		github.Post("/app/manifest/start", rbacmw.RequireAdmin(), githubappshandlers.StartGitHubManifest)
		github.Post("/app/connect-with-key", rbacmw.RequireAdmin(), githubappshandlers.ConnectWithPrivateKey)
		github.Post("/app/install/start", rbacmw.RequireAdmin(), githubappshandlers.StartGitHubInstall)
		github.Delete("/disconnect", rbacmw.RequireAdmin(), githubstatushandlers.DisconnectGitHubAccount)

		// GitHub read - Member+
		github.Get("/config", githubconfighandlers.GetGitHubConfig)
		github.Get("/status", githubstatushandlers.GetGitHubStatus)
		github.Get("/repositories", githubreposhandlers.ListGitHubRepositories)
		github.Get("/repos/:owner/:repo/branches", githubreposhandlers.GetRepositoryBranches)
		github.Get("/connections", githubreposhandlers.GetRepositoryConnections)

		// GitHub repo operations - Member+
		github.Post("/connect", githubreposhandlers.ConnectRepository)
		github.Post("/apps/:app_name/connect", rbacmw.RequireAppMember(), githubreposhandlers.ConnectExistingAppToRepository)
		github.Delete("/apps/:app_name/disconnect", rbacmw.RequireAppMember(), githubreposhandlers.DisconnectRepository)
		github.Put("/apps/:app_name/auto-deploy", rbacmw.RequireAppMember(), githubreposhandlers.ToggleAutoDeploy)
	}

	// SSE endpoints for real-time streaming
	api.Get("/sse/runs/:run_id", ssehandlers.DeploymentLogsSSE)
	api.Get("/sse/apps/:app_name/logs", ssehandlers.PodLogsSSE)
	api.Get("/sse/apps/:app_name/metrics", ssehandlers.MetricsSSE)
	api.Get("/sse/cluster/metrics", ssehandlers.ClusterMetricsSSE)

	// App resource management (Admin only)
	api.Get("/apps/:app_name/resources", rbacmw.RequireAppMember(), appshandlers.GetAppResources)
	api.Put("/apps/:app_name/resources", rbacmw.RequireAdmin(), appshandlers.UpdateAppResources)

	// =====================
	// CitizenAuth Integration
	// =====================

	service := api.Group("/service")
	{
		webhooks := service.Group("/webhooks")
		webhooks.Use(middleware.APIKeyAuth())
		webhooks.Use(middleware.RequireServiceAuth())
		webhooks.Use(middleware.RequireScope("webhooks"))
		{
			webhooks.Post("/permission-update", webhookshandlers.WebhookPermissionUpdate)
			webhooks.Post("/session-update", webhookshandlers.WebhookSessionUpdate)
			webhooks.Post("/instance-lifecycle", webhookshandlers.WebhookInstanceLifecycle)
		}

		permissions := service.Group("/permissions")
		permissions.Use(middleware.APIKeyAuth())
		permissions.Use(middleware.RequireServiceAuth())
		permissions.Use(middleware.RequireScope("permissions:read"))
		{
			permissions.Get("/", webhookshandlers.GetPermissionsForCitizenAuth)
		}

		service.Post("/instances/handshake", citizenauthhandlers.InstanceHandshake)
	}
}
