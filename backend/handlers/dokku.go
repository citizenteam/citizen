package handlers

import (
	"bufio"
	"context"
	"backend/utils"
	"backend/database"
	"backend/database/api"
	"backend/models"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ListApps lists all Citizen apps
func ListApps(c *fiber.Ctx) error {
	apps, err := utils.ListApps()
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
	domains, err := utils.ListDomains(appName)
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

	// Create app
	output, err := utils.CreateApp(strings.ToLower(data.AppName))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while creating the app: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusCreated).JSON(utils.NewCitizenResponse(
		true,
		"Application successfully created",
		fiber.Map{
			"app_name": strings.ToLower(data.AppName),
			"output":   output,
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
	output, err := utils.DestroyApp(appName)
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

// SetPort sets the port of an app
func SetPort(c *fiber.Ctx) error {
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
		Port string `json:"port"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request content",
			nil,
		))
	}

	// Check port number
	if data.Port == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Port number is required",
			nil,
		))
	}

	// Set port
	output, err := utils.SetPort(appName, data.Port)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while setting the port: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Port set successfully",
		fiber.Map{
			"app_name": appName,
			"port":     data.Port,
			"output":   output,
		},
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
	output, err := utils.AddDomain(appName, data.Domain)
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
	output, err := utils.RemoveDomain(appName, data.Domain)
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

// DeployApp deploys an app from a git repository
func DeployApp(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var deployData struct {
		GitURL    string `json:"git_url"`
		GitBranch string `json:"git_branch"`
		Builder   string `json:"builder"`
		Buildpack string `json:"buildpack"`
	}

	if err := c.BodyParser(&deployData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	if deployData.GitURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Git URL is required",
			nil,
		))
	}

	// 🔑 Get user ID for GitHub authentication
	var userID *int
	if userIDValue := c.Locals("user_id"); userIDValue != nil {
		if uid, ok := userIDValue.(int); ok {
			userID = &uid
			fmt.Printf("[DEPLOY] 🔑 User authenticated: %d\n", uid)
		}
	}

	// Branch priority: 1. Frontend request, 2. Database connected repo, 3. Default "main"
	if deployData.GitBranch == "" {
		// If no branch provided in request, check database for connected repository
		deployBranch, err := api.GitHub.GetGitHubRepositoryDeployBranch(context.Background(), appName)
		if err == nil && deployBranch != "" {
			// Use the deploy branch from connected repository
			deployData.GitBranch = deployBranch
			fmt.Printf("[DEPLOY] Using deploy branch from connected repository: %s\n", deployBranch)
		} else {
			// Final fallback to default
			deployData.GitBranch = "main"
			fmt.Printf("[DEPLOY] Using default branch: main\n")
		}
	} else {
		fmt.Printf("[DEPLOY] Using branch from request: %s\n", deployData.GitBranch)
	}

	// 🔧 AUTO-DETECT AND SET PORT BEFORE DEPLOY (WITH GITHUB TOKEN SUPPORT)
	var portInfo *utils.ConfigPort
	var portSetMessage string
	
	// Log port detection start
	fmt.Printf("[PORT DETECTION] ==================== STARTING PORT DETECTION ====================\n")
	fmt.Printf("[PORT DETECTION] Repository: %s\n", deployData.GitURL)
	fmt.Printf("[PORT DETECTION] Branch: %s\n", deployData.GitBranch)
	fmt.Printf("[PORT DETECTION] App Name: %s\n", appName)
	fmt.Printf("[PORT DETECTION] User ID: %v\n", userID)
	
	// Get current port from database
	var currentPort int
	var currentPortSource string
	
	deployment, err := api.Deployments.GetDeploymentByAppName(context.Background(), appName)
	if err == nil && deployment.Status == "deployed" {
		currentPort = deployment.Port
		currentPortSource = deployment.PortSource
		fmt.Printf("[PORT DETECTION] 📊 Current port in database: %d (source: %s)\n", currentPort, currentPortSource)
	} else {
		fmt.Printf("[PORT DETECTION] 📊 No current port in database, will set if detected\n")
	}
	
	// Try to detect port from config files (WITH GITHUB TOKEN)
	if configPort, err := utils.DetectPortFromGitRepo(deployData.GitURL, deployData.GitBranch, userID); err == nil {
		portInfo = configPort
		fmt.Printf("[PORT DETECTION] ✅ Port detected: %d from %s\n", configPort.Port, configPort.Source)
		
		// Check if port changed
		if currentPort != 0 && currentPort == configPort.Port {
			portSetMessage = fmt.Sprintf("✅ Port %d unchanged from %s (skipping re-config)", configPort.Port, configPort.Source)
			fmt.Printf("[PORT DETECTION] ↻ Port %d unchanged, skipping re-configuration\n", configPort.Port)
		} else {
			fmt.Printf("[PORT DETECTION] 🔄 Port changed from %d to %d, updating configuration\n", currentPort, configPort.Port)
			
			// 1. Set PORT environment variable so app runs on detected port
			portEnv := map[string]string{
				"PORT": fmt.Sprintf("%d", configPort.Port),
			}
			if _, envErr := utils.SetEnv(appName, portEnv); envErr != nil {
				fmt.Printf("[PORT DETECTION] ⚠️ Failed to set PORT environment variable: %v\n", envErr)
			} else {
				fmt.Printf("[PORT DETECTION] ✅ PORT environment variable set to %d\n", configPort.Port)
			}
			
			// 2. Set port mapping so nginx routes to correct port
			if _, portErr := utils.SetPort(appName, fmt.Sprintf("%d", configPort.Port)); portErr == nil {
				portSetMessage = fmt.Sprintf("✅ Port %d auto-configured from %s (both env & mapping)", configPort.Port, configPort.Source)
				fmt.Printf("[PORT DETECTION] ✅ Port %d successfully set in Citizen (mapping)\n", configPort.Port)
			} else {
				portSetMessage = fmt.Sprintf("⚠️ Port %d detected from %s, env set but mapping failed: %v", configPort.Port, configPort.Source, portErr)
				fmt.Printf("[PORT DETECTION] ❌ Failed to set port %d mapping in Citizen: %v\n", configPort.Port, portErr)
			}
		}
	} else {
		fmt.Printf("[PORT DETECTION] ⚠️ Config file detection failed: %v\n", err)
		
		// Try to extract port from package.json as fallback (WITH GITHUB TOKEN)
		if pkgPort, pkgErr := utils.ExtractPortFromPackageJson(deployData.GitURL, deployData.GitBranch, userID); pkgErr == nil {
			portInfo = pkgPort
			fmt.Printf("[PORT DETECTION] ✅ Port detected from package.json: %d from %s\n", pkgPort.Port, pkgPort.Source)
			
			// Check if port changed
			if currentPort != 0 && currentPort == pkgPort.Port {
				portSetMessage = fmt.Sprintf("✅ Port %d unchanged from %s (skipping re-config)", pkgPort.Port, pkgPort.Source)
				fmt.Printf("[PORT DETECTION] ↻ Port %d unchanged, skipping re-configuration\n", pkgPort.Port)
			} else {
				fmt.Printf("[PORT DETECTION] 🔄 Port changed from %d to %d, updating configuration\n", currentPort, pkgPort.Port)
				
				// 1. Set PORT environment variable so app runs on detected port
				portEnv := map[string]string{
					"PORT": fmt.Sprintf("%d", pkgPort.Port),
				}
				if _, envErr := utils.SetEnv(appName, portEnv); envErr != nil {
					fmt.Printf("[PORT DETECTION] ⚠️ Failed to set PORT environment variable: %v\n", envErr)
				} else {
					fmt.Printf("[PORT DETECTION] ✅ PORT environment variable set to %d\n", pkgPort.Port)
				}
				
				// 2. Set port mapping so nginx routes to correct port
				if _, portErr := utils.SetPort(appName, fmt.Sprintf("%d", pkgPort.Port)); portErr == nil {
					portSetMessage = fmt.Sprintf("✅ Port %d auto-configured from %s (both env & mapping)", pkgPort.Port, pkgPort.Source)
					fmt.Printf("[PORT DETECTION] ✅ Port %d successfully set in Citizen (mapping)\n", pkgPort.Port)
				} else {
					portSetMessage = fmt.Sprintf("⚠️ Port %d detected from %s, env set but mapping failed: %v", pkgPort.Port, pkgPort.Source, portErr)
					fmt.Printf("[PORT DETECTION] ❌ Failed to set port %d mapping in Citizen: %v\n", pkgPort.Port, portErr)
				}
			}
		} else {
			portSetMessage = "ℹ️ No port configuration found in config files, using existing/default port mapping"
			fmt.Printf("[PORT DETECTION] ℹ️ No port found in any config file, using existing/default\n")
		}
	}

	// 📝 Log deployment activity start
	var activityUserID *int
	if userIDValue := c.Locals("user_id"); userIDValue != nil {
		if uid, ok := userIDValue.(int); ok {
			activityUserID = &uid
		}
	}
	
	deployActivity, activityErr := database.LogDeployActivity(appName, deployData.GitURL, deployData.GitBranch, "", "", activityUserID, database.TriggerManual)
	if activityErr != nil {
		fmt.Printf("[ACTIVITY] ⚠️ Failed to log deploy activity: %v\n", activityErr)
	}

	// 🚀 Deploy from git repository with specific branch (WITH GITHUB TOKEN)
	output, err := utils.DeployFromGit(appName, deployData.GitURL, deployData.GitBranch, userID)
	if err != nil {
		// 📝 Update deployment activity as failed
		if deployActivity != nil {
			errorMsg := err.Error()
			database.UpdateActivity(deployActivity.ID, database.StatusError, &errorMsg)
		}
		
		// Deploy failed - include both error and any available output
		errorMessage := "Failed to deploy app: " + err.Error()
		
		// Try to get build logs for failed deploys
		buildLogs, _ := utils.GetBuildLogs(appName)
		
		responseData := fiber.Map{
			"output": output,
			"error_details": err.Error(),
		}
		
		// Add build logs if available
		if buildLogs != "" {
			responseData["build_logs"] = buildLogs
		}
		
		// Add port detection info even on failure
		if portInfo != nil {
			responseData["port_detection"] = fiber.Map{
				"detected_port": portInfo.Port,
				"source":        portInfo.Source,
				"message":       portSetMessage,
			}
		}
		
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			errorMessage,
			responseData,
		))
	}

	// 📝 Update deployment activity as successful
	if deployActivity != nil {
		database.UpdateActivity(deployActivity.ID, database.StatusSuccess, nil)
	}

	// 💾 Save deployment info to database
	newDeployment := &models.AppDeployment{
		AppName:    appName,
		GitURL:     deployData.GitURL,
		GitBranch:  deployData.GitBranch,
		Status:     "deployed",
		LastDeploy: time.Now(),
	}
	
	// Add port info if detected
	if portInfo != nil {
		newDeployment.Port = portInfo.Port
		newDeployment.PortSource = portInfo.Source
	}
	
	// Save the full deploy output for build logs
	if output != "" {
		// Store the full deploy output in deployment_logs field (TEXT field)
		newDeployment.DeploymentLogs = output
	}
	
	// Save to database
	if dbErr := database.SaveAppDeployment(newDeployment); dbErr != nil {
		fmt.Printf("[DB] ⚠️ Failed to save deployment info: %v\n", dbErr)
		// Don't fail the entire deployment because of DB issues
	}

	// Note: Traefik reload will be triggered automatically by dokku-traefik-watcher
	// after the container is restarted and fully ready

	// Success response with port detection info
	responseData := fiber.Map{
		"app_name": appName,
		"git_url":  deployData.GitURL,
		"branch":   deployData.GitBranch,
		"output":   output,
		"port_detection_message": portSetMessage,
	}
	
	if portInfo != nil {
		responseData["port_detection"] = fiber.Map{
			"detected_port": portInfo.Port,
			"source":        portInfo.Source,
			"message":       portSetMessage,
		}
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"App deployment started successfully",
		responseData,
	))
}

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
	output, err := utils.SetEnv(appName, data.EnvVars)
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

// GetAppInfo gets the information of an app
func GetAppInfo(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	info, err := utils.GetAppInfo(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			fmt.Sprintf("Failed to get app information: %v", err),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"App information retrieved successfully",
		info,
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
	output, err := utils.RestartApp(appName)
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

// BUILDPACK MANAGEMENT HANDLERS

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

	buildpacks, err := utils.ListBuildpacks(appName)
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

	output, err := utils.AddBuildpack(appName, data.BuildpackURL)
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

	output, err := utils.SetBuildpack(appName, data.BuildpackURL, data.Index)
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

	output, err := utils.RemoveBuildpack(appName, data.BuildpackURL)
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

	output, err := utils.ClearBuildpacks(appName)
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

	report, err := utils.GetBuildpackReport(appName)
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

	output, err := utils.SetBuilder(appName, data.BuilderType)
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

	report, err := utils.GetBuilderReport(appName)
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

// LOG YÖNETİMİ HANDLER'LARI

// GetAppLogs gets the logs of an app
func GetAppLogs(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Get query parameters
	tail := c.QueryInt("tail", 100) // Default 100 lines
	logType := c.Query("type", "app") // app, build, deploy
	processType := c.Query("process", "web") // web, worker, all

	var logs string
	var err error

	switch logType {
	case "build":
		logs, err = utils.GetBuildLogs(appName)
	case "deploy":
		logs, err = utils.GetDeployLogs(appName)
	case "all":
		// Logs for all processes
		logs, err = utils.GetAllProcessLogs(appName, tail)
	default:
		// Logs for a specific process or web process
		if processType == "all" {
			logs, err = utils.GetAllProcessLogs(appName, tail)
		} else {
			logs, err = utils.GetProcessSpecificLogs(appName, processType, tail)
		}
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to fetch logs: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Logs fetched successfully",
		fiber.Map{
			"logs": logs,
			"type": logType,
			"process": processType,
			"tail": tail,
			"timestamp": time.Now().Unix(),
		},
	))
}

// StreamAppLogs streams the logs of an app
func StreamAppLogs(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Access-Control-Allow-Headers", "Cache-Control")

	// Configure SSE using StreamWriter
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Get initial logs and send
		logs, err := utils.GetAppLogs(appName, 50, false)
		if err != nil {
			fmt.Fprintf(w, "data: {\"error\": \"%s\"}\n\n", err.Error())
			w.Flush()
			return
		}

		// Send logs in SSE format
		logData := map[string]interface{}{
			"logs": logs,
			"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
			"type": "initial",
		}
		
		jsonData, _ := json.Marshal(logData)
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		w.Flush()

		// Send periodic pings for keep-alive
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Send ping
				fmt.Fprintf(w, "data: {\"type\": \"ping\"}\n\n")
				w.Flush()
			case <-c.Context().Done():
				return
			}
		}
	})

	return nil
}

// GetLogInfo gets log information
func GetLogInfo(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	logInfo, err := utils.GetLogInfo(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"An error occurred while getting log information: "+err.Error(),
			nil,
		))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Log info retrieved successfully",
		logInfo,
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
	output, err := utils.RemoveEnv(appName, data.Key)
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
	envVars, err := utils.GetEnv(appName)
	if err != nil {
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

// GetLiveBuildLogs gets only build/deploy output (simplified)
func GetLiveBuildLogs(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Get build logs (deploy output only)
	buildLogs, err := utils.GetBuildLogs(appName)
	if err != nil {
		fmt.Printf("[LOGS] Failed to get build logs: %v\n", err)
		buildLogs = "No build logs available yet..."
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Build logs retrieved successfully",
		fiber.Map{
			"logs":           buildLogs,
			"has_build_logs": buildLogs != "",
			"timestamp":      time.Now().Unix(),
		},
	))
} 

// GetAllAppsInfo gets detailed information for all apps collectively
func GetAllAppsInfo(c *fiber.Ctx) error {
	allInfo, err := utils.GetAllAppsInfo()
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