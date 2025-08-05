package database

import (
	"context"
	"backend/config"
	"backend/utils"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	RedisClient *redis.Client
	ctx         = context.Background()
)

// InitRedis initializes Redis connection with retry logic and optimized settings
func InitRedis() {
	cfg, err := config.LoadConfig()
	if err != nil {
		utils.ErrorLog("Failed to load config for Redis: %v", err)
		return
	}
	
	// Create Redis client with enhanced configuration
	options := &redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
		
		// Connection pool settings
		PoolSize:           10,              // Connection pool size
		MinIdleConns:       2,               // Minimum idle connections
		MaxIdleConns:       5,               // Maximum idle connections
		ConnMaxLifetime:    time.Hour,       // Connection lifetime
		ConnMaxIdleTime:    time.Minute * 30, // Max idle time
		
		// Timeout settings
		DialTimeout:  time.Second * 5,  // Connection timeout
		ReadTimeout:  time.Second * 3,  // Read timeout
		WriteTimeout: time.Second * 3,  // Write timeout
		PoolTimeout:  time.Second * 4,  // Pool timeout
		
		// Retry settings
		MaxRetries:      3,
		MinRetryBackoff: time.Millisecond * 100,
		MaxRetryBackoff: time.Second * 2,
	}
	
	// Adjust settings based on environment
	if utils.IsProductionEnvironment() {
		options.PoolSize = 20
		options.MinIdleConns = 5
		options.MaxIdleConns = 10
	}
	
	RedisClient = redis.NewClient(options)
	
	utils.RedisDebugLog("Redis client created - Pool size: %d, DB: %d", options.PoolSize, options.DB)

	// Test connection with retry logic
	maxRetries := 5
	baseDelay := time.Second * 2

	for attempt := 1; attempt <= maxRetries; attempt++ {
		utils.RedisDebugLog("Redis connection attempt %d/%d", attempt, maxRetries)
		
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		pong, err := RedisClient.Ping(ctx).Result()
		cancel()
		
		if err != nil {
			utils.WarnLog("Redis connection attempt %d failed: %v", attempt, err)
			
			if attempt == maxRetries {
				utils.ErrorLog("Redis connection failed after %d attempts - continuing with fallback", maxRetries)
				// Don't exit, continue with in-memory storage as fallback
				RedisClient = nil
				return
			}
			
			// Exponential backoff
			delay := baseDelay * time.Duration(1<<(attempt-1))
			utils.RedisDebugLog("Retrying Redis connection in %v...", delay)
			time.Sleep(delay)
			continue
		}
		
		// Success!
		utils.StartupLog("Redis connected successfully: %s", pong)
		
		// Log Redis info in development
		if utils.IsDevelopmentEnvironment() {
			logRedisInfo()
		}
		
		return
	}
}

// logRedisInfo logs Redis server information in development
func logRedisInfo() {
	if RedisClient == nil {
		return
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	
	info, err := RedisClient.Info(ctx, "server").Result()
	if err != nil {
		utils.RedisDebugLog("Failed to get Redis info: %v", err)
		return
	}
	
	lines := strings.Split(info, "\r\n")
	serverInfo := make(map[string]string)
	
	for _, line := range lines {
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "#") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				serverInfo[parts[0]] = parts[1]
			}
		}
	}
	
	if version, ok := serverInfo["redis_version"]; ok {
		utils.RedisDebugLog("Redis server version: %s", version)
	}
	if mode, ok := serverInfo["redis_mode"]; ok {
		utils.RedisDebugLog("Redis mode: %s", mode)
	}
}

// GetRedisStats returns Redis connection and server statistics
func GetRedisStats() map[string]interface{} {
	if RedisClient == nil {
		return map[string]interface{}{
			"status": "disconnected",
		}
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	
	// Get pool stats
	poolStats := RedisClient.PoolStats()
	
	stats := map[string]interface{}{
		"status":       "connected",
		"pool_hits":    poolStats.Hits,
		"pool_misses":  poolStats.Misses,
		"pool_timeouts": poolStats.Timeouts,
		"total_conns":  poolStats.TotalConns,
		"idle_conns":   poolStats.IdleConns,
		"stale_conns":  poolStats.StaleConns,
	}
	
	// Try to get server info
	info, err := RedisClient.Info(ctx, "memory", "stats").Result()
	if err != nil {
		utils.RedisDebugLog("Failed to get Redis server info: %v", err)
		stats["server_info_error"] = err.Error()
		return stats
	}
	
	// Parse server info
	lines := strings.Split(info, "\r\n")
	for _, line := range lines {
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "#") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]
				
				// Include important metrics
				switch key {
				case "used_memory_human", "used_memory_peak_human", "total_commands_processed", 
					 "instantaneous_ops_per_sec", "keyspace_hits", "keyspace_misses":
					stats[key] = value
				}
			}
		}
	}
	
	return stats
}

// RedisHealthCheck performs a Redis health check
func RedisHealthCheck() error {
	if RedisClient == nil {
		return fmt.Errorf("redis client not initialized")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	
	// Test ping
	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		utils.ErrorLog("Redis health check ping failed: %v", err)
		return fmt.Errorf("redis ping failed: %w", err)
	}
	
	// Test set/get operation
	testKey := "health_check_test"
	testValue := fmt.Sprintf("health_check_%d", time.Now().Unix())
	
	err = RedisClient.Set(ctx, testKey, testValue, time.Second*5).Err()
	if err != nil {
		utils.ErrorLog("Redis health check set failed: %v", err)
		return fmt.Errorf("redis set operation failed: %w", err)
	}
	
	retrievedValue, err := RedisClient.Get(ctx, testKey).Result()
	if err != nil {
		utils.ErrorLog("Redis health check get failed: %v", err)
		return fmt.Errorf("redis get operation failed: %w", err)
	}
	
	if retrievedValue != testValue {
		return fmt.Errorf("redis health check value mismatch: expected %s, got %s", testValue, retrievedValue)
	}
	
	// Clean up test key
	RedisClient.Del(ctx, testKey)
	
	utils.RedisDebugLog("Redis health check passed")
	return nil
}

// IsRedisAvailable checks if Redis is available
func IsRedisAvailable() bool {
	return RedisClient != nil
}

// Generic Redis Functions for SSO with improved error handling

// SetWithTTL sets a key-value pair with TTL
func SetWithTTL(key string, value string, duration time.Duration) error {
	if RedisClient == nil {
		utils.RedisDebugLog("Redis not available, operation failed: SetWithTTL")
		return fmt.Errorf("redis client not initialized")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	
	err := RedisClient.Set(ctx, key, value, duration).Err()
	if err != nil {
		utils.RedisDebugLog("SetWithTTL failed for key %s: %v", key, err)
		return fmt.Errorf("failed to set key %s: %w", key, err)
	}
	
	utils.RedisDebugLog("SetWithTTL successful for key %s (TTL: %v)", key, duration)
	return nil
}

// Get retrieves a value by key
func Get(key string) (string, error) {
	if RedisClient == nil {
		utils.RedisDebugLog("Redis not available, operation failed: Get")
		return "", fmt.Errorf("redis client not initialized")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	
	val, err := RedisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		utils.RedisDebugLog("Key not found: %s", key)
		return "", fmt.Errorf("key not found")
	}
	if err != nil {
		utils.RedisDebugLog("Get failed for key %s: %v", key, err)
		return "", fmt.Errorf("failed to get key %s: %w", key, err)
	}
	
	utils.RedisDebugLog("Get successful for key %s", key)
	return val, nil
}

// Delete removes a key
func Delete(key string) error {
	if RedisClient == nil {
		utils.RedisDebugLog("Redis not available, operation failed: Delete")
		return fmt.Errorf("redis client not initialized")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	
	err := RedisClient.Del(ctx, key).Err()
	if err != nil {
		utils.RedisDebugLog("Delete failed for key %s: %v", key, err)
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}
	
	utils.RedisDebugLog("Delete successful for key %s", key)
	return nil
}

// Exists checks if a key exists
func Exists(key string) (bool, error) {
	if RedisClient == nil {
		utils.RedisDebugLog("Redis not available, operation failed: Exists")
		return false, fmt.Errorf("redis client not initialized")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	
	count, err := RedisClient.Exists(ctx, key).Result()
	if err != nil {
		utils.RedisDebugLog("Exists check failed for key %s: %v", key, err)
		return false, fmt.Errorf("failed to check key existence %s: %w", key, err)
	}
	
	exists := count > 0
	utils.RedisDebugLog("Exists check for key %s: %v", key, exists)
	return exists, nil
}

// SetJSON stores a JSON object with TTL
func SetJSON(key string, value interface{}, duration time.Duration) error {
	if RedisClient == nil {
		return fmt.Errorf("redis client not initialized")
	}
	
	jsonData, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON for key %s: %w", key, err)
	}
	
	return SetWithTTL(key, string(jsonData), duration)
}

// GetJSON retrieves and unmarshals a JSON object
func GetJSON(key string, dest interface{}) error {
	if RedisClient == nil {
		return fmt.Errorf("redis client not initialized")
	}
	
	jsonStr, err := Get(key)
	if err != nil {
		return err
	}
	
	err = json.Unmarshal([]byte(jsonStr), dest)
	if err != nil {
		utils.RedisDebugLog("JSON unmarshal failed for key %s: %v", key, err)
		return fmt.Errorf("failed to unmarshal JSON for key %s: %w", key, err)
	}
	
	return nil
}

// CleanupExpiredKeys removes expired keys matching a pattern (use with caution)
func CleanupExpiredKeys(pattern string) (int, error) {
	if RedisClient == nil {
		return 0, fmt.Errorf("redis client not initialized")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	
	keys, err := RedisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to scan keys with pattern %s: %w", pattern, err)
	}
	
	deletedCount := 0
	for _, key := range keys {
		// Check if key exists and delete it
		exists, err := RedisClient.Exists(ctx, key).Result()
		if err != nil {
			utils.WarnLog("Failed to check existence of key %s: %v", key, err)
			continue
		}
		
		if exists > 0 {
			err = RedisClient.Del(ctx, key).Err()
			if err != nil {
				utils.WarnLog("Failed to delete key %s: %v", key, err)
				continue
			}
			deletedCount++
		}
	}
	
	utils.RedisDebugLog("Cleanup completed - deleted %d keys matching pattern %s", deletedCount, pattern)
	return deletedCount, nil
} 