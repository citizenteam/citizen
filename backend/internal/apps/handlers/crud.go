package handlers

import (
	"backend/internal/database"
	"backend/internal/database/api"
	citizenauthservices "backend/internal/citizenauth/services"
	"backend/internal/models"
	"backend/internal/platform"
	"backend/internal/utils"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// ListApps lists all Citizen apps
func ListApps(c *fiber.Ctx) error {
	apps, err := platform.GetAdapter().ListApps()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while listing apps: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Apps listed successfully",
		apps,
	))
}

// CreateApp creates a new Citizen app
func CreateApp(c *fiber.Ctx) error {
	// Parse request body
	var data struct {
		AppName string `json:"app_name"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	// Check app name
	if data.AppName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	appName := strings.ToLower(strings.TrimSpace(data.AppName))

	// Create app
	output, err := platform.GetAdapter().CreateApp(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while creating the app: "+err.Error(),
			nil,
		))
	}

	domainResp, err := citizenauthservices.RegisterAppDomain(c.Context(), appName)
	if err != nil {
		// cleanup created app to avoid orphaned state
		if _, destroyErr := platform.GetAdapter().DestroyApp(appName); destroyErr != nil {
			fmt.Printf("[WARN] Failed to rollback app %s after domain error: %v\n", appName, destroyErr)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to register custom hostname: "+err.Error(),
			nil,
		))
	}

	if domainResp != nil && domainResp.Domain != "" {
		if updateErr := api.Deployments.UpdateDeploymentDomain(context.Background(), appName, domainResp.Domain); updateErr != nil {
			fmt.Printf("[WARN] Failed to persist deployment domain for %s: %v\n", appName, updateErr)
		}
		if err := api.Settings.UpsertPublicCustomDomain(context.Background(), appName, domainResp.Domain, false); err != nil {
			fmt.Printf("[WARN] Failed to persist public domain for %s: %v\n", appName, err)
		}
	}

	// Seed deployment metadata so the UI has context before the first deploy
	placeholderDeployment := &models.AppDeployment{
		AppName: appName,
		Status:  "pending",
		Port:    5000,
	}
	if domainResp != nil {
		placeholderDeployment.Domain = domainResp.Domain
	}
	if err := database.SaveAppDeployment(placeholderDeployment); err != nil {
		fmt.Printf("[WARN] Failed to seed deployment metadata for %s: %v\n", appName, err)
	}

	return c.Status(fiber.StatusCreated).JSON(utils.NewCitizenResponse(
		true,
		"Application successfully created",
		fiber.Map{
			"app_name": appName,
			"output":   output,
			"domain":   domainResp,
		},
	))
}

// DestroyApp deletes a Citizen app
func DestroyApp(c *fiber.Ctx) error {
	// Get app name
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Delete app
	output, err := platform.GetAdapter().DestroyApp(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while deleting the app: "+err.Error(),
			nil,
		))
	}

	// 💾 Remove ALL app data from database
	if dbErr := database.DeleteAllAppData(appName); dbErr != nil {
		fmt.Printf("[DB] ⚠️ Failed to remove all app data: %v\n", dbErr)
		// Don't fail the entire deletion because of DB issues
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Application successfully deleted",
		fiber.Map{
			"app_name": appName,
			"output":   output,
		},
	))
}

// RestartApp restarts an app from new
func RestartApp(c *fiber.Ctx) error {
	// Get app name
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// 📝 Log restart activity start
	var userID *int
	if userIDValue := c.Locals("user_id"); userIDValue != nil {
		if uid, ok := userIDValue.(int); ok {
			userID = &uid
		}
	}

	restartActivity, activityErr := database.LogRestartActivity(appName, userID)
	if activityErr != nil {
		fmt.Printf("[ACTIVITY] ⚠️ Failed to log restart activity: %v\n", activityErr)
	}

	// Restart app from new
	output, err := platform.GetAdapter().RestartApp(appName)
	if err != nil {
		// 📝 Update restart activity as failed
		if restartActivity != nil {
			errorMsg := err.Error()
			database.UpdateActivity(restartActivity.ID, database.StatusError, &errorMsg)
		}

		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while restarting the app: "+err.Error(),
			nil,
		))
	}

	// 📝 Update restart activity as successful
	if restartActivity != nil {
		database.UpdateActivity(restartActivity.ID, database.StatusSuccess, nil)
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Application successfully restarted",
		fiber.Map{
			"app_name": appName,
			"output":   output,
		},
	))
}

// GetAppInfo gets the information of an app
func GetAppInfo(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	ctx := context.Background()

	var deployment *models.AppDeployment
	dep, depErr := api.Deployments.GetDeploymentByAppName(ctx, appName)
	if depErr != nil {
		if !errors.Is(depErr, pgx.ErrNoRows) {
			return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
				false,
				fmt.Sprintf("Failed to load deployment metadata: %v", depErr),
				nil,
			))
		}
	} else {
		deployment = dep
	}

	runtimeInfo, runtimeErr := platform.GetAdapter().GetAppInfo(appName)
	if runtimeErr != nil && !errors.Is(runtimeErr, platform.ErrAppNotFound) {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			fmt.Sprintf("Failed to get runtime state: %v", runtimeErr),
			nil,
		))
	}
	if errors.Is(runtimeErr, platform.ErrAppNotFound) {
		runtimeInfo = nil
	}

	customDomains, _ := api.Settings.GetCustomDomains(ctx, appName)
	domainSet := make(map[string]struct{})
	var domains []string
	addDomain := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := domainSet[value]; exists {
			return
		}
		domainSet[value] = struct{}{}
		domains = append(domains, value)
	}
	if deployment != nil {
		addDomain(deployment.Domain)
	}
	for _, domain := range customDomains {
		addDomain(domain)
	}

	port := 5000
	if deployment != nil && deployment.Port > 0 {
		port = deployment.Port
	}

	response := fiber.Map{
		"app_name":       appName,
		"custom_domains": customDomains,
		"port":           port,
		"ports":          fiber.Map{"http": fmt.Sprintf("%d", port)},
		"running":        false,
		"deployed":       false,
		"status":         "pending",
	}

	if deployment != nil {
		if deployment.Status != "" {
			response["status"] = deployment.Status
		}
		if deployment.GitURL != "" {
			response["git_url"] = deployment.GitURL
		}
		if deployment.GitBranch != "" {
			response["git_branch"] = deployment.GitBranch
		}
		if deployment.GitCommit != "" {
			response["git_commit"] = deployment.GitCommit
		}
		if deployment.Builder != "" {
			response["builder"] = deployment.Builder
		}
		if deployment.Buildpack != "" {
			response["buildpack"] = deployment.Buildpack
		}
		if deployment.PortSource != "" {
			response["port_source"] = deployment.PortSource
		}
		if !deployment.LastDeploy.IsZero() {
			response["last_deploy"] = deployment.LastDeploy
		}
	}

	if runtimeInfo != nil {
		response["runtime"] = runtimeInfo
		if runtimeDomains := stringSliceFromInterface(runtimeInfo["domains"]); len(runtimeDomains) > 0 {
			for _, domain := range runtimeDomains {
				addDomain(domain)
			}
		}
		if portsVal, ok := runtimeInfo["ports"]; ok {
			response["ports"] = portsVal
		}
		if running, ok := runtimeInfo["running"].(bool); ok {
			response["running"] = running
		} else if hasPositiveValue(runtimeInfo["ready_replicas"]) || hasPositiveValue(runtimeInfo["available_replicas"]) {
			response["running"] = true
		}
		if deployed, ok := runtimeInfo["deployed"].(bool); ok {
			response["deployed"] = deployed
		} else if hasPositiveValue(runtimeInfo["updated_replicas"]) {
			response["deployed"] = true
		}
	} else if response["status"] == "deployed" {
		response["deployed"] = true
	}

	if publicSetting, err := api.Settings.GetAppPublicSetting(ctx, appName); err == nil && publicSetting != nil {
		addDomain(publicSetting.CustomDomain)
		response["is_public"] = publicSetting.IsPublic
	}

	response["domains"] = domains

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"App information retrieved successfully",
		response,
	))
}

// GetAllAppsInfo gets detailed information for all apps collectively
func GetAllAppsInfo(c *fiber.Ctx) error {
	allInfo, err := platform.GetAdapter().GetAllAppsInfo()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			fmt.Sprintf("Failed to get detailed information for all apps: %v", err),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Detailed information for all apps retrieved successfully",
		allInfo,
	))
}
