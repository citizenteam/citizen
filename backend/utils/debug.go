package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// Environment detection
func IsProductionEnvironment() bool {
	env := strings.ToLower(os.Getenv("ENVIRONMENT"))
	return env == "prod" || env == "production"
}

func IsDevelopmentEnvironment() bool {
	return !IsProductionEnvironment()
}

// Structured log entry for JSON logging
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Component string `json:"component,omitempty"`
	Message   string `json:"message"`
	Details   interface{} `json:"details,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Log format detection
func shouldUseJSONLogging() bool {
	format := strings.ToLower(os.Getenv("LOG_FORMAT"))
	return format == "json"
}

// Enhanced logging functions with structured output
func DebugLog(format string, args ...interface{}) {
	if IsDevelopmentEnvironment() {
		if shouldUseJSONLogging() {
			entry := LogEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Level:     "DEBUG",
				Message:   sprintf(format, args...),
			}
			if jsonData, err := json.Marshal(entry); err == nil {
				log.Println(string(jsonData))
			} else {
				log.Printf("[DEBUG] "+format, args...)
			}
		} else {
			log.Printf("[DEBUG] "+format, args...)
		}
	}
}

func InfoLog(format string, args ...interface{}) {
	if shouldUseJSONLogging() {
		entry := LogEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Level:     "INFO",
			Message:   sprintf(format, args...),
		}
		if jsonData, err := json.Marshal(entry); err == nil {
			log.Println(string(jsonData))
		} else {
			log.Printf("[INFO] "+format, args...)
		}
	} else {
		log.Printf("[INFO] "+format, args...)
	}
}

func ErrorLog(format string, args ...interface{}) {
	if shouldUseJSONLogging() {
		entry := LogEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Level:     "ERROR",
			Message:   sprintf(format, args...),
		}
		if jsonData, err := json.Marshal(entry); err == nil {
			log.Println(string(jsonData))
		} else {
			log.Printf("[ERROR] "+format, args...)
		}
	} else {
		log.Printf("[ERROR] "+format, args...)
	}
}

func WarnLog(format string, args ...interface{}) {
	if shouldUseJSONLogging() {
		entry := LogEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Level:     "WARN",
			Message:   sprintf(format, args...),
		}
		if jsonData, err := json.Marshal(entry); err == nil {
			log.Println(string(jsonData))
		} else {
			log.Printf("[WARN] "+format, args...)
		}
	} else {
		log.Printf("[WARN] "+format, args...)
	}
}

// Component-specific debug logs with structured output
func ComponentDebugLog(component, format string, args ...interface{}) {
	if IsDevelopmentEnvironment() {
		if shouldUseJSONLogging() {
			entry := LogEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Level:     "DEBUG",
				Component: component,
				Message:   sprintf(format, args...),
			}
			if jsonData, err := json.Marshal(entry); err == nil {
				log.Println(string(jsonData))
			} else {
				log.Printf("[%s DEBUG] "+format, append([]interface{}{component}, args...)...)
			}
		} else {
			log.Printf("[%s DEBUG] "+format, append([]interface{}{component}, args...)...)
		}
	}
}

// Auth specific debug logs
func AuthDebugLog(format string, args ...interface{}) {
	ComponentDebugLog("AUTH", format, args...)
}

func SSHDebugLog(format string, args ...interface{}) {
	ComponentDebugLog("SSH", format, args...)
}

// Session debug logs with session context
func SessionDebugLog(sessionID string, format string, args ...interface{}) {
	if IsDevelopmentEnvironment() {
		if shouldUseJSONLogging() {
			entry := LogEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Level:     "DEBUG",
				Component: "SESSION",
				Message:   sprintf(format, args...),
				Details:   map[string]string{"session_id": sessionID},
			}
			if jsonData, err := json.Marshal(entry); err == nil {
				log.Println(string(jsonData))
			} else {
				allArgs := append([]interface{}{sessionID}, args...)
				log.Printf("[SESSION] ID:'%s' - "+format, allArgs...)
			}
		} else {
			allArgs := append([]interface{}{sessionID}, args...)
			log.Printf("[SESSION] ID:'%s' - "+format, allArgs...)
		}
	}
}

// Request debug logs with request context
func RequestDebugLog(method, path string, format string, args ...interface{}) {
	if IsDevelopmentEnvironment() {
		if shouldUseJSONLogging() {
			entry := LogEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Level:     "DEBUG",
				Component: "REQUEST",
				Message:   sprintf(format, args...),
				Details: map[string]string{
					"method": method,
					"path":   path,
				},
			}
			if jsonData, err := json.Marshal(entry); err == nil {
				log.Println(string(jsonData))
			} else {
				allArgs := append([]interface{}{method, path}, args...)
				log.Printf("[REQUEST] %s %s - "+format, allArgs...)
			}
		} else {
			allArgs := append([]interface{}{method, path}, args...)
			log.Printf("[REQUEST] %s %s - "+format, allArgs...)
		}
	}
}

// Performance debug logs with timing
func PerfDebugLog(operation string, startTime time.Time, format string, args ...interface{}) {
	if IsDevelopmentEnvironment() {
		duration := time.Since(startTime)
		if shouldUseJSONLogging() {
			entry := LogEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Level:     "DEBUG",
				Component: "PERFORMANCE",
				Message:   sprintf(format, args...),
				Details: map[string]interface{}{
					"operation": operation,
					"duration_ms": duration.Milliseconds(),
					"duration": duration.String(),
				},
			}
			if jsonData, err := json.Marshal(entry); err == nil {
				log.Println(string(jsonData))
			} else {
				allArgs := append([]interface{}{operation, duration}, args...)
				log.Printf("[PERF] %s took %v - "+format, allArgs...)
			}
		} else {
			allArgs := append([]interface{}{operation, duration}, args...)
			log.Printf("[PERF] %s took %v - "+format, allArgs...)
		}
	}
}

// Security debug logs (always log security events, even in production)
func SecurityLog(format string, args ...interface{}) {
	if shouldUseJSONLogging() {
		entry := LogEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Level:     "SECURITY",
			Component: "SECURITY",
			Message:   sprintf(format, args...),
		}
		if jsonData, err := json.Marshal(entry); err == nil {
			log.Println(string(jsonData))
		} else {
			log.Printf("[SECURITY] "+format, args...)
		}
	} else {
		log.Printf("[SECURITY] "+format, args...)
	}
}

// Database debug logs
func DatabaseDebugLog(format string, args ...interface{}) {
	ComponentDebugLog("DATABASE", format, args...)
}

// Redis debug logs  
func RedisDebugLog(format string, args ...interface{}) {
	ComponentDebugLog("REDIS", format, args...)
}

// Startup logs (always shown, optimized for environment)
func StartupLog(format string, args ...interface{}) {
	if IsProductionEnvironment() {
		// Minimize startup noise in production
		if shouldUseJSONLogging() {
			entry := LogEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Level:     "INFO",
				Component: "STARTUP",
				Message:   sprintf(format, args...),
			}
			if jsonData, err := json.Marshal(entry); err == nil {
				log.Println(string(jsonData))
			} else {
				log.Printf("[STARTUP] "+format, args...)
			}
		} else {
			log.Printf("✅ "+format, args...)
		}
	} else {
		// More verbose in development
		log.Printf("[STARTUP] "+format, args...)
	}
}

// Helper function for sprintf
func sprintf(format string, args ...interface{}) string {
	if len(args) == 0 {
		return format
	}
	// Use fmt.Sprintf for string formatting
	return fmt.Sprintf(format, args...)
}

// Environment info logging
func LogEnvironmentInfo() {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "dev"
	}
	
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	
	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "" {
		logFormat = "text"
	}
	
	StartupLog("Environment: %s, Log Level: %s, Log Format: %s", env, logLevel, logFormat)
} 