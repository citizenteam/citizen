package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"backend/pkg/logger"

	"github.com/gofiber/fiber/v2"
)

// VerifyWebhookSignature verifies HMAC signature for CitizenAuth webhooks
func VerifyWebhookSignature(c *fiber.Ctx, body []byte, timestamp, signature string) bool {
	webhookSecret, _ := c.Locals("webhook_secret").(string)
	if webhookSecret == "" {
		webhookSecret = os.Getenv("CITIZENAUTH_WEBHOOK_SECRET")
	}
	if webhookSecret == "" {
		logger.Default().WithComponent("webhook-validator").
			Warn("Webhook secret not set, skipping signature verification")
		return true // Allow webhooks if secret not configured (development)
	}

	// Compute HMAC
	message := fmt.Sprintf("%s.%s", timestamp, string(body))
	h := hmac.New(sha256.New, []byte(webhookSecret))
	h.Write([]byte(message))
	expectedSignature := "sha256=" + hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}
