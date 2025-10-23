package handlers

import (
	"backend/database"
	"backend/services"
	"backend/utils"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

var permissionService = services.NewPermissionService()

// WebhookSessionUpdate handles login/logout events from CitizenAuth
// POST /api/v1/service/webhooks/session-update
func WebhookSessionUpdate(c *fiber.Ctx) error {
	// 1. Validate webhook signature
	signature := c.Get("X-Webhook-Signature")
	timestamp := c.Get("X-Webhook-Timestamp")
	
	if signature == "" || timestamp == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Missing webhook headers",
			nil,
		))
	}
	
	// Verify timestamp (prevent replay attacks - max 5 minutes old)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Now().Unix()-ts > 300 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid or expired timestamp",
			nil,
		))
	}
	
	// Verify HMAC signature
	body := c.Body()
	if !verifyWebhookSignature(body, timestamp, signature) {
		log.Printf("❌ [WEBHOOK-SESSION] Invalid signature from %s", c.IP())
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"Invalid webhook signature",
			nil,
		))
	}
	
	// 2. Parse webhook payload
	var payload struct {
		Event      string `json:"event"`       // session.created, session.destroyed
		UserID     string `json:"user_id"`     // CitizenAuth user UUID
		Email      string `json:"email"`
		Name       string `json:"name"`
		SessionID  string `json:"session_id"`  // CitizenAuth session ID
		Token      string `json:"token"`       // JWT token (for login)
		Timestamp  int64  `json:"timestamp"`
	}
	
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid webhook payload",
			nil,
		))
	}
	
	// 3. Process webhook event
	switch payload.Event {
	case "session.created":
		// Login event - create local SSO session
		// TODO: Map CitizenAuth UUID to local user ID (for now use dummy)
		localUserID := 1
		
		deviceID := "CitizenAuth-SSO"
		ssoSessionID := createOrUpdateSSOSession(localUserID, c.Hostname(), deviceID)
		
		log.Printf("✅ [WEBHOOK-SESSION] Session created: user=%s, local_session=%s", 
			payload.Email, ssoSessionID)
	
	case "session.destroyed":
		// Logout event - clear local SSO sessions
		// Map CitizenAuth UUID to local user ID
		var localUserID int
		query := `SELECT get_local_user_id($1)`
		err := database.DB.QueryRow(c.Context(), query, payload.UserID).Scan(&localUserID)
		
		if err != nil || localUserID == 0 {
			log.Printf("⚠️  [WEBHOOK-SESSION] User mapping not found for %s, skipping", payload.UserID)
			return c.JSON(utils.NewCitizenResponse(
				true,
				"User mapping not found (user never logged in here)",
				nil,
			))
		}
		
		log.Printf("🔗 [WEBHOOK-SESSION] Mapped user %s to local ID %d", payload.Email, localUserID)
		
		// Clear all SSO sessions for this user
		clearUserSSOSessions(localUserID)
		
		log.Printf("✅ [WEBHOOK-SESSION] Session destroyed: user=%s (local ID: %d)", payload.Email, localUserID)
	
	default:
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			fmt.Sprintf("Unknown event type: %s", payload.Event),
			nil,
		))
	}
	
	return c.JSON(utils.NewCitizenResponse(
		true,
		"Session webhook processed",
		fiber.Map{
			"event":   payload.Event,
			"user_id": payload.UserID,
		},
	))
}

// WebhookPermissionUpdate handles permission updates from CitizenAuth
// POST /api/v1/service/webhooks/permission-update
func WebhookPermissionUpdate(c *fiber.Ctx) error {
	// 1. Validate webhook signature
	signature := c.Get("X-Webhook-Signature")
	timestamp := c.Get("X-Webhook-Timestamp")
	
	if signature == "" || timestamp == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Missing webhook headers",
			nil,
		))
	}
	
	// Verify timestamp (prevent replay attacks - max 5 minutes old)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Now().Unix()-ts > 300 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid or expired timestamp",
			nil,
		))
	}
	
	// Verify HMAC signature
	body := c.Body()
	if !verifyWebhookSignature(body, timestamp, signature) {
		log.Printf("❌ [WEBHOOK] Invalid signature from %s", c.IP())
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"Invalid webhook signature",
			nil,
		))
	}
	
	// 2. Parse webhook payload
	var payload struct {
		Event     string `json:"event"`      // permission.granted, permission.revoked
		UserID    string `json:"user_id"`
		AppID     string `json:"app_id"`
		Role      string `json:"role"`
		GrantedBy string `json:"granted_by"`
		Timestamp int64  `json:"timestamp"`
	}
	
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid webhook payload",
			nil,
		))
	}
	
	// 3. Process webhook event
	switch payload.Event {
	case "permission.granted":
		err = permissionService.GrantPermission(c.Context(), payload.UserID, payload.AppID, payload.Role, payload.GrantedBy)
		if err != nil {
			log.Printf("❌ [WEBHOOK] Failed to grant permission: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
				false,
				"Failed to process webhook",
				nil,
			))
		}
		log.Printf("✅ [WEBHOOK] Permission granted: user=%s, app=%s, role=%s", 
			payload.UserID, payload.AppID, payload.Role)
	
	case "permission.revoked":
		err = permissionService.RevokePermission(c.Context(), payload.UserID, payload.AppID)
		if err != nil {
			log.Printf("❌ [WEBHOOK] Failed to revoke permission: %v", err)
			// Don't fail - permission might not exist
		}
		log.Printf("✅ [WEBHOOK] Permission revoked: user=%s, app=%s", 
			payload.UserID, payload.AppID)
	
	default:
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			fmt.Sprintf("Unknown event type: %s", payload.Event),
			nil,
		))
	}
	
	return c.JSON(utils.NewCitizenResponse(
		true,
		"Webhook processed successfully",
		fiber.Map{
			"event":   payload.Event,
			"user_id": payload.UserID,
			"app_id":  payload.AppID,
		},
	))
}

// GetPermissionsForCitizenAuth returns user permissions (for CitizenAuth UI)
// GET /api/v1/service/permissions?user_id=xxx
func GetPermissionsForCitizenAuth(c *fiber.Ctx) error {
	userID := c.Query("user_id")
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"user_id query parameter required",
			nil,
		))
	}
	
	ctx := c.Context()
	permissions, err := permissionService.GetUserPermissions(ctx, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false,
			"Failed to get permissions",
			nil,
		))
	}
	
	// Convert permissions to response format
	var apps []fiber.Map
	for _, perm := range permissions {
		apps = append(apps, fiber.Map{
			"app_id":     perm.AppID,
			"role":       perm.Role,
			"granted_at": perm.GrantedAt,
		})
	}
	
	return c.JSON(utils.NewCitizenResponse(
		true,
		"Permissions retrieved",
		fiber.Map{
			"user_id":     userID,
			"permissions": apps,
			"count":       len(apps),
		},
	))
}

// verifyWebhookSignature verifies HMAC signature
func verifyWebhookSignature(body []byte, timestamp, signature string) bool {
	webhookSecret := os.Getenv("CITIZENAUTH_WEBHOOK_SECRET")
	if webhookSecret == "" {
		log.Println("⚠️  [WEBHOOK] CITIZENAUTH_WEBHOOK_SECRET not set, skipping signature verification")
		return true // Allow webhooks if secret not configured (development)
	}
	
	// Compute HMAC
	message := fmt.Sprintf("%s.%s", timestamp, string(body))
	h := hmac.New(sha256.New, []byte(webhookSecret))
	h.Write([]byte(message))
	expectedSignature := "sha256=" + hex.EncodeToString(h.Sum(nil))
	
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

