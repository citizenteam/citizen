package handlers

import (
	"backend/database/api"
	"backend/models"
	"backend/utils"
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)


// Database helper functions for app settings

// setCustomDomainToDB saves custom domain to database
func setCustomDomainToDB(appName, domain string) (*models.AppCustomDomain, error) {
	err := api.Settings.CreateCustomDomain(context.Background(), appName, domain)
	if err != nil {
		return nil, err
	}
	
	// Return the created domain
	return &models.AppCustomDomain{
		AppName:   appName,
		Domain:    domain,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// getCustomDomainsByAppFromDB retrieves custom domains by app name
func getCustomDomainsByAppFromDB(appName string) ([]models.AppCustomDomain, error) {
	domains, err := api.Settings.GetCustomDomains(context.Background(), appName)
	if err != nil {
		return nil, err
	}
	
	var result []models.AppCustomDomain
	for _, domain := range domains {
		result = append(result, models.AppCustomDomain{
			AppName:   appName,
			Domain:    domain,
			IsActive:  true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
	}
	return result, nil
}

// removeCustomDomainFromDB removes (deactivates) custom domain from database
func removeCustomDomainFromDB(appName, domain string) error {
	return api.Settings.DeactivateCustomDomain(context.Background(), appName, domain)
}

// getActiveCustomDomainsFromDB gets all active custom domains
func getActiveCustomDomainsFromDB() ([]models.AppCustomDomain, error) {
	return api.Settings.GetAllActiveCustomDomains(context.Background())
}

// setPublicAppToDB saves public app setting to database
func setPublicAppToDB(appName string, isPublic bool) (*models.AppPublicSetting, error) {
	err := api.Settings.UpsertAppPublicSetting(context.Background(), appName, isPublic)
	if err != nil {
		return nil, err
	}
	
	// Return the created/updated setting
	return &models.AppPublicSetting{
		AppName:   appName,
		IsPublic:  isPublic,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// getPublicAppSettingFromDB retrieves public app setting
func getPublicAppSettingFromDB(appName string) (*models.AppPublicSetting, error) {
	return api.Settings.GetAppPublicSetting(context.Background(), appName)
}

// isAppPublic checks if an app is public
func isAppPublic(appName string) bool {
	isPublic, err := api.Settings.IsAppPublic(context.Background(), appName)
	if err != nil {
		return false // Default to private
	}
	return isPublic
}

// SetCustomDomain sets a custom domain for an application
func SetCustomDomain(c *fiber.Ctx) error {
	// Get application name from URL parameter
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Application name is required",
			nil,
		))
	}

	// Parse request content (only domain expected)
	var body struct {
		Domain string `json:"domain"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	if body.Domain == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Domain name is required",
			nil,
		))
	}

	// First check if the domain already exists in the database
	existingDbDomains, err := api.Settings.GetCustomDomains(context.Background(), appName)
	if err == nil {
		for _, existingDomain := range existingDbDomains {
			if existingDomain == body.Domain {
				return c.Status(fiber.StatusConflict).JSON(utils.NewCitizenResponse(
					false,
					"Domain already registered in database",
					nil,
				))
			}
		}
	}

	// Check if the domain already exists in Citizen
	existingCitizenDomains, err := utils.ListDomains(appName)
	if err == nil {
		for _, existingDomain := range existingCitizenDomains {
			if existingDomain == body.Domain {
				return c.Status(fiber.StatusConflict).JSON(utils.NewCitizenResponse(
					false,
					"Domain already registered in Citizen",
					nil,
				))
			}
		}
	}

	// STEP 1: Save custom domain to database
	domain, err := setCustomDomainToDB(appName, body.Domain)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Error occurred while saving domain to database: "+err.Error(),
			nil,
		))
	}

	// STEP 1.1: Also update the domain field in app_deployments table (for traefik watcher)
	updateErr := api.Deployments.UpdateDeploymentDomain(context.Background(), appName, body.Domain)
	if updateErr != nil {
		fmt.Printf("[WARN] app_deployments domain update failed for %s - %s: %v\n", appName, body.Domain, updateErr)
		// This error is not critical, just log and continue
	}

	// STEP 2: Add domain to Citizen
	output, err := utils.AddDomain(appName, body.Domain)
	if err != nil {
		// If error in Citizen, rollback the database record
		if removeErr := api.Settings.DeleteCustomDomain(context.Background(), appName, body.Domain); removeErr != nil {
			// If rollback also fails, log as critical
			fmt.Printf("[CRITICAL] Domain rollback failed for %s - %s: %v\n", appName, body.Domain, removeErr)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Error occurred while adding domain to Citizen: "+err.Error(),
			nil,
		))
	}

	// STEP 3: Send Traefik signal (optional, continues even if error)
	if reloadErr := utils.ReloadTraefik(); reloadErr != nil {
		fmt.Printf("[WARN] Traefik reload failed for domain %s: %v\n", body.Domain, reloadErr)
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Custom domain successfully configured",
		fiber.Map{
			"domain":        domain,
			"citizen_output": output,
		},
	))
}

// GetCustomDomains lists custom domains of an application
func GetCustomDomains(c *fiber.Ctx) error {
	// Get application name
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Application name is required",
			nil,
		))
	}

	// Get custom domains
	domains, err := getCustomDomainsByAppFromDB(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Error occurred while listing custom domains: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Custom domains successfully listed",
		domains,
	))
}

// RemoveCustomDomain removes custom domain from an application
func RemoveCustomDomain(c *fiber.Ctx) error {
	// Get parameters
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Application name is required",
			nil,
		))
	}

	// Parse request content
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

	if data.Domain == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Domain name is required",
			nil,
		))
	}

	// First check if the domain really exists in the database
	existingDbDomains, err := api.Settings.GetCustomDomains(context.Background(), appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Error occurred while retrieving domains from database: "+err.Error(),
			nil,
		))
	}
	
	domainExistsInDb := false
	for _, existingDomain := range existingDbDomains {
		if existingDomain == data.Domain {
			domainExistsInDb = true
			break
		}
	}
	
	if !domainExistsInDb {
		return c.Status(fiber.StatusNotFound).JSON(utils.NewCitizenResponse(
			false,
			"Domain not found in database",
			nil,
		))
	}

	// STEP 1: Remove domain from Citizen
	output, err := utils.RemoveDomain(appName, data.Domain)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Error occurred while removing domain from Citizen: "+err.Error(),
			nil,
		))
	}

	// STEP 2: Remove domain from database
	err = api.Settings.DeleteCustomDomain(context.Background(), appName, data.Domain)
	if err != nil {
		// If deletion from database fails, add back to Citizen (rollback)
		if _, addBackErr := utils.AddDomain(appName, data.Domain); addBackErr != nil {
			// If rollback also fails, log as critical
			fmt.Printf("[CRITICAL] Domain rollback failed for %s - %s: Citizen remove succeeded but DB delete failed, and Citizen add-back failed: %v\n", appName, data.Domain, addBackErr)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Error occurred while removing domain from database: "+err.Error(),
			nil,
		))
	}

	// STEP 2.1: Also clear the domain field in app_deployments table (for traefik watcher)
	updateErr := api.Deployments.UpdateDeploymentDomain(context.Background(), appName, "")
	if updateErr != nil {
		fmt.Printf("[WARN] app_deployments domain clear failed for %s: %v\n", appName, updateErr)
		// This error is not critical, just log and continue
	}

	// STEP 3: Send Traefik signal (optional, continues even if error)
	if reloadErr := utils.ReloadTraefik(); reloadErr != nil {
		fmt.Printf("[WARN] Traefik reload failed for domain removal %s: %v\n", data.Domain, reloadErr)
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Custom domain successfully removed",
		fiber.Map{
			"app_name":      appName,
			"domain":        data.Domain,
			"citizen_output": output,
		},
	))
}

// GetAllActiveCustomDomains lists all active custom domains (for admin)
func GetAllActiveCustomDomains(c *fiber.Ctx) error {
	domains, err := getActiveCustomDomainsFromDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Error occurred while listing active custom domains: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Active custom domains successfully listed",
		domains,
	))
}

// SetPublicApp sets the public setting of an application
func SetPublicApp(c *fiber.Ctx) error {
	// Get application name from URL parameter
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Application name is required",
			nil,
		))
	}

	// Parse request content
	var body struct {
		IsPublic bool `json:"is_public"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	// Save public app setting to database
	setting, err := setPublicAppToDB(appName, body.IsPublic)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Error occurred while setting public app: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Public app setting successfully updated",
		setting,
	))
}

// GetPublicAppSetting retrieves the public setting of an application
func GetPublicAppSetting(c *fiber.Ctx) error {
	// Get application name
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Application name is required",
			nil,
		))
	}

	// Get public app setting
	setting, err := getPublicAppSettingFromDB(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Error occurred while retrieving public app setting: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Public app setting successfully retrieved",
		setting,
	))
}
