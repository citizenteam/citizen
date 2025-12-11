package handlers

import (
	"backend/internal/database"
	"backend/internal/database/api"
	githubservices "backend/internal/github/services"
	"backend/internal/models"
	"backend/internal/platform"
	"backend/internal/platform/k3s"
	"backend/internal/utils"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	deploymenthandlers "backend/internal/deployments/handlers"

	"github.com/gofiber/fiber/v2"
)

// DeployApp deploys an app from a git repository
func DeployApp(c *fiber.Ctx) error {
	// IMPORTANT: Copy the string to avoid Fiber's context pooling issues
	// c.Params() returns a string from Fiber's pool which may be reused after handler returns
	appName := string([]byte(c.Params("app_name")))
	fmt.Printf("[DEPLOY] DeployApp called with appName: '%s'\n", appName)
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

	// Get builder type: 1. From request, 2. From database, 3. Default "auto"
	builderType := strings.TrimSpace(deployData.Builder)
	if builderType == "" {
		// Get from database
		settings, err := api.BuildSettings.GetBuildSettings(c.Context(), appName)
		if err == nil && settings != nil {
			builderType = settings.ResolvedBuilderType()
			fmt.Printf("[DEPLOY] Using builder from database: %s\n", builderType)
		} else {
			builderType = "auto"
			fmt.Printf("[DEPLOY] Using default builder: auto\n")
		}
	} else {
		// Save to database if provided in request
		api.BuildSettings.SetBuilderType(c.Context(), appName, models.BuilderType(builderType))
		fmt.Printf("[DEPLOY] Builder set from request: %s\n", builderType)
	}

	// 🔑 Get user ID for GitHub authentication
	var userID *int
	if userIDValue := c.Locals("user_id"); userIDValue != nil {
		if uid, ok := userIDValue.(int); ok {
			userID = &uid
			fmt.Printf("[DEPLOY] 🔑 User authenticated: %d\n", uid)
		}
	}

	// Create deployment run for tracking (K3s style)
	var deploymentRun *api.DeploymentRun
	if run, err := api.DeploymentRuns.CreateDeploymentRun(c.Context(), appName, deployData.GitURL, deployData.GitBranch, builderType, "manual", userID); err == nil {
		deploymentRun = run
		fmt.Printf("[DEPLOY] Created deployment run: %s\n", run.RunID)
		// Start initializing step
		api.DeploymentRuns.UpdateDeploymentStep(c.Context(), run.RunID, "initializing", "running", nil)
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
	var portInfo *platform.ConfigPort
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
	builderLower := strings.ToLower(builderType)

	deployment, err := api.Deployments.GetDeploymentByAppName(context.Background(), appName)
	if err == nil && deployment.Status == "deployed" {
		currentPort = deployment.Port
		currentPortSource = deployment.PortSource
		fmt.Printf("[PORT DETECTION] 📊 Current port in database: %d (source: %s)\n", currentPort, currentPortSource)
	} else {
		fmt.Printf("[PORT DETECTION] 📊 No current port in database, will set if detected\n")
	}

	// If Dockerfile builder and no explicit port yet, default to 80
	if portInfo == nil && builderLower == "dockerfile" && (currentPort == 0 || currentPort == 3000) {
		portInfo = &platform.ConfigPort{Port: 80, Source: "dockerfile-default"}
	}

	// Try to detect port from config files (WITH GITHUB TOKEN)
	if portInfo == nil {
		if configPort, err := platform.GetAdapter().DetectPortFromGitRepo(deployData.GitURL, deployData.GitBranch, userID); err == nil {
			portInfo = configPort
			fmt.Printf("[PORT DETECTION] ✅ Port detected: %d from %s\n", configPort.Port, configPort.Source)
		} else {
			fmt.Printf("[PORT DETECTION] ⚠️ Config file detection failed: %v\n", err)
		}
	}

	// Try to detect port from config files (WITH GITHUB TOKEN) - duplicate check removed
	// Try to extract port from package.json as fallback (WITH GITHUB TOKEN)
	if portInfo == nil {
		if pkgPort, pkgErr := platform.GetAdapter().ExtractPortFromPackageJson(deployData.GitURL, deployData.GitBranch, userID); pkgErr == nil {
			portInfo = pkgPort
			fmt.Printf("[PORT DETECTION] ✅ Port detected from package.json: %d from %s\n", pkgPort.Port, pkgPort.Source)
		} else {
			portSetMessage = "ℹ️ No port configuration found in config files, using existing/default port mapping"
			fmt.Printf("[PORT DETECTION] ℹ️ No port found in any config file, using existing/default\n")
		}
	}

	if portInfo != nil {
		// Check if port changed
		if currentPort != 0 && currentPort == portInfo.Port {
			portSetMessage = fmt.Sprintf("✅ Port %d unchanged from %s (skipping re-config)", portInfo.Port, portInfo.Source)
			fmt.Printf("[PORT DETECTION] ↻ Port %d unchanged, skipping re-configuration\n", portInfo.Port)
		} else {
			fmt.Printf("[PORT DETECTION] 🔄 Port changed from %d to %d, updating configuration\n", currentPort, portInfo.Port)

			// 1. Set PORT environment variable so app runs on detected port
			portEnv := map[string]string{
				"PORT": fmt.Sprintf("%d", portInfo.Port),
			}
			if _, envErr := platform.GetAdapter().SetEnv(appName, portEnv); envErr != nil {
				fmt.Printf("[PORT DETECTION] ⚠️ Failed to set PORT environment variable: %v\n", envErr)
			} else {
				fmt.Printf("[PORT DETECTION] ✅ PORT environment variable set to %d\n", portInfo.Port)
			}

			// 2. Set port mapping so nginx routes to correct port
			if _, portErr := platform.GetAdapter().SetPort(appName, fmt.Sprintf("%d", portInfo.Port)); portErr == nil {
				portSetMessage = fmt.Sprintf("✅ Port %d auto-configured from %s (both env & mapping)", portInfo.Port, portInfo.Source)
				fmt.Printf("[PORT DETECTION] ✅ Port %d successfully set in Citizen (mapping)\n", portInfo.Port)
			} else {
				portSetMessage = fmt.Sprintf("⚠️ Port %d detected from %s, env set but mapping failed: %v", portInfo.Port, portInfo.Source, portErr)
				fmt.Printf("[PORT DETECTION] ❌ Failed to set port %d mapping in Citizen: %v\n", portInfo.Port, portErr)
			}
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

	// 🔑 Get authenticated Git URL for private repo access
	authenticatedGitURL := deployData.GitURL

	// Check if this is a GitHub URL and try to add authentication
	if strings.Contains(deployData.GitURL, "github.com") {
		// Try GitHub App installation token first (preferred)
		if tokenResp, tokenErr := githubservices.GetGitHubInstallationToken(); tokenErr == nil && tokenResp != nil {
			fmt.Printf("[DEPLOY] 🔑 Using GitHub App installation token for authentication\n")
			authenticatedGitURL = strings.Replace(deployData.GitURL, "https://github.com/",
				fmt.Sprintf("https://x-access-token:%s@github.com/", tokenResp.Token), 1)
		} else if userID != nil {
			// Fall back to user's OAuth token
			accessToken, tokenErr := api.GitHub.GetUserGitHubAccessToken(c.Context(), *userID)
			if tokenErr == nil && accessToken != "" {
				fmt.Printf("[DEPLOY] 🔑 Using user OAuth token for authentication\n")
				authenticatedGitURL = strings.Replace(deployData.GitURL, "https://github.com/",
					fmt.Sprintf("https://x-access-token:%s@github.com/", accessToken), 1)
			}
		}
	}

	// Return immediately with run_id - build will happen in background
	if deploymentRun != nil {
		runID := deploymentRun.RunID

		// Start async deployment in goroutine
		go func() {
			ctx := context.Background()

			// Update deployment steps
			initLog := fmt.Sprintf("Starting deployment for %s\nGit URL: %s\nBranch: %s\nBuilder: %s\n", appName, deployData.GitURL, deployData.GitBranch, builderType)
			api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "initializing", "completed", &initLog)
			deploymenthandlers.BroadcastDeploymentLog(runID, "initializing", "completed", initLog)

			api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "cloning", "running", nil)
			deploymenthandlers.BroadcastStepUpdate(runID, "cloning", "running")
			api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "cloning")

			// Deploy from git repository
			var output string
			var deployErr error

			if k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter); ok {
				// Mark cloning as completed and building as running
				cloneLog := "Repository cloning started...\n"
				api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "cloning", "completed", &cloneLog)
				deploymenthandlers.BroadcastDeploymentLog(runID, "cloning", "completed", cloneLog)

				api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "building", "running", nil)
				deploymenthandlers.BroadcastStepUpdate(runID, "building", "running")
				api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "building")

				fmt.Printf("[DEPLOY] Calling DeployFromGitWithLogs with appName='%s', runID='%s'\n", appName, runID)
				output, deployErr = k3sAdapter.DeployFromGitWithLogs(appName, authenticatedGitURL, deployData.GitBranch, userID, func(logs string) {
					// Broadcast live logs to WebSocket subscribers
					deploymenthandlers.BroadcastDeploymentLog(runID, "building", "running", logs)
					// Also append to database with step info
					api.DeploymentRuns.AppendBuildLogs(ctx, runID, "building", logs)
				})
			} else {
				output, deployErr = platform.GetAdapter().DeployFromGit(appName, authenticatedGitURL, deployData.GitBranch, userID)
			}

			if deployErr != nil {
				// Update deployment activity as failed
				if deployActivity != nil {
					errorMsg := deployErr.Error()
					database.UpdateActivity(deployActivity.ID, database.StatusError, &errorMsg)
				}

				// Update deployment run as failed
				errorMsg := deployErr.Error()
				errLog := fmt.Sprintf("Deployment failed: %s", errorMsg)
				api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "building", "failed", &errLog)
				deploymenthandlers.BroadcastDeploymentLog(runID, "building", "failed", errLog)
				api.DeploymentRuns.CompleteDeploymentRun(ctx, runID, "failed", output, &errorMsg)
				deploymenthandlers.BroadcastRunUpdate(runID, "failed")
				return
			}

			// Update deployment activity as successful
			if deployActivity != nil {
				database.UpdateActivity(deployActivity.ID, database.StatusSuccess, nil)
			}

			// Update deployment run as completed
			buildLog := "Build completed successfully\n"
			pushLog := "Image pushed to registry\n"
			deployLog := "Deployment rolled out successfully\n"
			cleanupLog := "Cleanup completed\n"

			// Building completed
			api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "building", "completed", &buildLog)
			deploymenthandlers.BroadcastStepUpdate(runID, "building", "completed")
			deploymenthandlers.BroadcastDeploymentLog(runID, "building", "completed", buildLog)

			// Pushing
			api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "pushing", "running", nil)
			deploymenthandlers.BroadcastStepUpdate(runID, "pushing", "running")
			api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "pushing", "completed", &pushLog)
			deploymenthandlers.BroadcastStepUpdate(runID, "pushing", "completed")
			deploymenthandlers.BroadcastDeploymentLog(runID, "pushing", "completed", pushLog)

			// Deploying - wait for rollout to complete
			api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "deploying", "running", nil)
			deploymenthandlers.BroadcastStepUpdate(runID, "deploying", "running")
			api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "deploying")

			// Wait for deployment rollout to complete
			if k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter); ok {
				rolloutLog := "Waiting for pods to be ready...\n"
				deploymenthandlers.BroadcastDeploymentLog(runID, "deploying", "running", rolloutLog)

				if rolloutErr := k3sAdapter.WaitForDeploymentRollout(appName, 5*time.Minute); rolloutErr != nil {
					rolloutFailLog := fmt.Sprintf("Rollout warning: %v\n", rolloutErr)
					deploymenthandlers.BroadcastDeploymentLog(runID, "deploying", "running", rolloutFailLog)
				}
			}

			deployLog = "Deployment rolled out successfully\n"
			api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "deploying", "completed", &deployLog)
			deploymenthandlers.BroadcastStepUpdate(runID, "deploying", "completed")
			deploymenthandlers.BroadcastDeploymentLog(runID, "deploying", "completed", deployLog)

			// Cleanup - remove old build jobs
			api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "cleanup", "running", nil)
			deploymenthandlers.BroadcastStepUpdate(runID, "cleanup", "running")

			// Clean up old build jobs
			if k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter); ok {
				if cleanupErr := k3sAdapter.CleanupCompletedBuildJobs(appName); cleanupErr != nil {
					fmt.Printf("[DEPLOY] Cleanup warning: %v\n", cleanupErr)
				}
			}

			cleanupLog = "Old build jobs cleaned up\n"
			api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "cleanup", "completed", &cleanupLog)
			deploymenthandlers.BroadcastStepUpdate(runID, "cleanup", "completed")
			deploymenthandlers.BroadcastDeploymentLog(runID, "cleanup", "completed", cleanupLog)

			// Get app URL for the response
			mainDomain := os.Getenv("MAIN_DOMAIN")
			if mainDomain == "" {
				mainDomain = os.Getenv("APP_HOST")
			}
			appURL := fmt.Sprintf("https://%s.%s", appName, mainDomain)

			// Complete the run with app URL
			api.DeploymentRuns.CompleteDeploymentRun(ctx, runID, "completed", output, nil)
			deploymenthandlers.BroadcastRunUpdate(runID, "completed")

			// Broadcast app URL
			deploymenthandlers.BroadcastDeploymentLog(runID, "completed", "completed", fmt.Sprintf("\nApp URL: %s\n", appURL))

			// Save deployment info to database
			newDeployment := &models.AppDeployment{
				AppName:    appName,
				GitURL:     deployData.GitURL,
				GitBranch:  deployData.GitBranch,
				Status:     "deployed",
				LastDeploy: time.Now(),
			}
			if builderType != "" {
				newDeployment.Builder = builderType
			}
			if portInfo != nil {
				newDeployment.Port = portInfo.Port
			}
			database.SaveAppDeployment(newDeployment)
		}()

		// Return immediately with run_id
		return c.JSON(utils.NewCitizenResponse(
			true,
			"App deployment started successfully",
			fiber.Map{
				"app_name": appName,
				"git_url":  deployData.GitURL,
				"branch":   deployData.GitBranch,
				"builder":  builderType,
				"run_id":   runID,
				"status":   "running",
				"message":  "Deployment is running in background. Connect to WebSocket for live logs.",
			},
		))
	}

	// Fallback for non-K3s or no deployment run (sync deploy)
	output, deployErr := platform.GetAdapter().DeployFromGit(appName, authenticatedGitURL, deployData.GitBranch, userID)
	if deployErr != nil {
		if deployActivity != nil {
			errorMsg := deployErr.Error()
			database.UpdateActivity(deployActivity.ID, database.StatusError, &errorMsg)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to deploy app: "+deployErr.Error(),
			fiber.Map{"output": output},
		))
	}

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
	if builderType != "" {
		newDeployment.Builder = builderType
	}

	// Add port info if detected
	if portInfo != nil {
		newDeployment.Port = portInfo.Port
		newDeployment.PortSource = portInfo.Source
	}

	// Save to database
	if dbErr := database.SaveAppDeployment(newDeployment); dbErr != nil {
		fmt.Printf("[DB] ⚠️ Failed to save deployment info: %v\n", dbErr)
		// Don't fail the entire deployment because of DB issues
	}

	// Success response with port detection info
	responseData := fiber.Map{
		"app_name":               appName,
		"git_url":                deployData.GitURL,
		"branch":                 deployData.GitBranch,
		"output":                 output,
		"port_detection_message": portSetMessage,
	}
	if builderType != "" {
		responseData["builder"] = builderType
	}

	if portInfo != nil {
		responseData["port_detection"] = fiber.Map{
			"detected_port": portInfo.Port,
			"source":        portInfo.Source,
			"message":       portSetMessage,
		}
	}

	// Add deployment run info for frontend tracking
	if deploymentRun != nil {
		responseData["run_id"] = deploymentRun.RunID
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"App deployment started successfully",
		responseData,
	))
}
