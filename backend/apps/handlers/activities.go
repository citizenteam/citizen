package handlers

import (
	"backend/database"
	"backend/utils"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GetAppActivities gets the activities of an app
func GetAppActivities(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Use new activity system
	activities, err := database.GetAppActivities(appName, 10)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to fetch activities: "+err.Error(),
			nil,
		))
	}

	// Format for frontend
	var formattedActivities []fiber.Map
	for _, activity := range activities {
		formattedActivity := fiber.Map{
			"id":        activity.ID,
			"type":      string(activity.Type),
			"message":   activity.Message,
			"timestamp": activity.StartedAt.Format(time.RFC3339),
			"status":    string(activity.Status),
		}

		// Add details if available
		if activity.Details != nil {
			formattedActivity["details"] = activity.Details
		}

		// Add duration if available
		if activity.Duration != nil {
			formattedActivity["duration"] = *activity.Duration
		}

		// Add error message if available
		if activity.ErrorMessage != nil {
			formattedActivity["error_message"] = *activity.ErrorMessage
		}

		// Add trigger type
		formattedActivity["trigger_type"] = string(activity.TriggerType)

		formattedActivities = append(formattedActivities, formattedActivity)
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Activities retrieved successfully",
		fiber.Map{
			"activities": formattedActivities,
			"total":      len(formattedActivities),
		},
	))
}
