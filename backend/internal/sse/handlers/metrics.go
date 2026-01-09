package handlers

import (
	"backend/internal/platform"
	"backend/internal/platform/k3s"
	"backend/internal/utils"
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
)

// MetricsSSE streams pod metrics via SSE for real-time monitoring
func MetricsSSE(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "app_name is required", nil,
		))
	}

	// Default refresh interval in seconds
	refreshInterval := c.QueryInt("interval", 5)
	if refreshInterval < 2 {
		refreshInterval = 2 // Minimum 2 seconds
	}
	if refreshInterval > 60 {
		refreshInterval = 60 // Maximum 60 seconds
	}

	// Check Accept header for SSE
	accept := c.Get("Accept")
	if accept != "text/event-stream" {
		// REST fallback - return current metrics
		k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false, "Metrics only supported for K3s", nil,
			))
		}
		metrics, err := k3sAdapter.GetPodMetrics(appName)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
				false, "Failed to get metrics: "+err.Error(), nil,
			))
		}
		return c.JSON(utils.NewCitizenResponse(true, "Metrics", fiber.Map{"metrics": metrics}))
	}

	// SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	c.Set("Transfer-Encoding", "chunked")

	log.Printf("[SSE] Metrics stream started for app %s (interval: %ds)", appName, refreshInterval)

	k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Metrics only supported for K3s", nil,
		))
	}

	// Stream metrics
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ticker := time.NewTicker(time.Duration(refreshInterval) * time.Second)
		defer ticker.Stop()

		// Send initial metrics immediately
		sendMetrics(w, k3sAdapter, appName)

		for {
			select {
			case <-ticker.C:
				if !sendMetrics(w, k3sAdapter, appName) {
					log.Printf("[SSE] Metrics stream ended for %s", appName)
					return
				}
			}
		}
	})

	return nil
}

// sendMetrics fetches and sends metrics, returns false if connection should close
func sendMetrics(w *bufio.Writer, adapter *k3s.K3sAdapter, appName string) bool {
	metrics, err := adapter.GetPodMetrics(appName)

	var data []byte
	if err != nil {
		data, _ = json.Marshal(map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
			"time":  time.Now().UTC().Format(time.RFC3339),
		})
	} else {
		// Calculate aggregate metrics
		var totalCPUPercent, totalMemPercent float64
		var totalMemBytes int64

		for _, m := range metrics {
			totalCPUPercent += m.CPUPercent
			totalMemPercent += m.MemoryPercent
			totalMemBytes += m.MemoryBytes
		}

		data, _ = json.Marshal(map[string]interface{}{
			"type": "metrics",
			"pods": metrics,
			"summary": map[string]interface{}{
				"pod_count":    len(metrics),
				"total_cpu":    fmt.Sprintf("%.1f%%", totalCPUPercent),
				"total_memory": fmt.Sprintf("%.1f%%", totalMemPercent),
				"total_mem_mb": totalMemBytes / (1024 * 1024),
			},
			"time": time.Now().UTC().Format(time.RFC3339),
		})
	}

	if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", data); err != nil {
		log.Printf("[SSE] Metrics write error for %s: %v", appName, err)
		return false
	}

	if err := w.Flush(); err != nil {
		log.Printf("[SSE] Metrics flush error for %s: %v", appName, err)
		return false
	}

	return true
}
