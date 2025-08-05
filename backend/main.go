package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"backend/database"
	"backend/handlers"
	"backend/routes"
	"backend/utils"

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

	// Start database connection (check skip flag)
	if os.Getenv("SKIP_DB_PING") != "true" {
		utils.StartupLog("Connecting to database...")
		database.ConnectDB()
		defer database.CloseDB()
		
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
		
		// Load GitHub config from database
		utils.StartupLog("Loading GitHub configuration...")
		loadGitHubConfigFromDB()
	} else {
		utils.WarnLog("SKIP_DB_PING=true - Database connection skipped")
	}
	
	// Test SSH connection (non-blocking)
	go func() {
		utils.StartupLog("Testing SSH connection...")
		err := utils.SSHConnect()
		if err != nil {
			utils.WarnLog("SSH connection failed during startup: %v", err)
			utils.InfoLog("SSH connection will be retried on first API call")
		} else {
			utils.StartupLog("SSH connection established successfully")
		}
	}()

	// Start Fiber application
	utils.StartupLog("Initializing web server...")
	app := fiber.New(fiber.Config{
		AppName:      "Citizen API",
		BodyLimit:    10 * 1024 * 1024, // 10MB max request body
		ReadTimeout:  30 * time.Second,  // 30 second read timeout
		WriteTimeout: 30 * time.Second,  // 30 second write timeout
		ServerHeader: "",                // Hide server info
		ErrorHandler: customErrorHandler,
	})

	// Add middleware
	setupMiddleware(app)

	// Main route
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Citizen API is running",
			"version": "1.0.0",
			"environment": os.Getenv("ENVIRONMENT"),
			"timestamp": time.Now().UTC().Format(time.RFC3339),
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
	
	log.Fatal(app.Listen(":" + port))
}

// setupMiddleware configures all middleware
func setupMiddleware(app *fiber.App) {
	// Enhanced logger middleware
	if utils.IsDevelopmentEnvironment() {
		app.Use(logger.New(logger.Config{
			Format: "[${time}] ${status} - ${method} ${path} - ${latency}\n",
			TimeFormat: "15:04:05",
		}))
	} else {
		// Minimal logging in production
		app.Use(logger.New(logger.Config{
			Format: "${time} ${status} ${method} ${path} ${latency}\n",
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
	var corsOrigins string
	var allowedMethods string
	var allowedHeaders string
	
	if isProduction {
		// Production: Subdomain support
		mainDomain := os.Getenv("MAIN_DOMAIN")
		if mainDomain == "" {
			mainDomain = "localhost" // Fallback for testing
		}
		corsOrigins = fmt.Sprintf("https://%s,https://*.%s", mainDomain, mainDomain)
		allowedMethods = "GET,POST,PUT,DELETE,OPTIONS"
		allowedHeaders = "Origin,Content-Type,Accept,Authorization,X-Requested-With,Cookie"
	} else {
		// Development: Dynamic CORS policy for localhost subdomain support
		corsOrigins = "*" // Allow all origins in development
		allowedMethods = "GET,POST,PUT,DELETE,OPTIONS,PATCH,HEAD"
		allowedHeaders = "Origin,Content-Type,Accept,Authorization,X-Requested-With,Cookie,X-Forwarded-For,X-Real-IP,User-Agent,Referer"
	}
	
	utils.StartupLog("CORS Origins: %s", corsOrigins)
	
	if isProduction {
		// Production: Use strict CORS
		app.Use(cors.New(cors.Config{
			AllowOrigins:     corsOrigins,
			AllowCredentials: true,
			AllowMethods:     allowedMethods,
			AllowHeaders:     allowedHeaders,
			ExposeHeaders:    "Set-Cookie",
		}))
	} else {
		// Development: Dynamic CORS for localhost subdomains
		app.Use(cors.New(cors.Config{
			AllowOriginsFunc: func(origin string) bool {
				// Allow localhost and any *.localhost subdomain
				if strings.Contains(origin, "localhost") {
					return true
				}
				// Allow common dev ports
				if strings.Contains(origin, "127.0.0.1") {
					return true
				}
				return false
			},
			AllowCredentials: true,
			AllowMethods:     allowedMethods,
			AllowHeaders:     allowedHeaders,
			ExposeHeaders:    "Set-Cookie",
		}))
	}
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
		"error": true,
		"message": message,
		"code": code,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// startBackgroundTasks starts background maintenance tasks
func startBackgroundTasks() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	utils.StartupLog("Background cleanup tasks started")
	
	for {
		select {
		case <-ticker.C:
			// Clean expired SSO tokens
			handlers.CleanExpiredSSOTokens()
			utils.DebugLog("Expired SSO tokens cleanup completed")
		}
	}
}

// loadGitHubConfigFromDB loads GitHub configuration from database on startup
func loadGitHubConfigFromDB() {
	utils.DatabaseDebugLog("Loading GitHub config from database...")
	
	// Try to load config from database
	clientID, clientSecret, redirectURI, webhookSecret, err := handlers.LoadGitHubConfigFromDB()
	if err != nil {
		utils.DatabaseDebugLog("No GitHub config found in database: %v", err)
		return
	}
	
	// Setup GitHub OAuth in memory
	err = utils.SetupGitHubOAuth(clientID, clientSecret, redirectURI, webhookSecret)
	if err != nil {
		utils.ErrorLog("Failed to setup GitHub OAuth from database: %v", err)
		return
	}
	
	utils.StartupLog("GitHub configuration loaded from database")
}
