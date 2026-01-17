package handlers

import (
	"backend/internal/database/api"
	deploymenthandlers "backend/internal/deployments/handlers"
	"backend/internal/platform"
	"backend/internal/platform/k3s"
	"backend/internal/services"
	"backend/internal/utils"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	MaxUploadSize    = 500 * 1024 * 1024 // 500 MB
	UploadBaseDir    = "/opt/citizen/data/user-uploads"
	AllowedExtension = ".tar.gz"
)

// UploadTarball handles tar.gz file uploads for local deployment
func UploadTarball(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	// Get user email from context
	email, ok := c.Locals("email").(string)
	if !ok || email == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User email not found in context",
			nil,
		))
	}

	// Create user-specific directory (email slug)
	userSlug := slugifyEmail(email)
	userDir := filepath.Join(UploadBaseDir, userSlug)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to create upload directory",
			nil,
		))
	}

	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"No file uploaded",
			nil,
		))
	}

	// Validate file size
	if file.Size > MaxUploadSize {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			fmt.Sprintf("File too large. Maximum size: %d MB", MaxUploadSize/(1024*1024)),
			nil,
		))
	}

	// Validate file extension
	if !strings.HasSuffix(file.Filename, AllowedExtension) && !strings.HasSuffix(file.Filename, ".tgz") {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Only .tar.gz or .tgz files are allowed",
			nil,
		))
	}

	// Generate timestamped filename
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.tar.gz", appName, timestamp)
	filePath := filepath.Join(userDir, filename)

	// Save file
	if err := c.SaveFile(file, filePath); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to save file",
			nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"File uploaded successfully",
		fiber.Map{
			"filename":    filename,
			"size":        file.Size,
			"path":        filepath.Join(userSlug, filename),
			"uploaded_at": time.Now(),
		},
	))
}

// ListUserUploads lists all uploads for the current user
func ListUserUploads(c *fiber.Ctx) error {
	email, ok := c.Locals("email").(string)
	if !ok || email == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User email not found in context",
			nil,
		))
	}

	userSlug := slugifyEmail(email)
	userDir := filepath.Join(UploadBaseDir, userSlug)

	// Check if directory exists
	if _, err := os.Stat(userDir); os.IsNotExist(err) {
		return c.JSON(utils.NewCitizenResponse(
			true,
			"No uploads found",
			fiber.Map{
				"uploads": []interface{}{},
			},
		))
	}

	// Read directory
	entries, err := os.ReadDir(userDir)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to read uploads directory",
			nil,
		))
	}

	// Build upload list
	uploads := []fiber.Map{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		uploads = append(uploads, fiber.Map{
			"filename":   entry.Name(),
			"size":       info.Size(),
			"path":       filepath.Join(userSlug, entry.Name()),
			"created_at": info.ModTime(),
		})
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Uploads retrieved successfully",
		fiber.Map{
			"uploads": uploads,
		},
	))
}

// DeleteUserUpload deletes a specific upload
func DeleteUserUpload(c *fiber.Ctx) error {
	filename := c.Params("filename")
	if filename == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Filename is required",
			nil,
		))
	}

	email, ok := c.Locals("email").(string)
	if !ok || email == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User email not found in context",
			nil,
		))
	}

	userSlug := slugifyEmail(email)
	filePath := filepath.Join(UploadBaseDir, userSlug, filename)

	// Security: Ensure path is within user directory
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid file path",
			nil,
		))
	}

	userDirAbs, _ := filepath.Abs(filepath.Join(UploadBaseDir, userSlug))
	if !strings.HasPrefix(absPath, userDirAbs) {
		return c.Status(fiber.StatusForbidden).JSON(utils.NewCitizenResponse(
			false,
			"Access denied",
			nil,
		))
	}

	// Delete file
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return c.Status(fiber.StatusNotFound).JSON(utils.NewCitizenResponse(
				false,
				"File not found",
				nil,
			))
		}
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to delete file",
			nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(
		true,
		"File deleted successfully",
		nil,
	))
}

// DeployFromLocal deploys from an uploaded tar.gz file
func DeployFromLocal(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"App name is required",
			nil,
		))
	}

	var req struct {
		Filename string `json:"filename"`
		Builder  string `json:"builder"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid request body",
			nil,
		))
	}

	if req.Filename == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Filename is required",
			nil,
		))
	}

	// Get user email and ID
	email, ok := c.Locals("email").(string)
	if !ok || email == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"User email not found in context",
			nil,
		))
	}

	// Get user ID from context (can be nil for device tokens without user_id mapping)
	var userID *int
	if rawUserID := c.Locals("citizenauth_user_id"); rawUserID != nil {
		if userIDStr, ok := rawUserID.(string); ok {
			// Parse string to int
			var uid int
			fmt.Sscanf(userIDStr, "%d", &uid)
			userID = &uid
		}
	}

	// Construct file path
	userSlug := slugifyEmail(email)
	tarballPath := filepath.Join(UploadBaseDir, userSlug, req.Filename)

	// Security: Verify file exists and is within user directory
	absPath, err := filepath.Abs(tarballPath)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid file path",
			nil,
		))
	}

	userDirAbs, _ := filepath.Abs(filepath.Join(UploadBaseDir, userSlug))
	if !strings.HasPrefix(absPath, userDirAbs) {
		return c.Status(fiber.StatusForbidden).JSON(utils.NewCitizenResponse(
			false,
			"Access denied",
			nil,
		))
	}

	if _, err := os.Stat(tarballPath); os.IsNotExist(err) {
		return c.Status(fiber.StatusNotFound).JSON(utils.NewCitizenResponse(
			false,
			"File not found",
			nil,
		))
	}

	// Get builder type
	builderType := strings.TrimSpace(req.Builder)
	if builderType == "" {
		builderType = "auto"
	}

	// Create deployment run for tracking
	deploymentRun, err := api.DeploymentRuns.CreateDeploymentRun(
		c.Context(),
		appName,
		"local://"+req.Filename,
		"",
		builderType,
		"manual",
		userID,
	)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to create deployment run: "+err.Error(),
			nil,
		))
	}

	runID := deploymentRun.RunID

	// Check if deployment queue is enabled
	if IsDeploymentQueueEnabled() {
		// Use queue-based deployment
		queue := services.GetDeploymentQueue()

		job := &services.DeploymentJob{
			AppName:     appName,
			RunID:       runID,
			GitURL:      "local://" + tarballPath, // Store full path in GitURL field
			GitBranch:   "",                       // No branch for local deployment
			Builder:     builderType,
			TriggerType: "manual",
			TriggeredBy: userID,
			Priority:    services.PriorityNormal,
		}

		if err := queue.Enqueue(context.Background(), job); err != nil {
			utils.ErrorLog("[DEPLOY-LOCAL] Failed to enqueue deployment: %v", err)
			// Fall through to goroutine-based deployment
		} else {
			// Get queue position
			queuedJobs, _ := queue.GetQueuedJobs(context.Background())
			position := 0
			for i, j := range queuedJobs {
				if j.ID == job.ID {
					position = i + 1
					break
				}
			}

			// Update initializing step
			initLog := fmt.Sprintf("Local deployment queued for %s\nPosition in queue: %d\nSource file: %s\nBuilder: %s\n",
				appName, position, req.Filename, builderType)
			api.DeploymentRuns.UpdateDeploymentStep(context.Background(), runID, "initializing", "completed", &initLog)
			deploymenthandlers.BroadcastDeploymentLog(runID, "initializing", "completed", initLog)

			// Set status to queued
			api.DeploymentRuns.UpdateDeploymentRunStatus(context.Background(), runID, "queued")
			deploymenthandlers.BroadcastStepUpdate(runID, "initializing", "completed")

			return c.JSON(utils.NewCitizenResponse(
				true,
				"Local deployment queued successfully",
				fiber.Map{
					"app_name":       appName,
					"source":         "local",
					"filename":       req.Filename,
					"builder":        builderType,
					"run_id":         runID,
					"job_id":         job.ID,
					"status":         "queued",
					"queue_position": position,
					"message":        fmt.Sprintf("Deployment queued (position: %d). Connect to WebSocket for live updates.", position),
				},
			))
		}
	}

	// Fallback: Start async deployment in goroutine (legacy mode or queue failed)
	go deployFromLocalAsync(appName, tarballPath, req.Filename, builderType, runID)

	return c.JSON(utils.NewCitizenResponse(
		true,
		"Local deployment started successfully",
		fiber.Map{
			"app_name": appName,
			"source":   "local",
			"filename": req.Filename,
			"builder":  builderType,
			"run_id":   runID,
			"status":   "running",
			"message":  "Deployment started. Connect to WebSocket for live updates.",
		},
	))
}

// slugifyEmail converts email to filesystem-safe slug
// john.doe@example.com → john-doe-example-com
func slugifyEmail(email string) string {
	slug := strings.ToLower(email)
	slug = strings.ReplaceAll(slug, "@", "-")
	slug = strings.ReplaceAll(slug, ".", "-")
	slug = strings.ReplaceAll(slug, "+", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	return slug
}

// deployFromLocalAsync handles async deployment from local tar.gz file
func deployFromLocalAsync(appName, tarballPath, filename, builderType, runID string) {
	ctx := context.Background()

	// Update deployment steps
	initLog := fmt.Sprintf("Starting local deployment for %s\nSource file: %s\nBuilder: %s\n", appName, filename, builderType)
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "initializing", "completed", &initLog)
	deploymenthandlers.BroadcastDeploymentLog(runID, "initializing", "completed", initLog)

	// Step 1: Extract tar.gz
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "extracting", "running", nil)
	deploymenthandlers.BroadcastStepUpdate(runID, "extracting", "running")
	api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "extracting")

	tempDir := filepath.Join("/tmp", "citizen-deploy-"+runID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		errLog := fmt.Sprintf("Failed to create temp directory: %v", err)
		api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "extracting", "failed", &errLog)
		deploymenthandlers.BroadcastDeploymentLog(runID, "extracting", "failed", errLog)
		api.DeploymentRuns.CompleteDeploymentRun(ctx, runID, "failed", "", &errLog)
		deploymenthandlers.BroadcastRunUpdate(runID, "failed")
		return
	}

	// Extract tarball
	extractLog := fmt.Sprintf("Extracting %s to temporary directory...\n", filename)
	deploymenthandlers.BroadcastDeploymentLog(runID, "extracting", "running", extractLog)

	cmd := exec.Command("tar", "-xzf", tarballPath, "-C", tempDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		errLog := fmt.Sprintf("Failed to extract tarball: %v\nOutput: %s", err, string(output))
		api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "extracting", "failed", &errLog)
		deploymenthandlers.BroadcastDeploymentLog(runID, "extracting", "failed", errLog)
		api.DeploymentRuns.CompleteDeploymentRun(ctx, runID, "failed", "", &errLog)
		deploymenthandlers.BroadcastRunUpdate(runID, "failed")
		os.RemoveAll(tempDir)
		return
	}

	extractCompleteLog := "Tarball extracted successfully\n"
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "extracting", "completed", &extractCompleteLog)
	deploymenthandlers.BroadcastDeploymentLog(runID, "extracting", "completed", extractCompleteLog)
	deploymenthandlers.BroadcastStepUpdate(runID, "extracting", "completed")

	// Step 2: Build from local directory
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "building", "running", nil)
	deploymenthandlers.BroadcastStepUpdate(runID, "building", "running")
	api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "building")

	var output string
	var deployErr error

	k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter)
	if !ok {
		errLog := "K3s adapter not available"
		api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "building", "failed", &errLog)
		deploymenthandlers.BroadcastDeploymentLog(runID, "building", "failed", errLog)
		api.DeploymentRuns.CompleteDeploymentRun(ctx, runID, "failed", "", &errLog)
		deploymenthandlers.BroadcastRunUpdate(runID, "failed")
		os.RemoveAll(tempDir)
		return
	}

	// Deploy from local path with live logs
	output, deployErr = k3sAdapter.DeployFromLocalPathWithLogs(appName, tempDir, builderType, runID, func(logs string) {
		// Broadcast live logs to WebSocket subscribers
		deploymenthandlers.BroadcastDeploymentLog(runID, "building", "running", logs)
		// Also append to database with step info
		api.DeploymentRuns.AppendBuildLogs(ctx, runID, "building", logs)
	})

	// Clean up temp directory
	os.RemoveAll(tempDir)

	if deployErr != nil {
		errorMsg := deployErr.Error()
		errLog := fmt.Sprintf("Build failed: %s", errorMsg)
		api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "building", "failed", &errLog)
		deploymenthandlers.BroadcastDeploymentLog(runID, "building", "failed", errLog)

		// Cleanup failed build jobs
		fmt.Printf("[DEPLOY-LOCAL] 🧹 Cleaning up failed build jobs for %s\n", appName)
		if cleanupErr := k3sAdapter.CleanupCompletedBuildJobs(appName); cleanupErr != nil {
			fmt.Printf("[DEPLOY-LOCAL] Cleanup warning: %v\n", cleanupErr)
		}

		api.DeploymentRuns.CompleteDeploymentRun(ctx, runID, "failed", output, &errorMsg)
		deploymenthandlers.BroadcastRunUpdate(runID, "failed")
		return
	}

	// Build completed successfully
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

	// Deploying
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "deploying", "running", nil)
	deploymenthandlers.BroadcastStepUpdate(runID, "deploying", "running")
	api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "deploying")
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "deploying", "completed", &deployLog)
	deploymenthandlers.BroadcastStepUpdate(runID, "deploying", "completed")
	deploymenthandlers.BroadcastDeploymentLog(runID, "deploying", "completed", deployLog)

	// Cleanup
	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "cleanup", "running", nil)
	deploymenthandlers.BroadcastStepUpdate(runID, "cleanup", "running")

	// Cleanup build jobs
	if cleanupErr := k3sAdapter.CleanupCompletedBuildJobs(appName); cleanupErr != nil {
		cleanupLog = fmt.Sprintf("Cleanup completed with warnings: %v\n", cleanupErr)
	}

	api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, "cleanup", "completed", &cleanupLog)
	deploymenthandlers.BroadcastStepUpdate(runID, "cleanup", "completed")
	deploymenthandlers.BroadcastDeploymentLog(runID, "cleanup", "completed", cleanupLog)

	// Mark deployment as completed
	api.DeploymentRuns.CompleteDeploymentRun(ctx, runID, "completed", output, nil)
	deploymenthandlers.BroadcastRunUpdate(runID, "completed")

	fmt.Printf("[DEPLOY-LOCAL] ✅ Local deployment completed for %s (run: %s)\n", appName, runID)
}
