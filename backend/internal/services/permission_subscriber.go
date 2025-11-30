package services

import (
	"backend/internal/database"
	"context"
	"encoding/json"
	"log"

	"github.com/redis/go-redis/v9"
)

// PermissionSubscriber listens to permission change events from CitizenAuth
type PermissionSubscriber struct {
	permissionService *PermissionService
	redisClient       *redis.Client
	channel           string
}

// PermissionChangeEvent represents a permission change event
type PermissionChangeEvent struct {
	Type      string `json:"type"`      // permission_update, permission_revoke
	UserID    string `json:"user_id"`
	AppID     string `json:"app_id"`
	Role      string `json:"role,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// NewPermissionSubscriber creates a new permission subscriber
func NewPermissionSubscriber(permService *PermissionService) *PermissionSubscriber {
	return &PermissionSubscriber{
		permissionService: permService,
		redisClient:       database.RedisClient,
		channel:           "auth:permission_change",
	}
}

// Start begins listening to permission change events
// This should be called as a background goroutine in main.go
func (ps *PermissionSubscriber) Start(ctx context.Context) error {
	if ps.redisClient == nil {
		log.Println("⚠️  [PERMISSION-SUB] Redis not available, subscriber disabled")
		return nil
	}

	// Subscribe to permission change channel
	pubsub := ps.redisClient.Subscribe(ctx, ps.channel)
	defer pubsub.Close()

	log.Printf("📡 [PERMISSION-SUB] Subscribed to channel: %s", ps.channel)

	// Receive subscription confirmation
	_, err := pubsub.Receive(ctx)
	if err != nil {
		log.Printf("❌ [PERMISSION-SUB] Failed to subscribe: %v", err)
		return err
	}

	log.Println("✅ [PERMISSION-SUB] Subscription confirmed, listening for events...")

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
			log.Println("🛑 [PERMISSION-SUB] Shutting down subscriber")
			return ctx.Err()
		}
	}
}

// handlePermissionChange processes permission change events
func (ps *PermissionSubscriber) handlePermissionChange(payload string) {
	var event PermissionChangeEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		log.Printf("❌ [PERMISSION-SUB] Failed to parse event: %v", err)
		return
	}

	log.Printf("📨 [PERMISSION-SUB] Received event: type=%s, user=%s, app=%s",
		event.Type, event.UserID, event.AppID)

	// Invalidate cache for this user
	// Currently we don't have cache, but when implemented:
	// ps.invalidateUserCache(event.UserID)

	log.Printf("✅ [PERMISSION-SUB] Event processed successfully")
}

// invalidateUserCache invalidates permission cache for a specific user
// Permission caching will be implemented when Redis-based caching layer is added
// This will reduce database queries for frequent permission checks
func (ps *PermissionSubscriber) invalidateUserCache(userID string) {
	// Future implementation:
	// - Delete key: citizen:perms:user:{userID} from Redis
	// - Force fresh DB query on next permission check
	
	log.Printf("🔄 [PERMISSION-SUB] Cache invalidation for user %s (not yet implemented)", userID)
}

