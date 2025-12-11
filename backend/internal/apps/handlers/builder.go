package handlers

import (
	"backend/internal/database/api"
	"backend/internal/models"
	"backend/internal/platform"
	"backend/internal/utils"
	"log"

	"github.com/gofiber/fiber/v2"
)

// SetBuilder sets the builder of an app
func SetBuilder(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var data struct {
		BuilderType string `json:"builder_type"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	// Check valid builder types
	validBuilders := []string{"herokuish", "pack", "dockerfile", "nixpacks"}
	isValid := false
	for _, valid := range validBuilders {
		if data.BuilderType == valid {
			isValid = true
			break
		}
	}

	if !isValid {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid builder type. Valid types: herokuish, pack, dockerfile, nixpacks",
			nil,
		))
	}

	output, err := platform.GetAdapter().SetBuilder(appName, data.BuilderType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while setting the builder: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Builder set successfully",
		fiber.Map{
			"app_name":     appName,
			"builder_type": data.BuilderType,
			"output":       output,
		},
	))
}

// GetBuilderReport gets the builder report of an app
func GetBuilderReport(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	report, err := platform.GetAdapter().GetBuilderReport(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while getting the builder report: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Builder report retrieved successfully",
		report,
	))
}

// GetBuildSettings returns build settings for an app
func GetBuildSettings(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	settings, err := api.BuildSettings.GetBuildSettings(c.Context(), appName)
	if err != nil {
		log.Printf("[BUILD] Failed to get build settings for %s: %v", appName, err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to get build settings",
			nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Build settings retrieved",
		fiber.Map{
			"app_name":        settings.AppName,
			"builder_type":    settings.BuilderType,
			"dockerfile_path": settings.DockerfilePath,
			"resolved_type":   settings.ResolvedBuilderType(),
		},
	))
}

// SetBuildSettings updates build settings for an app
func SetBuildSettings(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var req struct {
		BuilderType    string `json:"builder_type"`
		DockerfilePath string `json:"dockerfile_path"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	// Validate builder type
	builderType := models.BuilderType(req.BuilderType)
	switch builderType {
	case models.BuilderTypeAuto, models.BuilderTypeDockerfile, models.BuilderTypeNixpacks:
		// Valid
	case "":
		builderType = models.BuilderTypeAuto
	default:
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid builder type. Use: auto, dockerfile, or nixpacks",
			nil,
		))
	}

	settings := &models.AppBuildSettings{
		AppName:        appName,
		BuilderType:    builderType,
		DockerfilePath: req.DockerfilePath,
	}

	if settings.DockerfilePath == "" {
		settings.DockerfilePath = "Dockerfile"
	}

	if err := api.BuildSettings.SaveBuildSettings(c.Context(), settings); err != nil {
		log.Printf("[BUILD] Failed to save build settings for %s: %v", appName, err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to save build settings",
			nil,
		))
	}

	log.Printf("[BUILD] ✅ Build settings updated for %s: type=%s", appName, builderType)

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Build settings updated",
		fiber.Map{
			"app_name":        settings.AppName,
			"builder_type":    settings.BuilderType,
			"dockerfile_path": settings.DockerfilePath,
			"resolved_type":   settings.ResolvedBuilderType(),
		},
	))
}

// SetBuilderType is a convenience endpoint to just set the builder type
func SetBuilderType(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var req struct {
		BuilderType string `json:"builder_type"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	// Validate and normalize builder type
	builderType := models.BuilderType(req.BuilderType)
	switch builderType {
	case models.BuilderTypeAuto, models.BuilderTypeDockerfile, models.BuilderTypeNixpacks:
		// Valid
	case "":
		builderType = models.BuilderTypeAuto
	default:
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid builder type. Use: auto, dockerfile, or nixpacks",
			nil,
		))
	}

	if err := api.BuildSettings.SetBuilderType(c.Context(), appName, builderType); err != nil {
		log.Printf("[BUILD] Failed to set builder type for %s: %v", appName, err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to set builder type",
			nil,
		))
	}

	log.Printf("[BUILD] ✅ Builder type set for %s: %s", appName, builderType)

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Builder type updated",
		fiber.Map{
			"app_name":     appName,
			"builder_type": builderType,
		},
	))
}
