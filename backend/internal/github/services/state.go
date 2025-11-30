package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"backend/pkg/errors"
	"backend/pkg/logger"
)

// StateService manages OAuth state, manifest state, and installation state
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

// GenerateOAuthState generates a secure state parameter for OAuth flow
// Format: "user_{userID}_{timestamp}_{randomComponent}"
func (s *StateService) GenerateOAuthState(userID int) (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", errors.Wrap(err, errors.ErrCodeInternal, "failed to generate secure random bytes")
	}
	randomComponent := hex.EncodeToString(randomBytes)
	state := fmt.Sprintf("user_%d_%d_%s", userID, time.Now().Unix(), randomComponent)
	return state, nil
}

// ValidateOAuthState validates OAuth state parameter and extracts user ID
// Returns userID if valid, error otherwise
func (s *StateService) ValidateOAuthState(state string, maxAgeSeconds int64) (int, error) {
	if state == "" {
		return 0, errors.BadRequest("missing state parameter")
	}

	// Validate state format: "user_{userID}_{timestamp}_{randomComponent}"
	if !strings.HasPrefix(state, "user_") {
		s.log.WithField("state", state[:min(20, len(state))]).Warn("Invalid state format")
		return 0, errors.BadRequest("invalid state parameter format")
	}

	// Extract and validate parts
	parts := strings.Split(state, "_")
	if len(parts) != 4 {
		s.log.WithField("parts_count", len(parts)).Warn("Invalid state parts count")
		return 0, errors.BadRequest("invalid state parameter format")
	}

	// Extract userID
	stateUserIDStr := parts[1]
	userID, err := strconv.Atoi(stateUserIDStr)
	if err != nil {
		s.log.WithField("user_id_str", stateUserIDStr).Warn("Invalid userID in state")
		return 0, errors.BadRequest("invalid state parameter")
	}

	timestampStr := parts[2]
	randomComponent := parts[3]

	// Validate random component format (should be 32 hex chars)
	if len(randomComponent) != 32 {
		s.log.WithFields(map[string]interface{}{
			"user_id":          userID,
			"random_component": randomComponent[:min(10, len(randomComponent))],
			"expected_length":  32,
			"actual_length":    len(randomComponent),
		}).Warn("Invalid random component length")
		return 0, errors.BadRequest("invalid state parameter")
	}

	// Validate that random component is hex
	for _, char := range randomComponent {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			s.log.WithFields(map[string]interface{}{
				"user_id":          userID,
				"random_component": randomComponent[:min(10, len(randomComponent))],
			}).Warn("Invalid random component format (not hex)")
			return 0, errors.BadRequest("invalid state parameter")
		}
	}

	// Validate timestamp
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		s.log.WithFields(map[string]interface{}{
			"user_id":   userID,
			"timestamp": timestampStr,
		}).Warn("Invalid timestamp in state")
		return 0, errors.BadRequest("invalid state parameter")
	}

	// Check if state is not too old
	maxAge := maxAgeSeconds
	currentTime := time.Now().Unix()
	if currentTime-timestamp > maxAge {
		s.log.WithFields(map[string]interface{}{
			"user_id": userID,
			"age":     currentTime - timestamp,
			"max_age": maxAge,
		}).Warn("Expired state")
		return 0, errors.BadRequest("state parameter expired")
	}

	s.log.WithFields(map[string]interface{}{
		"user_id": userID,
		"state":   state[:min(20, len(state))],
	}).Debug("OAuth state validated successfully")

	return userID, nil
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
