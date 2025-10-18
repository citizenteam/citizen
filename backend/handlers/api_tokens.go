package handlers

import (
	"backend/database/api"
	"backend/models"
	"backend/utils"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// CreateAPIToken creates a new API token for the authenticated user
func CreateAPIToken(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)

	var req models.APITokenRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	// Validate required fields
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Token name is required",
			nil,
		))
	}

	// Create token
	tokenResponse, err := api.APITokens.CreateAPIToken(c.Context(), userID, &req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to create token: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusCreated).JSON(utils.NewCitizenResponse(
		true,
		"API token created successfully",
		tokenResponse,
	))
}

// ListAPITokens returns all API tokens for the authenticated user
func ListAPITokens(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)

	tokens, err := api.APITokens.ListAPITokens(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to list tokens: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"API tokens retrieved successfully",
		tokens,
	))
}

// DeleteAPIToken deletes an API token
func DeleteAPIToken(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)
	tokenIDStr := c.Params("token_id")

	tokenID, err := strconv.Atoi(tokenIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid token ID",
			nil,
		))
	}

	err = api.APITokens.DeleteAPIToken(c.Context(), userID, tokenID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to delete token: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"API token deleted successfully",
		nil,
	))
}

// GetAppAPIAccess returns API access configuration for a specific app
func GetAppAPIAccess(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	accessConfig, err := api.AppAPIAccess.GetAppAPIAccess(c.Context(), appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to get app API access settings: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"App API access settings retrieved successfully",
		accessConfig,
	))
}

// SetAppAPIAccess sets API access configuration for a specific app
func SetAppAPIAccess(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var req struct {
		APIAccessEnabled   bool `json:"api_access_enabled"`
		RateLimitPerMinute int  `json:"rate_limit_per_minute,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	// Debug log
	fmt.Printf("SetAppAPIAccess - App: %s, Enabled: %v, RateLimit: %d\n", appName, req.APIAccessEnabled, req.RateLimitPerMinute)

	// Set defaults
	if req.RateLimitPerMinute <= 0 {
		req.RateLimitPerMinute = 60
	}

	// All operations are allowed when API access is enabled
	allowedOperations := []string{"*"}
	err := api.AppAPIAccess.SetAppAPIAccess(c.Context(), appName, req.APIAccessEnabled, allowedOperations, req.RateLimitPerMinute)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to update app API access settings: "+err.Error(),
			nil,
		))
	}

	// Return the updated config directly without re-querying database
	// This avoids transaction/connection timing issues
	accessConfig := &models.AppAPIAccessResponse{
		AppName:              appName,
		APIAccessEnabled:     req.APIAccessEnabled,
		AllowedOperations:    allowedOperations,
		RateLimitPerMinute:   req.RateLimitPerMinute,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	fmt.Printf("SetAppAPIAccess RESPONSE - App: %s, Enabled: %v\n", appName, accessConfig.APIAccessEnabled)

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"App API access settings updated successfully",
		accessConfig,
	))
}

// GetAvailableOperations returns simple message that API tokens have full access
func GetAvailableOperations(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"API tokens have full access to all operations",
		fiber.Map{
			"message": "API tokens provide complete access to all application operations when API access is enabled for an app.",
			"operations": []string{"*"},
			"descriptions": map[string]string{
				"*": "Full access to all operations including deploy, restart, logs, env vars, domains, etc.",
			},
		},
	))
}