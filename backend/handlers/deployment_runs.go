package handlers

import (
	"backend/database/api"
	"backend/platform"
	"backend/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

// DeploymentLogBroadcaster handles real-time log broadcasting
type DeploymentLogBroadcaster struct {
	mu          sync.RWMutex
	subscribers map[string]map[*websocket.Conn]bool // runID -> connections
}

var logBroadcaster = &DeploymentLogBroadcaster{
	subscribers: make(map[string]map[*websocket.Conn]bool),
}

// Subscribe adds a websocket connection to a deployment run
func (b *DeploymentLogBroadcaster) Subscribe(runID string, conn *websocket.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.subscribers[runID] == nil {
		b.subscribers[runID] = make(map[*websocket.Conn]bool)
	}
	b.subscribers[runID][conn] = true
	log.Printf("[WS] Client subscribed to deployment %s", runID)
}

// Unsubscribe removes a websocket connection
func (b *DeploymentLogBroadcaster) Unsubscribe(runID string, conn *websocket.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.subscribers[runID] != nil {
		delete(b.subscribers[runID], conn)
		if len(b.subscribers[runID]) == 0 {
			delete(b.subscribers, runID)
		}
	}
	log.Printf("[WS] Client unsubscribed from deployment %s", runID)
}

// Broadcast sends a message to all subscribers of a deployment run
func (b *DeploymentLogBroadcaster) Broadcast(runID string, message interface{}) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.subscribers[runID] == nil {
		return
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("[WS] Failed to marshal message: %v", err)
		return
	}

	for conn := range b.subscribers[runID] {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[WS] Failed to send message: %v", err)
		}
	}
}

// BroadcastLog sends a log update to subscribers
func BroadcastDeploymentLog(runID, stepName, status, logs string) {
	logBroadcaster.Broadcast(runID, map[string]interface{}{
		"type":      "log",
		"run_id":    runID,
		"step":      stepName,
		"status":    status,
		"logs":      logs,
		"timestamp": time.Now().Unix(),
	})
}

// BroadcastStepUpdate sends a step status update to subscribers
func BroadcastStepUpdate(runID, stepName, status string) {
	logBroadcaster.Broadcast(runID, map[string]interface{}{
		"type":      "step_update",
		"run_id":    runID,
		"step":      stepName,
		"status":    status,
		"timestamp": time.Now().Unix(),
	})
}

// BroadcastRunUpdate sends a run status update to subscribers
func BroadcastRunUpdate(runID, status string) {
	logBroadcaster.Broadcast(runID, map[string]interface{}{
		"type":      "run_update",
		"run_id":    runID,
		"status":    status,
		"timestamp": time.Now().Unix(),
	})
}

// WebSocket upgrader configuration
var wsUpgrader = websocket.FastHTTPUpgrader{
	CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
		return true // Allow all origins for now, can be restricted later
	},
}

// DeploymentLogsWebSocketHandler handles WebSocket connections for deployment logs
func DeploymentLogsWebSocketHandler(c *fiber.Ctx) error {
	runID := c.Params("run_id")
	if runID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"run_id is required",
			nil,
		))
	}

	// Check if it's a websocket upgrade request
	if !websocket.FastHTTPIsWebSocketUpgrade(c.Context()) {
		// If not a WebSocket request, return the deployment run data via REST
		ctx := context.Background()
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

	// Upgrade to WebSocket
	err := wsUpgrader.Upgrade(c.Context(), func(conn *websocket.Conn) {
		defer conn.Close()

		log.Printf("[WS] New connection for deployment %s", runID)

		// Subscribe to this deployment
		logBroadcaster.Subscribe(runID, conn)
		defer logBroadcaster.Unsubscribe(runID, conn)

		// Send current state
		ctx := context.Background()
		run, err := api.DeploymentRuns.GetDeploymentRun(ctx, runID)
		if err == nil && run != nil {
			initialState, _ := json.Marshal(map[string]interface{}{
				"type": "initial_state",
				"run":  run,
			})
			conn.WriteMessage(websocket.TextMessage, initialState)
		}

		// Keep connection alive and listen for messages
		for {
			messageType, msg, err := conn.ReadMessage()
			if err != nil {
				log.Printf("[WS] Connection closed for deployment %s: %v", runID, err)
				break
			}

			// Handle ping/pong
			if messageType == websocket.PingMessage {
				conn.WriteMessage(websocket.PongMessage, nil)
			}

			// Handle client messages (if needed)
			log.Printf("[WS] Received message from client: %s", string(msg))
		}
	})

	if err != nil {
		log.Printf("[WS] Upgrade error for deployment %s: %v", runID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"WebSocket upgrade failed",
			nil,
		))
	}

	return nil
}

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
	appName := c.Params("app_name")
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

	initLog := fmt.Sprintf("🚀 Starting deployment for %s\n📦 Git URL: %s\n🌿 Branch: %s\n🔨 Builder: %s\n", appName, gitURL, gitBranch, builder)
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

	// Execute actual deployment via platform adapter
	adapter := platform.GetAdapter()
	output, err := adapter.DeployFromGit(appName, gitURL, gitBranch, userID)
	finalLogs = output

	if err != nil {
		deployErr = err
		// Mark remaining steps as failed
		errLog := fmt.Sprintf("❌ Deployment failed: %v", err)
		updateStep(ctx, runID, "cloning", "completed", nil)
		updateStep(ctx, runID, "building", "failed", &errLog)
		BroadcastDeploymentLog(runID, "building", "failed", errLog)
		return
	}

	// Mark steps as completed
	cloneLog := "✅ Repository cloned successfully\n"
	updateStep(ctx, runID, "cloning", "completed", &cloneLog)
	BroadcastDeploymentLog(runID, "cloning", "completed", cloneLog)

	buildLog := "✅ Build completed successfully\n"
	updateStep(ctx, runID, "building", "completed", &buildLog)
	BroadcastDeploymentLog(runID, "building", "completed", buildLog)

	// Pushing
	updateStep(ctx, runID, "pushing", "running", nil)
	BroadcastStepUpdate(runID, "pushing", "running")
	pushLog := "✅ Image pushed to registry\n"
	updateStep(ctx, runID, "pushing", "completed", &pushLog)
	BroadcastDeploymentLog(runID, "pushing", "completed", pushLog)

	// Deploying
	updateStep(ctx, runID, "deploying", "running", nil)
	BroadcastStepUpdate(runID, "deploying", "running")
	api.DeploymentRuns.UpdateDeploymentRunStatus(ctx, runID, "deploying")
	deployLog := "✅ Deployment rolled out successfully\n"
	updateStep(ctx, runID, "deploying", "completed", &deployLog)
	BroadcastDeploymentLog(runID, "deploying", "completed", deployLog)

	// Cleanup
	updateStep(ctx, runID, "cleanup", "running", nil)
	BroadcastStepUpdate(runID, "cleanup", "running")
	cleanupLog := "✅ Cleanup completed\n"
	updateStep(ctx, runID, "cleanup", "completed", &cleanupLog)
	BroadcastDeploymentLog(runID, "cleanup", "completed", cleanupLog)

	// Append full build output
	if output != "" {
		api.DeploymentRuns.AppendBuildLogs(ctx, runID, "\n--- Build Output ---\n"+output)
	}
}

func updateStep(ctx context.Context, runID, stepName, status string, logs *string) {
	if err := api.DeploymentRuns.UpdateDeploymentStep(ctx, runID, stepName, status, logs); err != nil {
		log.Printf("[DEPLOY] Failed to update step %s: %v", stepName, err)
	}
}
