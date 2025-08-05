package handlers

import (
	"os"
	"runtime"
	"time"
	"backend/database"
	"backend/utils"

	"github.com/gofiber/fiber/v2"
)

// HealthStatus represents the overall health status
type HealthStatus struct {
	Status      string                 `json:"status"`
	Timestamp   string                 `json:"timestamp"`
	Environment string                 `json:"environment"`
	Version     string                 `json:"version"`
	Service     string                 `json:"service"`
	Uptime      string                 `json:"uptime"`
	Components  map[string]ComponentHealth `json:"components"`
	Metrics     SystemMetrics          `json:"metrics"`
}

// ComponentHealth represents health status of individual components
type ComponentHealth struct {
	Status    string                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Error     string                 `json:"error,omitempty"`
	LastCheck string                 `json:"last_check"`
}

// SystemMetrics contains system performance metrics
type SystemMetrics struct {
	Memory    MemoryMetrics `json:"memory"`
	Goroutines int          `json:"goroutines"`
	GCRuns    uint32        `json:"gc_runs"`
}

// MemoryMetrics contains memory usage information
type MemoryMetrics struct {
	Alloc      uint64 `json:"alloc_mb"`
	TotalAlloc uint64 `json:"total_alloc_mb"`
	Sys        uint64 `json:"sys_mb"`
	HeapAlloc  uint64 `json:"heap_alloc_mb"`
	HeapSys    uint64 `json:"heap_sys_mb"`
}

var startTime = time.Now()

// HealthCheck returns comprehensive health status of the application
func HealthCheck(c *fiber.Ctx) error {
	utils.RequestDebugLog(c.Method(), c.Path(), "Health check requested")
	
	now := time.Now()
	environment := os.Getenv("ENVIRONMENT")
	if environment == "" {
		environment = "development"
	}

	healthStatus := HealthStatus{
		Status:      "healthy",
		Timestamp:   now.UTC().Format(time.RFC3339),
		Environment: environment,
		Version:     "1.0.0",
		Service:     "citizen-backend",
		Uptime:      time.Since(startTime).String(),
		Components:  make(map[string]ComponentHealth),
		Metrics:     getSystemMetrics(),
	}

	// Check database health
	dbHealth := checkDatabaseHealth()
	healthStatus.Components["database"] = dbHealth

	// Check Redis health
	redisHealth := checkRedisHealth()
	healthStatus.Components["redis"] = redisHealth

	// Check SSH connectivity (optional - don't fail on SSH issues)
	sshHealth := checkSSHHealth()
	healthStatus.Components["ssh"] = sshHealth

	// Determine overall health status
	overallHealthy := true
	criticalComponents := []string{"database"} // Only database is critical

	for _, component := range criticalComponents {
		if health, exists := healthStatus.Components[component]; exists {
			if health.Status != "healthy" {
				overallHealthy = false
				break
			}
		}
	}

	if !overallHealthy {
		healthStatus.Status = "unhealthy"
		utils.WarnLog("Health check failed - service marked as unhealthy")
		return c.Status(fiber.StatusServiceUnavailable).JSON(healthStatus)
	}

	utils.DebugLog("Health check passed - all critical components healthy")
	return c.Status(fiber.StatusOK).JSON(healthStatus)
}

// checkDatabaseHealth performs comprehensive database health check
func checkDatabaseHealth() ComponentHealth {
	now := time.Now().UTC().Format(time.RFC3339)
	
	if database.DB == nil {
		return ComponentHealth{
			Status:    "unhealthy",
			Message:   "Database connection not initialized",
			Error:     "Database connection is nil",
			LastCheck: now,
		}
	}

	// Perform database health check
	err := database.HealthCheck()
	if err != nil {
		return ComponentHealth{
			Status:    "unhealthy",
			Message:   "Database health check failed",
			Error:     err.Error(),
			LastCheck: now,
		}
	}

	// Get database statistics
	stats := database.GetDBStats()

	return ComponentHealth{
		Status:    "healthy",
		Message:   "Database connection healthy",
		Details:   stats,
		LastCheck: now,
	}
}

// checkRedisHealth performs comprehensive Redis health check
func checkRedisHealth() ComponentHealth {
	now := time.Now().UTC().Format(time.RFC3339)
	
	if !database.IsRedisAvailable() {
		return ComponentHealth{
			Status:    "degraded",
			Message:   "Redis not available - using fallback mode",
			Details: map[string]interface{}{
				"fallback_mode": true,
			},
			LastCheck: now,
		}
	}

	// Perform Redis health check
	err := database.HealthCheck()
	if err != nil {
		return ComponentHealth{
			Status:    "degraded",
			Message:   "Redis health check failed - fallback mode active",
			Error:     err.Error(),
			Details: map[string]interface{}{
				"fallback_mode": true,
			},
			LastCheck: now,
		}
	}

	// Get Redis statistics
	stats := database.GetRedisStats()

	return ComponentHealth{
		Status:    "healthy",
		Message:   "Redis connection healthy",
		Details:   stats,
		LastCheck: now,
	}
}

// checkSSHHealth performs SSH connectivity check
func checkSSHHealth() ComponentHealth {
	now := time.Now().UTC().Format(time.RFC3339)
	
	// SSH is not critical for basic API functionality
	// This is more of an informational check
	
	sshHost := os.Getenv("SSH_HOST")
	if sshHost == "" {
		return ComponentHealth{
			Status:    "not_configured",
			Message:   "SSH connection not configured",
			LastCheck: now,
		}
	}

	// For now, just return configured status
	// A more comprehensive check could be implemented later
	return ComponentHealth{
		Status:    "configured",
		Message:   "SSH connection configured",
		Details: map[string]interface{}{
			"ssh_host": sshHost,
		},
		LastCheck: now,
	}
}

// getSystemMetrics collects system performance metrics
func getSystemMetrics() SystemMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return SystemMetrics{
		Memory: MemoryMetrics{
			Alloc:      bToMb(m.Alloc),
			TotalAlloc: bToMb(m.TotalAlloc),
			Sys:        bToMb(m.Sys),
			HeapAlloc:  bToMb(m.HeapAlloc),
			HeapSys:    bToMb(m.HeapSys),
		},
		Goroutines: runtime.NumGoroutine(),
		GCRuns:     m.NumGC,
	}
}

// bToMb converts bytes to megabytes
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

// DetailedHealthCheck returns detailed health information (admin endpoint)
func DetailedHealthCheck(c *fiber.Ctx) error {
	// This could be protected by admin auth in the future
	utils.RequestDebugLog(c.Method(), c.Path(), "Detailed health check requested")
	
	detailed := fiber.Map{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   "citizen-backend",
		"uptime":    time.Since(startTime).String(),
	}

	// Add database details
	if database.DB != nil {
		detailed["database"] = database.GetDBStats()
	}

	// Add Redis details
	if database.IsRedisAvailable() {
		detailed["redis"] = database.GetRedisStats()
	}

	// Add system metrics
	detailed["metrics"] = getSystemMetrics()

	// Add environment info
	detailed["environment"] = fiber.Map{
		"ENVIRONMENT":   os.Getenv("ENVIRONMENT"),
		"LOG_LEVEL":     os.Getenv("LOG_LEVEL"),
		"LOG_FORMAT":    os.Getenv("LOG_FORMAT"),
		"MAIN_DOMAIN":   os.Getenv("MAIN_DOMAIN"),
		"REDIS_HOST":    os.Getenv("REDIS_HOST"),
		"DB_HOST":       os.Getenv("DB_HOST"),
	}

	return c.Status(fiber.StatusOK).JSON(detailed)
}

// ReadinessCheck checks if the service is ready to accept requests
func ReadinessCheck(c *fiber.Ctx) error {
	// Simple readiness check - database must be available
	if database.DB == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ready": false,
			"reason": "database not available",
		})
	}

	// Quick database ping
	err := database.HealthCheck()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ready": false,
			"reason": "database not ready",
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"ready": true,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// LivenessCheck checks if the service is alive (basic functionality)
func LivenessCheck(c *fiber.Ctx) error {
	// Very basic liveness check
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"alive": true,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service": "citizen-backend",
	})
}

// RedisStatus returns detailed Redis status (legacy endpoint)
func RedisStatus(c *fiber.Ctx) error {
	if !database.IsRedisAvailable() {
		return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
			true,
			"Redis not available - fallback mode active",
			fiber.Map{
				"available": false,
				"fallback_mode": true,
			},
		))
	}

	stats := database.GetRedisStats()
	
	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Redis status",
		stats,
	))
}

// ClearRedisTestData clears test data from Redis (development only)
func ClearRedisTestData(c *fiber.Ctx) error {
	if utils.IsProductionEnvironment() {
		return c.Status(fiber.StatusForbidden).JSON(utils.NewCitizenResponse(
			false,
			"Test data cleanup not allowed in production",
			nil,
		))
	}

	if !database.IsRedisAvailable() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(utils.NewCitizenResponse(
			false,
			"Redis not available",
			nil,
		))
	}

	// Clean up test patterns
	patterns := []string{
		"test:*",
		"health_check_test*",
		"session:test:*",
	}

	totalDeleted := 0
	for _, pattern := range patterns {
		deleted, err := database.CleanupExpiredKeys(pattern)
		if err != nil {
			utils.WarnLog("Failed to cleanup pattern %s: %v", pattern, err)
			continue
		}
		totalDeleted += deleted
	}
	
	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Test data cleanup completed",
		fiber.Map{
			"deleted_keys": totalDeleted,
			"patterns": patterns,
		},
	))
} 