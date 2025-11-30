package handlers

import (
	authservices "backend/internal/auth/services"
	"backend/internal/database"
	"backend/internal/services"
	"backend/internal/utils"
	webhookmodels "backend/internal/webhooks/models"
	webhookservices "backend/internal/webhooks/services"
	"backend/pkg/errors"
	"backend/pkg/logger"
	"backend/pkg/response"
	"fmt"
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
		return response.BadRequest(c, "Missing webhook headers")
	}

	// Verify timestamp (prevent replay attacks - max 5 minutes old)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Now().Unix()-ts > 300 {
		return response.BadRequest(c, "Invalid or expired timestamp")
	}

	// Verify HMAC signature
	body := c.Body()
	if !webhookservices.VerifyWebhookSignature(c, body, timestamp, signature) {
		logger.Default().WithComponent("webhook-session").
			WithField("ip", c.IP()).
			Error("Invalid signature")
		return response.Unauthorized(c, "Invalid webhook signature")
	}

	// 2. Parse webhook payload
	var payload webhookmodels.SessionWebhookPayload

	if err := c.BodyParser(&payload); err != nil {
		return response.BadRequest(c, "Invalid webhook payload")
	}

	// 3. Process webhook event
	switch payload.Event {
	case "session.created":
		// Login event - ensure CitizenAuth user is mapped to a local user
		if payload.OrganizationID == "" {
			return response.BadRequest(c, "organization_id required")
		}

		log := logger.Default().WithComponent("webhook-session").
			WithField("user_id", payload.UserID).
			WithField("organization_id", payload.OrganizationID)

		assigned, assignErr := permissionService.IsUserAssignedToInstance(c.Context(), payload.UserID, payload.OrganizationID)
		if assignErr != nil {
			log.WithField("error", assignErr).Error("Failed to verify assignment")
			return response.InternalServerError(c, "Failed to verify user assignment")
		}
		if !assigned {
			log.Warn("Ignoring login - user not assigned to this instance")
			return response.SuccessWithMessage(c, "User not assigned to this instance", fiber.Map{
				"user_id": payload.UserID,
			})
		}

		var localUserID int
		mapQuery := `SELECT get_or_create_local_user($1, $2, $3, $4)`
		err := database.DB.QueryRow(c.Context(), mapQuery, payload.UserID, payload.Email, payload.Name, payload.OrganizationID).Scan(&localUserID)
		if err != nil {
			log.WithField("error", err).Error("Failed to map CitizenAuth user")
			return response.InternalServerError(c, "Failed to map user from CitizenAuth")
		}

		deviceID := "CitizenAuth-SSO"
		if payload.SessionID != "" {
			deviceID = fmt.Sprintf("CitizenAuth-%s", payload.SessionID)
		}
		ssoSessionID := authservices.CreateOrUpdateSSOSession(localUserID, c.Hostname(), deviceID, &payload.OrganizationID)
		c.Locals("organization_id", payload.OrganizationID)

		log.WithFields(map[string]interface{}{
			"citizen_auth_user": payload.UserID,
			"local_user":        localUserID,
			"session":           ssoSessionID,
		}).Info("Session created")

	case "session.destroyed":
		// Logout event - clear local SSO sessions
		log := logger.Default().WithComponent("webhook-session").
			WithField("user_id", payload.UserID).
			WithField("email", payload.Email)

		// Map CitizenAuth UUID to local user ID
		var localUserID int
		query := `SELECT get_local_user_id($1, $2)`
		err := database.DB.QueryRow(c.Context(), query, payload.UserID, payload.OrganizationID).Scan(&localUserID)

		if err != nil || localUserID == 0 {
			log.Warn("User mapping not found, skipping")
			return response.SuccessWithMessage(c, "User mapping not found (user never logged in here)", nil)
		}

		log.WithField("local_user_id", localUserID).Info("Mapped user")

		// Clear all SSO sessions for this user
		authservices.ClearUserSSOSessions(localUserID)

		log.WithField("local_user_id", localUserID).Info("Session destroyed")

	default:
		return response.BadRequest(c, fmt.Sprintf("Unknown event type: %s", payload.Event))
	}

	return response.SuccessWithMessage(c, "Session webhook processed", fiber.Map{
		"event":   payload.Event,
		"user_id": payload.UserID,
	})
}

// WebhookPermissionUpdate handles permission updates from CitizenAuth
// POST /api/v1/service/webhooks/permission-update
func WebhookPermissionUpdate(c *fiber.Ctx) error {
	// 1. Validate webhook signature
	signature := c.Get("X-Webhook-Signature")
	timestamp := c.Get("X-Webhook-Timestamp")

	if signature == "" || timestamp == "" {
		return response.BadRequest(c, "Missing webhook headers")
	}

	// Verify timestamp (prevent replay attacks - max 5 minutes old)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Now().Unix()-ts > 300 {
		return response.BadRequest(c, "Invalid or expired timestamp")
	}

	// Verify HMAC signature
	body := c.Body()
	if !webhookservices.VerifyWebhookSignature(c, body, timestamp, signature) {
		logger.Default().WithComponent("webhook-permission").
			WithField("ip", c.IP()).
			Error("Invalid signature")
		return response.Unauthorized(c, "Invalid webhook signature")
	}

	// 2. Parse webhook payload
	var payload webhookmodels.PermissionWebhookPayload

	if err := c.BodyParser(&payload); err != nil {
		return response.BadRequest(c, "Invalid webhook payload")
	}

	log := logger.Default().WithComponent("webhook-permission").
		WithField("event", payload.Event).
		WithField("user_id", payload.UserID).
		WithField("app_id", payload.AppID)

	// 3. Process webhook event
	if err := webhookservices.ProcessPermissionEvent(c.Context(), payload); err != nil {
		if payload.Event == "permission.granted" && payload.OrganizationID == "" {
			return response.BadRequest(c, "organization_id required")
		}
		log.WithField("error", err).Error("Failed to process permission event")

		var appErr *errors.Error
		if errors.As(err, &appErr) {
			return response.ErrorWithCode(c, fiber.StatusInternalServerError, appErr.Code, appErr.Message)
		}
		return response.InternalServerError(c, "Failed to process webhook: "+err.Error())
	}

	return response.SuccessWithMessage(c, "Webhook processed successfully", fiber.Map{
		"event":   payload.Event,
		"user_id": payload.UserID,
		"app_id":  payload.AppID,
	})
}

// WebhookInstanceLifecycle handles lifecycle events like instance deletion from CitizenAuth.
func WebhookInstanceLifecycle(c *fiber.Ctx) error {
	signature := c.Get("X-Webhook-Signature")
	timestamp := c.Get("X-Webhook-Timestamp")

	if signature == "" || timestamp == "" {
		return response.BadRequest(c, "Missing webhook headers")
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Now().Unix()-ts > 300 {
		return response.BadRequest(c, "Invalid or expired timestamp")
	}

	body := c.Body()
	if !webhookservices.VerifyWebhookSignature(c, body, timestamp, signature) {
		logger.Default().WithComponent("webhook-lifecycle").
			WithField("ip", c.IP()).
			Error("Invalid signature")
		return response.Unauthorized(c, "Invalid webhook signature")
	}

	var payload webhookmodels.InstanceLifecycleWebhookPayload

	if err := c.BodyParser(&payload); err != nil {
		return response.BadRequest(c, "Invalid webhook payload")
	}

	log := logger.Default().WithComponent("webhook-lifecycle").
		WithField("event", payload.Event)

	// Process lifecycle events
	switch payload.Event {
	case "instance.deleted":
		if payload.InstanceID == "" {
			return response.BadRequest(c, "instance_id required")
		}

		instanceUUID, err := uuid.Parse(payload.InstanceID)
		if err != nil {
			return response.BadRequest(c, "Invalid instance_id format")
		}

		log = log.WithField("instance_id", instanceUUID)

		orgID, err := database.DeleteCitizenauthInstance(c.Context(), instanceUUID)
		if err != nil {
			if errors.Is(err, database.ErrCitizenauthInstanceNotFound) {
				log.Info("Instance already cleaned")
				return response.SuccessWithMessage(c, "Instance already cleaned", fiber.Map{
					"instance_id": instanceUUID,
				})
			}

			log.WithField("error", err).Error("Failed to delete instance")
			return response.InternalServerError(c, "Failed to cleanup instance")
		}

		log.WithFields(map[string]interface{}{
			"organization_id": orgID,
			"reason":          payload.Reason,
		}).Info("Instance deleted")

		return response.SuccessWithMessage(c, "Instance cleanup completed", fiber.Map{
			"instance_id":     instanceUUID,
			"organization_id": orgID,
		})

	case "app.challenge.created":
		if payload.Domain == "" || payload.ChallengeURL == "" || payload.ChallengeBody == "" {
			return response.BadRequest(c, "domain, challenge_url and challenge_body required")
		}
		host, path, err := webhookservices.ParseChallengeURL(payload.ChallengeURL)
		if err != nil {
			return response.BadRequest(c, err.Error())
		}
		if err := utils.AddHTTPChallengeEntry(host, path, payload.ChallengeBody); err != nil {
			log.WithField("error", err).Error("Failed to persist challenge")
			return response.InternalServerError(c, "Failed to store HTTP challenge")
		}
		if err := utils.ReloadTraefik(); err != nil {
			log.WithField("error", err).Warn("Traefik reload after challenge add failed")
		}
		return response.SuccessWithMessage(c, "HTTP challenge registered", fiber.Map{
			"domain": payload.Domain,
		})

	case "app.challenge.completed":
		if payload.Domain == "" || payload.ChallengeURL == "" {
			return response.BadRequest(c, "domain and challenge_url required")
		}
		host, path, err := webhookservices.ParseChallengeURL(payload.ChallengeURL)
		if err != nil {
			return response.BadRequest(c, err.Error())
		}
		if removed, err := utils.RemoveHTTPChallengeEntry(host, path); err != nil {
			log.WithField("error", err).Error("Failed to remove challenge")
			return response.InternalServerError(c, "Failed to remove HTTP challenge")
		} else if !removed {
			log.WithField("challenge_url", payload.ChallengeURL).Info("Challenge already removed")
		}
		if err := utils.ReloadTraefik(); err != nil {
			log.WithField("error", err).Warn("Traefik reload after challenge cleanup failed")
		}
		return response.SuccessWithMessage(c, "HTTP challenge cleanup completed", fiber.Map{
			"domain": payload.Domain,
		})

	default:
		return response.BadRequest(c, fmt.Sprintf("Unknown lifecycle event: %s", payload.Event))
	}
}

// GetPermissionsForCitizenAuth returns user permissions (for CitizenAuth UI)
// GET /api/v1/service/permissions?user_id=xxx
func GetPermissionsForCitizenAuth(c *fiber.Ctx) error {
	userID := c.Query("user_id")
	orgID := c.Query("organization_id")
	if userID == "" || orgID == "" {
		return response.BadRequest(c, "user_id and organization_id query parameters required")
	}

	ctx := c.Context()
	permissions, err := permissionService.GetUserPermissions(ctx, userID, orgID)
	if err != nil {
		logger.Default().WithComponent("webhook-permissions").
			WithField("user_id", userID).
			WithField("organization_id", orgID).
			WithField("error", err).
			Error("Failed to get permissions")
		return response.InternalServerError(c, "Failed to get permissions")
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

	return response.SuccessWithMessage(c, "Permissions retrieved", fiber.Map{
		"user_id":         userID,
		"organization_id": orgID,
		"permissions":     apps,
		"count":           len(apps),
	})
}
