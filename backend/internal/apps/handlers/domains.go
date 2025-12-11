package handlers

import (
	"backend/internal/database"
	"backend/internal/platform"
	"backend/internal/utils"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

// ListDomains lists the domains of an app
func ListDomains(c *fiber.Ctx) error {
	// Get app name
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Get domains
	domains, err := platform.GetAdapter().ListDomains(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while listing domains: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Domains listed successfully",
		domains,
	))
}

// AddDomain adds a domain to an app
func AddDomain(c *fiber.Ctx) error {
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
		Domain string `json:"domain"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	// Check domain name
	if data.Domain == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Domain name is required",
			nil,
		))
	}

	// 📝 Log domain add activity start
	var userID *int
	if userIDValue := c.Locals("user_id"); userIDValue != nil {
		if uid, ok := userIDValue.(int); ok {
			userID = &uid
		}
	}

	domainActivity, activityErr := database.LogDomainActivity(appName, data.Domain, "add", userID)
	if activityErr != nil {
		fmt.Printf("[ACTIVITY] ⚠️ Failed to log domain activity: %v\n", activityErr)
	}

	// Add domain
	output, err := platform.GetAdapter().AddDomain(appName, data.Domain)
	if err != nil {
		// 📝 Update domain activity as failed
		if domainActivity != nil {
			errorMsg := err.Error()
			database.UpdateActivity(domainActivity.ID, database.StatusError, &errorMsg)
		}

		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while adding the domain: "+err.Error(),
			nil,
		))
	}

	// 📝 Update domain activity as successful
	if domainActivity != nil {
		database.UpdateActivity(domainActivity.ID, database.StatusSuccess, nil)
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Domain added successfully",
		fiber.Map{
			"app_name": appName,
			"domain":   data.Domain,
			"output":   output,
		},
	))
}

// RemoveDomain removes a domain from an app
func RemoveDomain(c *fiber.Ctx) error {
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
		Domain string `json:"domain"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	// Check domain name
	if data.Domain == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Domain name is required",
			nil,
		))
	}

	// 📝 Log domain remove activity start
	var userID *int
	if userIDValue := c.Locals("user_id"); userIDValue != nil {
		if uid, ok := userIDValue.(int); ok {
			userID = &uid
		}
	}

	domainActivity, activityErr := database.LogDomainActivity(appName, data.Domain, "remove", userID)
	if activityErr != nil {
		fmt.Printf("[ACTIVITY] ⚠️ Failed to log domain activity: %v\n", activityErr)
	}

	// Remove domain
	output, err := platform.GetAdapter().RemoveDomain(appName, data.Domain)
	if err != nil {
		// 📝 Update domain activity as failed
		if domainActivity != nil {
			errorMsg := err.Error()
			database.UpdateActivity(domainActivity.ID, database.StatusError, &errorMsg)
		}

		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while removing the domain: "+err.Error(),
			nil,
		))
	}

	// 📝 Update domain activity as successful
	if domainActivity != nil {
		database.UpdateActivity(domainActivity.ID, database.StatusSuccess, nil)
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Domain removed successfully",
		fiber.Map{
			"app_name": appName,
			"domain":   data.Domain,
			"output":   output,
		},
	))
}
