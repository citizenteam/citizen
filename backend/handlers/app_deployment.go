package handlers

import (
	"backend/database"
	"backend/models"
	"backend/utils"

	"github.com/gofiber/fiber/v2"
)

// GetAppDeployment retrieves deployment information for a specific app
func GetAppDeployment(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	deployment, err := database.GetAppDeployment(appName)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(utils.NewCitizenResponse(
			false,
			"App deployment not found: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"App deployment retrieved successfully",
		deployment,
	))
}

// GetAllAppDeployments retrieves all app deployments
func GetAllAppDeployments(c *fiber.Ctx) error {
	deployments, err := database.GetAllAppDeployments()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to retrieve app deployments: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"App deployments retrieved successfully",
		deployments,
	))
}

// UpdateAppDeployment updates app deployment information
func UpdateAppDeployment(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var updateData models.AppDeploymentRequest
	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body: "+err.Error(),
			nil,
		))
	}

	// Get existing deployment
	deployment, err := database.GetAppDeployment(appName)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(utils.NewCitizenResponse(
			false,
			"App deployment not found: "+err.Error(),
			nil,
		))
	}

	// Update fields
	if updateData.Domain != "" {
		deployment.Domain = updateData.Domain
	}
	if updateData.Port != 0 {
		deployment.Port = updateData.Port
	}
	if updateData.Builder != "" {
		deployment.Builder = updateData.Builder
	}
	if updateData.Buildpack != "" {
		deployment.Buildpack = updateData.Buildpack
	}
	if updateData.GitURL != "" {
		deployment.GitURL = updateData.GitURL
	}
	if updateData.GitBranch != "" {
		deployment.GitBranch = updateData.GitBranch
	}
	if updateData.PortSource != "" {
		deployment.PortSource = updateData.PortSource
	}

	// Save updated deployment
	if err := database.SaveAppDeployment(deployment); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to update app deployment: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"App deployment updated successfully",
		deployment,
	))
}

// UpdateAppDeploymentStatus updates the deployment status
func UpdateAppDeploymentStatus(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var statusData struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.BodyParser(&statusData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body: "+err.Error(),
			nil,
		))
	}

	if statusData.Status == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Status is required",
			nil,
		))
	}

	// Update status in database
	if err := database.UpdateAppDeploymentStatus(appName, statusData.Status); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to update deployment status: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Deployment status updated successfully",
		fiber.Map{
			"app_name": appName,
			"status":   statusData.Status,
		},
	))
} 