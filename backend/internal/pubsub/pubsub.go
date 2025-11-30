package pubsub

import (
	"log"
	"sync"
)

// Hub is the central pub/sub hub for SSE channels
type Hub struct {
	mu       sync.RWMutex
	channels map[string]map[chan []byte]bool // topic -> channels
}

// Global hub instance
var GlobalHub = NewHub()

// NewHub creates a new pub/sub hub
func NewHub() *Hub {
	return &Hub{
		channels: make(map[string]map[chan []byte]bool),
	}
}

// SubscribeChannel adds a channel to a topic
func (h *Hub) SubscribeChannel(topic string, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.channels[topic] == nil {
		h.channels[topic] = make(map[chan []byte]bool)
	}
	h.channels[topic][ch] = true
	log.Printf("[PUBSUB] Subscribed to topic: %s (total: %d)", topic, len(h.channels[topic]))
}

// UnsubscribeChannel removes a channel from a topic
func (h *Hub) UnsubscribeChannel(topic string, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.channels[topic] != nil {
		delete(h.channels[topic], ch)
		if len(h.channels[topic]) == 0 {
			delete(h.channels, topic)
		}
	}
	log.Printf("[PUBSUB] Unsubscribed from topic: %s", topic)
}

// PublishToChannel sends data to all channel subscribers
func (h *Hub) PublishToChannel(topic string, data []byte) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	channels := h.channels[topic]
	if channels == nil || len(channels) == 0 {
		return 0
	}

	sent := 0
	for ch := range channels {
		select {
		case ch <- data:
			sent++
		default:
			// Channel full, skip
			log.Printf("[PUBSUB] Channel full, skipping message for topic: %s", topic)
		}
	}
	return sent
}

// GetSubscriberCount returns the number of subscribers for a topic
func (h *Hub) GetSubscriberCount(topic string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.channels[topic] == nil {
		return 0
	}
	return len(h.channels[topic])
}

// GetTopics returns all active topics
func (h *Hub) GetTopics() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	topics := make([]string, 0, len(h.channels))
	for topic := range h.channels {
		topics = append(topics, topic)
	}
	return topics
}

// =============================================================================
// Topic Helpers - Standardized topic naming
// =============================================================================

// DeploymentTopic returns the topic name for deployment logs
func DeploymentTopic(runID string) string {
	return "deployment:" + runID
}

// PodLogsTopic returns the topic name for pod logs
func PodLogsTopic(appName string) string {
	return "pods:" + appName
}

// AppEventsTopic returns the topic name for app events
func AppEventsTopic(appName string) string {
	return "app:" + appName
}

// =============================================================================
// Convenience functions using global hub
// =============================================================================

// SubscribeChannel to global hub
func SubscribeChannel(topic string, ch chan []byte) {
	GlobalHub.SubscribeChannel(topic, ch)
}

// UnsubscribeChannel from global hub
func UnsubscribeChannel(topic string, ch chan []byte) {
	GlobalHub.UnsubscribeChannel(topic, ch)
}

// PublishToChannel to global hub
func PublishToChannel(topic string, data []byte) int {
	return GlobalHub.PublishToChannel(topic, data)
}

// GetSubscriberCount from global hub
func GetSubscriberCount(topic string) int {
	return GlobalHub.GetSubscriberCount(topic)
}
