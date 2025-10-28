package handlers

import (
	"backend/database"
	"backend/services"
	"backend/utils"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
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
	if !verifyWebhookSignature(c, body, timestamp, signature) {
		log.Printf("❌ [WEBHOOK-SESSION] Invalid signature from %s", c.IP())
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"Invalid webhook signature",
			nil,
		))
	}

	// 2. Parse webhook payload
	var payload struct {
		Event          string `json:"event"`
		UserID         string `json:"user_id"`
		Email          string `json:"email"`
		Name           string `json:"name"`
		OrganizationID string `json:"organization_id"`
		SessionID      string `json:"session_id"`
		Token          string `json:"token"`
		Timestamp      int64  `json:"timestamp"`
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
		// Login event - ensure CitizenAuth user is mapped to a local user
		if payload.OrganizationID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"organization_id required",
				nil,
			))
		}

		assigned, assignErr := permissionService.IsUserAssignedToInstance(c.Context(), payload.UserID, payload.OrganizationID)
		if assignErr != nil {
			log.Printf("❌ [WEBHOOK-SESSION] Failed to verify assignment for %s: %v", payload.UserID, assignErr)
			return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
				false,
				"Failed to verify user assignment",
				nil,
			))
		}
		if !assigned {
			log.Printf("🚫 [WEBHOOK-SESSION] Ignoring login for %s - user not assigned to this instance", payload.UserID)
			return c.JSON(utils.NewCitizenResponse(
				true,
				"User not assigned to this instance",
				fiber.Map{
					"user_id": payload.UserID,
				},
			))
		}

		var localUserID int
		mapQuery := `SELECT get_or_create_local_user($1, $2, $3, $4)`
		err := database.DB.QueryRow(c.Context(), mapQuery, payload.UserID, payload.Email, payload.Name, payload.OrganizationID).Scan(&localUserID)
		if err != nil {
			log.Printf("❌ [WEBHOOK-SESSION] Failed to map CitizenAuth user %s: %v", payload.UserID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
				false,
				"Failed to map user from CitizenAuth",
				nil,
			))
		}

		deviceID := "CitizenAuth-SSO"
		if payload.SessionID != "" {
			deviceID = fmt.Sprintf("CitizenAuth-%s", payload.SessionID)
		}
		ssoSessionID := createOrUpdateSSOSession(localUserID, c.Hostname(), deviceID, &payload.OrganizationID)
		c.Locals("organization_id", payload.OrganizationID)

		log.Printf("✅ [WEBHOOK-SESSION] Session created: citizenAuthUser=%s localUser=%d session=%s",
			payload.UserID, localUserID, ssoSessionID)

	case "session.destroyed":
		// Logout event - clear local SSO sessions
		// Map CitizenAuth UUID to local user ID
		var localUserID int
		query := `SELECT get_local_user_id($1, $2)`
		err := database.DB.QueryRow(c.Context(), query, payload.UserID, payload.OrganizationID).Scan(&localUserID)

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
	if !verifyWebhookSignature(c, body, timestamp, signature) {
		log.Printf("❌ [WEBHOOK] Invalid signature from %s", c.IP())
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"Invalid webhook signature",
			nil,
		))
	}

	// 2. Parse webhook payload
	var payload struct {
		Event          string `json:"event"` // permission.granted, permission.revoked
		UserID         string `json:"user_id"`
		OrganizationID string `json:"organization_id"`
		AppID          string `json:"app_id"`
		Role           string `json:"role"`
		GrantedBy      string `json:"granted_by"`
		Timestamp      int64  `json:"timestamp"`
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
		if payload.OrganizationID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"organization_id required",
				nil,
			))
		}
		err = permissionService.GrantPermission(c.Context(), payload.UserID, payload.OrganizationID, payload.AppID, payload.Role, payload.GrantedBy)
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
		err = permissionService.RevokePermission(c.Context(), payload.UserID, payload.OrganizationID, payload.AppID)
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

// WebhookInstanceLifecycle handles lifecycle events like instance deletion from CitizenAuth.
func WebhookInstanceLifecycle(c *fiber.Ctx) error {
	signature := c.Get("X-Webhook-Signature")
	timestamp := c.Get("X-Webhook-Timestamp")

	if signature == "" || timestamp == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Missing webhook headers",
			nil,
		))
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Now().Unix()-ts > 300 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid or expired timestamp",
			nil,
		))
	}

	body := c.Body()
	if !verifyWebhookSignature(c, body, timestamp, signature) {
		log.Printf("❌ [WEBHOOK-LIFECYCLE] Invalid signature from %s", c.IP())
		return c.Status(fiber.StatusUnauthorized).JSON(utils.NewCitizenResponse(
			false,
			"Invalid webhook signature",
			nil,
		))
	}

	var payload struct {
		Event      string `json:"event"`
		InstanceID string `json:"instance_id"`
		Reason     string `json:"reason"`
	}

	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"Invalid webhook payload",
			nil,
		))
	}

	switch payload.Event {
	case "instance.deleted":
		if payload.InstanceID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"instance_id required",
				nil,
			))
		}

		instanceUUID, err := uuid.Parse(payload.InstanceID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
				false,
				"Invalid instance_id format",
				nil,
			))
		}

		orgID, err := database.DeleteCitizenauthInstance(c.Context(), instanceUUID)
		if err != nil {
			if errors.Is(err, database.ErrCitizenauthInstanceNotFound) {
				log.Printf("ℹ️  [WEBHOOK-LIFECYCLE] Instance %s already cleaned", instanceUUID)
				return c.JSON(utils.NewCitizenResponse(
					true,
					"Instance already cleaned",
					fiber.Map{
						"instance_id": instanceUUID,
					},
				))
			}

			log.Printf("❌ [WEBHOOK-LIFECYCLE] Failed to delete instance %s: %v", instanceUUID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
				false,
				"Failed to cleanup instance",
				nil,
			))
		}

		log.Printf("🧹 [WEBHOOK-LIFECYCLE] Instance %s deleted (org %s). Reason: %s", instanceUUID, orgID, payload.Reason)

		return c.JSON(utils.NewCitizenResponse(
			true,
			"Instance cleanup completed",
			fiber.Map{
				"instance_id":     instanceUUID,
				"organization_id": orgID,
			},
		))

	default:
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			fmt.Sprintf("Unknown lifecycle event: %s", payload.Event),
			nil,
		))
	}
}

// GetPermissionsForCitizenAuth returns user permissions (for CitizenAuth UI)
// GET /api/v1/service/permissions?user_id=xxx
func GetPermissionsForCitizenAuth(c *fiber.Ctx) error {
	userID := c.Query("user_id")
	orgID := c.Query("organization_id")
	if userID == "" || orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false,
			"user_id and organization_id query parameters required",
			nil,
		))
	}

	ctx := c.Context()
	permissions, err := permissionService.GetUserPermissions(ctx, userID, orgID)
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
			"user_id":         userID,
			"organization_id": orgID,
			"permissions":     apps,
			"count":           len(apps),
		},
	))
}

// verifyWebhookSignature verifies HMAC signature
func verifyWebhookSignature(c *fiber.Ctx, body []byte, timestamp, signature string) bool {
	webhookSecret, _ := c.Locals("webhook_secret").(string)
	if webhookSecret == "" {
		webhookSecret = os.Getenv("CITIZENAUTH_WEBHOOK_SECRET")
	}
	if webhookSecret == "" {
		log.Println("⚠️  [WEBHOOK] Webhook secret not set, skipping signature verification")
		return true // Allow webhooks if secret not configured (development)
	}

	// Compute HMAC
	message := fmt.Sprintf("%s.%s", timestamp, string(body))
	h := hmac.New(sha256.New, []byte(webhookSecret))
	h.Write([]byte(message))
	expectedSignature := "sha256=" + hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}
