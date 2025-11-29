package handlers

import (
	"backend/database/api"
	"backend/platform"
	"backend/utils"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	ssebroadcast "backend/sse/services"

	"github.com/gofiber/fiber/v2"
)

// =============================================================================
// Broadcast Functions - SSE only
// =============================================================================

// BroadcastDeploymentLog sends a log update to SSE subscribers
func BroadcastDeploymentLog(runID, stepName, status, logs string) {
	ssebroadcast.PublishDeploymentLog(runID, stepName, status, logs)
}

// BroadcastStepUpdate sends a step status update to SSE subscribers
func BroadcastStepUpdate(runID, stepName, status string) {
	ssebroadcast.PublishStepUpdate(runID, stepName, status)
}

// BroadcastRunUpdate sends a run status update to SSE subscribers
func BroadcastRunUpdate(runID, status string) {
	ssebroadcast.PublishRunUpdate(runID, status)
}

// =============================================================================
// REST API Handlers
// =============================================================================

// GetDeploymentRuns returns deployment runs for an app
func GetDeploymentRuns(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	limit := c.QueryInt("limit", 20)

	ctx := c.Context()
	runs, err := api.DeploymentRuns.GetDeploymentRunsForApp(ctx, appName, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to get deployment runs: "+err.Error(),
			nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Deployment runs retrieved successfully",
		fiber.Map{
			"runs":  runs,
			"total": len(runs),
		},
	))
}

// GetDeploymentRun returns a specific deployment run with steps
func GetDeploymentRun(c *fiber.Ctx) error {
	runID := c.Params("run_id")
	if runID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Run ID is required",
			nil,
		))
	}

	ctx := c.Context()
	run, err := api.DeploymentRuns.GetDeploymentRun(ctx, runID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(utils.NewCitizenResponse(
			false,
			"Deployment run not found: "+err.Error(),
			nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Deployment run retrieved successfully",
		run,
	))
}

// GetLatestDeploymentRun returns the latest deployment run for an app
func GetLatestDeploymentRun(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	ctx := c.Context()
	run, err := api.DeploymentRuns.GetLatestDeploymentRun(ctx, appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to get latest deployment run: "+err.Error(),
			nil,
		))
	}

	if run == nil {
		return c.JSON(utils.NewCitizenResponse(
			true,
			"No deployment runs found",
			nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Latest deployment run retrieved successfully",
		run,
	))
}

// TriggerDeployment triggers a new deployment and returns immediately with run_id
func TriggerDeployment(c *fiber.Ctx) error {
	// Copy string to avoid Fiber's context pooling issues with goroutines
	appName := string([]byte(c.Params("app_name")))
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Check if adapter is K3s
	adapterType := strings.ToLower(strings.TrimSpace(os.Getenv("PLATFORM_ADAPTER")))
	if adapterType == "" {
		adapterType = "k3s" // Default
	}
	if adapterType != "k3s" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Deployment runs only supported for K3s adapter",
			nil,
		))
	}

	adapter := platform.GetAdapter()
	if adapter == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Platform adapter not configured",
			nil,
		))
	}

	var req struct {
		GitURL    string `json:"git_url"`
		GitBranch string `json:"git_branch"`
		Builder   string `json:"builder"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body: "+err.Error(),
			nil,
		))
	}

	if req.GitURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Git URL is required",
			nil,
		))
	}

	if req.GitBranch == "" {
		req.GitBranch = "main"
	}

	if req.Builder == "" {
		req.Builder = "auto"
	}

	// Get user ID from context
	var userID *int
	if uid := c.Locals("user_id"); uid != nil {
		if id, ok := uid.(int); ok {
			userID = &id
		}
	}

	// Create deployment run
	ctx := context.Background()
	run, err := api.DeploymentRuns.CreateDeploymentRun(ctx, appName, req.GitURL, req.GitBranch, req.Builder, "manual", userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to create deployment run: "+err.Error(),
			nil,
		))
	}

	// Start deployment in background
	go executeDeployment(run.RunID, appName, req.GitURL, req.GitBranch, req.Builder, userID)

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Deployment started",
		fiber.Map{
			"run_id":   run.RunID,
			"app_name": appName,
			"status":   "pending",
		},
	))
}

// executeDeployment runs the deployment process and updates status
func executeDeployment(runID, appName, gitURL, gitBranch, builder string, userID *int) {
	ctx := context.Background()
	var finalLogs string
	var deployErr error

	defer func() {
		// Complete the run
		status := "completed"
		var errMsg *string
		if deployErr != nil {
			status = "failed"
			msg := deployErr.Error()
			errMsg = &msg
		}
		api.DeploymentRuns.CompleteDeploymentRun(ctx, runID, status, finalLogs, errMsg)
		BroadcastRunUpdate(runID, status)
	}()

	// Step 1: Initializing
	updateStep(ctx, runID, "initializing", "running", nil)
	BroadcastStepUpdate(runID, "initializing", "running")
	api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "initializing")

	time.Sleep(500 * time.Millisecond) // Small delay for UX

	// Add GitHub authentication token to git URL for private repos
	authenticatedGitURL := gitURL
	if strings.Contains(gitURL, "github.com") {
		// Try GitHub App installation token first (preferred)
		if tokenResp, tokenErr := utils.GetGitHubInstallationToken(); tokenErr == nil && tokenResp != nil {
			log.Printf("[DEPLOY] Using GitHub App installation token for authentication")
			authenticatedGitURL = strings.Replace(gitURL, "https://github.com/",
				fmt.Sprintf("https://x-access-token:%s@github.com/", tokenResp.Token), 1)
		} else if userID != nil {
			// Fallback to user's GitHub access token
			accessToken, tokenErr := api.GitHub.GetUserGitHubAccessToken(ctx, *userID)
			if tokenErr == nil && accessToken != "" {
				log.Printf("[DEPLOY] Using user's GitHub access token for authentication")
				authenticatedGitURL = strings.Replace(gitURL, "https://github.com/",
					fmt.Sprintf("https://x-access-token:%s@github.com/", accessToken), 1)
			}
		}
	}

	initLog := fmt.Sprintf("Starting deployment for %s\nGit URL: %s\nBranch: %s\nBuilder: %s\n", appName, gitURL, gitBranch, builder)
	updateStep(ctx, runID, "initializing", "completed", &initLog)
	BroadcastDeploymentLog(runID, "initializing", "completed", initLog)

	// Step 2: Cloning (handled by K3s job, we just track it)
	updateStep(ctx, runID, "cloning", "running", nil)
	BroadcastStepUpdate(runID, "cloning", "running")
	api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "cloning")

	// Step 3-5: Building, Pushing, Deploying - All handled by K3s adapter
	updateStep(ctx, runID, "building", "running", nil)
	BroadcastStepUpdate(runID, "building", "running")
	api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "building")

	// Execute actual deployment via platform adapter (using authenticated URL)
	adapter := platform.GetAdapter()
	output, err := adapter.DeployFromGit(appName, authenticatedGitURL, gitBranch, userID)
	finalLogs = output

	if err != nil {
		deployErr = err
		// Mark remaining steps as failed
		errLog := fmt.Sprintf("Deployment failed: %v", err)
		updateStep(ctx, runID, "cloning", "completed", nil)
		updateStep(ctx, runID, "building", "failed", &errLog)
		BroadcastDeploymentLog(runID, "building", "failed", errLog)
		return
	}

	// Mark steps as completed
	cloneLog := "Repository cloned successfully\n"
	updateStep(ctx, runID, "cloning", "completed", &cloneLog)
	BroadcastDeploymentLog(runID, "cloning", "completed", cloneLog)

	buildLog := "Build completed successfully\n"
	updateStep(ctx, runID, "building", "completed", &buildLog)
	BroadcastDeploymentLog(runID, "building", "completed", buildLog)

	// Pushing
	updateStep(ctx, runID, "pushing", "running", nil)
	BroadcastStepUpdate(runID, "pushing", "running")
	pushLog := "Image pushed to registry\n"
	updateStep(ctx, runID, "pushing", "completed", &pushLog)
	BroadcastDeploymentLog(runID, "pushing", "completed", pushLog)

	// Deploying
	updateStep(ctx, runID, "deploying", "running", nil)
	BroadcastStepUpdate(runID, "deploying", "running")
	api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "deploying")
	deployLog := "Deployment rolled out successfully\n"
	updateStep(ctx, runID, "deploying", "completed", &deployLog)
	BroadcastDeploymentLog(runID, "deploying", "completed", deployLog)

	// Cleanup
	updateStep(ctx, runID, "cleanup", "running", nil)
	BroadcastStepUpdate(runID, "cleanup", "running")
	cleanupLog := "Cleanup completed\n"
	updateStep(ctx, runID, "cleanup", "completed", &cleanupLog)
	BroadcastDeploymentLog(runID, "cleanup", "completed", cleanupLog)

	// Append full build output
	if output != "" {
		api.DeploymentRuns.AppendBuildLogs(ctx, runID, "output", "\n--- Build Output ---\n"+output)
	}
}

func updateStep(ctx context.Context, runID, stepName, status string, logs *string) {
	if err := api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, stepName, status, logs); err != nil {
		log.Printf("[DEPLOY] Failed to update step %s: %v", stepName, err)
	}
}
