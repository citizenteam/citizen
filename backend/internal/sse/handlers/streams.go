package handlers

import (
	"backend/internal/database/api"
	"backend/internal/platform"
	"backend/internal/platform/k3s"
	"backend/internal/pubsub"
	"backend/internal/utils"
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

	// SSE headers - important for streaming
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	c.Set("Transfer-Encoding", "chunked")

	log.Printf("[SSE] Deployment logs stream started for run %s", runID)

	// Get initial state before streaming
	ctx := context.Background()
	run, err := api.DeploymentRuns.GetDeploymentRun(ctx, runID)

	// Stream context - subscription must happen INSIDE the stream writer
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Large buffer for fast log streaming
		msgChan := make(chan []byte, 10000)
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
			if err := w.Flush(); err != nil {
				log.Printf("[SSE] Initial flush error for run %s: %v", runID, err)
				return
			}
		}

		// Heartbeat ticker - keep connection alive
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case msg := <-msgChan:
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
				if err := w.Flush(); err != nil {
					log.Printf("[SSE] Deployment logs write error for run %s: %v", runID, err)
					return
				}

				// Check if run completed
				var data map[string]interface{}
				if json.Unmarshal(msg, &data) == nil {
					if msgType, ok := data["type"].(string); ok && msgType == "run_update" {
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
				if _, err := fmt.Fprintf(w, ": heartbeat\n\n"); err != nil {
					log.Printf("[SSE] Deployment heartbeat error for run %s", runID)
					return
				}
				if err := w.Flush(); err != nil {
					log.Printf("[SSE] Deployment heartbeat error for run %s", runID)
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
