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

// ClusterMetricsSSE streams cluster-wide metrics via SSE
func ClusterMetricsSSE(c *fiber.Ctx) error {
	// Default refresh interval in seconds
	refreshInterval := c.QueryInt("interval", 5)
	if refreshInterval < 2 {
		refreshInterval = 2
	}
	if refreshInterval > 60 {
		refreshInterval = 60
	}

	// Check Accept header for SSE
	accept := c.Get("Accept")
	if accept != "text/event-stream" {
		// REST fallback
		k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false, "Cluster metrics only supported for K3s", nil,
			))
		}
		metrics, err := k3sAdapter.GetClusterMetrics()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
				false, "Failed to get cluster metrics: "+err.Error(), nil,
			))
		}
		return c.JSON(utils.NewCitizenResponse(true, "Cluster metrics", fiber.Map{"metrics": metrics}))
	}

	// SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	c.Set("Transfer-Encoding", "chunked")

	log.Printf("[SSE] Cluster metrics stream started (interval: %ds)", refreshInterval)

	k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Cluster metrics only supported for K3s", nil,
		))
	}

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ticker := time.NewTicker(time.Duration(refreshInterval) * time.Second)
		defer ticker.Stop()

		// Send initial metrics
		sendClusterMetrics(w, k3sAdapter)

		for {
			select {
			case <-ticker.C:
				if !sendClusterMetrics(w, k3sAdapter) {
					log.Printf("[SSE] Cluster metrics stream ended")
					return
				}
			}
		}
	})

	return nil
}

func sendClusterMetrics(w *bufio.Writer, adapter *k3s.K3sAdapter) bool {
	metrics, err := adapter.GetClusterMetrics()

	var data []byte
	if err != nil {
		data, _ = json.Marshal(map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
			"time":  time.Now().UTC().Format(time.RFC3339),
		})
	} else {
		data, _ = json.Marshal(map[string]interface{}{
			"type":    "cluster_metrics",
			"metrics": metrics,
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	}

	if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", data); err != nil {
		log.Printf("[SSE] Cluster metrics write error: %v", err)
		return false
	}

	if err := w.Flush(); err != nil {
		log.Printf("[SSE] Cluster metrics flush error: %v", err)
		return false
	}

	return true
}
