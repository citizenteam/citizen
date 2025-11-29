package services

import (
	"backend/pubsub"
	"encoding/json"
	"fmt"
	"log"
)

// PublishDeploymentLog publishes deployment log via pubsub (for SSE subscribers)
func PublishDeploymentLog(runID, step, status, logs string) {
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

// PublishStepUpdate publishes step status change
// Also sends as a log message to ensure delivery (workaround for proxy buffering)
func PublishStepUpdate(runID, step, status string) {
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

// PublishRunUpdate publishes run status change
// Also sends as a log message to ensure delivery (workaround for proxy buffering)
func PublishRunUpdate(runID, status string, appUrl ...string) {
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
