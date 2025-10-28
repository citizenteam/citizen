package handlers

import (
	"backend/database"
	"backend/utils"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// InstanceHandshakeRequest represents the payload sent by CitizenAuth during handshake.
type InstanceHandshakeRequest struct {
	InstanceID     string `json:"instance_id"`
	OrganizationID string `json:"organization_id"`
	Domain         string `json:"domain"`
	CitizenauthURL string `json:"citizenauth_url"`
	APIKey         string `json:"api_key"`
	WebhookSecret  string `json:"webhook_secret"`
}

// InstanceHandshake consumes the bootstrap secrets after CitizenAuth registration.
func InstanceHandshake(c *fiber.Ctx) error {
	var req InstanceHandshakeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(false, "Invalid handshake payload", nil))
	}

	if req.InstanceID == "" || req.OrganizationID == "" || req.APIKey == "" || req.WebhookSecret == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(false, "Missing required fields", nil))
	}

	headerKey := c.Get("X-API-Key")
	if headerKey == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(false, "Missing X-API-Key header", nil))
	}
	if headerKey != req.APIKey {
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(false, "API key mismatch", nil))
	}

	cfg, err := parseHandshakeConfig(req)
	if err != nil {
		log.Printf("❌ [HANDSHAKE] Invalid identifiers: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(false, "Invalid handshake identifiers", nil))
	}

	cfg.Domain = req.Domain

	if err := database.UpsertCitizenauthInstance(c.Context(), cfg); err != nil {
		log.Printf("❌ [HANDSHAKE] Failed to persist instance secrets: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(false, "Failed to store handshake", nil))
	}

	log.Printf("🤝 [HANDSHAKE] Secrets stored for instance %s (org %s)", cfg.InstanceUUID, cfg.OrganizationID)

	return c.JSON(utils.NewCitizenResponse(true, "Handshake completed", fiber.Map{
		"instance_id":     cfg.InstanceUUID,
		"organization_id": cfg.OrganizationID,
	}))
}

func parseHandshakeConfig(req InstanceHandshakeRequest) (*database.CitizenauthInstanceConfig, error) {
	instanceUUID, err := uuid.Parse(req.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("invalid instance UUID: %w", err)
	}

	organizationID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	return &database.CitizenauthInstanceConfig{
		InstanceUUID:   instanceUUID,
		OrganizationID: organizationID,
		Domain:         req.Domain,
		CitizenauthURL: req.CitizenauthURL,
		APIKey:         req.APIKey,
		APIKeyPrefix:   first8(req.APIKey),
		WebhookSecret:  req.WebhookSecret,
	}, nil
}

func first8(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}
