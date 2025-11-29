package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

var manifestStates = newStateStore()
var installStates = newStateStore()

// simple in-memory state store for manifest/install flows
// States are single-use and auto-expire to prevent replay attacks
type stateStore struct {
	mu    sync.Mutex
	items map[string]time.Time
}

func newStateStore() *stateStore {
	store := &stateStore{
		items: make(map[string]time.Time),
	}
	// Start background cleanup goroutine
	go store.cleanupLoop()
	return store
}

// cleanupLoop removes expired states every 5 minutes to prevent memory leaks
func (s *stateStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.cleanup(30 * time.Minute) // Remove states older than 30 minutes
	}
}

// cleanup removes states older than maxAge
func (s *stateStore) cleanup(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for state, created := range s.items {
		if now.Sub(created) > maxAge {
			delete(s.items, state)
		}
	}
}

func (s *stateStore) add(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Limit max stored states to prevent DoS
	if len(s.items) > 10000 {
		// Remove oldest 1000 items
		s.trimOldestLocked(1000)
	}
	s.items[state] = time.Now()
}

// trimOldestLocked removes n oldest items (must be called with lock held)
func (s *stateStore) trimOldestLocked(n int) {
	type stateAge struct {
		state   string
		created time.Time
	}
	var states []stateAge
	for state, created := range s.items {
		states = append(states, stateAge{state, created})
	}
	// Sort by age (oldest first) - simple bubble sort for small n
	for i := 0; i < len(states)-1 && i < n; i++ {
		for j := i + 1; j < len(states); j++ {
			if states[j].created.Before(states[i].created) {
				states[i], states[j] = states[j], states[i]
			}
		}
	}
	// Delete oldest n
	for i := 0; i < n && i < len(states); i++ {
		delete(s.items, states[i].state)
	}
}

func (s *stateStore) validate(state string, maxAge time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	created, ok := s.items[state]
	if !ok {
		return false
	}
	// Always delete to ensure single-use (replay protection)
	delete(s.items, state)
	return time.Since(created) <= maxAge
}

func (s *stateStore) exists(state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.items[state]
	return ok
}

// generateSecureSecret generates a cryptographically secure secret
func generateSecureSecret() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// htmlEscapeSingleQuotes escapes single quotes for embedding JSON in HTML attribute
func htmlEscapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "&#39;")
}
