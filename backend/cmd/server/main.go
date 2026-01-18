package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	authservices "backend/internal/auth/services"
	"backend/internal/database"
	"backend/internal/database/api"
	"backend/internal/models"
	deploymenthandlers "backend/internal/deployments/handlers"
	githubservices "backend/internal/github/services"
	"backend/internal/middleware"
	"backend/internal/platform"
	"backend/internal/platform/k3s"
	"backend/internal/routes"
	"backend/internal/services"
	"backend/internal/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"
)

func main() {
	// Start startup process
	utils.StartupLog("🚀 Starting Citizen Backend...")

	// Environment information
	utils.LogEnvironmentInfo()

	// Load environment variables (only for non-Docker development)
	err := godotenv.Load("config.env")
	if err != nil {
		utils.DebugLog("config.env file not found (normal in Docker environment)")
	} else {
		utils.StartupLog("Loaded config.env file")
	}

	// Load local development .env file
	err = godotenv.Load(".env")
	if err != nil {
		utils.DebugLog(".env file not found (normal in Docker environment)")
	} else {
		utils.StartupLog("Loaded .env file")
	}

	// Initialize encryption system (required for production)
	utils.StartupLog("Initializing encryption system...")
	if err := utils.InitEncryption(); err != nil {
		utils.ErrorLog("Encryption initialization failed: %v", err)
		log.Fatalf("Encryption initialization failed: %v", err)
	}

	// Validate encryption system
	if err := utils.ValidateEncryptionSetup(); err != nil {
		utils.ErrorLog("Encryption validation failed: %v", err)
		log.Fatalf("Encryption validation failed: %v", err)
	}
	utils.StartupLog("Encryption system initialized successfully")

	// Initialize K3s platform adapter
	utils.StartupLog("Initializing K3s platform adapter...")
	if err := initPlatformAdapter(); err != nil {
		utils.ErrorLog("K3s adapter initialization failed: %v", err)
		log.Fatalf("K3s adapter initialization failed: %v", err)
	}

	// Start database connection (check skip flag)
	if os.Getenv("SKIP_DB_PING") != "true" {
		utils.StartupLog("Connecting to database...")
		database.ConnectDB()

		// Run migrations
		utils.StartupLog("Running database migrations...")
		if err := database.RunMigrations(); err != nil {
			utils.ErrorLog("Migration failed: %v", err)
			log.Fatalf("Migration failed: %v", err)
		}
		utils.StartupLog("Database migrations completed")

		// Create admin user (if environment variables are set)
		if err := database.CreateAdminUserFromEnv(); err != nil {
			utils.WarnLog("Failed to create admin user: %v", err)
		}

		// Start Redis connection
		utils.StartupLog("Connecting to Redis...")
		database.InitRedis()

		// Initialize JWT validator for CitizenAuth integration (after Redis)
		utils.StartupLog("🔑 Initializing JWT validator...")
		if err := middleware.InitJWTValidator(); err != nil {
			utils.WarnLog("JWT validator initialization failed: %v", err)
			utils.WarnLog("CitizenAuth JWT authentication will not be available")
		} else {
			utils.StartupLog("✅ JWT validator initialized successfully")
		}

		// Start permission change subscriber (Redis Pub/Sub) - after Redis init
		utils.StartupLog("📡 Starting permission change subscriber...")
		go func() {
			time.Sleep(2 * time.Second) // Wait for Redis to be fully ready
			subscriber := services.NewPermissionSubscriber()

			ctx := context.Background()
			if err := subscriber.Start(ctx); err != nil {
				utils.ErrorLog("Permission subscriber error: %v", err)
			}
		}()

		// Load GitHub config from database
		utils.StartupLog("Loading GitHub configuration...")
		loadGitHubConfigFromDB()

		// Start deployment queue if enabled (default: true, configurable via DB)
		if database.IsRedisAvailable() {
			queueEnabled := api.SystemSettings.GetSettingBool(context.Background(), "deployment_queue_enabled", true)
			if queueEnabled {
				utils.StartupLog("📦 Starting deployment queue...")
				startDeploymentQueue()
			} else {
				utils.StartupLog("📦 Deployment queue disabled (configure via System Settings)")
			}
		} else {
			utils.StartupLog("📦 Deployment queue disabled (Redis not available)")
		}
	} else {
		utils.WarnLog("SKIP_DB_PING=true - Database connection skipped")
	}

	// Start Fiber application
	utils.StartupLog("Initializing web server...")
	app := fiber.New(fiber.Config{
		AppName:      "Citizen API",
		BodyLimit:    10 * 1024 * 1024, // 10MB max request body
		ReadTimeout:  10 * time.Minute, // 10 min (SSE needs long timeout)
		WriteTimeout: 10 * time.Minute, // 10 min (SSE needs long timeout)
		IdleTimeout:  2 * time.Minute,  // 2 min idle (prevents zombie connections)
		ServerHeader: "",               // Hide server info
		ErrorHandler: customErrorHandler,
	})

	// Add middleware
	setupMiddleware(app)

	// Main route
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message":     "Citizen API is running",
			"version":     "1.0.0",
			"environment": os.Getenv("ENVIRONMENT"),
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Background cleanup task
	go startBackgroundTasks()

	// Setup routes
	utils.StartupLog("Setting up API routes...")
	routes.SetupRoutes(app)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	utils.StartupLog("🎯 Server starting on port %s", port)
	utils.StartupLog("✅ Citizen Backend ready!")

	// Graceful shutdown setup
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := app.Listen(":" + port); err != nil {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErr:
		log.Fatalf("Server error: %v", err)
	case sig := <-shutdownChan:
		utils.StartupLog("🛑 Received signal %v, initiating graceful shutdown...", sig)
	}

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	utils.StartupLog("Shutting down server...")
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		utils.ErrorLog("Server shutdown error: %v", err)
	}

	// Stop deployment queue
	utils.StartupLog("Stopping deployment queue...")
	services.GetDeploymentQueue().Stop()

	utils.StartupLog("Closing database connections...")
	database.CloseDB()

	utils.StartupLog("Closing Redis connections...")
	database.CloseRedis()

	utils.StartupLog("✅ Graceful shutdown complete")
}

// setupMiddleware configures all middleware
func setupMiddleware(app *fiber.App) {
	// Enhanced logger middleware
	if utils.IsDevelopmentEnvironment() {
		app.Use(logger.New(logger.Config{
			Format:     "[${time}] ${status} - ${method} ${path} - ${latency}\n",
			TimeFormat: "15:04:05",
		}))
	} else {
		// Minimal logging in production
		app.Use(logger.New(logger.Config{
			Format:     "${time} ${status} ${method} ${path} ${latency}\n",
			TimeFormat: time.RFC3339,
		}))
	}

	// Environment configuration - used by multiple middleware
	environment := strings.ToLower(os.Getenv("ENVIRONMENT"))
	isProduction := environment == "prod" || environment == "production"

	// Security Headers Middleware
	app.Use(func(c *fiber.Ctx) error {
		// Basic security headers
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=(), payment=(), usb=(), magnetometer=(), gyroscope=(), speaker=()")

		// Environment-specific security headers
		if isProduction {
			// HSTS only in production with HTTPS
			c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")

			// Strict CSP for production
			csp := "default-src 'self'; " +
				"script-src 'self' 'unsafe-inline'; " +
				"style-src 'self' 'unsafe-inline'; " +
				"img-src 'self' data: https:; " +
				"font-src 'self'; " +
				"connect-src 'self'; " +
				"media-src 'self'; " +
				"object-src 'none'; " +
				"child-src 'none'; " +
				"worker-src 'none'; " +
				"frame-ancestors 'none'; " +
				"form-action 'self'; " +
				"base-uri 'self'; " +
				"manifest-src 'self'"
			c.Set("Content-Security-Policy", csp)
		} else {
			// More permissive CSP for development
			csp := "default-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
				"script-src 'self' 'unsafe-inline' 'unsafe-eval' localhost:* 127.0.0.1:*; " +
				"style-src 'self' 'unsafe-inline'; " +
				"img-src 'self' data: blob: localhost:* 127.0.0.1:*; " +
				"font-src 'self' data:; " +
				"connect-src 'self' localhost:* 127.0.0.1:* ws://localhost:* ws://127.0.0.1:*; " +
				"media-src 'self'; " +
				"object-src 'none'; " +
				"child-src 'self'; " +
				"worker-src 'self' blob:; " +
				"frame-ancestors 'self'; " +
				"form-action 'self'"
			c.Set("Content-Security-Policy", csp)
		}

		return c.Next()
	})

	// Enhanced CORS configuration
	setupCORS(app, isProduction)
}

// setupCORS configures CORS based on environment
func setupCORS(app *fiber.App, isProduction bool) {
	allowedMethods := "GET,POST,PUT,DELETE,OPTIONS"
	allowedHeaders := "Origin,Content-Type,Accept,Authorization,X-Requested-With,Cookie"
	corsOrigins := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))

	if corsOrigins == "" {
		if isProduction {
			mainDomain := sanitizeDomain(os.Getenv("MAIN_DOMAIN"))
			if mainDomain == "" {
				mainDomain = autoDetectMainDomain()
				if mainDomain != "" {
					utils.StartupLog("Detected MAIN_DOMAIN from CitizenAuth configuration: %s", mainDomain)
				}
			}
			if mainDomain == "" {
				mainDomain = "localhost"
				utils.WarnLog("MAIN_DOMAIN is not configured; falling back to localhost for CORS. Set MAIN_DOMAIN or CORS_ALLOWED_ORIGINS to allow dashboard access from custom domains.")
			}
			citizenAuthURL := strings.TrimSpace(os.Getenv("CITIZENAUTH_URL"))
			if citizenAuthURL == "" {
				utils.WarnLog("CITIZENAUTH_URL not set in production, CORS may not work correctly for CitizenAuth")
			} else {
				corsOrigins = fmt.Sprintf("https://%s,https://*.%s,%s", mainDomain, mainDomain, citizenAuthURL)
			}
			if corsOrigins == "" {
				corsOrigins = fmt.Sprintf("https://%s,https://*.%s", mainDomain, mainDomain)
			}
		} else {
			// Development defaults
			citizenAuthURL := strings.TrimSpace(os.Getenv("CITIZENAUTH_URL"))
			if citizenAuthURL != "" {
				corsOrigins = fmt.Sprintf("http://localhost:3000,http://127.0.0.1:3000,http://localhost:5173,%s", citizenAuthURL)
			} else {
				corsOrigins = "http://localhost:3000,http://127.0.0.1:3000,http://localhost:5173"
			}
			allowedMethods = "GET,POST,PUT,DELETE,OPTIONS,PATCH,HEAD"
			allowedHeaders = "Origin,Content-Type,Accept,Authorization,X-Requested-With,Cookie,X-Forwarded-For,X-Real-IP,User-Agent,Referer"
		}
	}

	allowCredentials := true
	if corsOrigins == "*" {
		// Fiber panics if credentials are allowed with wildcard origins.
		allowCredentials = false
	}

	utils.StartupLog("CORS Origins: %s", corsOrigins)

	app.Use(cors.New(cors.Config{
		AllowOrigins:     corsOrigins,
		AllowCredentials: allowCredentials,
		AllowMethods:     allowedMethods,
		AllowHeaders:     allowedHeaders,
		ExposeHeaders:    "Set-Cookie",
	}))
}

func autoDetectMainDomain() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	instance, err := database.GetActiveCitizenauthInstance(ctx)
	if err != nil {
		if errors.Is(err, database.ErrCitizenauthInstanceNotFound) {
			utils.WarnLog("CORS: no CitizenAuth instance found to infer MAIN_DOMAIN")
		} else {
			utils.WarnLog("CORS: failed to load CitizenAuth instance for MAIN_DOMAIN: %v", err)
		}
		return ""
	}

	if instance.Domain == nil || strings.TrimSpace(*instance.Domain) == "" {
		utils.WarnLog("CORS: active CitizenAuth instance missing domain value")
		return ""
	}

	domain := sanitizeDomain(*instance.Domain)
	if domain == "" {
		utils.WarnLog("CORS: invalid domain value received from CitizenAuth instance: %s", *instance.Domain)
	}
	return domain
}

func sanitizeDomain(value string) string {
	domain := strings.TrimSpace(value)
	if domain == "" {
		return ""
	}
	if strings.HasPrefix(domain, "https://") {
		domain = strings.TrimPrefix(domain, "https://")
	} else if strings.HasPrefix(domain, "http://") {
		domain = strings.TrimPrefix(domain, "http://")
	}
	domain = strings.TrimPrefix(domain, "//")
	domain = strings.Trim(domain, "/")
	return domain
}

// customErrorHandler handles errors in a structured way
func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal Server Error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	utils.ErrorLog("HTTP Error %d: %s - Path: %s", code, message, c.Path())

	return c.Status(code).JSON(fiber.Map{
		"error":     true,
		"message":   message,
		"code":      code,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// startBackgroundTasks starts background maintenance tasks
func startBackgroundTasks() {
	// SSO cleanup every 5 minutes
	ssoTicker := time.NewTicker(5 * time.Minute)
	
	// Stale deployment crawler every 30 seconds
	staleDeploymentTicker := time.NewTicker(30 * time.Second)

	utils.StartupLog("Background cleanup tasks started")
	utils.StartupLog("🔍 Stale deployment crawler started (30s interval)")

	go func() {
		defer ssoTicker.Stop()
		for range ssoTicker.C {
			// Clean expired SSO tokens
			authservices.CleanExpiredSSOTokens()
			utils.DebugLog("Expired SSO tokens cleanup completed")
		}
	}()

	// Stale deployment crawler
	defer staleDeploymentTicker.Stop()
	for range staleDeploymentTicker.C {
		cleanupStaleDeployments()
	}
}

// cleanupStaleDeployments finds and marks stale deployments as failed
func cleanupStaleDeployments() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find deployments that are stuck (running for more than 30 minutes or superseded)
	staleRuns, err := database.GetStaleDeploymentRuns(ctx, 30*time.Minute)
	if err != nil {
		utils.DebugLog("Stale deployment check failed: %v", err)
		return
	}

	if len(staleRuns) == 0 {
		return
	}

	utils.WarnLog("🧹 Found %d stale deployment(s), marking as failed...", len(staleRuns))

	for _, run := range staleRuns {
		// Determine reason
		var reason string
		timeSinceStart := time.Since(run.StartedAt)
		if timeSinceStart > 30*time.Minute {
			reason = fmt.Sprintf("Deployment timed out after %v (server may have restarted)", timeSinceStart.Round(time.Second))
		} else {
			reason = "Deployment superseded by a newer deployment"
		}

		// Mark as failed
		if err := database.FailStaleDeploymentRun(ctx, run.RunID, reason); err != nil {
			utils.ErrorLog("Failed to mark stale deployment %s as failed: %v", run.RunID, err)
			continue
		}

		utils.WarnLog("🗑️ Marked deployment %s (%s) as failed: %s", run.RunID, run.AppName, reason)

		// Cleanup K3s resources if possible
		cleanupStaleDeploymentK3sResources(run.AppName)
	}
}

// cleanupStaleDeploymentK3sResources cleans up K3s build jobs for a stale deployment
func cleanupStaleDeploymentK3sResources(appName string) {
	// Try to get K3s adapter and cleanup build jobs
	if k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter); ok {
		if cleanupErr := k3sAdapter.CleanupCompletedBuildJobs(appName); cleanupErr != nil {
			utils.DebugLog("K3s cleanup for %s: %v", appName, cleanupErr)
		} else {
			utils.DebugLog("K3s build jobs cleaned up for %s", appName)
		}
	}
}

// loadGitHubConfigFromDB loads GitHub App configuration from database on startup
func loadGitHubConfigFromDB() {
	utils.DatabaseDebugLog("Loading GitHub config from database...")

	// Try to load config from database
	webhookSecret, appID, appSlug, appName, installationID, err := githubservices.LoadGitHubConfigFromDB()
	if err != nil {
		utils.DatabaseDebugLog("No GitHub config found in database: %v", err)
		return
	}

	// Setup webhook secret in memory
	githubservices.SetupGitHubWebhookSecret(webhookSecret)

	// Setup GitHub App in memory (private key is loaded from DB when needed)
	if appID != nil {
		// Mark that private key exists (actual key is fetched from DB when needed)
		privateKeyPlaceholder := "exists"
		githubservices.SetupGitHubApp(*appID, appSlug, &privateKeyPlaceholder, installationID, appName)
	}

	utils.StartupLog("GitHub App configuration loaded from database")
}

// startDeploymentQueue initializes and starts the deployment queue
func startDeploymentQueue() {
	// Get worker count from database (default: 3)
	workerCount := api.SystemSettings.GetSettingInt(context.Background(), "deployment_queue_workers", 3)
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > 10 {
		workerCount = 10
	}

	// Set the job processor to use the handlers package function
	// This avoids circular imports by using a function variable
	services.DefaultJobProcessor = func(ctx context.Context, job *services.DeploymentJob) error {
		// Import handlers dynamically to process jobs
		// The actual deployment logic is in handlers.ProcessQueuedDeployment
		return processQueuedDeploymentJob(ctx, job)
	}

	// Get and start the queue with configured worker count
	queue := services.NewDeploymentQueue(workerCount)
	if err := queue.Start(); err != nil {
		utils.ErrorLog("Failed to start deployment queue: %v", err)
		return
	}

	utils.StartupLog("✅ Deployment queue started with %d workers", workerCount)
}

// processQueuedDeploymentJob processes a deployment job from the queue
// This wraps the actual deployment logic to handle the job lifecycle
func processQueuedDeploymentJob(ctx context.Context, job *services.DeploymentJob) error {
	utils.StartupLog("📦 [QUEUE] Processing job %s for app %s", job.ID, job.AppName)

	// Get K3s adapter
	k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter)
	if !ok {
		return fmt.Errorf("K3s adapter not available")
	}

	runID := job.RunID
	appName := job.AppName

	// Update deployment steps - job is now running
	initLog := fmt.Sprintf("Starting deployment for %s\nGit URL: %s\nBranch: %s\nBuilder: %s\n", 
		appName, job.GitURL, job.GitBranch, job.Builder)
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "initializing", "completed", &initLog)
	deploymenthandlers.BroadcastDeploymentLog(runID, "initializing", "completed", initLog)

	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "cloning", "running", nil)
	deploymenthandlers.BroadcastStepUpdate(runID, "cloning", "running")
	api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "cloning")

	// Mark cloning as completed and building as running
	cloneLog := "Repository cloning started...\n"
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "cloning", "completed", &cloneLog)
	deploymenthandlers.BroadcastDeploymentLog(runID, "cloning", "completed", cloneLog)

	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "building", "running", nil)
	deploymenthandlers.BroadcastStepUpdate(runID, "building", "running")
	api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "building")

	// Check if this is a local deployment (tar.gz) or git deployment
	var output string
	var deployErr error

	if strings.HasPrefix(job.GitURL, "local://") {
		// Local deployment - extract tar.gz and build from local path
		tarballPath := strings.TrimPrefix(job.GitURL, "local://")
		utils.StartupLog("📦 [QUEUE] Local deployment detected, tarball: %s", tarballPath)

		// Create temp directory for extraction
		tempDir := fmt.Sprintf("/tmp/citizen-deploy-%s", runID)
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		defer os.RemoveAll(tempDir)

		// Extract tar.gz
		extractLog := fmt.Sprintf("Extracting %s...\n", tarballPath)
		deploymenthandlers.BroadcastDeploymentLog(runID, "building", "running", extractLog)
		api.DeploymentRuns.AppendBuildLogs(ctx, runID, "building", extractLog)

		extractCmd := exec.Command("tar", "-xzf", tarballPath, "-C", tempDir)
		extractOutput, extractErr := extractCmd.CombinedOutput()
		if extractErr != nil {
			return fmt.Errorf("failed to extract tarball: %w (output: %s)", extractErr, string(extractOutput))
		}

		// Build from local path
		output, deployErr = k3sAdapter.DeployFromLocalPathWithLogs(appName, tempDir, job.Builder, runID, func(logs string) {
			deploymenthandlers.BroadcastDeploymentLog(runID, "building", "running", logs)
			api.DeploymentRuns.AppendBuildLogs(ctx, runID, "building", logs)
		})
	} else {
		// Git deployment - clone and build from git URL
		output, deployErr = k3sAdapter.DeployFromGitWithLogs(appName, job.GitURL, job.GitBranch, job.TriggeredBy, func(logs string) {
			// Broadcast live logs to SSE subscribers
			deploymenthandlers.BroadcastDeploymentLog(runID, "building", "running", logs)
			// Also append to database
			api.DeploymentRuns.AppendBuildLogs(ctx, runID, "building", logs)
		})
	}

	if deployErr != nil {
		errorMsg := deployErr.Error()
		errLog := fmt.Sprintf("Deployment failed: %s", errorMsg)
		api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "building", "failed", &errLog)
		deploymenthandlers.BroadcastDeploymentLog(runID, "building", "failed", errLog)
		
		// Cleanup failed build jobs
		k3sAdapter.CleanupCompletedBuildJobs(appName)
		
		api.DeploymentRuns.CompleteDeploymentRun(ctx, runID, "failed", output, &errorMsg)
		deploymenthandlers.BroadcastRunUpdate(runID, "failed")
		return deployErr
	}

	// Update steps as completed
	buildLog := "Build completed successfully\n"
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "building", "completed", &buildLog)
	deploymenthandlers.BroadcastStepUpdate(runID, "building", "completed")
	deploymenthandlers.BroadcastDeploymentLog(runID, "building", "completed", buildLog)

	pushLog := "Image pushed to registry\n"
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "pushing", "running", nil)
	deploymenthandlers.BroadcastStepUpdate(runID, "pushing", "running")
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "pushing", "completed", &pushLog)
	deploymenthandlers.BroadcastDeploymentLog(runID, "pushing", "completed", pushLog)

	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "deploying", "running", nil)
	deploymenthandlers.BroadcastStepUpdate(runID, "deploying", "running")
	api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "deploying")

	// Wait for rollout
	rolloutLog := "Waiting for deployment rollout...\n"
	deploymenthandlers.BroadcastDeploymentLog(runID, "deploying", "running", rolloutLog)
	k3sAdapter.WaitForDeploymentRollout(appName, 5*time.Minute)

	deployLog := "Deployment rolled out successfully\n"
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "deploying", "completed", &deployLog)
	deploymenthandlers.BroadcastDeploymentLog(runID, "deploying", "completed", deployLog)

	// Cleanup
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "cleanup", "running", nil)
	deploymenthandlers.BroadcastStepUpdate(runID, "cleanup", "running")
	k3sAdapter.CleanupCompletedBuildJobs(appName)
	cleanupLog := "Old build jobs cleaned up\n"
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "cleanup", "completed", &cleanupLog)
	deploymenthandlers.BroadcastDeploymentLog(runID, "cleanup", "completed", cleanupLog)

	// Complete
	api.DeploymentRuns.CompleteDeploymentRun(ctx, runID, "completed", output, nil)
	deploymenthandlers.BroadcastRunUpdate(runID, "completed")

	// Update app_deployments table status to "deployed"
	newDeployment := &models.AppDeployment{
		AppName:    appName,
		GitURL:     job.GitURL,
		GitBranch:  job.GitBranch,
		Builder:    job.Builder,
		Status:     "deployed",
		LastDeploy: time.Now(),
	}
	if err := database.SaveAppDeployment(newDeployment); err != nil {
		utils.ErrorLog("📦 [QUEUE] Failed to update app_deployments status: %v", err)
	} else {
		utils.StartupLog("📦 [QUEUE] Updated app_deployments status to 'deployed' for %s", appName)
	}

	utils.StartupLog("📦 [QUEUE] Deployment completed for job %s (app: %s)", job.ID, appName)
	return nil
}
