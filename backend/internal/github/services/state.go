package services

import (
	"sync"
	"time"

	"backend/pkg/logger"
)

// StateService manages manifest state and installation state for GitHub App flows
type StateService struct {
	manifestStates *stateStore
	installStates  *stateStore
	log            *logger.Logger
}

// stateStore is a simple in-memory state store for manifest/install flows
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

// touch checks if state exists and refreshes its timestamp
// This extends the validity period when the state is actively being used
func (s *stateStore) touch(state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[state]; ok {
		s.items[state] = time.Now()
		return true
	}
	return false
}

// NewStateService creates a new state service
var globalStateService *StateService
var stateServiceOnce sync.Once

func GetStateService() *StateService {
	stateServiceOnce.Do(func() {
		globalStateService = &StateService{
			manifestStates: newStateStore(),
			installStates:  newStateStore(),
			log:            logger.Default().WithComponent("github-state"),
		}
	})
	return globalStateService
}

// GenerateManifestState generates a secure state for manifest flow
func (s *StateService) GenerateManifestState() string {
	state := GenerateSecureSecret()
	s.manifestStates.add(state)
	return state
}

// ValidateManifestState validates manifest state
func (s *StateService) ValidateManifestState(state string, maxAge time.Duration) bool {
	return s.manifestStates.validate(state, maxAge)
}

// ExistsManifestState checks if manifest state exists (without consuming it)
func (s *StateService) ExistsManifestState(state string) bool {
	return s.manifestStates.exists(state)
}

// TouchManifestState checks if manifest state exists and refreshes its timestamp
// Use this when the state is actively being used to extend validity period
func (s *StateService) TouchManifestState(state string) bool {
	return s.manifestStates.touch(state)
}

// GenerateInstallState generates a secure state for install flow
func (s *StateService) GenerateInstallState() string {
	state := GenerateSecureSecret()
	s.installStates.add(state)
	return state
}

// ValidateInstallState validates install state
func (s *StateService) ValidateInstallState(state string, maxAge time.Duration) bool {
	return s.installStates.validate(state, maxAge)
}
