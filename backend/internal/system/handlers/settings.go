package handlers

import (
	"backend/internal/database/api"
	"backend/internal/utils"

	"github.com/gofiber/fiber/v2"
)

// GetSystemSettings returns all system settings
func GetSystemSettings(c *fiber.Ctx) error {
	settings, err := api.SystemSettings.GetAllSettings(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to get system settings",
			nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"System settings retrieved",
		settings,
	))
}

// GetBuildSettings returns build-related settings
func GetBuildSettings(c *fiber.Ctx) error {
	settings, err := api.SystemSettings.GetBuildSettings(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to get build settings",
			nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Build settings retrieved",
		settings,
	))
}

// UpdateBuildSettings updates build-related settings
func UpdateBuildSettings(c *fiber.Ctx) error {
	var body struct {
		DeploymentQueueEnabled *bool `json:"deployment_queue_enabled"`
		DeploymentQueueWorkers *int  `json:"deployment_queue_workers"`
		BuildTimeoutMinutes    *int  `json:"build_timeout_minutes"`
		AutoCleanupOldBuilds   *bool `json:"auto_cleanup_old_builds"`
		MaxBuildsPerApp        *int  `json:"max_builds_per_app"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	settings := make(map[string]interface{})

	if body.DeploymentQueueEnabled != nil {
		settings["deployment_queue_enabled"] = *body.DeploymentQueueEnabled
	}
	if body.DeploymentQueueWorkers != nil {
		if *body.DeploymentQueueWorkers < 1 {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"Deployment queue workers must be at least 1",
				nil,
			))
		}
		if *body.DeploymentQueueWorkers > 10 {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"Deployment queue workers cannot exceed 10",
				nil,
			))
		}
		settings["deployment_queue_workers"] = *body.DeploymentQueueWorkers
	}
	if body.BuildTimeoutMinutes != nil {
		if *body.BuildTimeoutMinutes < 5 {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"Build timeout must be at least 5 minutes",
				nil,
			))
		}
		settings["build_timeout_minutes"] = *body.BuildTimeoutMinutes
	}
	if body.AutoCleanupOldBuilds != nil {
		settings["auto_cleanup_old_builds"] = *body.AutoCleanupOldBuilds
	}
	if body.MaxBuildsPerApp != nil {
		if *body.MaxBuildsPerApp < 1 {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"Max builds per app must be at least 1",
				nil,
			))
		}
		settings["max_builds_per_app"] = *body.MaxBuildsPerApp
	}

	if len(settings) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"No settings to update",
			nil,
		))
	}

	if err := api.SystemSettings.UpdateBuildSettings(c.Context(), settings); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to update build settings: "+err.Error(),
			nil,
		))
	}

	// Return updated settings
	updatedSettings, _ := api.SystemSettings.GetBuildSettings(c.Context())
	return c.JSON(utils.NewCitizenResponse(
		true,
		"Build settings updated",
		updatedSettings,
	))
}

// GetQueueStatus returns the current deployment queue status
func GetQueueStatus(c *fiber.Ctx) error {
	// Import here to avoid circular dependency
	// We'll get queue stats from the services package
	return c.JSON(utils.NewCitizenResponse(
		true,
		"Queue status",
		fiber.Map{
			"queue_enabled": api.SystemSettings.GetSettingBool(c.Context(), "deployment_queue_enabled", true),
			"workers":       api.SystemSettings.GetSettingInt(c.Context(), "deployment_queue_workers", 3),
		},
	))
}
