package services

import (
	"backend/internal/database"
	"backend/internal/services"
	webhookmodels "backend/internal/webhooks/models"
	"context"
	"fmt"
	"log"
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
	switch payload.Event {
	case "session.created":
		// Login event - ensure CitizenAuth user is mapped to a local user
		if payload.OrganizationID == "" {
			return fmt.Errorf("organization_id required")
		}

		assigned, assignErr := permissionService.IsUserAssignedToInstance(ctx, payload.UserID, payload.OrganizationID)
		if assignErr != nil {
			log.Printf("❌ [WEBHOOK-SESSION] Failed to verify assignment for %s: %v", payload.UserID, assignErr)
			return fmt.Errorf("failed to verify user assignment: %w", assignErr)
		}
		if !assigned {
			log.Printf("🚫 [WEBHOOK-SESSION] Ignoring login for %s - user not assigned to this instance", payload.UserID)
			return nil // Not an error, just ignored
		}

		var localUserID int
		mapQuery := `SELECT get_or_create_local_user($1, $2, $3, $4)`
		err := database.DB.QueryRow(ctx, mapQuery, payload.UserID, payload.Email, payload.Name, payload.OrganizationID).Scan(&localUserID)
		if err != nil {
			log.Printf("❌ [WEBHOOK-SESSION] Failed to map CitizenAuth user %s: %v", payload.UserID, err)
			return fmt.Errorf("failed to map user from CitizenAuth: %w", err)
		}

		// Session creation requires Fiber context (c.Hostname(), c.Locals())
		// which is only available in handlers. See webhooks/handlers/citizenauth.go
		// for the actual implementation.
		log.Printf("✅ [WEBHOOK-SESSION] User mapped: citizenAuthUser=%s localUser=%d",
			payload.UserID, localUserID)

	case "session.destroyed":
		// Logout event - clear local SSO sessions
		var localUserID int
		query := `SELECT get_local_user_id($1, $2)`
		err := database.DB.QueryRow(ctx, query, payload.UserID, payload.OrganizationID).Scan(&localUserID)

		if err != nil || localUserID == 0 {
			log.Printf("⚠️  [WEBHOOK-SESSION] User mapping not found for %s, skipping", payload.UserID)
			return nil // Not an error
		}

		log.Printf("🔗 [WEBHOOK-SESSION] Mapped user %s to local ID %d", payload.Email, localUserID)
		// Session clearing is handled in handlers using authservices.ClearUserSSOSessions()
		log.Printf("✅ [WEBHOOK-SESSION] User mapped for logout: user=%s (local ID: %d)", payload.Email, localUserID)

	default:
		return fmt.Errorf("unknown event type: %s", payload.Event)
	}

	return nil
}

// ProcessPermissionEvent processes permission webhook events
func ProcessPermissionEvent(ctx context.Context, payload webhookmodels.PermissionWebhookPayload) error {
	switch payload.Event {
	case "permission.granted":
		if payload.OrganizationID == "" {
			return fmt.Errorf("organization_id required")
		}
		err := permissionService.GrantPermission(ctx, payload.UserID, payload.OrganizationID, payload.AppID, payload.Role, payload.GrantedBy)
		if err != nil {
			log.Printf("❌ [WEBHOOK] Failed to grant permission: %v", err)
			return fmt.Errorf("failed to grant permission: %w", err)
		}
		log.Printf("✅ [WEBHOOK] Permission granted: user=%s, app=%s, role=%s",
			payload.UserID, payload.AppID, payload.Role)

	case "permission.revoked":
		err := permissionService.RevokePermission(ctx, payload.UserID, payload.OrganizationID, payload.AppID)
		if err != nil {
			log.Printf("❌ [WEBHOOK] Failed to revoke permission: %v", err)
			// Don't fail - permission might not exist
		}
		log.Printf("✅ [WEBHOOK] Permission revoked: user=%s, app=%s",
			payload.UserID, payload.AppID)

	default:
		return fmt.Errorf("unknown event type: %s", payload.Event)
	}

	return nil
}

// ProcessInstanceLifecycleEvent processes instance lifecycle webhook events
func ProcessInstanceLifecycleEvent(ctx context.Context, payload webhookmodels.InstanceLifecycleWebhookPayload) error {
	switch payload.Event {
	case "instance.deleted":
		if payload.InstanceID == "" {
			return fmt.Errorf("instance_id required")
		}

		instanceUUID, err := uuid.Parse(payload.InstanceID)
		if err != nil {
			return fmt.Errorf("invalid instance_id format: %w", err)
		}

		orgID, err := database.DeleteCitizenauthInstance(ctx, instanceUUID)
		if err != nil {
			return fmt.Errorf("failed to delete instance: %w", err)
		}

		log.Printf("🧹 [WEBHOOK-LIFECYCLE] Instance %s deleted (org %s). Reason: %s", instanceUUID, orgID, payload.Reason)
		return nil

	case "app.challenge.created":
		if payload.Domain == "" || payload.ChallengeURL == "" || payload.ChallengeBody == "" {
			return fmt.Errorf("domain, challenge_url and challenge_body required")
		}
		// Note: Challenge processing is handled in handlers using utils
		log.Printf("✅ [WEBHOOK-LIFECYCLE] Challenge created for %s", payload.Domain)
		return nil

	case "app.challenge.completed":
		if payload.Domain == "" || payload.ChallengeURL == "" {
			return fmt.Errorf("domain and challenge_url required")
		}
		// Note: Challenge processing is handled in handlers using utils
		log.Printf("✅ [WEBHOOK-LIFECYCLE] Challenge completed for %s", payload.Domain)
		return nil

	default:
		return fmt.Errorf("unknown lifecycle event: %s", payload.Event)
	}
}

// ParseChallengeURL parses challenge URL and returns host and path
func ParseChallengeURL(raw string) (string, string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("invalid challenge_url")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		host = strings.TrimSpace(parsed.Host)
	}
	if host == "" {
		return "", "", fmt.Errorf("challenge_url missing host")
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	return host, path, nil
}
