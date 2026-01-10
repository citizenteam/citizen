package handlers

import (
	"backend/internal/platform"
	"backend/internal/platform/k3s"
	"backend/internal/utils"

	"github.com/gofiber/fiber/v2"
)

// UpdateAppResourcesRequest represents request body for updating app resources
type UpdateAppResourcesRequest struct {
	CPULimit   string `json:"cpu_limit"`   // e.g., "500m", "1"
	CPURequest string `json:"cpu_request"` // e.g., "100m", "0.5"
	MemLimit   string `json:"mem_limit"`   // e.g., "512Mi", "1Gi"
	MemRequest string `json:"mem_request"` // e.g., "256Mi", "512Mi"
}

// UpdateAppResources updates CPU and memory limits for an app
func UpdateAppResources(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "app_name is required", nil,
		))
	}

	var req UpdateAppResourcesRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Invalid request body", nil,
		))
	}

	// At least one field must be provided
	if req.CPULimit == "" && req.CPURequest == "" && req.MemLimit == "" && req.MemRequest == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "At least one resource field must be provided", nil,
		))
	}

	k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Resource updates only supported for K3s", nil,
		))
	}

	err := k3sAdapter.UpdateAppResources(appName, req.CPULimit, req.CPURequest, req.MemLimit, req.MemRequest)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false, "Failed to update resources: "+err.Error(), nil,
		))
	}

	return c.JSON(utils.NewCitizenResponse(true, "App resources updated successfully", fiber.Map{
		"app_name":    appName,
		"cpu_limit":   req.CPULimit,
		"cpu_request": req.CPURequest,
		"mem_limit":   req.MemLimit,
		"mem_request": req.MemRequest,
	}))
}

// GetAppResources returns current resource configuration for an app
func GetAppResources(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "app_name is required", nil,
		))
	}

	k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Resource info only supported for K3s", nil,
		))
	}

	metrics, err := k3sAdapter.GetPodMetrics(appName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false, "Failed to get app resources: "+err.Error(), nil,
		))
	}

	// Extract resource info from first pod
	var resources fiber.Map
	if len(metrics) > 0 {
		m := metrics[0]
		resources = fiber.Map{
			"cpu_limit":      m.CPULimit,
			"mem_limit":      m.MemoryLimit,
			"cpu_usage":      m.CPUUsage,
			"mem_usage":      m.MemoryUsage,
			"cpu_percent":    m.CPUPercent,
			"memory_percent": m.MemoryPercent,
		}
	}

	return c.JSON(utils.NewCitizenResponse(true, "App resources", fiber.Map{
		"app_name":  appName,
		"resources": resources,
	}))
}
