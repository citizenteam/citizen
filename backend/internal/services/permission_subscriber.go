package services

import (
	"context"
	"encoding/json"

	"backend/internal/database"
	rbacservice "backend/internal/rbac/service"
	"backend/pkg/logger"

	"github.com/redis/go-redis/v9"
)

var subLog = logger.Default().WithComponent("permission-subscriber")

// PermissionSubscriber listens to permission change events from CitizenAuth
type PermissionSubscriber struct {
	rbac        *rbacservice.Service
	redisClient *redis.Client
	channel     string
}

// PermissionChangeEvent represents a permission change event
type PermissionChangeEvent struct {
	Type           string `json:"type"`            // permission_update, permission_revoke
	UserID         string `json:"user_id"`         // CitizenAuth user UUID
	OrganizationID string `json:"organization_id"` // Organization UUID
	AppID          string `json:"app_id"`
	Role           string `json:"role,omitempty"`
	Timestamp      int64  `json:"timestamp"`
}

// NewPermissionSubscriber creates a new permission subscriber
func NewPermissionSubscriber() *PermissionSubscriber {
	return &PermissionSubscriber{
		rbac:        rbacservice.Default(),
		redisClient: database.RedisClient,
		channel:     "citizen:permission_change", // Local channel for cache invalidation
	}
}

// Start begins listening to permission change events
// This should be called as a background goroutine in main.go
func (ps *PermissionSubscriber) Start(ctx context.Context) error {
	if ps.redisClient == nil {
		subLog.Warn("Redis not available, subscriber disabled")
		return nil
	}

	// Subscribe to permission change channel
	pubsub := ps.redisClient.Subscribe(ctx, ps.channel)
	defer pubsub.Close()

	subLog.WithField("channel", ps.channel).Info("Subscribed to channel")

	// Receive subscription confirmation
	_, err := pubsub.Receive(ctx)
	if err != nil {
		subLog.WithField("error", err.Error()).Error("Failed to subscribe")
		return err
	}

	subLog.Info("Subscription confirmed, listening for events...")

	// Listen for messages
	ch := pubsub.Channel()
	for {
		select {
		case msg := <-ch:
			if msg == nil {
				continue
			}
			go ps.handlePermissionChange(msg.Payload)
		case <-ctx.Done():
			subLog.Info("Shutting down subscriber")
			return ctx.Err()
		}
	}
}

// handlePermissionChange processes permission change events
func (ps *PermissionSubscriber) handlePermissionChange(payload string) {
	var event PermissionChangeEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		subLog.WithField("error", err.Error()).Error("Failed to parse event")
		return
	}

	log := subLog.WithFields(map[string]interface{}{
		"type":    event.Type,
		"user_id": event.UserID,
		"app_id":  event.AppID,
		"org_id":  event.OrganizationID,
	})

	log.Debug("Received permission change event")

	// Invalidate RBAC cache for this user
	if event.UserID != "" && event.OrganizationID != "" {
		ps.rbac.InvalidateCache(event.UserID, event.OrganizationID)
		log.Info("RBAC cache invalidated")
	}
}

// SubscribeToExternalChannel subscribes to CitizenAuth's permission channel
// This is for events published directly from CitizenAuth (not local events)
func (ps *PermissionSubscriber) SubscribeToExternalChannel(ctx context.Context, channel string) error {
	if ps.redisClient == nil {
		subLog.Warn("Redis not available, external subscriber disabled")
		return nil
	}

	pubsub := ps.redisClient.Subscribe(ctx, channel)
	defer pubsub.Close()

	subLog.WithField("channel", channel).Info("Subscribed to external channel")

	_, err := pubsub.Receive(ctx)
	if err != nil {
		subLog.WithField("error", err.Error()).Error("Failed to subscribe to external channel")
		return err
	}

	ch := pubsub.Channel()
	for {
		select {
		case msg := <-ch:
			if msg == nil {
				continue
			}
			go ps.handlePermissionChange(msg.Payload)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
