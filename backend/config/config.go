package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config structure holds the application configuration settings
type Config struct {
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode     string
	EncryptionKey string
	Port          string
	
	// SSH Connection Settings
	SSHHost     string
	SSHPort     int
	SSHUser     string
	SSHPassword string
	SSHKeyPath  string
	
	// Redis Configuration
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int
}

// LoadConfig loads configuration settings from environment variables
func LoadConfig() (*Config, error) {
	var missingVars []string
	
	// Required environment variables check
	requiredVars := map[string]string{
		"DB_HOST":     os.Getenv("DB_HOST"),
		"DB_USER":     os.Getenv("DB_USER"),
		"DB_PASSWORD": os.Getenv("DB_PASSWORD"),
		"DB_NAME":     os.Getenv("DB_NAME"),
		"SSH_HOST":    os.Getenv("SSH_HOST"),
		"SSH_USER":    os.Getenv("SSH_USER"),
	}
	
	for key, value := range requiredVars {
		if value == "" {
			missingVars = append(missingVars, key)
		}
	}
	
	if len(missingVars) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missingVars)
	}
	
	// Parse ports with validation
	dbPort, err := parsePort("DB_PORT", "5432")
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %w", err)
	}
	
	sshPort, err := parsePort("SSH_PORT", "22")
	if err != nil {
		return nil, fmt.Errorf("invalid SSH_PORT: %w", err)
	}
	
	redisDB, err := parseRedisDB("REDIS_DB", "0")
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_DB: %w", err)
	}

	return &Config{
		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     dbPort,
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
		DBSSLMode:     getEnvWithDefault("DB_SSL_MODE", "require"), // Secure default
		EncryptionKey: os.Getenv("ENCRYPTION_KEY"), // No default - will be validated elsewhere
		Port:          getEnvWithDefault("PORT", "3000"),
		
		// SSH Settings
		SSHHost:     os.Getenv("SSH_HOST"),
		SSHPort:     sshPort,
		SSHUser:     os.Getenv("SSH_USER"),
		SSHPassword: os.Getenv("SSH_PASSWORD"), // Can be empty if using key auth
		SSHKeyPath:  getEnvWithDefault("SSH_KEY_PATH", "~/.ssh/id_rsa"),
		
		// Redis Configuration - optional, can have defaults for non-critical services
		RedisHost:     getEnvWithDefault("REDIS_HOST", "localhost"),
		RedisPort:     getEnvWithDefault("REDIS_PORT", "6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"), // No default for security
		RedisDB:       redisDB,
	}, nil
}

// getEnvWithDefault returns environment variable value or default if empty
// Only use for non-sensitive configurations
func getEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// parsePort validates and parses port number from environment variable
func parsePort(envKey, defaultValue string) (int, error) {
	portStr := getEnvWithDefault(envKey, defaultValue)
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid port number '%s': %w", portStr, err)
	}
	
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port number out of range (1-65535): %d", port)
	}
	
	return port, nil
}

// parseRedisDB validates and parses Redis DB number
func parseRedisDB(envKey, defaultValue string) (int, error) {
	dbStr := getEnvWithDefault(envKey, defaultValue)
	db, err := strconv.Atoi(dbStr)
	if err != nil {
		return 0, fmt.Errorf("invalid Redis DB number '%s': %w", dbStr, err)
	}
	
	if db < 0 || db > 15 {
		return 0, fmt.Errorf("Redis DB number out of range (0-15): %d", db)
	}
	
	return db, nil
}

// ValidateConfig checks if all required configuration is present
func (c *Config) ValidateConfig() error {
	var errors []string
	
	if c.DBHost == "" {
		errors = append(errors, "DB_HOST is required")
	}
	if c.DBUser == "" {
		errors = append(errors, "DB_USER is required")
	}
	if c.DBPassword == "" {
		errors = append(errors, "DB_PASSWORD is required")
	}
	if c.DBName == "" {
		errors = append(errors, "DB_NAME is required")
	}
	if c.SSHHost == "" {
		errors = append(errors, "SSH_HOST is required")
	}
	if c.SSHUser == "" {
		errors = append(errors, "SSH_USER is required")
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed: %v", errors)
	}
	
	return nil
} 