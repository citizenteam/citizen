package handlers

import (
	"backend/database"
	"backend/platform"
	"backend/utils"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

// SetEnv sets the environment variables of an app
func SetEnv(c *fiber.Ctx) error {
	// Get app name
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Parse request body
	var data struct {
		EnvVars map[string]string `json:"env_vars"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	// Check environment variables
	if data.EnvVars == nil || len(data.EnvVars) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"At least one environment variable is required",
			nil,
		))
	}

	// Check PORT variable and prevent manual modification
	if _, exists := data.EnvVars["PORT"]; exists {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"PORT environment variable cannot be modified manually. It is automatically set during deployment.",
			nil,
		))
	}

	// 📝 Log env activities for each variable
	var userID *int
	if userIDValue := c.Locals("user_id"); userIDValue != nil {
		if uid, ok := userIDValue.(int); ok {
			userID = &uid
		}
	}

	var envActivities []*database.Activity
	for key := range data.EnvVars {
		envActivity, activityErr := database.LogEnvActivity(appName, key, "set", userID)
		if activityErr != nil {
			fmt.Printf("[ACTIVITY] ⚠️ Failed to log env activity for %s: %v\n", key, activityErr)
		} else {
			envActivities = append(envActivities, envActivity)
		}
	}

	// Set environment variables
	output, err := platform.GetAdapter().SetEnv(appName, data.EnvVars)
	if err != nil {
		// 📝 Update env activities as failed
		for _, activity := range envActivities {
			if activity != nil {
				errorMsg := err.Error()
				database.UpdateActivity(activity.ID, database.StatusError, &errorMsg)
			}
		}

		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while setting environment variables: "+err.Error(),
			nil,
		))
	}

	// 📝 Update env activities as successful
	for _, activity := range envActivities {
		if activity != nil {
			database.UpdateActivity(activity.ID, database.StatusSuccess, nil)
		}
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Environment variables set successfully",
		fiber.Map{
			"app_name": appName,
			"env_vars": data.EnvVars,
			"output":   output,
		},
	))
}

// GetEnv gets the environment variables of an app
func GetEnv(c *fiber.Ctx) error {
	// Get app name
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Get environment variables
	envVars, err := platform.GetAdapter().GetEnv(appName)
	if err != nil {
		if errors.Is(err, platform.ErrAppNotFound) {
			return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
				true,
				"App has no deployment yet; environment variables are not set",
				fiber.Map{},
			))
		}
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while getting environment variables: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Environment variables retrieved successfully",
		envVars,
	))
}

// RemoveEnv removes an environment variable from an app
func RemoveEnv(c *fiber.Ctx) error {
	// Get app name
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Parse request body
	var data struct {
		Key string `json:"key"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	// Check environment variable key
	if data.Key == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Environment variable key is required",
			nil,
		))
	}

	// Prevent manual removal of PORT variable
	if data.Key == "PORT" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"PORT environment variable cannot be removed manually. It is automatically managed during deployment.",
			nil,
		))
	}

	// 📝 Log env remove activity start
	var userID *int
	if userIDValue := c.Locals("user_id"); userIDValue != nil {
		if uid, ok := userIDValue.(int); ok {
			userID = &uid
		}
	}

	envActivity, activityErr := database.LogEnvActivity(appName, data.Key, "remove", userID)
	if activityErr != nil {
		fmt.Printf("[ACTIVITY] ⚠️ Failed to log env activity: %v\n", activityErr)
	}

	// Remove environment variable
	output, err := platform.GetAdapter().RemoveEnv(appName, data.Key)
	if err != nil {
		// 📝 Update env activity as failed
		if envActivity != nil {
			errorMsg := err.Error()
			database.UpdateActivity(envActivity.ID, database.StatusError, &errorMsg)
		}

		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while removing the environment variable: "+err.Error(),
			nil,
		))
	}

	// 📝 Update env activity as successful
	if envActivity != nil {
		database.UpdateActivity(envActivity.ID, database.StatusSuccess, nil)
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Environment variable removed successfully",
		fiber.Map{
			"app_name": appName,
			"key":      data.Key,
			"output":   output,
		},
	))
}
