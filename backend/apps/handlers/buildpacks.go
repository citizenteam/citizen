package handlers

import (
	"backend/platform"
	"backend/utils"

	"github.com/gofiber/fiber/v2"
)

// ListBuildpacks lists the buildpacks of an app
func ListBuildpacks(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	buildpacks, err := platform.GetAdapter().ListBuildpacks(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while listing buildpacks: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Buildpacks listed successfully",
		buildpacks,
	))
}

// AddBuildpack adds a buildpack to an app
func AddBuildpack(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var data struct {
		BuildpackURL string `json:"buildpack_url"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	if data.BuildpackURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Buildpack URL is required",
			nil,
		))
	}

	output, err := platform.GetAdapter().AddBuildpack(appName, data.BuildpackURL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while adding the buildpack: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Buildpack added successfully",
		fiber.Map{
			"app_name":      appName,
			"buildpack_url": data.BuildpackURL,
			"output":        output,
		},
	))
}

// SetBuildpack sets the buildpack of an app
func SetBuildpack(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var data struct {
		BuildpackURL string `json:"buildpack_url"`
		Index        int    `json:"index,omitempty"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	if data.BuildpackURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Buildpack URL is required",
			nil,
		))
	}

	output, err := platform.GetAdapter().SetBuildpack(appName, data.BuildpackURL, data.Index)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while setting the buildpack: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Buildpack set successfully",
		fiber.Map{
			"app_name":      appName,
			"buildpack_url": data.BuildpackURL,
			"index":         data.Index,
			"output":        output,
		},
	))
}

// RemoveBuildpack removes a buildpack from an app
func RemoveBuildpack(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var data struct {
		BuildpackURL string `json:"buildpack_url"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	if data.BuildpackURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Buildpack URL is required",
			nil,
		))
	}

	output, err := platform.GetAdapter().RemoveBuildpack(appName, data.BuildpackURL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while removing the buildpack: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Buildpack removed successfully",
		fiber.Map{
			"app_name":      appName,
			"buildpack_url": data.BuildpackURL,
			"output":        output,
		},
	))
}

// ClearBuildpacks clears all buildpacks of an app
func ClearBuildpacks(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	output, err := platform.GetAdapter().ClearBuildpacks(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while clearing buildpacks: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Buildpacks cleared successfully",
		fiber.Map{
			"app_name": appName,
			"output":   output,
		},
	))
}

// GetBuildpackReport gets the buildpack report of an app
func GetBuildpackReport(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	report, err := platform.GetAdapter().GetBuildpackReport(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while getting the buildpack report: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Buildpack report retrieved successfully",
		report,
	))
}
