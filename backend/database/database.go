package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"backend/config"
	"backend/database/api"
	"backend/utils"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var DB *pgxpool.Pool

// ConnectDB establishes database connection with retry logic and optimized pooling
func ConnectDB() {
	cfg, err := config.LoadConfig()
	if err != nil {
		utils.ErrorLog("Failed to load config for database: %v", err)
		log.Fatalf("Failed to load config for database: %v", err)
	}

	// Build connection string
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode)

	// Enhanced connection pool configuration
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		utils.ErrorLog("Failed to parse database config: %v", err)
		log.Fatalf("Failed to parse database config: %v", err)
	}

	// Optimize connection pool settings based on environment
	if utils.IsProductionEnvironment() {
		// Production settings - more conservative
		poolConfig.MaxConns = 25
		poolConfig.MinConns = 5
		poolConfig.MaxConnLifetime = time.Hour
		poolConfig.MaxConnIdleTime = time.Minute * 30
		poolConfig.HealthCheckPeriod = time.Minute * 1
	} else {
		// Development settings - lighter load
		poolConfig.MaxConns = 10
		poolConfig.MinConns = 2
		poolConfig.MaxConnLifetime = time.Minute * 30
		poolConfig.MaxConnIdleTime = time.Minute * 10
		poolConfig.HealthCheckPeriod = time.Minute * 2
	}
	
	// Connection timeout settings
	poolConfig.ConnConfig.ConnectTimeout = time.Second * 10

	utils.DatabaseDebugLog("Pool config - MaxConns: %d, MinConns: %d, MaxLifetime: %v", 
		poolConfig.MaxConns, poolConfig.MinConns, poolConfig.MaxConnLifetime)

	// Retry connection with exponential backoff
	maxRetries := 5
	baseDelay := time.Second * 2

	for attempt := 1; attempt <= maxRetries; attempt++ {
		utils.DatabaseDebugLog("Database connection attempt %d/%d", attempt, maxRetries)
		
		DB, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
		if err != nil {
			utils.WarnLog("Database connection attempt %d failed: %v", attempt, err)
			if attempt == maxRetries {
				utils.ErrorLog("All database connection attempts failed")
				log.Fatalf("Database connection failed after %d attempts: %v", maxRetries, err)
			}
			
			// Exponential backoff
			delay := baseDelay * time.Duration(1<<(attempt-1))
			utils.DatabaseDebugLog("Retrying in %v...", delay)
			time.Sleep(delay)
			continue
		}

		// Test connection with ping
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		err = DB.Ping(ctx)
		cancel()
		
		if err != nil {
			utils.WarnLog("Database ping failed on attempt %d: %v", attempt, err)
			DB.Close()
			DB = nil
			
			if attempt == maxRetries {
				utils.ErrorLog("Database ping failed after %d attempts", maxRetries)
				log.Fatalf("Database ping failed after %d attempts: %v", maxRetries, err)
			}
			
			delay := baseDelay * time.Duration(1<<(attempt-1))
			utils.DatabaseDebugLog("Retrying in %v...", delay)
			time.Sleep(delay)
			continue
		}

		// Success!
		break
	}

	utils.StartupLog("Database connection established successfully")
	utils.DatabaseDebugLog("Connection pool stats - Max: %d, Available: %d", 
		DB.Stat().MaxConns(), DB.Stat().IdleConns())

	// Initialize the database API with the connection pool
	api.InitDB(DB)
	utils.StartupLog("Database API initialized")
}

// CloseDB gracefully closes the database connection
func CloseDB() {
	if DB != nil {
		utils.DatabaseDebugLog("Closing database connection...")
		
		// Get final stats before closing
		stats := DB.Stat()
		utils.DatabaseDebugLog("Final pool stats - Total: %d, Idle: %d, Used: %d", 
			stats.TotalConns(), stats.IdleConns(), stats.AcquiredConns())
		
		DB.Close()
		utils.StartupLog("Database connection closed")
	}
}

// GetDBStats returns database connection pool statistics
func GetDBStats() map[string]interface{} {
	if DB == nil {
		return map[string]interface{}{
			"status": "disconnected",
		}
	}
	
	stats := DB.Stat()
	return map[string]interface{}{
		"status":          "connected",
		"max_conns":       stats.MaxConns(),
		"total_conns":     stats.TotalConns(),
		"idle_conns":      stats.IdleConns(),
		"acquired_conns":  stats.AcquiredConns(),
		"new_conns_count": stats.NewConnsCount(),
		"acquire_count":   stats.AcquireCount(),
		"cancel_count":    stats.CanceledAcquireCount(),
	}
}

// HealthCheck performs a database health check
func HealthCheck() error {
	if DB == nil {
		return fmt.Errorf("database connection not initialized")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	
	// Simple ping test
	err := DB.Ping(ctx)
	if err != nil {
		utils.ErrorLog("Database health check failed: %v", err)
		return fmt.Errorf("database ping failed: %w", err)
	}
	
	// Test with a simple query
	var result int
	err = DB.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		utils.ErrorLog("Database query test failed: %v", err)
		return fmt.Errorf("database query test failed: %w", err)
	}
	
	utils.DatabaseDebugLog("Database health check passed")
	return nil
}

// CreateAdminUserFromEnv creates admin user from environment variables
func CreateAdminUserFromEnv() error {
	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")
	email := os.Getenv("ADMIN_EMAIL")
	
	// Skip if environment variables are not set
	if username == "" || password == "" || email == "" {
		utils.DatabaseDebugLog("Admin user environment variables not found, skipping admin creation")
		return nil
	}
	
	utils.DatabaseDebugLog("Creating admin user: %s", username)
	
	// Hash password
	hashedPassword, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("failed to hash admin password: %w", err)
	}

	// Create admin user with upsert
	createAdminUser := `
	INSERT INTO users (username, password, email)
	VALUES ($1, $2, $3)
	ON CONFLICT (username) DO UPDATE SET
		password = EXCLUDED.password,
		email = EXCLUDED.email,
		updated_at = CURRENT_TIMESTAMP;`

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	_, err = DB.Exec(ctx, createAdminUser, username, hashedPassword, email)
	if err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}
	
	utils.StartupLog("Admin user created/updated successfully (username: %s, email: %s)", username, email)
	return nil
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}
