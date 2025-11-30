package logger_test

import (
	"os"

	"backend/pkg/logger"
)

func ExampleLogger() {
	// Initialize logger
	logger.Init(logger.Config{
		Level:     "DEBUG",
		Format:    "text",
		Component: "example",
	})

	// Use package-level functions
	logger.Debug("Debug message")
	logger.Info("Info message")
	logger.Warn("Warning message")
	logger.Error("Error message")

	// Use logger instance
	log := logger.Default()
	log.WithField("user_id", 123).Info("User logged in")
	log.WithFields(map[string]interface{}{
		"request_id": "abc123",
		"method":     "GET",
		"path":       "/api/users",
	}).Info("Request processed")
}

func ExampleLoggerWithComponent() {
	log := logger.Default().WithComponent("auth")
	log.Info("Authentication started")
	log.WithField("user_id", 456).Debug("User authenticated")
}

func ExampleLoggerJSON() {
	logger.Init(logger.Config{
		Level:     "INFO",
		Format:    "json",
		Component: "api",
		Output:    os.Stdout,
	})

	logger.Info("Server started", "port", 3000)
}
