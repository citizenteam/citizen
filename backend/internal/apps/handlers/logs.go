package handlers

import (
	"backend/internal/platform"
	"backend/internal/utils"
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GetAppLogs gets the logs of an app
func GetAppLogs(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Get query parameters
	tail := c.QueryInt("tail", 100)          // Default 100 lines
	logType := c.Query("type", "app")        // app, build, deploy
	processType := c.Query("process", "web") // web, worker, all

	var logs string
	var err error

	switch logType {
	case "build":
		logs, err = platform.GetAdapter().GetBuildLogs(appName)
	case "deploy":
		logs, err = platform.GetAdapter().GetDeployLogs(appName)
	case "all":
		// Logs for all processes
		logs, err = platform.GetAdapter().GetAllProcessLogs(appName, tail)
	default:
		// Logs for a specific process or web process
		if processType == "all" {
			logs, err = platform.GetAdapter().GetAllProcessLogs(appName, tail)
		} else {
			logs, err = platform.GetAdapter().GetProcessSpecificLogs(appName, processType, tail)
		}
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to fetch logs: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Logs fetched successfully",
		fiber.Map{
			"logs":      logs,
			"type":      logType,
			"process":   processType,
			"tail":      tail,
			"timestamp": time.Now().Unix(),
		},
	))
}

// StreamAppLogs streams the logs of an app
func StreamAppLogs(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Access-Control-Allow-Headers", "Cache-Control")

	// Configure SSE using StreamWriter
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Get initial logs and send
		logs, err := platform.GetAdapter().GetAppLogs(appName, 50, false)
		if err != nil {
			fmt.Fprintf(w, "data: {\"error\": \"%s\"}\n\n", err.Error())
			w.Flush()
			return
		}

		// Send logs in SSE format
		logData := map[string]interface{}{
			"logs":      logs,
			"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
			"type":      "initial",
		}

		jsonData, _ := json.Marshal(logData)
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		w.Flush()

		// Send periodic pings for keep-alive
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Send ping
				fmt.Fprintf(w, "data: {\"type\": \"ping\"}\n\n")
				w.Flush()
			case <-c.Context().Done():
				return
			}
		}
	})

	return nil
}

// GetLogInfo gets log information
func GetLogInfo(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	logInfo, err := platform.GetAdapter().GetLogInfo(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while getting log information: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Log info retrieved successfully",
		logInfo,
	))
}

// GetLiveBuildLogs gets only build/deploy output (simplified)
func GetLiveBuildLogs(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Get build logs (deploy output only)
	buildLogs, err := platform.GetAdapter().GetBuildLogs(appName)
	if err != nil {
		fmt.Printf("[LOGS] Failed to get build logs: %v\n", err)
		buildLogs = "No build logs available yet..."
	}

	appLogs, appErr := platform.GetAdapter().GetAppLogs(appName, 200, false)
	if appErr != nil {
		fmt.Printf("[LOGS] Failed to get app logs: %v\n", appErr)
	}

	combinedLogs := buildLogs
	if strings.TrimSpace(appLogs) != "" {
		combinedLogs += "\n\n----- Application logs -----\n" + appLogs
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Build logs retrieved successfully",
		fiber.Map{
			"logs":           combinedLogs,
			"has_build_logs": buildLogs != "",
			"has_app_logs":   strings.TrimSpace(appLogs) != "",
			"timestamp":      time.Now().Unix(),
		},
	))
}
