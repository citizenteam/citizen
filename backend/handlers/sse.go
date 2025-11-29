package handlers

import (
	"backend/database/api"
	"backend/platform"
	"backend/platform/k3s"
	"backend/pubsub"
	"backend/utils"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
)

// SSE - Server-Sent Events handlers
// No WebSocket needed - works over regular HTTP

// DeploymentLogsSSE streams deployment logs via SSE
func DeploymentLogsSSE(c *fiber.Ctx) error {
	runID := c.Params("run_id")
	if runID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "run_id is required", nil,
		))
	}

	// Check Accept header for SSE
	accept := c.Get("Accept")
	if accept != "text/event-stream" {
		// REST fallback - return current state
		ctx := context.Background()
		run, err := api.DeploymentRuns.GetDeploymentRun(ctx, runID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(utils.NewCitizenResponse(
				false, "Deployment run not found", nil,
			))
		}
		return c.JSON(utils.NewCitizenResponse(true, "Deployment run", run))
	}

	// SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no") // Disable nginx buffering

	log.Printf("[SSE] Deployment logs stream started for run %s", runID)

	// Get initial state before streaming
	ctx := context.Background()
	run, err := api.DeploymentRuns.GetDeploymentRun(ctx, runID)

	// Stream context - subscription must happen INSIDE the stream writer
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Create message channel and subscribe INSIDE the stream
		msgChan := make(chan []byte, 100)
		topic := pubsub.DeploymentTopic(runID)
		pubsub.GlobalHub.SubscribeChannel(topic, msgChan)
		defer pubsub.GlobalHub.UnsubscribeChannel(topic, msgChan)

		log.Printf("[SSE] Stream started for run %s, subscribed to topic %s", runID, topic)

		// Send initial state
		if err == nil && run != nil {
			initialState, _ := json.Marshal(map[string]interface{}{
				"type": "initial_state",
				"run":  run,
			})
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", initialState)
			w.Flush()
		}

		// Heartbeat ticker - keep connection alive (3s for stability)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case msg := <-msgChan:
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
				w.Flush()

				// Check if run completed
				var data map[string]interface{}
				if json.Unmarshal(msg, &data) == nil {
					if msgType, ok := data["type"].(string); ok && msgType == "run_update" {
						// Safely check for status field
						if statusVal, ok := data["status"].(string); ok {
							if statusVal == "completed" || statusVal == "failed" {
								log.Printf("[SSE] Run %s completed with status %s, closing stream", runID, statusVal)
								time.Sleep(500 * time.Millisecond)
								return
							}
						}
					}
				}

			case <-ticker.C:
				// Heartbeat to keep connection alive
				_, err := fmt.Fprintf(w, ": heartbeat\n\n")
				if err != nil {
					log.Printf("[SSE] Heartbeat error for run %s, client disconnected", runID)
					return
				}
				if err := w.Flush(); err != nil {
					log.Printf("[SSE] Heartbeat error for run %s, client disconnected", runID)
					return
				}
			}
		}
	})

	return nil
}

// PodLogsSSE streams pod logs via SSE
func PodLogsSSE(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "app_name is required", nil,
		))
	}

	tailLines := c.QueryInt("tail", 500)

	// Check Accept header for SSE
	accept := c.Get("Accept")
	if accept != "text/event-stream" {
		// REST fallback
		k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false, "Pod logs only supported for K3s", nil,
			))
		}
		logs, err := k3sAdapter.GetAppLogs(appName, tailLines, false)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
				false, "Failed to get logs: "+err.Error(), nil,
			))
		}
		return c.JSON(utils.NewCitizenResponse(true, "Logs", fiber.Map{"logs": logs}))
	}

	// SSE headers - important for streaming
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	c.Set("Transfer-Encoding", "chunked")

	log.Printf("[SSE] Pod logs stream started for app %s", appName)

	k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Pod logs only supported for K3s", nil,
		))
	}

	// Stream context with channel-based communication
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Channel for messages (logs + control) - large buffer for fast logs
		msgChan := make(chan []byte, 10000)
		stopChan := make(chan struct{})

		// Send initial logs
		logs, err := k3sAdapter.GetAppLogs(appName, tailLines, false)
		if err == nil {
			initialData, _ := json.Marshal(map[string]interface{}{
				"type": "initial_logs",
				"logs": logs,
			})
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", initialData)
			if err := w.Flush(); err != nil {
				log.Printf("[SSE] Initial flush error for %s: %v", appName, err)
				return
			}
		}

		// Start log streaming goroutine
		namespace := fmt.Sprintf("citizen-app-%s", appName)
		go func() {
			defer close(msgChan)
			err := k3sAdapter.StreamPodLogs(namespace, appName, func(logLine string) {
				select {
				case <-stopChan:
					return
				default:
					data, _ := json.Marshal(map[string]interface{}{
						"type": "log",
						"logs": logLine,
					})
					select {
					case msgChan <- data:
					case <-stopChan:
						return
					}
				}
			})
			if err != nil {
				log.Printf("[SSE] StreamPodLogs ended for %s: %v", appName, err)
			}
		}()

		// Heartbeat ticker
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		defer close(stopChan)

		for {
			select {
			case msg, ok := <-msgChan:
				if !ok {
					// Channel closed, stream ended
					log.Printf("[SSE] Pod logs stream ended for %s", appName)
					return
				}
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
				if err := w.Flush(); err != nil {
					log.Printf("[SSE] Pod logs write error for %s: %v", appName, err)
					return
				}

			case <-ticker.C:
				// Heartbeat to keep connection alive
				if _, err := fmt.Fprintf(w, ": heartbeat\n\n"); err != nil {
					log.Printf("[SSE] Pod logs heartbeat WRITE error for %s: %v", appName, err)
					return
				}
				if err := w.Flush(); err != nil {
					log.Printf("[SSE] Pod logs heartbeat FLUSH error for %s: %v", appName, err)
					return
				}
				log.Printf("[SSE] Pod logs heartbeat OK for %s", appName)
			}
		}
	})

	return nil
}

// sendSSE helper to send SSE message
func sendSSE(c *fiber.Ctx, event string, data []byte) {
	c.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
}

// =============================================================================
// Publish helpers - for deployment code to call
// =============================================================================

// SSEPublishDeploymentLog publishes deployment log via pubsub (for SSE subscribers)
func SSEPublishDeploymentLog(runID, step, status, logs string) {
	topic := pubsub.DeploymentTopic(runID)
	data := map[string]interface{}{
		"type":   "log",
		"run_id": runID,
		"step":   step,
		"status": status,
		"logs":   logs,
	}
	jsonData, _ := json.Marshal(data)
	count := pubsub.GlobalHub.PublishToChannel(topic, jsonData)
	log.Printf("[SSE] Published deployment log to %d subscribers (run: %s, step: %s, len: %d)",
		count, runID, step, len(logs))
}

// SSEPublishStepUpdate publishes step status change
// Also sends as a log message to ensure delivery (workaround for proxy buffering)
func SSEPublishStepUpdate(runID, step, status string) {
	topic := pubsub.DeploymentTopic(runID)

	// Send as log message (these always get through)
	logData := map[string]interface{}{
		"type":        "log",
		"run_id":      runID,
		"step":        step,
		"status":      "running",
		"logs":        fmt.Sprintf("[STEP] %s: %s\n", step, status),
		"step_update": map[string]string{"step": step, "status": status},
	}
	jsonLogData, _ := json.Marshal(logData)
	count := pubsub.GlobalHub.PublishToChannel(topic, jsonLogData)
	log.Printf("[SSE] Published step_update (as log) to %d subscribers (run: %s, step: %s, status: %s)",
		count, runID, step, status)
}

// SSEPublishRunUpdate publishes run status change
// Also sends as a log message to ensure delivery (workaround for proxy buffering)
func SSEPublishRunUpdate(runID, status string, appUrl ...string) {
	topic := pubsub.DeploymentTopic(runID)

	// Send as log message (these always get through)
	logData := map[string]interface{}{
		"type":       "log",
		"run_id":     runID,
		"step":       "completed",
		"status":     "running",
		"logs":       fmt.Sprintf("[RUN] Deployment %s\n", status),
		"run_update": map[string]string{"status": status},
	}
	if len(appUrl) > 0 && appUrl[0] != "" {
		logData["run_update"].(map[string]string)["app_url"] = appUrl[0]
	}
	jsonLogData, _ := json.Marshal(logData)
	count := pubsub.GlobalHub.PublishToChannel(topic, jsonLogData)
	log.Printf("[SSE] Published run_update (as log) to %d subscribers (run: %s, status: %s)",
		count, runID, status)
}
