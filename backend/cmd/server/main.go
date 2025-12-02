package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	authservices "backend/internal/auth/services"
	"backend/internal/database"
	githubservices "backend/internal/github/services"
	"backend/internal/middleware"
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
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	utils.StartupLog("Background cleanup tasks started")

	for range ticker.C {
		// Clean expired SSO tokens
		authservices.CleanExpiredSSOTokens()
		utils.DebugLog("Expired SSO tokens cleanup completed")
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
