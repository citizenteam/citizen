package services

import (
	"backend/internal/database"
	"backend/internal/services"
	webhookmodels "backend/internal/webhooks/models"
	"backend/pkg/errors"
	"backend/pkg/logger"
	"context"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

var permissionService = services.NewPermissionService()

// ProcessSessionEvent processes session webhook events (login/logout)
// Note: This function is currently not used. Session creation requires Fiber context
// (c.Hostname(), c.Locals()) which is only available in handlers. The actual session
// creation logic is implemented directly in webhooks/handlers/citizenauth.go
func ProcessSessionEvent(ctx context.Context, c *fiber.Ctx, payload webhookmodels.SessionWebhookPayload) error {
	log := logger.Default().WithComponent("webhook-processor").
		WithField("event", payload.Event).
		WithField("user_id", payload.UserID)

	switch payload.Event {
	case "session.created":
		// Login event - ensure CitizenAuth user is mapped to a local user
		if payload.OrganizationID == "" {
			return errors.BadRequest("organization_id required")
		}

		assigned, assignErr := permissionService.IsUserAssignedToInstance(ctx, payload.UserID, payload.OrganizationID)
		if assignErr != nil {
			log.WithField("error", assignErr).Error("Failed to verify assignment")
			return errors.Wrap(assignErr, errors.ErrCodeInternal, "failed to verify user assignment")
		}
		if !assigned {
			log.Warn("Ignoring login - user not assigned to this instance")
			return nil // Not an error, just ignored
		}

		var localUserID int
		mapQuery := `SELECT get_or_create_local_user($1, $2, $3, $4)`
		err := database.DB.QueryRow(ctx, mapQuery, payload.UserID, payload.Email, payload.Name, payload.OrganizationID).Scan(&localUserID)
		if err != nil {
			log.WithField("error", err).Error("Failed to map CitizenAuth user")
			return errors.Wrap(err, errors.ErrCodeInternal, "failed to map user from CitizenAuth")
		}

		// Session creation requires Fiber context (c.Hostname(), c.Locals())
		// which is only available in handlers. See webhooks/handlers/citizenauth.go
		// for the actual implementation.
		log.WithField("local_user_id", localUserID).Info("User mapped")

	case "session.destroyed":
		// Logout event - clear local SSO sessions
		var localUserID int
		query := `SELECT get_local_user_id($1, $2)`
		err := database.DB.QueryRow(ctx, query, payload.UserID, payload.OrganizationID).Scan(&localUserID)

		if err != nil || localUserID == 0 {
			log.WithField("email", payload.Email).Warn("User mapping not found, skipping")
			return nil // Not an error
		}

		log.WithFields(map[string]interface{}{
			"email":         payload.Email,
			"local_user_id": localUserID,
		}).Info("User mapped for logout")
		// Session clearing is handled in handlers using authservices.ClearUserSSOSessions()

	default:
		return errors.BadRequestf("unknown event type: %s", payload.Event)
	}

	return nil
}

// ProcessPermissionEvent processes permission webhook events
func ProcessPermissionEvent(ctx context.Context, payload webhookmodels.PermissionWebhookPayload) error {
	log := logger.Default().WithComponent("webhook-permission").
		WithField("event", payload.Event).
		WithField("user_id", payload.UserID).
		WithField("app_id", payload.AppID)

	switch payload.Event {
	case "permission.granted":
		if payload.OrganizationID == "" {
			return errors.BadRequest("organization_id required")
		}
		err := permissionService.GrantPermission(ctx, payload.UserID, payload.OrganizationID, payload.AppID, payload.Role, payload.GrantedBy)
		if err != nil {
			log.WithField("error", err).Error("Failed to grant permission")
			return errors.Wrap(err, errors.ErrCodeInternal, "failed to grant permission")
		}
		log.WithField("role", payload.Role).Info("Permission granted")

	case "permission.revoked":
		if payload.OrganizationID == "" {
			log.Warn("permission.revoked received without organization_id - attempting revoke anyway")
		}
		err := permissionService.RevokePermission(ctx, payload.UserID, payload.OrganizationID, payload.AppID)
		if err != nil {
			log.WithFields(map[string]interface{}{
				"error":   err,
				"user_id": payload.UserID,
				"org_id":  payload.OrganizationID,
				"app_id":  payload.AppID,
			}).Error("Failed to revoke permission")
			// Return error so CitizenAuth knows the revoke failed
			return errors.Wrap(err, errors.ErrCodeInternal, "failed to revoke permission in Citizen backend")
		}

		// NOTE: We do NOT clear SSO sessions here because:
		// 1. User may have access to other apps
		// 2. ForwardAuth validates app-level RBAC on every request
		// 3. RBAC cache is already invalidated by RevokePermission()
		// The user will get 403 Forbidden when trying to access this specific app

		log.WithFields(map[string]interface{}{
			"user_id": payload.UserID,
			"app_id":  payload.AppID,
		}).Info("Permission revoked successfully - app access will be denied via ForwardAuth RBAC")

	default:
		return errors.BadRequestf("unknown event type: %s", payload.Event)
	}

	return nil
}

// ProcessInstanceLifecycleEvent processes instance lifecycle webhook events
func ProcessInstanceLifecycleEvent(ctx context.Context, payload webhookmodels.InstanceLifecycleWebhookPayload) error {
	log := logger.Default().WithComponent("webhook-lifecycle").
		WithField("event", payload.Event)

	switch payload.Event {
	case "instance.deleted":
		if payload.InstanceID == "" {
			return errors.BadRequest("instance_id required")
		}

		instanceUUID, err := uuid.Parse(payload.InstanceID)
		if err != nil {
			return errors.BadRequestf("invalid instance_id format: %v", err)
		}

		log = log.WithField("instance_id", instanceUUID)

		orgID, err := database.DeleteCitizenauthInstance(ctx, instanceUUID)
		if err != nil {
			log.WithField("error", err).Error("Failed to delete instance")
			return errors.Wrap(err, errors.ErrCodeInternal, "failed to delete instance")
		}

		log.WithFields(map[string]interface{}{
			"organization_id": orgID,
			"reason":          payload.Reason,
		}).Info("Instance deleted")
		return nil

	case "app.challenge.created":
		if payload.Domain == "" || payload.ChallengeURL == "" || payload.ChallengeBody == "" {
			return errors.BadRequest("domain, challenge_url and challenge_body required")
		}
		// Note: Challenge processing is handled in handlers using utils
		log.WithField("domain", payload.Domain).Info("Challenge created")
		return nil

	case "app.challenge.completed":
		if payload.Domain == "" || payload.ChallengeURL == "" {
			return errors.BadRequest("domain and challenge_url required")
		}
		// Note: Challenge processing is handled in handlers using utils
		log.WithField("domain", payload.Domain).Info("Challenge completed")
		return nil

	default:
		return errors.BadRequestf("unknown lifecycle event: %s", payload.Event)
	}
}

// ParseChallengeURL parses challenge URL and returns host and path
func ParseChallengeURL(raw string) (string, string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", errors.BadRequest("invalid challenge_url")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		host = strings.TrimSpace(parsed.Host)
	}
	if host == "" {
		return "", "", errors.BadRequest("challenge_url missing host")
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	return host, path, nil
}
